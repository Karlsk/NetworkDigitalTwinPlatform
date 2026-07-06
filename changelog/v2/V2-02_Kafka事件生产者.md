# V2-02: Kafka Producer 实现 + HandleWebhook 改造

**工时**: 1 天
**前置**: V2-01
**风险等级**: 中
**Phase**: Phase 1 — Kafka 事件流

---

## 背景

V2-01 定义了 `EventPublisher` 接口和 Channel 内存实现。本任务实现 Kafka Producer，
并改造 `SyncService.HandleWebhook` 使用 `EventPublisher` 接口替代直接写入 channel。

**两层架构**: 本任务属于**事件总线层（EventBus Layer）** 的实现。
事件总线层与数据源层独立，详见 [事件总线两层架构设计](../../docs/事件总线两层架构设计.md)。

### 改造前

```go
// internal/service/sync_service.go (V1 现状)
func (s *SyncService) HandleWebhook(event SyncEvent) error {
    select {
    case s.eventChan <- event:
        return nil
    default:
        return errors.New("event queue full")
    }
}
```

### 改造后

```go
// SyncService 使用 EventPublisher 接口
func (s *SyncService) HandleWebhook(ctx context.Context, event events.SyncEvent) error {
    return s.publisher.Publish(ctx, event)
}
```

---

## 实现步骤

### Step 1: 实现 Kafka Producer

新建 `internal/events/kafka_producer.go`：

```go
package events

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"

    "github.com/IBM/sarama"
)

// kafkaPublisher 基于 Kafka 的 EventPublisher 实现。
type kafkaPublisher struct {
    producer sarama.SyncProducer
    topic    string
}

// NewKafkaPublisher 创建 Kafka Producer。
// 使用 SyncProducer（同步发送），确保消息发送成功后才返回。
func NewKafkaPublisher(brokers []string, topic string, config *sarama.Config) (EventPublisher, error) {
    producer, err := sarama.NewSyncProducer(brokers, config)
    if err != nil {
        return nil, fmt.Errorf("create kafka producer: %w", err)
    }
    return &kafkaPublisher{producer: producer, topic: topic}, nil
}

func (p *kafkaPublisher) Publish(_ context.Context, event SyncEvent) error {
    data, err := json.Marshal(event)
    if err != nil {
        return fmt.Errorf("marshal event: %w", err)
    }

    _, _, err = p.producer.SendMessage(&sarama.ProducerMessage{
        Topic: p.topic,
        Value: sarama.ByteEncoder(data),
    })
    if err != nil {
        return fmt.Errorf("send kafka message: %w", err)
    }
    return nil
}

func (p *kafkaPublisher) Close() error {
    return p.producer.Close()
}
```

### Step 2: Kafka Producer 配置

新建 `internal/events/kafka_config.go`：

```go
package events

import (
    "fmt"
    "time"

    "github.com/IBM/sarama"
)

// NewSaramaConfig 创建 sarama 配置。
func NewSaramaConfig(saslUser, saslPass string) (*sarama.Config, error) {
    cfg := sarama.NewConfig()
    cfg.Producer.RequiredAcks = sarama.WaitForAll   // 等待所有副本确认
    cfg.Producer.Retry.Max = 3                       // 最多重试 3 次
    cfg.Producer.Return.Successes = true             // SyncProducer 必须设置
    cfg.Consumer.Return.Errors = true
    cfg.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
    cfg.Consumer.Offsets.Initial = sarama.OffsetOldest // 从最早消息开始消费

    // SASL 认证（可选）
    if saslUser != "" {
        cfg.Net.SASL.Enable = true
        cfg.Net.SASL.User = saslUser
        cfg.Net.SASL.Password = saslPass
        cfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
    }

    cfg.Net.DialTimeout = 10 * time.Second
    cfg.Net.ReadTimeout = 10 * time.Second
    cfg.Net.WriteTimeout = 10 * time.Second

    return cfg, nil
}
```

### Step 3: 改造 SyncService

修改 `internal/service/sync_service.go`：

