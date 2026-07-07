# I-03: Mock Connector 实现

## 1. 任务概述

实现 Mock Connector，从 `testdata/` 目录读取 JSON 文件作为模拟数据源。提供 3 台设备、~12 个接口、若干 ISIS/Link/切片数据（~20 个节点），用于开发调试和集成测试。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 数据流管线 |
| 预估工时 | 1 天 |
| 前置任务 | D-03, D-04 |
| 交付物 | `internal/connector/mock/mock.go` + `testdata/` 目录下的 JSON 文件 |

## 2. 详细实现步骤

### Step 1: Mock Connector 实现

**文件**: `internal/connector/mock/mock.go`

```go
package mock

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"

    "gitlab.com/pml/network-digital-twin/internal/connector"
)

type MockConnector struct {
    name    string
    dataDir string
    types   []string
}

func NewMockConnector(name, dataDir string, types []string) *MockConnector {
    return &MockConnector{name: name, dataDir: dataDir, types: types}
}

func (m *MockConnector) Metadata() connector.ConnectorMetadata {
    return connector.ConnectorMetadata{
        Name:        m.name,
        Type:        "mock",
        EntityTypes: m.types,
    }
}

func (m *MockConnector) Collect(ctx context.Context, entityType string) ([]connector.Resource, error) {
    // 按实体类型映射到 JSON 文件名
    fileName := entityTypeToFile[entityType]
    filePath := filepath.Join(m.dataDir, fileName)

    data, err := os.ReadFile(filePath)
    if err != nil {
        return nil, fmt.Errorf("read mock data %s: %w", filePath, err)
    }

    var items []map[string]any
    if err := json.Unmarshal(data, &items); err != nil {
        return nil, fmt.Errorf("parse mock data %s: %w", filePath, err)
    }

    resources := make([]connector.Resource, 0, len(items))
    for i, item := range items {
        id, _ := item["id"].(string)
        if id == "" {
            id = fmt.Sprintf("%s-%d", entityType, i)
        }
        resources = append(resources, connector.Resource{
            Kind:       entityType,
            ID:         id,
            Properties: item,
        })
    }

    return resources, nil
}

func (m *MockConnector) Stream(ctx context.Context, entityType string) (<-chan connector.Resource, error) {
    return nil, fmt.Errorf("stream not implemented: %w", connector.ErrNotImplemented)
}

// entityTypeToFile 实体类型到 JSON 文件名的映射
var entityTypeToFile = map[string]string{
    "Device":          "devices.json",
    "Interface":       "interfaces.json",
    "ISIS":            "isis.json",
    "Link":            "links.json",
    "Network_Slice":   "network_slices.json",
    "Alarm":           "alarms.json",
}
```

### Step 2: 测试数据

**文件**: `testdata/mock_netbox/devices.json`（3 台设备）

```json
[
  {
    "serial_number": "SN12345",
    "hostname": "Router Core 01",
    "vendor": "Huawei",
    "hw_model": "NE40E",
    "mgmt_ip": "10.0.0.1",
    "chassis_mac": "AA:BB:CC:01:02:03",
    "status": "Up",
    "device_type": "Core",
    "interfaces": ["iface:SN12345_GE1/0/1", "iface:SN12345_GE1/0/2", "iface:SN12345_GE1/0/3", "iface:SN12345_GE1/0/4"]
  },
  {
    "serial_number": "SN12346",
    "hostname": "Router Edge 01",
    "vendor": "Huawei",
    "hw_model": "NE20E",
    "mgmt_ip": "10.0.0.2",
    "chassis_mac": "AA:BB:CC:01:02:04",
    "status": "Up",
    "device_type": "Edge",
    "interfaces": ["iface:SN12346_GE1/0/1", "iface:SN12346_GE1/0/2", "iface:SN12346_GE1/0/3", "iface:SN12346_GE1/0/4"],
    "upstream_links": ["iface:SN12345_GE1/0/1"]
  },
  {
    "serial_number": "SN12347",
    "hostname": "Switch Access 01",
    "vendor": "Huawei",
    "hw_model": "S5700",
    "mgmt_ip": "10.0.0.3",
    "chassis_mac": "AA:BB:CC:01:02:05",
    "status": "Up",
    "device_type": "Access",
    "interfaces": ["iface:SN12347_GE1/0/1", "iface:SN12347_GE1/0/2", "iface:SN12347_GE1/0/3", "iface:SN12347_GE1/0/4"],
    "upstream_links": ["iface:SN12346_GE1/0/1"]
  }
]
```

**文件**: `testdata/mock_netbox/interfaces.json`（12 个接口）

