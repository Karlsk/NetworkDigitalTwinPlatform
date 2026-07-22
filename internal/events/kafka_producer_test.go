// Package events Kafka Producer 单元测试（TDD RED 阶段）
package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/IBM/sarama"
)

// mockSyncProducer 实现 syncProducer 窄接口用于单元测试。
type mockSyncProducer struct {
	sendFunc func(msg []byte) (partition int32, offset int64, err error)
	closed   bool
}

func (m *mockSyncProducer) SendMessage(msg []byte, topic string) (int32, int64, error) {
	if m.sendFunc != nil {
		return m.sendFunc(msg)
	}
	return 0, 1, nil
}

func (m *mockSyncProducer) Close() error {
	m.closed = true
	return nil
}

// TestKafkaPublishSuccess 验证消息发送成功，JSON 序列化正确。
func TestKafkaPublishSuccess(t *testing.T) {
	var captured []byte
	mock := &mockSyncProducer{
		sendFunc: func(msg []byte) (int32, int64, error) {
			captured = msg
			return 0, 1, nil
		},
	}

	pub := &kafkaPublisher{producer: mock, topic: "test-topic"}

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Connector:  "mock-netbox",
		Data: []map[string]any{
			{"hostname": "router-01", "vendor": "Huawei"},
		},
		Relations: []Relation{
			{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:SN001_eth0"},
		},
	}

	err := pub.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("Publish() error = %v, want nil", err)
	}

	// 验证 JSON 序列化正确
	var decoded SyncEvent
	if err := json.Unmarshal(captured, &decoded); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}
	if decoded.Action != "update" {
		t.Errorf("decoded.Action = %q, want %q", decoded.Action, "update")
	}
	if decoded.EntityType != "Device" {
		t.Errorf("decoded.EntityType = %q, want %q", decoded.EntityType, "Device")
	}
	if decoded.Connector != "mock-netbox" {
		t.Errorf("decoded.Connector = %q, want %q", decoded.Connector, "mock-netbox")
	}
	if len(decoded.Data) != 1 {
		t.Fatalf("decoded.Data length = %d, want 1", len(decoded.Data))
	}
	if decoded.Data[0]["hostname"] != "router-01" {
		t.Errorf("decoded.Data[0][hostname] = %v, want %q", decoded.Data[0]["hostname"], "router-01")
	}
	if len(decoded.Relations) != 1 {
		t.Fatalf("decoded.Relations length = %d, want 1", len(decoded.Relations))
	}
	if decoded.Relations[0].Type != "HAS_INTERFACE" {
		t.Errorf("decoded.Relations[0].Type = %q, want %q", decoded.Relations[0].Type, "HAS_INTERFACE")
	}
}

// TestKafkaPublishError 验证 Producer 失败时返回 error。
func TestKafkaPublishError(t *testing.T) {
	mock := &mockSyncProducer{
		sendFunc: func(msg []byte) (int32, int64, error) {
			return 0, 0, errors.New("kafka broker unavailable")
		},
	}

	pub := &kafkaPublisher{producer: mock, topic: "test-topic"}

	err := pub.Publish(context.Background(), SyncEvent{Action: "update"})
	if err == nil {
		t.Fatal("Publish() should return error when producer fails")
	}
}

// TestKafkaClose 验证 Close 调用无 panic 且正确关闭 producer。
func TestKafkaClose(t *testing.T) {
	mock := &mockSyncProducer{}
	pub := &kafkaPublisher{producer: mock, topic: "test-topic"}

	err := pub.Close()
	if err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if !mock.closed {
		t.Error("Close() did not close the producer")
	}
}

// TestKafkaPublisherPingNilClient 验证 Ping 在 client 为 nil 时返回 error。
func TestKafkaPublisherPingNilClient(t *testing.T) {
	pub := &kafkaPublisher{producer: &mockSyncProducer{}, topic: "t"}
	err := pub.Ping()
	if err == nil {
		t.Fatal("Ping() with nil client should return error")
	}
}

// TestNewKafkaPublisherInvalidBrokers 验证无效 broker 地址返回 error。
func TestNewKafkaPublisherInvalidBrokers(t *testing.T) {
	cfg := sarama.NewConfig()
	cfg.Producer.Return.Successes = true
	_, err := NewKafkaPublisher([]string{"localhost:59999"}, "test-topic", cfg)
	if err == nil {
		t.Fatal("NewKafkaPublisher() with unreachable broker should return error")
	}
}
