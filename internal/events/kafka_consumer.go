// Package events 提供基于 Kafka Consumer Group 的 EventConsumer 实现。
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"
)

// consumerGroup 窄接口，解耦 sarama.ConsumerGroup 便于单元测试。
type consumerGroup interface {
	Consume(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error
	Errors() <-chan error
	Close() error
}

// kafkaConsumer 基于 Kafka Consumer Group 的 EventConsumer 实现（事件总线层）。
// 与 kafkaProducer 共享同一个 Kafka Topic，构成"共享通道"架构。
// 注意：这与数据源层的 KafkaDataSourceConsumer 不同，后者消费外部 Topic。
type kafkaConsumer struct {
	client  consumerGroup
	topic   string
	groupID string
}

// NewKafkaConsumer 创建 Kafka Consumer Group。
// topic 必须与 kafkaProducer 使用的 topic 相同，确保共享通道。
func NewKafkaConsumer(brokers []string, topic, groupID string, config *sarama.Config) (EventConsumer, error) {
	client, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer group: %w", err)
	}
	return &kafkaConsumer{
		client:  client,
		topic:   topic,
		groupID: groupID,
	}, nil
}

// Consume 启动消费循环，阻塞直到 ctx 取消。
// 消费的事件来自 kafkaProducer 写入的同一个 Topic。
func (c *kafkaConsumer) Consume(ctx context.Context, handler func(ctx context.Context, event SyncEvent) error) error {
	// 后台处理 consumer group errors
	go func() {
		for err := range c.client.Errors() {
			slog.Error("kafka consumer group error", "error", err)
		}
	}()

	h := &consumerHandler{handler: handler}

	for {
		if err := c.client.Consume(ctx, []string{c.topic}, h); err != nil {
			return fmt.Errorf("kafka consume: %w", err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// Close 关闭 Kafka consumer group，释放资源。
func (c *kafkaConsumer) Close() error {
	return c.client.Close()
}

// consumerHandler 实现 sarama.ConsumerGroupHandler。
// 核心业务逻辑：JSON 反序列化 -> handler 调用 -> MarkMessage 提交 offset。
type consumerHandler struct {
	handler func(ctx context.Context, event SyncEvent) error
}

// Setup 在每次 rebalance 后、ConsumeClaim 之前调用。
func (h *consumerHandler) Setup(_ sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup 在每次 rebalance 前、所有 ConsumeClaim 退出后调用。
func (h *consumerHandler) Cleanup(_ sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 消费单个 partition 的消息。
// 无效 JSON 跳过 + 日志，handler error 跳过 + 日志，不中断消费。
// 所有消息都 MarkMessage（at-least-once + 幂等）。
func (h *consumerHandler) ConsumeClaim(
	session sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for msg := range claim.Messages() {
		var event SyncEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			slog.Error("unmarshal kafka message",
				"error", err,
				"offset", msg.Offset,
				"partition", msg.Partition,
			)
			session.MarkMessage(msg, "")
			continue
		}

		if err := h.handler(session.Context(), event); err != nil {
			slog.Error("handle event failed",
				"error", err,
				"offset", msg.Offset,
				"partition", msg.Partition,
			)
			// 仍然提交 offset：IncrementalSync 幂等，避免无限重试
			session.MarkMessage(msg, "")
			continue
		}

		session.MarkMessage(msg, "")
	}
	return nil
}

// ---------------------------------------------------------------------------
// KafkaDataSourceConsumer（数据源层）
// ---------------------------------------------------------------------------

// KafkaDataSourceConsumer 数据源层 Kafka 消费者（独立类型）。
// 从外部 Kafka Topic 消费事件，通过 publisher.Publish() 写入 EventBus。
// 这与事件总线层的 kafkaConsumer 不同：
//   - EventBus Consumer: 消费内部 Topic → IncrementalSync
//   - DataSource Consumer: 消费外部 Topic → publisher.Publish()
type KafkaDataSourceConsumer struct {
	brokers []string
	topic   string
	groupID string
	config  *sarama.Config
	inner   EventConsumer // 底层 kafkaConsumer，Close 时释放
}

// NewKafkaDataSourceConsumer 创建数据源层 Kafka 消费者。
func NewKafkaDataSourceConsumer(
	brokers []string, topic, groupID string, config *sarama.Config,
) (*KafkaDataSourceConsumer, error) {
	inner, err := NewKafkaConsumer(brokers, topic, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("create kafka data source consumer: %w", err)
	}
	return &KafkaDataSourceConsumer{
		brokers: brokers,
		topic:   topic,
		groupID: groupID,
		config:  config,
		inner:   inner,
	}, nil
}

// Start 启动数据源消费循环，阻塞直到 ctx 取消。
// 消费外部 Topic 的事件并通过 publisher.Publish() 写入 EventBus。
// 禁止直接调用 IncrementalSync（必须通过 publisher 写入 EventBus）。
func (d *KafkaDataSourceConsumer) Start(ctx context.Context, publisher EventPublisher) error {
	slog.Info("kafka data source consumer started",
		"topic", d.topic,
		"group_id", d.groupID,
	)
	return d.inner.Consume(ctx, func(ctx context.Context, event SyncEvent) error {
		return publisher.Publish(ctx, event)
	})
}

// Close 关闭数据源消费者，释放底层 Kafka Consumer Group 资源。
func (d *KafkaDataSourceConsumer) Close() error {
	if d.inner != nil {
		return d.inner.Close()
	}
	return nil
}
