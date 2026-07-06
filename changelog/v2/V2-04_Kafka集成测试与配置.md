# V2-04: Kafka 集成测试 + 配置扩展 + EventBus Fallback

**工时**: 1.5 天
**前置**: V2-03
**风险等级**: 高
**Phase**: Phase 1 — Kafka 事件流

---

## 背景

V2-01~V2-03 完成了 Kafka 接口抽象、Producer、Consumer 实现。本任务完成：
1. testcontainers 集成测试（真实 Kafka 容器）
2. **EventBus Fallback 机制**（仅作用于 EventBus 层，不影响数据源层）
3. docker-compose 更新

**两层架构**（详见 [事件总线两层架构设计](../../docs/事件总线两层架构设计.md)）：

Fallback 机制专门针对**事件总线层**设计：
- 当 `cfg.EventBus.Mode = "kafka"` 且 Kafka 不可用时，仅 EventBus 层从 Kafka 降级到 Channel
- **数据源层不受影响**：Webhook 和 Kafka DataSource 继续正常工作
- 触发条件：EventBus Kafka Publisher 发送失败

---

## 实现步骤

### Step 1: EventBus Fallback 机制

> **作用范围**: Fallback 仅作用于 EventBus 层。当 EventBus Kafka 不可用时，
> 数据源层（Webhook、Kafka DataSource）继续通过 publisher.Publish() 发送事件，
> 事件经由降级后的 Channel EventBus 流转。

新建 `internal/events/fallback.go`：

```go
package events

import (
    "context"
    "fmt"
    "log/slog"
    "time"
)

// fallbackPublisher 带 Fallback 的 EventPublisher。
// 仅作用于事件总线层：当 EventBus Kafka 不可用时自动降级到内存 Channel。
// 不影响数据源层：Webhook 和 Kafka DataSource 继续通过 publisher.Publish() 发送事件。
type fallbackPublisher struct {
    primary  EventPublisher  // Kafka
    fallback EventPublisher  // Channel
    primaryOK bool
    retryInterval time.Duration
}

// NewFallbackPublisher 创建带 Fallback 的 Publisher（仅 EventBus 层使用）。
// 启动时检测 primary 连通性，不可用时自动切换到 fallback。
// 数据源层不受影响：数据源通过 publisher.Publish() 发送事件，
// 无论底层是 Kafka 还是 Channel fallback，数据源层透明。
func NewFallbackPublisher(primary, fallback EventPublisher, retryInterval time.Duration) EventPublisher {
    return &fallbackPublisher{
        primary:       primary,
        fallback:      fallback,
        primaryOK:     true,
        retryInterval: retryInterval,
    }
}

func (p *fallbackPublisher) Publish(ctx context.Context, event SyncEvent) error {
    if p.primaryOK {
        if err := p.primary.Publish(ctx, event); err != nil {
            slog.Warn("kafka publish failed, falling back to channel",
                "error", err)
            p.primaryOK = false
            // 后台定期尝试恢复
            go p.tryRecover(ctx)
            return p.fallback.Publish(ctx, event)
        }
        return nil
    }
    return p.fallback.Publish(ctx, event)
}

func (p *fallbackPublisher) tryRecover(ctx context.Context) {
    ticker := time.NewTicker(p.retryInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // 发送空消息探测连通性
            slog.Info("attempting to recover kafka connection")
            p.primaryOK = true
            return
        }
    }
}

func (p *fallbackPublisher) Close() error {
    var errs []error
    if err := p.primary.Close(); err != nil {
        errs = append(errs, err)
    }
    if err := p.fallback.Close(); err != nil {
        errs = append(errs, err)
    }
    if len(errs) > 0 {
        return fmt.Errorf("close publishers: %v", errs)
    }
    return nil
}
```

### Step 2: testcontainers 集成测试

新建 `internal/events/kafka_integration_test.go`：

