# D-02: Schema 数据结构 + 本体 YAML 定义

## 1. 任务概述

定义 Schema Registry 所需的所有 Go 结构体（EntityType、RelationType 等 K8s CRD 风格），并编写 6 个 EntityType + 4 个 RelationType 的 YAML 本体定义文件。这是系统"单一数据源"的基础，后续 Normalizer、GraphAssembler、Validator 都依赖这些定义。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 1: 设计阶段 |
| 预估工时 | 2 天 |
| 前置任务 | D-01 |
| 交付物 | `internal/schema/types.go` + 7 个 YAML 文件（device, interface, isis, link, network_slice, alarm, relations） |

## 2. 详细实现步骤

### Day 1: 数据结构定义

**文件**: `internal/schema/types.go`

```go
package schema

// EntityType 本体实体类型定义 (K8s CRD 风格)
type EntityType struct {
    APIVersion string         `yaml:"apiVersion"`
    Kind       string         `yaml:"kind"` // "EntityType"
    Metadata   Metadata       `yaml:"metadata"`
    Spec       EntityTypeSpec `yaml:"spec"`
}

type EntityTypeSpec struct {
    Identity       IdentitySpec                `yaml:"identity"`
    URITemplate    string                      `yaml:"uriTemplate"`
    FieldMapping   map[string]string            `yaml:"fieldMapping"`
    Normalize      []NormalizeRule              `yaml:"normalize"`
    RelationFields map[string]RelationFieldSpec `yaml:"relationFields"`
    Properties     map[string]PropertySpec      `yaml:"properties"`
}

// IdentitySpec 不可变身份标识 (URI 永久不变)
type IdentitySpec struct {
    StableKeys []string `yaml:"stableKeys"`
}

// NormalizeRule 字段标准化规则
type NormalizeRule struct {
    Field   string `yaml:"field"`
    Pattern string `yaml:"pattern"`
    Replace string `yaml:"replace"`
}

// RelationFieldSpec 关系字段映射 (Properties 字段 → RelationType)
type RelationFieldSpec struct {
    RelationType string `yaml:"relationType"`
}

// PropertySpec 属性定义
type PropertySpec struct {
    Type     string   `yaml:"type"`
    Required bool     `yaml:"required"`
    Enum     []string `yaml:"enum"`
    Default  any      `yaml:"default"`
}

// RelationType 关系类型定义
type RelationType struct {
    APIVersion string           `yaml:"apiVersion"`
    Kind       string           `yaml:"kind"` // "RelationType"
    Metadata   Metadata         `yaml:"metadata"`
    Spec       RelationTypeSpec `yaml:"spec"`
}

type RelationTypeSpec struct {
    Source []string `yaml:"source"`
    Target []string `yaml:"target"`
}

// Metadata 元数据
type Metadata struct {
    Name   string   `yaml:"name"`
    Labels []string `yaml:"labels"`
}
```

### Day 2: 本体 YAML 定义

#### `ontology/device.yaml` — Device EntityType（完整示例）

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Device
  labels: [Resource, Network]
spec:
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"
  fieldMapping:
    mgmt_ip: management_ip
    hw_model: model
  normalize:
    - field: hostname
      pattern: " "
      replace: "_"
  relationFields:
    interfaces:
      relationType: HAS_INTERFACE
    upstream_links:
      relationType: CONNECTS_TO
  properties:
    serial_number:
      type: string
      required: true
    hostname:
      type: string
      required: true
    vendor:
      type: string
    model:
      type: string
    management_ip:
      type: string
    chassis_mac:
      type: string
    status:
      type: string
      enum: [Up, Down, Maintenance]
      default: "Up"
    device_type:
      type: string
      enum: [Core, Edge, Access]
```

#### `ontology/interface.yaml` — Interface EntityType

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Interface
  labels: [Resource, Network]
spec:
  identity:
    stableKeys: [device_serial, if_name]
  uriTemplate: "iface:{device_serial}_{if_name}"
  fieldMapping: {}
  normalize: []
  relationFields: {}
  properties:
    device_serial:
      type: string
      required: true
    if_name:
      type: string
      required: true
    status:
      type: string
      enum: [Up, Down]
      default: "Up"
    bandwidth:
      type: int
    description:
      type: string
```

#### `ontology/isis.yaml` — ISIS EntityType

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: ISIS
  labels: [Protocol, Network]
