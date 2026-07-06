# V2-03: Kafka Consumer Group 实现

**工时**: 1.5 天
**前置**: V2-02
**风险等级**: 中
**Phase**: Phase 1 — Kafka 事件流

---

## 背景

V2-02 已完成事件总线层的“共享通道”架构改造：

- `EventPublisher` / `EventConsumer` 接口对称设计，共享同一个底层通道
- `NewChannelEventBus(bufferSize)` 返回共享 `chan SyncEvent` 的 `(EventPublisher, EventConsumer)` 对
- `HandleWebhook` 仅写入 `publisher`，不再双重写入
- `StartConsumer` 通过 `consumer.Consume(handler)` 从共享通道消费事件
- `toServiceEvent` 已完成 `events.SyncEvent` → `service.SyncEvent` 转换（含 Relations 映射）

**两层架构**（详见 [事件总线两层架构设计](../../docs/事件总线两层架构设计.md)）：

本任务涉及两种不同的 Consumer 角色，需明确区分：

| Consumer 角色 | 所属层 | 职责 | 配置项 |
|----------------|--------|------|--------|
| **EventBus Consumer** | 事件总线层 | 从共享通道消费事件，调用 IncrementalSync | `cfg.EventBus.Mode` |
| **DataSource Consumer** | 数据源层 | 从外部 Kafka Topic 消费事件，写入 EventBus | `cfg.Kafka.Enabled` |

**数据流架构（V2-02 已实现）**:

```
数据源层:  HandleWebhook ──→ publisher.Publish(event) ──┐
         Kafka DataSource ──→ publisher.Publish(event) ──┤
                                                         ↓
事件总线层:  [共享通道: chan SyncEvent 或 Kafka Topic]
                                                         ↓
         EventBus Consumer → StartConsumer → IncrementalSync
```

**两种模式共享同一架构**：

| 模式 | Publisher | Consumer | 共享通道 |
|------|-----------|----------|----------|
| Channel（内存） | `channelPublisher` | `channelConsumer` | `chan SyncEvent` |
| Kafka（持久化） | `kafkaProducer` | `kafkaConsumer` | Kafka Topic |

本任务（V2-03）实现两种 Kafka Consumer：

1. **EventBus Kafka Consumer**: 事件总线层的消费者，与 kafkaPublisher 共享同一个 Kafka Topic，替换 nopConsumer
2. **Kafka DataSource Consumer**: 数据源层的消费者，从外部 Kafka Topic 消费事件并通过 publisher.Publish() 写入 EventBus

Consumer Group 优势：
- 多消费者并行处理（水平扩展）
- 自动 Rebalance（消费者增减时自动分配 Partition）
- Offset 提交（重启后从上次位置继续消费）

---

## 实现步骤

### Step 1: 实现 EventBus Kafka Consumer Group

新建 `internal/events/kafka_consumer.go`：

