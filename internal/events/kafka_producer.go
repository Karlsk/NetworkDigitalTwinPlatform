// Package events 提供基于 Kafka 的 EventPublisher 实现。
package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/IBM/sarama"
)

// syncProducer 内部窄接口，解耦 sarama.SyncProducer 便于测试。
type syncProducer interface {
	SendMessage(msg []byte, topic string) (partition int32, offset int64, err error)
	Close() error
}

// saramaProducerAdapter 将 sarama.SyncProducer 适配为 syncProducer 窄接口。
type saramaProducerAdapter struct {
	producer sarama.SyncProducer
}

func (a *saramaProducerAdapter) SendMessage(msg []byte, topic string) (int32, int64, error) {
	return a.producer.SendMessage(&sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msg),
	})
}

func (a *saramaProducerAdapter) Close() error {
	return a.producer.Close()
}

// kafkaPublisher 基于 Kafka 的 EventPublisher 实现。
type kafkaPublisher struct {
	producer syncProducer
	client   sarama.Client // 共享 client，用于 Ping 连通性探测
	topic    string
}

// NewKafkaPublisher 创建 Kafka Producer。
// 使用 SyncProducer（同步发送），确保消息发送成功后才返回。
// 内部创建共享 sarama.Client，同时用于 SyncProducer 和 Ping 探测。
func NewKafkaPublisher(brokers []string, topic string, config *sarama.Config) (EventPublisher, error) {
	client, err := sarama.NewClient(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("create kafka client: %w", err)
	}
	producer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}
	return &kafkaPublisher{
		producer: &saramaProducerAdapter{producer: producer},
		client:   client,
		topic:    topic,
	}, nil
}

// Publish 将事件 JSON 序列化后发送到 Kafka topic。
func (p *kafkaPublisher) Publish(_ context.Context, event SyncEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	_, _, err = p.producer.SendMessage(data, p.topic)
	if err != nil {
		return fmt.Errorf("send kafka message: %w", err)
	}
	return nil
}

// Close 关闭 Kafka producer，释放资源。
func (p *kafkaPublisher) Close() error {
	return p.producer.Close()
}

// Ping 探测 Kafka 连通性（实现 pinger 接口）。
// fallbackPublisher.tryRecover 通过此方法判断 primary 是否恢复。
func (p *kafkaPublisher) Ping() error {
	if p.client == nil {
		return fmt.Errorf("kafka client not initialized")
	}
	if len(p.client.Brokers()) == 0 {
		return fmt.Errorf("no kafka brokers connected")
	}
	return nil
}
