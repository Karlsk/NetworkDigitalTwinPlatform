package connector

// Resource 是 Connector 产出的原始数据单元，
// 代表从外部数据源采集到的单个网络实体。
// Connector 只输出 Resource，不做字段映射或校验。
type Resource struct {
	Kind       string         // 实体类型，如 "Device"
	ID         string         // 原始 ID（数据源内部 ID）
	Properties map[string]any // 原始属性键值对
}

// ConnectorMetadata 描述连接器的身份和能力。
// ConnectorRegistry 以 Name 为 key 存储连接器。
type ConnectorMetadata struct {
	Name        string   // 连接器名称，如 "mock-netbox"
	Type        string   // 连接器类型，如 "netbox", "controller", "mock"
	EntityTypes []string // 支持的实体类型列表
}