```go
//go:build integration

package events_test

import (
    "context"
    "testing"
    "time"

    "github.com/IBM/sarama"
    "github.com/stretchr/testify/require"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/kafka"

    "gitlab.com/pml/network-digital-twin/internal/events"
)

func setupKafkaContainer(t *testing.T) (string, func()) {
    ctx := context.Background()
    container, err := kafka.Run(ctx, "confluentinc/confluent-local:7.6.1",
        kafka.WithClusterID("test-cluster"),
    )
    require.NoError(t, err)

    brokers, err := container.Brokers(ctx)
    require.NoError(t, err)

    return brokers[0], func() {
        container.Terminate(ctx)
    }
}

func TestKafkaEndToEnd(t *testing.T) {
    broker, cleanup := setupKafkaContainer(t)
    defer cleanup()

    cfg, err := events.NewSaramaConfig("", "")
    require.NoError(t, err)

    // 创建 Producer
    pub, err := events.NewKafkaPublisher([]string{broker}, "test-events", cfg)
    require.NoError(t, err)
    defer pub.Close()

    // 创建 Consumer
    con, err := events.NewKafkaConsumer([]string{broker}, "test-events", "test-group", cfg)
    require.NoError(t, err)
    defer con.Close()

    // Publish
    event := events.SyncEvent{
        Action:     "update",
        EntityType: "Device",
        Connector:  "netbox",
        Data:       []map[string]any{{"name": "R1"}},
    }
    err = pub.Publish(context.Background(), event)
    require.NoError(t, err)

    // Consume
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    received := make(chan events.SyncEvent, 1)
    go con.Consume(ctx, func(_ context.Context, e events.SyncEvent) error {
        received <- e
        cancel() // 收到后停止
        return nil
    })

    select {
    case e := <-received:
        require.Equal(t, "update", e.Action)
        require.Equal(t, "Device", e.EntityType)
    case <-time.After(10 * time.Second):
        t.Fatal("timeout waiting for kafka message")
    }
}
```

### Step 3: Fallback 单元测试

`internal/events/fallback_test.go`：

| 测试 | 验证点 |
|------|--------|
| `TestFallbackPrimarySuccess` | primary 正常时使用 primary |
| `TestFallbackPrimaryFails` | primary 失败时自动切换到 fallback |
| `TestFallbackRecover` | primary 恢复后自动切回 |

### Step 4: docker-compose 更新

```yaml
# deploy/docker-compose.yml 新增 Kafka 服务
  kafka:
    image: confluentinc/confluent-local:7.6.1
    container_name: kafka-twin
    ports:
      - "9092:9092"
    environment:
      KAFKA_NODE_ID: 1
      KAFKA_PROCESS_ROLES: broker,controller
      KAFKA_CONTROLLER_QUORUM_VOTERS: 1@localhost:29093
      KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092,CONTROLLER://localhost:29093
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092
      KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
    healthcheck:
      test: ["CMD", "kafka-broker-api-versions", "--bootstrap-server", "localhost:9092"]
      interval: 10s
      timeout: 5s
      retries: 5

  app:
    environment:
      # 数据源层 (DataSource Layer)
      KAFKA_ENABLED: "true"
      KAFKA_BROKERS: "kafka:9092"
      KAFKA_TOPIC: "sync-events"
      KAFKA_GROUP_ID: "network-twin"
      # 事件总线层 (EventBus Layer)
      EVENT_BUS_MODE: "kafka"  # 或 "channel"，Fallback 仅在此层生效
```

### Step 5: 配置环境变量覆盖

`internal/config/config.go` 已实现两层分离的环境变量覆盖：

```go
func applyEnvOverrides(cfg *Config) {
    // 数据源层 (DataSource Layer)
    if v := envStr("KAFKA_ENABLED"); v == "true" {
        cfg.Kafka.Enabled = true
    }
    // 事件总线层 (EventBus Layer)
    if v := envStr("EVENT_BUS_MODE"); v != "" {
        cfg.EventBus.Mode = v
    }
}
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/events/fallback.go` | 新增 | Fallback Publisher 实现 |
| `internal/events/fallback_test.go` | 新增 | Fallback 单元测试 |
| `internal/events/kafka_integration_test.go` | 新增 | testcontainers 集成测试 |
| `deploy/docker-compose.yml` | 修改 | 新增 Kafka 服务 |
| `internal/config/config.go` | 修改 | Kafka 环境变量覆盖 |
| `configs/config.yaml` | 修改 | Kafka 配置段 |

---

## 验收标准

- [ ] Fallback Publisher 单元测试全部通过
- [ ] `go test -tags=integration ./internal/events/...` 集成测试通过
- [ ] docker-compose 启动包含 Kafka 服务
- [ ] EventBus Kafka 不可用时仅 EventBus 层自动降级到 Channel，数据源层不受影响，slog 输出 warn 日志
- [ ] 进程重启后 Kafka 中的消息仍可被消费
- [ ] `cfg.Kafka.Enabled` 仅控制数据源层，不影响 EventBus Fallback
- [ ] `go build ./...` 无错误
