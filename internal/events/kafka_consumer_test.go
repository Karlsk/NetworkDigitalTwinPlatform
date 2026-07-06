// Package events Kafka Consumer 单元测试（TDD GREEN 阶段）
package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/IBM/sarama"
)

// ---------------------------------------------------------------------------
// Mock: sarama.ConsumerGroupSession
// ---------------------------------------------------------------------------

type mockConsumerGroupSession struct {
	ctx          context.Context
	markedMsgIDs []int64 // 记录 MarkMessage 的 offset
}

func (m *mockConsumerGroupSession) Claims() map[string][]int32    { return nil }
func (m *mockConsumerGroupSession) MemberID() string              { return "mock-member" }
func (m *mockConsumerGroupSession) GenerationID() int32           { return 1 }
func (m *mockConsumerGroupSession) Commit()                       {}
func (m *mockConsumerGroupSession) MarkOffset(string, int32, int64, string) {}
func (m *mockConsumerGroupSession) ResetOffset(string, int32, int64, string) {}
func (m *mockConsumerGroupSession) Context() context.Context      { return m.ctx }

func (m *mockConsumerGroupSession) MarkMessage(msg *sarama.ConsumerMessage, _ string) {
	m.markedMsgIDs = append(m.markedMsgIDs, msg.Offset)
}

// ---------------------------------------------------------------------------
// Mock: sarama.ConsumerGroupClaim
// ---------------------------------------------------------------------------

type mockConsumerGroupClaim struct {
	messages chan *sarama.ConsumerMessage
}

func (m *mockConsumerGroupClaim) Topic() string                        { return "test-topic" }
func (m *mockConsumerGroupClaim) Partition() int32                     { return 0 }
func (m *mockConsumerGroupClaim) InitialOffset() int64                 { return 0 }
func (m *mockConsumerGroupClaim) HighWaterMarkOffset() int64           { return 100 }
func (m *mockConsumerGroupClaim) Messages() <-chan *sarama.ConsumerMessage { return m.messages }

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

func marshalEvent(t *testing.T, event SyncEvent) []byte {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return data
}