```go
// SyncService 新增 publisher 字段
type SyncService struct {
    registry   *connector.ConnectorRegistry
    normalizer *normalizer.Normalizer
    assembler  *assembler.GraphAssembler
    graph      graph.GraphDB
    lock       *snapshot.GraphLock
    publisher  events.EventPublisher  // 新增: 替代 eventChan
    consumer   events.EventConsumer   // 新增: 替代 eventChan
}

// NewSyncService 接收 EventPublisher + EventConsumer
func NewSyncService(
    registry *connector.ConnectorRegistry,
    norm *normalizer.Normalizer,
    asm *assembler.GraphAssembler,
    gdb graph.GraphDB,
    lock *snapshot.GraphLock,
    publisher events.EventPublisher,
    consumer events.EventConsumer,
) *SyncService {
    return &SyncService{
        registry:   registry,
        normalizer: norm,
        assembler:  asm,
        graph:      gdb,
        lock:       lock,
        publisher:  publisher,
        consumer:   consumer,
    }
}

// HandleWebhook 委托 EventPublisher
func (s *SyncService) HandleWebhook(ctx context.Context, event events.SyncEvent) error {
    return s.publisher.Publish(ctx, event)
}

// StartConsumer 委托 EventConsumer
func (s *SyncService) StartConsumer(ctx context.Context) {
    go func() {
        err := s.consumer.Consume(ctx, func(ctx context.Context, event events.SyncEvent) error {
            s.lock.Lock()
            defer s.lock.Unlock()

            // 转换 events.SyncEvent → service.SyncEvent
            svcEvent := s.toServiceEvent(event)
            result, err := s.IncrementalSync(ctx, svcEvent)
            if err != nil {
                slog.Error("incremental sync failed", "action", event.Action, "error", err)
                return err
            }
            slog.Info("incremental sync completed",
                "action", event.Action,
                "nodes", result.NodesCreated,
                "duration_ms", result.Duration.Milliseconds(),
            )
            return nil
        })
        if err != nil && !errors.Is(err, context.Canceled) {
            slog.Error("consumer stopped with error", "error", err)
        }
        slog.Info("consumer stopped")
    }()
}
```

### Step 4: 改造 main.go 启动代码

修改 `cmd/server/main.go`：

```go
// 初始化事件总线层（EventBus Layer）
// cfg.EventBus.Mode 决定 EventBus 实现：
//   - "channel": 内存 Channel（默认，V1 兼容）
//   - "kafka":   Kafka Topic（持久化）
var publisher events.EventPublisher
var consumer events.EventConsumer

switch cfg.EventBus.Mode {
case "kafka":
    saramaCfg, err := events.NewSaramaConfig(cfg.EventBus.Kafka.SASLUser, cfg.EventBus.Kafka.SASLPass)
    if err != nil {
        slog.Error("create sarama config", "error", err)
        os.Exit(1)
    }
    publisher, err = events.NewKafkaPublisher(
        cfg.EventBus.Kafka.Brokers, cfg.EventBus.Kafka.Topic, saramaCfg,
    )
    if err != nil {
        slog.Error("create kafka publisher", "error", err)
        os.Exit(1)
    }
    // EventBus Kafka Consumer 在 V2-03 实现，当前使用 nopConsumer 占位
    consumer = nopConsumer{}
    slog.Info("event bus: Kafka mode",
        "brokers", cfg.EventBus.Kafka.Brokers, "topic", cfg.EventBus.Kafka.Topic)
default: // "channel"
    publisher, consumer = events.NewChannelEventBus(cfg.Channel.BufferSize)
    slog.Info("event bus: Channel mode", "buffer_size", cfg.Channel.BufferSize)
}

// 初始化数据源层（DataSource Layer）
// cfg.Kafka.Enabled 控制是否启动 Kafka DataSource Consumer
if cfg.Kafka.Enabled {
    slog.Info("data source: Kafka enabled",
        "brokers", cfg.Kafka.Brokers, "topic", cfg.Kafka.Topic)
    // TODO V2-03: 启动 Kafka DataSource Consumer → publisher.Publish(event)
}

syncSvc := service.NewSyncService(connRegistry, norm, asm, gdb, lock, publisher, consumer)
```

### Step 5: 单元测试

`internal/events/kafka_producer_test.go`（mock sarama.SyncProducer）：

| 测试 | 验证点 |
|------|--------|
| `TestKafkaPublishSuccess` | 消息发送成功，JSON 序列化正确 |
| `TestKafkaPublishError` | Producer 失败时返回 error |
| `TestKafkaClose` | Close 调用无 panic |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/events/kafka_producer.go` | 新增 | Kafka SyncProducer 实现 |
| `internal/events/kafka_config.go` | 新增 | sarama 配置工厂 |
| `internal/events/kafka_producer_test.go` | 新增 | Producer 单元测试 |
| `internal/service/sync_service.go` | 修改 | 替代 eventChan 为 EventPublisher/Consumer |
| `cmd/server/main.go` | 修改 | 启动代码适配 Kafka/Channel 双模式 |

---

## 注意事项

1. **SyncProducer vs AsyncProducer**: 选择 SyncProducer，HandleWebhook 需要确认消息发送成功后才返回 202
2. **序列化格式**: JSON，便于未来其他语言消费者解析
3. **幂等性**: IncrementalSync 的 Upsert 基于 MERGE，天然幂等，Kafka 重复投递不会导致数据重复
4. **向后兼容**: `event_bus.mode: "channel"` 时行为与 V1 完全一致
5. **两层分离**: EventBus 层使用 `cfg.EventBus.Mode` 控制，数据源层使用 `cfg.Kafka.Enabled` 控制，两者独立配置

---

## 验收标准

- [ ] Kafka Producer 实现编译通过
- [ ] SyncService 改造后 `HandleWebhook` 使用 EventPublisher 接口
- [ ] `kafka.enabled: false` 时行为与 V1 完全一致
- [ ] Producer 单元测试全部通过
- [ ] `go build ./...` 无错误
