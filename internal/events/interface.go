// Package events 定义事件总线抽象接口。
// 支持 Channel（内存）和 Kafka（持久化）两种实现。
package events

import "context"

// Relation 关系描述（内联定义，避免循环导入 assembler 包）。
// 对应 assembler.Relation 的简化版本，仅包含必要字段。
type Relation struct {
	Type  string         `json:"type"`            // 关系类型，如 "HAS_INTERFACE"
	From  string         `json:"from"`            // 源节点 URI
	To    string         `json:"to"`              // 目标节点 URI
	Props map[string]any `json:"props,omitempty"` // 关系属性（通常为空）
}

// SyncEvent 同步事件（复用 service.SyncEvent 结构）。
// 此处重新定义以避免循环导入。
// Action 支持三种值: "update", "delete", "delete_relation"。
type SyncEvent struct {
	Action     string           `json:"action"`                // "update", "delete", "delete_relation"
	EntityType string           `json:"entity_type"`           // 实体类型
	Connector  string           `json:"connector"`             // 连接器名称
	Data       []map[string]any `json:"data,omitempty"`        // update 时的数据 (Webhook 原始 JSON)
	URIs       []string         `json:"uris,omitempty"`        // delete 时的 URI 列表
	Relations  []Relation       `json:"relations,omitempty"`   // delete_relation 时的关系列表
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