```go
package events

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"

    "github.com/IBM/sarama"
)

// kafkaConsumer 基于 Kafka Consumer Group 的 EventConsumer 实现。
// 属于**事件总线层**，与 kafkaProducer 共享同一个 Kafka Topic，构成“共享通道”架构。
// 注意：这与数据源层的 Kafka DataSource Consumer 不同，后者消费外部 Topic。
type kafkaConsumer struct {
    client  sarama.ConsumerGroup
    topic   string
    groupID string
    ready   chan bool
}

// NewKafkaConsumer 创建 Kafka Consumer Group。
// topic 必须与 kafkaProducer 使用的 topic 相同，确保共享通道。
func NewKafkaConsumer(brokers []string, topic, groupID string, config *sarama.Config) (EventConsumer, error) {
    client, err := sarama.NewConsumerGroup(brokers, []string{groupID}, config)
    if err != nil {
        return nil, fmt.Errorf("create kafka consumer group: %w", err)
    }
    return &kafkaConsumer{
        client:  client,
        topic:   topic,
        groupID: groupID,
        ready:   make(chan bool),
    }, nil
}

// Consume 启动消费循环。阻塞直到 ctx 取消。
// 消费的事件来自 kafkaProducer 写入的同一个 Topic。
func (c *kafkaConsumer) Consume(ctx context.Context, handler func(ctx context.Context, event SyncEvent) error) error {
    // 后台处理 consumer group errors
    go func() {
        for err := range c.client.Errors() {
            slog.Error("kafka consumer error", "error", err)
        }
    }()

    h := &consumerHandler{handler: handler, ready: c.ready}

    for {
        if err := c.client.Consume(ctx, []string{c.topic}, h); err != nil {
            return fmt.Errorf("kafka consume: %w", err)
        }
        if ctx.Err() != nil {
            return ctx.Err()
        }
    }
}

func (c *kafkaConsumer) Close() error {
    return c.client.Close()
}

// consumerHandler 实现 sarama.ConsumerGroupHandler。
type consumerHandler struct {
    handler func(ctx context.Context, event SyncEvent) error
    ready   chan bool
}

func (h *consumerHandler) Setup(_ sarama.ConsumerGroupSession) error {
    close(h.ready)
    return nil
}

func (h *consumerHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
    return nil
}

func (h *consumerHandler) ConsumeClaim(
    session sarama.ConsumerGroupSession,
    claim sarama.ConsumerGroupClaim,
) error {
    for msg := range claim.Messages() {
        var event SyncEvent
        if err := json.Unmarshal(msg.Value, &event); err != nil {
            slog.Error("unmarshal kafka message", "error", err, "offset", msg.Offset)
            session.MarkMessage(msg, "")
            continue
        }

        if err := h.handler(session.Context(), event); err != nil {
            slog.Error("handle event failed", "error", err, "offset", msg.Offset)
            // 不提交 offset，下次重新消费（at-least-once 语义）
            // 但由于 IncrementalSync 幂等，也可提交 offset
            session.MarkMessage(msg, "")
            continue
        }

        session.MarkMessage(msg, "") // 提交 offset
    }
    return nil
}
```

### Step 2: cmd/server/main.go 集成 EventBus Kafka Consumer

将 V2-02 占位的 `nopConsumer` 替换为真实的 `kafkaConsumer`（事件总线层）：

```go
// 事件总线层 - Kafka 模式（V2-03 实现）
case "kafka":
    saramaCfg, err := events.NewSaramaConfig(cfg.EventBus.Kafka.SASLUser, cfg.EventBus.Kafka.SASLPass)
    // ...
    publisher, err = events.NewKafkaPublisher(
        cfg.EventBus.Kafka.Brokers, cfg.EventBus.Kafka.Topic, saramaCfg,
    )
    // EventBus Consumer：与 Publisher 共享同一个 Topic
    consumer, err = events.NewKafkaConsumer(
        cfg.EventBus.Kafka.Brokers,
        cfg.EventBus.Kafka.Topic,     // 与 producer 共享同一个 topic
        cfg.EventBus.Kafka.GroupID,
        saramaCfg,
    )
```

**关键**: `kafkaProducer` 和 `kafkaConsumer` 使用相同的 `EventBus.Kafka.Topic`，与 Channel 模式的 `NewChannelEventBus` 共享 `chan` 架构对称。

### Step 2.1: cmd/server/main.go 集成 Kafka DataSource Consumer（数据源层）

```go
// 数据源层 - Kafka DataSource Consumer（V2-03 实现）
if cfg.Kafka.Enabled {
    dsSaramaCfg, err := events.NewSaramaConfig(cfg.Kafka.SASLUser, cfg.Kafka.SASLPass)
    // ...
    dsConsumer, err := events.NewKafkaDataSourceConsumer(
        cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.GroupID, dsSaramaCfg,
    )
    // 启动数据源消费者：消费外部 Topic → publisher.Publish(event) → EventBus
    go dsConsumer.Start(ctx, publisher)
}
```

**关键区分**:
- **EventBus Consumer**: 消费 EventBus 内部 Topic → IncrementalSync（事件总线层）
- **DataSource Consumer**: 消费外部 Topic → publisher.Publish()（数据源层）

### Step 3: Shutdown 优雅退出

`cmd/server/main.go` 新增 shutdown 逻辑：

