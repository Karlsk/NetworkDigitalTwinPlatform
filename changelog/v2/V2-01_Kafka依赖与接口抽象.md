# V2-01: Kafka 依赖引入 + 事件接口抽象

**工时**: 1 天
**前置**: 无
**风险等级**: 低
**Phase**: Phase 1 — Kafka 事件流

---

## 背景

V1 现状：`SyncService` 使用内存 `chan SyncEvent` 缓冲 Webhook 事件（`eventChan`，buffer size=100）。
进程重启时 channel 中的事件丢失，无法保证消息持久化。

V2 目标：引入 Kafka 作为事件总线，通过接口抽象实现 Channel ↔ Kafka 无缝切换。

### V1 现状代码

```go
// internal/service/sync_service.go (当前)
type SyncService struct {
    // ...
    eventChan chan SyncEvent  // 内存 Channel，进程重启丢失
}

func (s *SyncService) HandleWebhook(event SyncEvent) error {
    select {
    case s.eventChan <- event:
        return nil
    default:
        return errors.New("event queue full")
    }
}

func (s *SyncService) StartConsumer(ctx context.Context) {
    go func() {
        for event := range s.eventChan {
            // ... 处理
        }
    }()
}
```

---

## 实现步骤

### Step 1: 引入 Kafka 依赖

```bash
go get github.com/IBM/sarama@latest
```

**选型理由**：
- `sarama` 是纯 Go Kafka 客户端，无 CGO 依赖
- 支持 Producer（同步/异步）+ Consumer Group
- 社区活跃，IBM 维护

### Step 2: 定义事件接口

新建 `internal/events/interface.go`：

```go
// Package events 定义事件总线抽象接口。
// 支持 Channel（内存）和 Kafka（持久化）两种实现。
package events

import "context"

// SyncEvent 同步事件（复用 service.SyncEvent 结构）。
// 此处重新定义以避免循环导入。
type SyncEvent struct {
    Action     string           `json:"action"`      // "update", "delete", "delete_relation"
    EntityType string           `json:"entity_type"`
    Connector  string           `json:"connector"`
    Data       []map[string]any `json:"data,omitempty"`
    URIs       []string         `json:"uris,omitempty"`
}

// EventPublisher 事件发布者接口。
// HandleWebhook 时调用 Publish 将事件写入事件总线。
type EventPublisher interface {
    // Publish 发布一个同步事件。
    // 实现应保证非阻塞或快速返回，失败时返回 error。
    Publish(ctx context.Context, event SyncEvent) error

    // Close 关闭发布者，释放资源。
    Close() error
}

// EventConsumer 事件消费者接口。
// 从事件总线读取事件并分发给消费者处理函数。
type EventConsumer interface {
    // Consume 启动消费循环，阻塞直到 ctx 取消。
    // 每收到一个事件，调用 handler 处理。
    // handler 返回 error 时记录日志但不中断消费。
    Consume(ctx context.Context, handler func(ctx context.Context, event SyncEvent) error) error

    // Close 关闭消费者，释放资源。
    Close() error
}
```

### Step 3: 实现 Channel（内存）适配

新建 `internal/events/channel.go`：

```go
// channelPublisher 基于内存 Channel 的 EventPublisher 实现（V1 兼容）。
type channelPublisher struct {
    ch chan SyncEvent
}

func NewChannelPublisher(bufferSize int) EventPublisher {
    return &channelPublisher{ch: make(chan SyncEvent, bufferSize)}
}

func (p *channelPublisher) Publish(_ context.Context, event SyncEvent) error {
    select {
    case p.ch <- event:
        return nil
    default:
        return errors.New("event queue full")
    }
}

func (p *channelPublisher) Close() error { return nil }

// channelConsumer 基于内存 Channel 的 EventConsumer 实现（V1 兼容）。
type channelConsumer struct {
    ch chan SyncEvent
}

func NewChannelConsumer(bufferSize int) EventConsumer {
    return &channelConsumer{ch: make(chan SyncEvent, bufferSize)}
}

func (c *channelConsumer) Consume(ctx context.Context, handler func(ctx context.Context, event SyncEvent) error) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case event := <-c.ch:
            if err := handler(ctx, event); err != nil {
                slog.Error("event handler failed", "error", err)
            }
        }
    }
}

func (c *channelConsumer) Close() error { return nil }
```

### Step 4: 配置扩展

`internal/config/config.go` 扩展：

```go
// KafkaConfig Kafka 连接配置。
type KafkaConfig struct {
    Enabled     bool     `mapstructure:"enabled"`      // false = 使用内存 Channel
    Brokers     []string `mapstructure:"brokers"`       // ["localhost:9092"]
    Topic       string   `mapstructure:"topic"`         // "sync-events"
    GroupID     string   `mapstructure:"group_id"`      // "network-twin"
    SASLUser    string   `mapstructure:"sasl_user"`     // 可选
    SASLPass    string   `mapstructure:"sasl_pass"`     // 可选
}
```

`configs/config.yaml` 新增：

```yaml
kafka:
  enabled: false          # V2 默认关闭，逐步切换
  brokers: ["localhost:9092"]
  topic: "sync-events"
  group_id: "network-twin"
```

默认 `enabled: false`，保持 Channel 行为不变。设为 `true` 时启用 Kafka。

### Step 5: 单元测试

`internal/events/channel_test.go`：

| 测试 | 验证点 |
|------|--------|
| `TestChannelPublishConsume` | Publish → Consume 全链路，消息顺序正确 |
| `TestChannelPublishFull` | 缓冲区满时返回 error |
| `TestChannelConsumeCancel` | ctx 取消后 Consume 返回 |

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `go.mod` | 修改 | 新增 `github.com/IBM/sarama` |
| `internal/events/interface.go` | 新增 | EventPublisher / EventConsumer 接口 |
| `internal/events/channel.go` | 新增 | Channel 内存实现 |
| `internal/events/channel_test.go` | 新增 | Channel 实现单元测试 |
| `internal/config/config.go` | 修改 | 新增 KafkaConfig |
| `configs/config.yaml` | 修改 | 新增 kafka 段 |

---

## 验收标准

- [x] `EventPublisher` / `EventConsumer` 接口定义完整，编译通过
- [x] Channel 内存实现通过全部单元测试
- [x] 配置 `kafka.enabled: false` 时行为与 V1 完全一致
- [x] `go build ./...` 无错误
- [x] `go test ./internal/events/...` 全部通过
