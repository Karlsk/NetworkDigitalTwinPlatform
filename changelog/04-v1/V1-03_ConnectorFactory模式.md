# V1-03: ConnectorFactory 模式

**工时**: 1 天
**前置**: V1-02
**风险等级**: 中
**Phase**: Phase 1 — 基础迁移

---

## 背景

当前 `cmd/server/main.go` 硬编码创建 MockConnector：
```go
netboxConn := mock.NewMockConnector("mock-netbox", "testdata/mock_netbox", ...)
connRegistry.Register(netboxConn)
```
V1 需根据 `connectors.yaml` 配置动态创建不同类型的 Connector。

---

## 实现内容

### 1. connector/config.go (新文件)

配置结构与加载：

```go
// ConnectorConfigEntry connectors.yaml 中单个 Connector 的配置。
type ConnectorConfigEntry struct {
    Name        string         `yaml:"name"`
    Type        string         `yaml:"type"`          // "mock" / "netbox" / "cmdb" / "controller"
    Config      map[string]any `yaml:"config"`        // 类型特定配置
    EntityTypes []string       `yaml:"entity_types"`  // 采集的实体类型列表
    Auth        AuthConfig     `yaml:"auth,omitempty"` // 认证配置
}

// AuthConfig 认证配置。
type AuthConfig struct {
    Type        string `yaml:"type"`          // "none" / "basic" / "token"
    TokenEnv    string `yaml:"token_env"`     // 从环境变量读取 token
    Username    string `yaml:"username"`      // basic auth 用户名
    PasswordEnv string `yaml:"password_env"`  // 从环境变量读取密码
}

// LoadConnectorConfig 从 YAML 文件加载 Connector 配置列表。
func LoadConnectorConfig(path string) ([]ConnectorConfigEntry, error)
```

### 2. connector/factory.go (新文件)

工厂模式：

```go
// ConnectorBuilder 工厂函数类型。
type ConnectorBuilder func(name string, cfg map[string]any, entityTypes []string) (Connector, error)

// ConnectorFactory 按 type 查找 Builder 并创建 Connector。
type ConnectorFactory struct {
    builders map[string]ConnectorBuilder
}

func NewConnectorFactory() *ConnectorFactory

// RegisterBuilder 注册某种 type 的构建器。
func (f *ConnectorFactory) RegisterBuilder(connType string, builder ConnectorBuilder)

// Create 按 ConnectorConfigEntry 创建单个 Connector。
func (f *ConnectorFactory) Create(entry ConnectorConfigEntry) (Connector, error)

// CreateFromConfig 加载 YAML 配置并批量注册到 ConnectorRegistry。
func (f *ConnectorFactory) CreateFromConfig(configPath string, registry *ConnectorRegistry) error
```

**内置 Mock Builder**:

```go
// 在 NewConnectorFactory 中默认注册 "mock" builder
func init() // 或在 NewConnectorFactory 中
// RegisterBuilder("mock", func(name string, cfg map[string]any, types []string) (Connector, error) {
//     dataDir := cfg["data_dir"].(string)
//     return mock.NewMockConnector(name, dataDir, types), nil
// })
```

### 3. configs/connectors.yaml 升级

```yaml
connectors:
  - name: mock-netbox
    type: mock
    config:
      data_dir: testdata/mock_netbox
    entity_types: [Device, Interface]

  - name: mock-cmdb
    type: mock
    config:
      data_dir: testdata/mock_cmdb
    entity_types: [ISIS, Link, Network_Slice]

  # V1-06 实现后取消注释:
  # - name: netbox-prod
  #   type: netbox
  #   config:
  #     base_url: https://netbox.example.com/api
  #     timeout: 30s
  #   auth:
  #     type: token
  #     token_env: NETBOX_TOKEN
  #   entity_types: [Device, Interface]
```

### 4. cmd/server/main.go 改造

```go
// MVP (硬编码):
// netboxConn := mock.NewMockConnector("mock-netbox", ...)
// connRegistry.Register(netboxConn)

// V1 (工厂模式):
factory := connector.NewConnectorFactory()
if err := factory.CreateFromConfig("configs/connectors.yaml", connRegistry); err != nil {
    log.Fatal("init connectors: ", err)
}
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/connector/config.go` | 新建 | ConnectorConfigEntry + LoadConnectorConfig |
| `internal/connector/factory.go` | 新建 | ConnectorFactory + ConnectorBuilder |
| `configs/connectors.yaml` | 修改 | 新增 entity_types 字段，结构升级 |
| `cmd/server/main.go` | 修改 | 用 ConnectorFactory 替代硬编码 |

---

## 验收标准

- [ ] 编译通过
- [ ] 仅修改 `connectors.yaml` 即可切换 Mock/真实 Connector（无需改代码）
- [ ] `ConnectorFactory.CreateFromConfig()` 正确创建所有已注册的 Connector
- [ ] `ConnectorFactory` 对未知 type 返回清晰错误: `"connector type %q: builder not registered"`
- [ ] Mock type builder 内置注册，启动即可用
- [ ] `cmd/server/main.go` 中无硬编码 Connector 创建