```go
// 优雅退出：先停 consumer，再停 publisher
<-ctx.Done()
slog.Info("shutting down event bus...")

// 给 consumer 时间完成当前事件
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := consumer.Close(); err != nil {
    slog.Error("close consumer", "error", err)
}
if err := publisher.Close(); err != nil {
    slog.Error("close publisher", "error", err)
}
```

### Step 4: 单元测试

`internal/events/kafka_consumer_test.go`（mock sarama.ConsumerGroup）：

| 测试 | 验证点 |
|------|--------|
| `TestKafkaConsumeMessage` | 消息反序列化正确，handler 被调用 |
| `TestKafkaConsumeInvalidJSON` | 无效 JSON 跳过，不中断消费 |
| `TestKafkaConsumeHandlerError` | handler 返回 error 时继续消费 |
| `TestKafkaConsumeCancel` | ctx 取消后 Consume 返回 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/events/kafka_consumer.go` | 新增 | Kafka Consumer Group 实现 |
| `internal/events/kafka_consumer_test.go` | 新增 | Consumer 单元测试 |
| `cmd/server/main.go` | 修改 | Kafka Consumer 集成 + shutdown 优雅退出 |

> **注意**: `sync_service.go` 的 `StartConsumer`、`toServiceEvent`、`HandleWebhook` 已在 V2-02 修正中完成，
> V2-03 不再需要修改 `sync_service.go`。

---

## 架构约束

### 共享通道原则

无论 Channel 模式还是 Kafka 模式，事件流始终遵循**单一共享通道**：

```
数据源层 (DataSource Layer)
├─ Webhook Handler ──→ publisher.Publish(event) ──┐
└─ Kafka DataSource ──→ publisher.Publish(event) ──┤
                                                  ↓
事件总线层 (EventBus Layer)
┌───────────────────────────────────────────────────┐
│  共享通道: chan SyncEvent 或 Kafka Topic             │
│  Publisher 端 ──→ [缓冲通道/Topic] ──→ Consumer 端  │
└───────────────────────────────────────────────────┘
                                                  ↓
                              StartConsumer → toServiceEvent → IncrementalSync
```

**禁止事项**:
- 禁止 HandleWebhook 直接写入 eventChan（绕过 publisher）
- 禁止 Kafka DataSource Consumer 直接调用 IncrementalSync（必须通过 publisher 写入 EventBus）
- 禁止 SyncService 持有独立的 eventChan 字段
- publisher 和 consumer 必须是同一个底层通道的两端

### V2-02 已完成的改造（本任务不需重复）

| 改造项 | 状态 |
|--------|------|
| `SyncService.eventChan` → `SyncService.consumer` | ✅ 已完成 |
| `NewSyncService` 签名增加 `consumer EventConsumer` | ✅ 已完成 |
| `HandleWebhook` 简化为只调 `publisher.Publish` | ✅ 已完成 |
| `StartConsumer` 使用 `consumer.Consume(handler)` | ✅ 已完成 |
| `toServiceEvent` 含 Relations 映射 | ✅ 已完成 |

---

## 注意事项

1. **At-least-once 语义**: 消费失败时不提交 offset，下次重新消费。IncrementalSync 的 Upsert 天然幂等
2. **Consumer Group Rebalance**: Rebalance 期间暂停消费，完成后自动恢复。配置合理的 `Session.Timeout` (10s) 和 `Heartbeat.Interval` (3s)
3. **Graceful Shutdown**: 先关闭 consumer（停止接收新消息），等待当前处理完成，再关闭 producer
4. **错误处理**: handler 返回 error 只记日志，不中断消费循环。致命错误（如 Neo4j 不可用）应触发 health check 告警

---

## 验收标准

- [ ] Kafka Consumer Group 实现编译通过
- [ ] 消息反序列化正确，handler 被正确调用
- [ ] 无效 JSON 消息被跳过，不中断消费
- [ ] ctx 取消后消费循环正确退出
- [ ] `cmd/server/main.go` Kafka 模式使用真实 `kafkaConsumer`（替换 `nopConsumer`）
- [ ] `go build ./...` 无错误
- [ ] `go test ./internal/events/...` 全部通过