spec:
  identity:
    stableKeys: [isis_id]
  uriTemplate: "isis:{isis_id}"
  fieldMapping: {}
  normalize: []
  relationFields:
    run_on:
      relationType: RUNS_ON
  properties:
    isis_id:
      type: string
      required: true
    system_id:
      type: string
      required: true
    area_id:
      type: string
    level:
      type: string
      enum: [L1, L2, L1L2]
      default: "L1L2"
    status:
      type: string
      enum: [Active, Inactive]
      default: "Active"
```

#### `ontology/link.yaml` — Link EntityType

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Link
  labels: [Resource, Network]
spec:
  identity:
    stableKeys: [link_id]
  uriTemplate: "link:{link_id}"
  fieldMapping: {}
  normalize: []
  relationFields:
    endpoints:
      relationType: ENDPOINT
  properties:
    link_id:
      type: string
      required: true
    name:
      type: string
    bandwidth:
      type: int
    status:
      type: string
      enum: [Up, Down]
      default: "Up"
```

#### `ontology/network_slice.yaml` — Network_Slice EntityType

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Network_Slice
  labels: [Service, Network]
spec:
  identity:
    stableKeys: [slice_id]
  uriTemplate: "slice:{slice_id}"
  fieldMapping: {}
  normalize: []
  relationFields: {}
  properties:
    slice_id:
      type: string
      required: true
    name:
      type: string
    sla_bandwidth:
      type: int
    sla_latency:
      type: int
```

#### `ontology/alarm.yaml` — Alarm EntityType

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Alarm
  labels: [Event, Network]
spec:
  identity:
    stableKeys: [alarm_id]
  uriTemplate: "alarm:{alarm_id}"
  fieldMapping: {}
  normalize: []
  relationFields:
    occurred_on:
      relationType: OCCURRED_ON
  properties:
    alarm_id:
      type: string
      required: true
    severity:
      type: string
      enum: [Critical, Major, Minor, Warning]
    message:
      type: string
    timestamp:
      type: string
```

#### `ontology/relations.yaml` — RelationType 定义（多文档 YAML）

```yaml
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_INTERFACE
spec:
  source: [Device]
  target: [Interface]
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: RUNS_ON
spec:
  source: [ISIS]
  target: [Interface]
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: ENDPOINT
spec:
  source: [Link]
  target: [Interface]
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: OCCURRED_ON
spec:
  source: [Alarm]
  target: [Interface]
```

## 3. 设计原理

- **K8s CRD 风格**：采用 `apiVersion` + `kind` + `metadata` + `spec` 的标准格式，开发者熟悉，支持多文档 YAML（`---` 分隔）
- **identity.stableKeys 不可变性**：URI 必须基于不可变标识（serial_number、chassis_mac），不能使用 hostname 或 management_ip，否则设备改名/换 IP 后 URI 失效，所有关系和快照历史断裂
- **relationFields 零代码扩展**：新增关系类型只需修改 YAML 中的 `relationFields` + `relations.yaml`，不需要改代码
- **fieldMapping + normalize 内嵌 Schema**：MVP 阶段映射规则不拆分为独立配置文件，保持单一数据源

### 与其他模块的交互

| 消费者 | 读取 Schema 的哪部分 | 用途 |
|--------|---------------------|------|
| Normalizer | EntityType | 属性类型、枚举、默认值 → 校验+标准化字段 |
| GraphAssembler | EntityType.relationFields + RelationType | relationFields 告知哪些字段推导关系，RelationType 校验源/目标类型 |
| Validator | EntityType | 校验最终数据合法性 |
| GraphDB | **不读 Schema** | 只接收 GraphModel，生成 Cypher + 执行 |

## 4. 验收标准

- [ ] `yaml.Unmarshal` 可以正确解析所有 YAML 文件到对应结构体
- [ ] 6 个 EntityType 的 stableKeys 均为不可变标识
- [ ] 4 个 RelationType 的 source/target 类型正确
- [ ] 多文档 YAML（relations.yaml）可被 `yaml.Decoder` 循环 Decode 正确解析
- [ ] `relationFields` 引用的 RelationType 在 `relations.yaml` 中都有定义
- [ ] 每个 EntityType 的 `properties` 中 stableKeys 字段标记为 `required: true`

## 5. 注意事项

- `identity.stableKeys` 必须是不可变标识，这是 URI 永久不变性的基石
- `uriTemplate` 只能引用 `stableKeys` 中的字段，不能使用可变属性
- `relationFields` 中的键名（如 `interfaces`）是 Properties 中的字段名，值中的 `relationType` 必须在 `relations.yaml` 中有定义
- Device 的 `relationFields` 中 `CONNECTS_TO` 在 `relations.yaml` 中暂时未定义，验证时再补充
- 每个 EntityType 至少包含 2-3 个 required 字段用于 Validator 测试