func makeMessage(offset int64, value []byte) *sarama.ConsumerMessage {
	return &sarama.ConsumerMessage{
		Topic:     "test-topic",
		Partition: 0,
		Offset:    offset,
		Value:     value,
	}
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

// TestKafkaConsumeMessage 验证消息反序列化正确，handler 被调用，MarkMessage 被调用。
func TestKafkaConsumeMessage(t *testing.T) {
	ctx := context.Background()
	session := &mockConsumerGroupSession{ctx: ctx}

	event := SyncEvent{
		Action:     "update",
		EntityType: "Device",
		Connector:  "mock-netbox",
		Data: []map[string]any{
			{"hostname": "router-01"},
		},
		Relations: []Relation{
			{Type: "HAS_INTERFACE", From: "device:SN001", To: "iface:eth0"},
		},
	}

	claim := &mockConsumerGroupClaim{
		messages: make(chan *sarama.ConsumerMessage, 1),
	}
	claim.messages <- makeMessage(42, marshalEvent(t, event))
	close(claim.messages)

	var received *SyncEvent
	handler := func(_ context.Context, e SyncEvent) error {
		received = &e
		return nil
	}

	h := &consumerHandler{handler: handler}
	if err := h.ConsumeClaim(session, claim); err != nil {
		t.Fatalf("ConsumeClaim() error = %v", err)
	}

	if received == nil {
		t.Fatal("handler was not called")
	}
	if received.Action != "update" {
		t.Errorf("Action = %q, want %q", received.Action, "update")
	}
	if received.EntityType != "Device" {
		t.Errorf("EntityType = %q, want %q", received.EntityType, "Device")
	}
	if received.Connector != "mock-netbox" {
		t.Errorf("Connector = %q, want %q", received.Connector, "mock-netbox")
	}
	if len(received.Data) != 1 || received.Data[0]["hostname"] != "router-01" {
		t.Errorf("Data = %v, want [{hostname: router-01}]", received.Data)
	}
	if len(received.Relations) != 1 || received.Relations[0].Type != "HAS_INTERFACE" {
		t.Errorf("Relations = %v, want [{Type: HAS_INTERFACE}]", received.Relations)
	}

	// 验证 MarkMessage 被调用
	if len(session.markedMsgIDs) != 1 || session.markedMsgIDs[0] != 42 {
		t.Errorf("markedMsgIDs = %v, want [42]", session.markedMsgIDs)
	}
}

// TestKafkaConsumeInvalidJSON 验证无效 JSON 跳过，不中断消费，后续消息正常处理。
func TestKafkaConsumeInvalidJSON(t *testing.T) {
	ctx := context.Background()
	session := &mockConsumerGroupSession{ctx: ctx}

	goodEvent := SyncEvent{Action: "update", EntityType: "Interface", Connector: "mock"}

	claim := &mockConsumerGroupClaim{
		messages: make(chan *sarama.ConsumerMessage, 3),
	}
	// 第一条：无效 JSON
	claim.messages <- makeMessage(1, []byte("not valid json"))
	// 第二条：正常消息
	claim.messages <- makeMessage(2, marshalEvent(t, goodEvent))
	// 第三条：空字节
	claim.messages <- makeMessage(3, []byte(""))
	close(claim.messages)

	var callCount int
	handler := func(_ context.Context, _ SyncEvent) error {
		callCount++
		return nil
	}

	h := &consumerHandler{handler: handler}
	if err := h.ConsumeClaim(session, claim); err != nil {
		t.Fatalf("ConsumeClaim() error = %v", err)
	}

	// 只有第二条正常消息触发 handler
	if callCount != 1 {
		t.Errorf("handler callCount = %d, want 1", callCount)
	}

	// 三条消息都应该被 MarkMessage（不阻塞消费）
	if len(session.markedMsgIDs) != 3 {
		t.Errorf("markedMsgIDs = %v, want 3 messages marked", session.markedMsgIDs)
	}
}

// TestKafkaConsumeHandlerError 验证 handler 返回 error 时继续消费，MarkMessage 仍被调用。
func TestKafkaConsumeHandlerError(t *testing.T) {
	ctx := context.Background()
	session := &mockConsumerGroupSession{ctx: ctx}

	event1 := SyncEvent{Action: "update", EntityType: "Device", Connector: "c1"}
	event2 := SyncEvent{Action: "delete", EntityType: "Link", Connector: "c2", URIs: []string{"link:1"}}

	claim := &mockConsumerGroupClaim{
		messages: make(chan *sarama.ConsumerMessage, 2),
	}
	claim.messages <- makeMessage(10, marshalEvent(t, event1))
	claim.messages <- makeMessage(11, marshalEvent(t, event2))
	close(claim.messages)

	var callCount int
	handler := func(_ context.Context, e SyncEvent) error {
		callCount++
		if e.Action == "update" {
			return errors.New("neo4j connection lost")
		}
		return nil
	}

	h := &consumerHandler{handler: handler}
	if err := h.ConsumeClaim(session, claim); err != nil {
		t.Fatalf("ConsumeClaim() error = %v", err)
	}

	// 两条消息都应该被处理
	if callCount != 2 {
		t.Errorf("handler callCount = %d, want 2", callCount)
	}

	// 两条消息都应该被 MarkMessage（幂等 + 避免无限重试）
	if len(session.markedMsgIDs) != 2 {
		t.Errorf("markedMsgIDs = %v, want 2 messages marked", session.markedMsgIDs)
	}
}

// TestKafkaConsumeCancel 验证 ctx 取消后 Consume 正确退出。
// 测试 kafkaConsumer.Consume 方法（通过 mock consumerGroup）。
func TestKafkaConsumeCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mock := &mockConsumerGroupClient{
		consumeFunc: func(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error {
			// 模拟正常返回（rebalance 或 ctx 取消）
			<-ctx.Done()
			return nil
		},
	}

	consumer := &kafkaConsumer{
		client:  mock,
		topic:   "test-topic",
		groupID: "test-group",
	}

	done := make(chan error, 1)
	go func() {
		done <- consumer.Consume(ctx, func(_ context.Context, _ SyncEvent) error {
			return nil
		})
	}()

	// 取消 context
	cancel()

	err := <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Consume() after cancel: error = %v, want nil or context.Canceled", err)
	}
}

