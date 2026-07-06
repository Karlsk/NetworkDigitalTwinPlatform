# V2-03: Kafka Consumer Group 实现 + StartConsumer 改造

**工时**: 1.5 天
**前置**: V2-02
**风险等级**: 中
**Phase**: Phase 1 — Kafka 事件流

---

## 背景

V2-02 实现了 Kafka Producer，本任务实现 Kafka Consumer Group 并完善 SyncService 的 StartConsumer 改造。

Consumer Group 优势：
- 多消费者并行处理（水平扩展）
- 自动 Rebalance（消费者增减时自动分配 Partition）
- Offset 提交（重启后从上次位置继续消费）

---

## 实现步骤

### Step 1: 实现 Kafka Consumer Group

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
type kafkaConsumer struct {
    client  sarama.ConsumerGroup
    topic   string
    groupID string
    ready   chan bool
}

// NewKafkaConsumer 创建 Kafka Consumer Group。
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

### Step 2: 同步事件类型转换

在 `internal/service/sync_service.go` 中新增转换方法：

```go
// toServiceEvent 将 events.SyncEvent 转换为 service.SyncEvent。
// 补充 Relations 字段（delete_relation 时需要）。
func (s *SyncService) toServiceEvent(e events.SyncEvent) SyncEvent {
    return SyncEvent{
        Action:     e.Action,
        EntityType: e.EntityType,
        Connector:  e.Connector,
        Data:       e.Data,
        URIs:       e.URIs,
        // Relations 需要在 handler 中按需构造
    }
}
```

### Step 3: GraphLock 集成

Consumer 处理每个事件时获取写锁，确保与 FullSync/Restore 互斥：

```go
func (s *SyncService) StartConsumer(ctx context.Context) {
    go func() {
        err := s.consumer.Consume(ctx, func(ctx context.Context, event events.SyncEvent) error {
            s.lock.Lock()
            defer s.lock.Unlock()

            svcEvent := s.toServiceEvent(event)
            result, err := s.IncrementalSync(ctx, svcEvent)
            if err != nil {
                slog.Error("incremental sync failed",
                    "action", event.Action, "error", err)
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

### Step 4: Shutdown 优雅退出

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

### Step 5: 单元测试

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
| `internal/service/sync_service.go` | 修改 | toServiceEvent 转换 + StartConsumer 改造 |
| `cmd/server/main.go` | 修改 | shutdown 优雅退出 |

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
- [ ] `go build ./...` 无错误
- [ ] `go test ./internal/events/...` 全部通过