```json
[
  {"device_serial": "SN12345", "if_name": "GE1/0/1", "status": "Up", "bandwidth": 10000, "description": "Uplink to Edge01"},
  {"device_serial": "SN12345", "if_name": "GE1/0/2", "status": "Up", "bandwidth": 10000, "description": "Uplink backup"},
  {"device_serial": "SN12345", "if_name": "GE1/0/3", "status": "Down", "bandwidth": 1000, "description": "Unused"},
  {"device_serial": "SN12345", "if_name": "GE1/0/4", "status": "Up", "bandwidth": 1000, "description": "Management"},
  {"device_serial": "SN12346", "if_name": "GE1/0/1", "status": "Up", "bandwidth": 10000, "description": "Uplink to Core01"},
  {"device_serial": "SN12346", "if_name": "GE1/0/2", "status": "Up", "bandwidth": 10000, "description": "Downlink to Access01"},
  {"device_serial": "SN12346", "if_name": "GE1/0/3", "status": "Up", "bandwidth": 1000, "description": "Customer A"},
  {"device_serial": "SN12346", "if_name": "GE1/0/4", "status": "Down", "bandwidth": 1000, "description": "Unused"},
  {"device_serial": "SN12347", "if_name": "GE1/0/1", "status": "Up", "bandwidth": 1000, "description": "Uplink to Edge01"},
  {"device_serial": "SN12347", "if_name": "GE1/0/2", "status": "Up", "bandwidth": 1000, "description": "Access port 1"},
  {"device_serial": "SN12347", "if_name": "GE1/0/3", "status": "Up", "bandwidth": 1000, "description": "Access port 2"},
  {"device_serial": "SN12347", "if_name": "GE1/0/4", "status": "Up", "bandwidth": 1000, "description": "Access port 3"}
]
```

**文件**: `testdata/mock_cmdb/isis.json`（3 条 ISIS 路由协议实例）

```json
[
  {"isis_id": "ISIS-001", "system_id": "0000.0000.0001", "area_id": "49.0001", "level": "L1L2", "status": "Active", "run_on": ["iface:SN12345_GE1/0/1"]},
  {"isis_id": "ISIS-002", "system_id": "0000.0000.0002", "area_id": "49.0001", "level": "L1L2", "status": "Active", "run_on": ["iface:SN12345_GE1/0/2"]},
  {"isis_id": "ISIS-003", "system_id": "0000.0000.0003", "area_id": "49.0001", "level": "L2", "status": "Inactive", "run_on": ["iface:SN12346_GE1/0/2"]}
]
```

**文件**: `testdata/mock_cmdb/links.json`（2 条链路）

```json
[
  {"link_id": "LINK-001", "name": "Core-to-Edge-Primary", "bandwidth": 10000, "status": "Up", "endpoints": ["iface:SN12345_GE1/0/1", "iface:SN12346_GE1/0/1"]},
  {"link_id": "LINK-002", "name": "Core-to-Edge-Backup", "bandwidth": 10000, "status": "Up", "endpoints": ["iface:SN12345_GE1/0/2", "iface:SN12346_GE1/0/2"]}
]
```

**文件**: `testdata/mock_cmdb/network_slices.json`（1 个切片）

```json
[
  {"slice_id": "SLICE-001", "name": "Enterprise-Slice", "sla_bandwidth": 10000, "sla_latency": 50}
]
```

## 3. 设计原理

- **Mock 数据格式**：JSON 数组，每个元素是一条记录的 Properties，字段名与 Schema 的 `fieldMapping` 对应
- **关系字段以 URI 形式存储**：如 `interfaces: ["iface:SN12345_GE1/0/1", ...]`，Normalizer 保留在 Properties 中，GraphAssembler 推导为图边
- **两个 Mock 数据源**：mock-netbox（设备/接口）和 mock-cmdb（ISIS/Link/切片），模拟真实的多源场景
- **Stream 返回 ErrNotImplemented**：MVP 不实现流式推送

## 4. 验收标准

- [ ] `Collect("Device")` 返回 3 个 Resource
- [ ] `Collect("Interface")` 返回 12 个 Resource
- [ ] `Collect("ISIS")` 返回 3 个 Resource
- [ ] `Collect("Link")` 返回 2 个 Resource
- [ ] `Collect("Network_Slice")` 返回 1 个 Resource
- [ ] `Stream()` 返回 ErrNotImplemented
- [ ] Mock 数据的字段名与 Schema 的 fieldMapping 对应（如 `mgmt_ip` 而非 `management_ip`）
- [ ] 关系字段（interfaces/upstream_links/run_on/endpoints）以 URI 列表形式存在

## 5. 注意事项

- Mock 数据中使用 `mgmt_ip`（源字段名），Schema 的 `fieldMapping` 将其映射为 `management_ip`
- 关系字段值是 URI 字符串，必须与目标实体的 `uriTemplate` 格式一致
- `device_serial` + `if_name` 是 Interface 的 stableKeys，URI 格式为 `iface:{device_serial}_{if_name}`
- Mock 数据总量约 20+ 个节点，用于后续集成测试的数量验证
- `connector.ErrNotImplemented` 需要在 `internal/connector/types.go` 中定义为 `var ErrNotImplemented = errors.New("not implemented")`