// TestKafkaConsumerClose 验证 Close 正确关闭底层 client。
func TestKafkaConsumerClose(t *testing.T) {
	mock := &mockConsumerGroupClient{}
	consumer := &kafkaConsumer{client: mock}

	if err := consumer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !mock.closed {
		t.Error("Close() did not close the underlying client")
	}
}

// ---------------------------------------------------------------------------
// Mock: consumerGroup 窄接口
// ---------------------------------------------------------------------------

type mockConsumerGroupClient struct {
	consumeFunc func(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error
	errCh       chan error
	closed      bool
}

func (m *mockConsumerGroupClient) Consume(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error {
	if m.consumeFunc != nil {
		return m.consumeFunc(ctx, topics, handler)
	}
	return nil
}

func (m *mockConsumerGroupClient) Errors() <-chan error {
	if m.errCh == nil {
		m.errCh = make(chan error)
	}
	return m.errCh
}

func (m *mockConsumerGroupClient) Close() error {
	m.closed = true
	return nil
}

// ---------------------------------------------------------------------------
// TestKafkaDataSourceConsumerForwarding 验证 DataSource Consumer 将事件转发给 publisher.Publish
// ---------------------------------------------------------------------------

func TestKafkaDataSourceConsumerForwarding(t *testing.T) {
	goCtx, goCancel := context.WithCancel(context.Background())
	defer goCancel()

	// 构建 mock publisher
	var published []SyncEvent
	mockPub := &mockEventPublisher{
		publishFunc: func(_ context.Context, e SyncEvent) error {
			published = append(published, e)
			return nil
		},
	}

	// 构建 mock consumerGroup：第一次消费一条消息，第二次 ctx 已取消直接退出
	callCount := 0
	mockCG := &mockConsumerGroupClient{
		consumeFunc: func(ctx context.Context, topics []string, handler sarama.ConsumerGroupHandler) error {
			callCount++
			if callCount > 1 {
				goCancel()
				return nil
			}
			// 模拟消费一条消息
			event := SyncEvent{
				Action:     "update",
				EntityType: "Device",
				Connector:  "external-kafka",
			}
			msg := makeMessage(1, marshalEvent(t, event))
			claim := &mockConsumerGroupClaim{
				messages: make(chan *sarama.ConsumerMessage, 1),
			}
			claim.messages <- msg
			close(claim.messages)

			session := &mockConsumerGroupSession{ctx: ctx}
			_ = handler.ConsumeClaim(session, claim)
			return nil
		},
	}

	// 构建内部 kafkaConsumer，注入 mock client
	innerConsumer := &kafkaConsumer{
		client:  mockCG,
		topic:   "external-topic",
		groupID: "ds-group",
	}

	// 构建 KafkaDataSourceConsumer，注入 inner consumer
	ds := &KafkaDataSourceConsumer{
		brokers: []string{"localhost:9092"},
		topic:   "external-topic",
		groupID: "ds-group",
		inner:   innerConsumer,
	}

	done := make(chan error, 1)
	go func() {
		done <- ds.Start(goCtx, mockPub)
	}()

	err := <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v", err)
	}

	if len(published) != 1 {
		t.Fatalf("published count = %d, want 1", len(published))
	}
	if published[0].Action != "update" {
		t.Errorf("published[0].Action = %q, want %q", published[0].Action, "update")
	}
	if published[0].Connector != "external-kafka" {
		t.Errorf("published[0].Connector = %q, want %q", published[0].Connector, "external-kafka")
	}
}

// ---------------------------------------------------------------------------
// Mock: EventPublisher
// ---------------------------------------------------------------------------

type mockEventPublisher struct {
	publishFunc func(ctx context.Context, event SyncEvent) error
	closed      bool
}

func (m *mockEventPublisher) Publish(ctx context.Context, event SyncEvent) error {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, event)
	}
	return nil
}

func (m *mockEventPublisher) Close() error {
	m.closed = true
	return nil
}
