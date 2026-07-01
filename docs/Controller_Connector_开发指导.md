# Controller Connector 开发指导手册

> 基于骨干网操作系统 API 接口规范 v1.5
>
> 目标: 实现 Controller Connector，从控制器采集所有本体（ontology）数据
>
> 约束: 所有本体信息必须通过 Controller API 获取，不允许使用其他数据源

---

## 1. 总体设计

### 1.1 实体范围

本次开发覆盖 8 个本体实体类型：

| 实体 | 数据来源特征 | 采集复杂度 |
|------|------------|----------|
| Device | 结构化 JSON API | 低 |
| Interface | 嵌套在 Device 响应中 | 中 |
| Link | 结构化 JSON API | 低 |
| Alarm | 结构化 JSON API | 低 |
| VPN | 结构化 JSON（自定义分页） | 中 |
| Tunnel | 深度嵌套 JSON（4-5 层） | 高 |
| ISIS | **文本回显**（厂商差异） | 高 |
| BGP | **文本回显**（厂商差异） | 高 |

### 1.2 数据流管线

```
Controller REST API
  → ControllerConnector.Collect()
    → transform.go（字段展平 + kebab→snake 转换）
    → []connector.Resource
      → Normalizer（fieldMapping + normalize + validate + URI）
        → GraphAssembler（节点 + 关系推导）
          → Neo4j
```

### 1.3 文件结构

```
internal/connector/controller/
  controller.go    -- 主体: Metadata/Collect/Stream/Ping + 8 个 collectXxx
  transform.go     -- 字段展平: 8 个 transform 函数
  parser.go        -- 文本解析器: parseISISText / parseBGPText（厂商感知）
  register.go      -- Builder() 工厂函数
```

---

## 2. 关键 API 筛选

### 2.1 认证接口

| 项目 | 说明 |
|------|------|
| **章节** | 1.1 获取 Token |
| **URL** | `POST /oauth/token` |
| **请求体** | `{"username": "admin", "password": "xxx", "device_id": "uuid"}` |
| **响应关键字段** | `access_token`（Bearer Token）、`expires_in`（秒，通常 3600） |
| **使用方式** | 后续所有请求 Header 携带 `Authorization: Bearer {access_token}` |

**注意事项**:
- Token 有过期时间，需在 ControllerConnector 中实现 Token 自动刷新机制
- 建议在每次 HTTP 请求前检查 Token 是否即将过期（预留 60s 缓冲）

### 2.2 Device — 设备信息

| 项目 | 全量接口 | 分页接口 |
|------|---------|---------|
| **章节** | 2.5 | 2.2 |
| **URL** | `GET /api/no/config/terra-pe:peInfos/peInfos` | `POST /api/no/config/terra-pe:peInfos/page?pageNumber=0&pageSize=100` |
| **方法** | GET | POST |
| **响应格式** | JSON 数组 | Spring Data 分页（见 3.2 节） |

**推荐**: 数据量小于 200 台设备时使用全量接口，否则使用分页接口。

**响应关键字段（PE 设备对象）**:

| API 字段 | 类型 | 说明 |
|---------|------|------|
| `id` | String | 设备唯一标识（UUID） |
| `name` | String | 设备名称（如 "NJ-SCT-R01"） |
| `pe-alias` | String | PE 别名 |
| `node-type` | String | 节点类型（PE/P/CE 等） |
| `vendor-id` | String | 厂商（H3C/ZTE/Huawei） |
| `platform-id` | String | 平台型号 |
| `product-name` | String | 产品型号 |
| `version` | String | 软件版本 |
| `management-ip` | String | 管理 IP |
| `connect-status` | String | 连接状态（UP/DOWN/UNKNOWN） |
| `pe-as` | Integer | AS 号 |
| `isis-process-id` | Integer | ISIS 进程 ID |
| `bgp-loopback-ip` | String | BGP Loopback IP |
| `locator` | String | SRv6 Locator |
| `pop-id` | String | POP 点 ID |
| `alerts` | Object | 告警统计（warning/minor/major/critical） |
| `peports` | Object | 端口信息（嵌套结构） |

**嵌套结构 `peports.peport-info[]`（端口/接口）**:

| API 字段 | 类型 | 说明 |
|---------|------|------|
| `id` | String | 端口 UUID |
| `name` | String | 端口名称（如 "HundredGigE1/0/25"） |
| `port-alias` | String | 端口别名 |
| `port-type` | String | 端口类型（UNI/NNI 等） |
| `port-speed` | String | 端口速率 |
| `status` | String | 端口状态（UP/DOWN） |
| `total-bandwidth` | Integer | 总带宽 (Mbps) |
| `cfg-bw` | Integer | 配置带宽 |
| `ipv4-addr` | String | IPv4 地址 |
| `intf-description` | String | 接口描述 |
| `physical-port` | Boolean | 是否物理端口 |
| `shutdown` | Boolean | 是否关闭 |

### 2.3 Link — 链路信息

| 项目 | 全量接口 | 分页接口 |
|------|---------|---------|
| **章节** | 2.17 | 2.16 |
| **URL** | `GET /api/sr/config/network-topology:network-topology/topology/linksInfo` | `GET .../linksInfo/page?page=0&size=100` |
| **方法** | GET | GET |
| **响应格式** | JSON 数组 | Spring Data 分页 |

**响应关键字段（链路对象）**:

| API 字段 | 类型 | 说明 |
|---------|------|------|
| `link-id` | String | 链路唯一标识（格式: `源节点:源端口>目的节点:目的端口`） |
| `link-status` | String | 链路状态（UP/DOWN） |
| `link-type` | String | 链路类型（LAN/P2P） |
| `cfg-bw` | Integer | 配置带宽 (Mbps) |
| `oper-bw` | Integer | 运营带宽 |
| `utilization-ratio` | Float | 带宽利用率 |
| `delay` | Float | 时延 (ms) |
| `loss` | Float | 丢包率 (%) |
| `jitter` | Float | 抖动 (ms) |
| `source.source-node` | String | 源节点名称 |
| `source.source-tp` | String | 源端口名称 |
| `destination.dest-node` | String | 目的节点名称 |
| `destination.dest-tp` | String | 目的端口名称 |

### 2.4 Alarm — 告警信息

| 项目 | 说明 |
|------|------|
| **章节** | 6.5 告警统计 |
| **URL** | `GET /monitor/alert/list?namespace=business&interval={interval}` |
| **方法** | GET |
| **interval 参数** | `5m` / `1h` / `1d` / `1M`（默认 5m） |

**响应结构**: `{"code": 0, "message": "请求成功", "data": [...]}`

**告警对象关键字段**:

| API 字段 | 类型 | 说明 |
|---------|------|------|
| `id` | String | 告警 UUID |
| `level` | String | 严重级别（CRITICAL/MAJOR/MINOR/WARNING） |
| `category` | String | 告警类别（如 "ISIS邻居Down"） |
| `msg` | String | 告警消息详情 |
| `source` | String | 告警源设备名 |
| `component` | String | 关联组件 |
| `time` | String | 告警时间 |
| `recoveryTime` | String | 恢复时间（null 表示未恢复） |
| `module` | String | 模块名（device 等） |
| `serialNo` | String | 告警序列号 |

### 2.5 VPN — 虚拟专网

#### L3VPN（第 3 章）

| 项目 | 说明 |
|------|------|
| **章节** | 3.3 分页查询 L3VPN |
| **URL** | `GET /api/no/config/ietf-l3vpn-ntw:l3vpn-ntw/page?pageNo=0&pageSize=100` |
| **方法** | GET |
| **响应格式** | 自定义分页（见 3.3 节） |

**L3VPN 对象关键字段**:

| API 字段 | 类型 | 说明 |
|---------|------|------|
| `vpn-services[].vpn-id` | String | VPN 唯一标识（如 "l3_3307"） |
| `vpn-services[].svc-name` | String | VPN 名称 |
| `vpn-services[].vpn-svc-type` | String | 服务类型（mpls-vpn/vxlan-evpn） |
| `vpn-services[].vpn-tunnel-type` | String | 隧道类型（sr-mpls/srv6/mpls） |
| `vpn-services[].vpn-service-topology` | String | 拓扑类型（any-to-any/hub-spoke） |
| `vpn-services[].site-count` | Integer | 站点数量 |
| `vpn-services[].sna-count` | Integer | SNA 数量 |
| `vpn-services[].pre-create-status` | String | 预创建状态（up/down） |
| `vpn-services[].create-time` | String | 创建时间 |

#### L2VPN（第 3 章）

| 项目 | 说明 |
|------|------|
| **章节** | 3.19 分页查询 L2VPN |
| **URL** | `GET /api/no/config/ietf-l2vpn-svc:l2vpn-svc/page?pageNo=0&pageSize=100` |
| **方法** | GET |
| **响应格式** | 自定义分页（与 L3VPN 相同） |

**L2VPN 对象关键字段**:

| API 字段 | 类型 | 说明 |
|---------|------|------|
| `vpn-services[].vpn-id` | String | VPN 唯一标识（如 "l2_3310"） |
| `vpn-services[].svc-name` | String | VPN 名称 |
| `vpn-services[].vpn-svc-type` | String | 服务类型（vpls/vpws） |
| `vpn-services[].vpn-tunnel-type` | String | 隧道类型 |
| `vpn-services[].svc-topo` | String | 服务拓扑 |

### 2.6 Tunnel — 隧道信息

| 项目 | 说明 |
|------|------|
| **章节** | 4.5 查询所有策略实例 |
| **URL** | `GET /api/sr/config/terra-te-svc:te-policy-instance/all` |
| **方法** | GET |
| **响应格式** | JSON 数组 |

**说明**: 仅使用此 API 作为 Tunnel 数据源。该 API 返回的策略实例中嵌套了完整的 tunnel 信息（explicit-tunnel → te-path → explicit-path），不使用 4.4（按设备查询 tunnel）和 8.15（SRv6 切片列表）。

**策略实例对象关键字段**:

| API 字段 | 类型 | 说明 |
|---------|------|------|
| `instance-id` | String | 实例唯一标识 |
| `policy-template-name` | String | 策略模板名称 |
| `cfg-status` | String | 配置状态（COMPLETED/PENDING/FAILED） |
| `cfg-action` | String | 配置动作（add/delete） |
| `unbound` | Boolean | 是否未绑定 |
| `te-policy-targets.src-device` | String | 源设备 |
| `te-policy-targets.dst-device` | String | 目的设备 |
| `te-policy-targets.flow-type` | String | 流类型（vpn） |
| `te-policy-targets.l3-vpn-id` | String | 关联的 VPN ID |
| `te-tuples[].color` | String | 颜色标识 |
| `te-tuples[].explicit-tunnel[].tunnel-id` | String | 隧道 ID |
| `te-tuples[].explicit-tunnel[].tunnel-name` | String | 隧道名称 |
| `te-tuples[].explicit-tunnel[].src-device` | String | 隧道源设备 |
| `te-tuples[].explicit-tunnel[].dst-device` | String | 隧道目的设备 |
| `te-tuples[].explicit-tunnel[].te-path[].oper-status` | String | 路径操作状态 |
| `te-tuples[].explicit-tunnel[].te-path[].delay` | Float | 路径时延 |
| `te-tuples[].explicit-tunnel[].te-path[].loss` | Float | 路径丢包 |

### 2.7 ISIS — ISIS 邻居信息（文本回显）

| 项目 | 说明 |
|------|------|
| **章节** | 5.10 查看全局 ISIS 邻居 |
| **URL** | `POST /restconf/operations/oper-rpc:isis-neighbor` |
| **方法** | POST |
| **响应格式** | **设备 CLI 回显字符串**（非结构化 JSON） |

**请求体**:
```json
{
  "input": {
    "pe-name": "NJ-SCT-R01",
    "process": 10,
    "verbose": true,
    "scope": "isis"
  }
}
```

**响应结构**: `{"output": {"current-config-result": "display isis peer ..."}}`

### 2.8 BGP — BGP 邻居信息（文本回显）

| 项目 | 说明 |
|------|------|
| **章节** | 5.9 查看全局 BGP 邻居 |
| **URL** | `POST /restconf/operations/oper-rpc:bgp-peer-config` |
| **方法** | POST |
| **响应格式** | **设备 CLI 回显字符串**（非结构化 JSON） |

**请求体**:
```json
{
  "input": {
    "pe-name": "NJ-SCT-R01",
    "scope": "IPv4"
  }
}
```

**响应结构**: `{"output": {"current-config-result": "display bgp peer ipv4 ..."}}`

**BGP 回显示例**:
```
display bgp peer ipv4
 BGP local router ID: 172.16.11.2
 Local AS number: 137749
 Total number of peers: 8       Peers in established state: 1
 * - Dynamically created peer
 Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
 1.1.1.3             137749        0        0    0        0 5523h36m Connect
 4.4.4.9               2311        0        0    0        0 5328h57m Connect
 172.16.11.4         137749   115005    18053    0      514 0230h43m Established
 2001:DB8:4:100::      65000        0        0    0        0 5523h39m Idle
```

### 2.9 补充推荐 API

| API | URL | 用途 |
|-----|-----|------|
| 6.6 全拓扑 | `GET /api/sr/config/network-topology:network-topology` | 一次性获取 nodes + links，可作为 Device/Interface/Link 的替代采集方案 |
| 6.7 节点拓扑 | `GET .../network-topology/nodes` | 仅节点（含 termination-point 端口列表） |
| 6.8 链路拓扑 | `GET .../network-topology/links` | 仅链路 |

---

## 3. 分页格式详解

Controller API 使用两套与现有 `HTTPClient.Paginate`（Netbox DRF 风格）**不兼容**的分页格式。

### 3.1 Spring Data 风格（Device 分页 / Link 分页）

```json
{
  "content": [ ... ],
  "total_elements": 6,
  "total_pages": 1,
  "last": true,
  "first": true,
  "size": 10,
  "number": 0,
  "number_of_elements": 6,
  "empty": false
}
```

**遍历方式**: `number` 从 0 开始递增，当 `last == true` 时停止。

### 3.2 VPN 自定义分页

```json
{
  "page_num": 1,
  "page_size": 20,
  "total_elements": 50,
  "total_pages": 3,
  "start_index": 0,
  "content": [ ... ],
  "start": 0,
  "end": 20
}
```

**遍历方式**: `page_num` 从 1 开始递增（注意与 Spring Data 的 0-based 不同），当 `page_num >= total_pages` 时停止。

### 3.3 建议实现

在 `controller.go` 中实现两个私有分页方法:

```go
// paginateSpringData 遍历 Spring Data 风格分页
func (c *ControllerConnector) paginateSpringData(ctx context.Context, path string, pageSize int,
    callback func(page []map[string]any) error) error

// paginateVPN 遍历 VPN 自定义分页
func (c *ControllerConnector) paginateVPN(ctx context.Context, baseURL string, pageSize int,
    callback func(page []map[string]any) error) error
```

---

## 4. 字段映射方案

### 4.1 Device

| API 字段（kebab-case） | Ontology 属性（snake_case） | 说明 |
|----------------------|--------------------------|------|
| `name` | `serial_number` | 稳定键（Controller 无独立序列号字段） |
| `name` | `hostname` | 设备名 |
| `vendor-id` | `vendor` | 厂商 |
| `product-name` | `model` | 型号 |
| `management-ip` | `management_ip` | 管理 IP |
| `connect-status` | `status` | 枚举映射: UP→Up, DOWN→Down, UNKNOWN→Down |
| `node-type` | `device_type` | 枚举映射: PE→Edge, P→Core, CE→Access |
| `pe-alias` | `alias` | 额外属性 |
| `platform-id` | `platform` | 额外属性 |
| `pe-as` | `as_number` | 额外属性 |

**fieldMapping 建议**: ontology/device.yaml 中配置:
```yaml
fieldMapping:
  mgmt_ip: management_ip     # 保持兼容
  hw_model: model
```

### 4.2 Interface

| API 字段 | Ontology 属性 | 说明 |
|---------|-------------|------|
| 所属设备 `name` | `device_serial` | 稳定键之一 |
| `peport-info.name` | `if_name` | 稳定键之二 |
| `peport-info.status` | `status` | UP→Up, DOWN→Down |
| `peport-info.total-bandwidth` | `bandwidth` | 带宽 (Mbps) |
| `peport-info.intf-description` | `description` | 接口描述 |
| `peport-info.port-type` | `port_type` | 额外属性 |
| `peport-info.port-speed` | `port_speed` | 额外属性 |
| `peport-info.ipv4-addr` | `ipv4_address` | 额外属性 |

### 4.3 Link

| API 字段 | Ontology 属性 | 说明 |
|---------|-------------|------|
| `link-id` | `link_id` | 稳定键 |
| `link-id` | `name` | 链路名称 |
| `cfg-bw` | `bandwidth` | 带宽 (Mbps) |
| `link-status` | `status` | UP→Up, DOWN→Down |
| `source.source-node` | — | 关系推导: ENDPOINT 源接口 |
| `source.source-tp` | — | 关系推导: 源端口 |
| `destination.dest-node` | — | 关系推导: ENDPOINT 目的接口 |
| `destination.dest-tp` | — | 关系推导: 目的端口 |

### 4.4 ISIS（文本解析）

| 解析字段 | Ontology 属性 | 说明 |
|---------|-------------|------|
| `pe-name` + `process` | `isis_id` | 稳定键（复合） |
| 从回显提取 | `system_id` | 如 "1720.1601.0002" |
| 从回显提取 | `area_id` | ISIS 区域 ID |
| 从回显提取 | `level` | L1/L2/L1L2 |
| 从回显提取 | `status` | Active/Inactive |

### 4.5 BGP（文本解析）

| 解析字段 | Ontology 属性 | 说明 |
|---------|-------------|------|
| `pe-name` + `peer_ip` | `bgp_id` | 稳定键（复合） |
| 从回显 "Peer" 列 | `peer_ip` | 对端 IP |
| 从回显 "AS" 列 | `peer_as` | 对端 AS 号 |
| 从回显 "State" 列 | `state` | Established/Connect/Idle 等 |
| 从回显 "Up/Down" 列 | `uptime` | 连接时长 |
| 从回显 header | `router_id` | 本地 Router ID |
| 从回显 header | `local_as` | 本地 AS 号 |

### 4.6 Alarm

| API 字段 | Ontology 属性 | 说明 |
|---------|-------------|------|
| `id` | `alarm_id` | 稳定键 |
| `level` | `severity` | 枚举映射: CRITICAL→Critical, MAJOR→Major, MINOR→Minor, WARNING→Warning |
| `msg` | `message` | 告警消息 |
| `time` | `timestamp` | 告警时间 |
| `source` | — | 关系推导: OCCURRED_ON 设备 |
| `category` | `category` | 额外属性 |

### 4.7 VPN

| API 字段 | Ontology 属性 | 说明 |
|---------|-------------|------|
| `vpn-id` | `vpn_id` | 稳定键 |
| `svc-name` | `name` | VPN 名称 |
| `vpn-svc-type` | `svc_type` | mpls-vpn/vxlan-evpn/vpls/vpws |
| `vpn-tunnel-type` | `tunnel_type` | sr-mpls/srv6/mpls |
| `vpn-service-topology` / `svc-topo` | `topology` | any-to-any/hub-spoke |
| `site-count` | `site_count` | 站点数量 |
| `sna-count` | `sna_count` | SNA 数量 |
| `pre-create-status` | `status` | up→Up, down→Down |
| `create-time` | `create_time` | 创建时间 |
| `update-time` | `update_time` | 更新时间 |

### 4.8 Tunnel（来自 policy-instance）

| API 字段 | Ontology 属性 | 说明 |
|---------|-------------|------|
| `instance-id` | `tunnel_id` | 稳定键 |
| `policy-template-name` | `name` | 策略名称 |
| `cfg-status` | `status` | COMPLETED→Up, PENDING→Down, FAILED→Down |
| `te-policy-targets.src-device` | `src_device` | 源设备 |
| `te-policy-targets.dst-device` | `dst_device` | 目的设备 |
| `te-policy-targets.l3-vpn-id` | `vpn_id` | 关联 VPN（关系推导） |
| 统计 `te-tuples` 数量 | `path_count` | 路径数量 |
| 首个 `explicit-tunnel.tunnel-name` | `tunnel_name` | 隧道名称 |

---

## 5. 文本回显类 API 处理方案（ISIS / BGP）

### 5.1 核心难点

ISIS 和 BGP API 返回设备 CLI 回显字符串，有三个特殊挑战:

1. **厂商差异**: H3C、ZTE、Huawei 等厂商的 `display bgp peer` / `display isis peer` 输出格式不同
2. **N+1 调用**: 需逐设备调用（每次指定 `pe-name`）
3. **交叉引用**: 回显文本不含设备基础信息，需关联 Device 数据补全 vendor 信息

### 5.2 推荐架构: 厂商感知文本解析器

在 `parser.go` 中实现按厂商分发的解析器:

```go
// parser.go

// ParseBGPText 根据厂商解析 BGP 邻居回显文本
func ParseBGPText(vendor string, text string) ([]map[string]any, error) {
    switch strings.ToUpper(vendor) {
    case "H3C":
        return parseBGPH3C(text)
    case "ZTE":
        return parseBGPZTE(text)
    default:
        return parseBGPGeneric(text) // 兜底: 通用正则
    }
}

// ParseISISText 根据厂商解析 ISIS 邻居回显文本
func ParseISISText(vendor string, text string) ([]map[string]any, error) {
    switch strings.ToUpper(vendor) {
    case "H3C":
        return parseISISH3C(text)
    case "ZTE":
        return parseISISZTE(text)
    default:
        return parseISISGeneric(text)
    }
}
```

### 5.3 BGP 回显解析示例

以 API 文档中的实际回显为例:

```
 BGP local router ID: 172.16.11.2
 Local AS number: 137749
 Total number of peers: 8       Peers in established state: 1
 Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
 1.1.1.3             137749        0        0    0        0 5523h36m Connect
 172.16.11.4         137749   115005    18053    0      514 0230h43m Established
```

解析步骤:
1. 提取 header 信息: `router_id`（正则匹配 "BGP local router ID: (.+)"）、`local_as`
2. 定位表头行 "Peer ... State"，确定各列的偏移位置
3. 逐行解析数据行，按空格分割提取 peer_ip, peer_as, msg_rcvd, msg_sent, uptime, state

### 5.4 Collect 依赖顺序

```go
// controller.go — Collect 方法的调度逻辑
func (c *ControllerConnector) Collect(ctx context.Context, entityType string) ([]Resource, error) {
    switch entityType {
    case "Device":
        return c.collectDevices(ctx)
    case "Interface":
        return c.collectInterfaces(ctx) // 复用 collectDevices 的缓存
    case "Link":
        return c.collectLinks(ctx)
    case "Alarm":
        return c.collectAlarms(ctx)
    case "VPN":
        return c.collectVPNs(ctx)
    case "Tunnel":
        return c.collectTunnels(ctx)
    case "ISIS":
        return c.collectISIS(ctx)    // 内部依赖 Device 数据获取 vendor
    case "BGP":
        return c.collectBGP(ctx)     // 内部依赖 Device 数据获取 vendor
    }
}
```

对于 ISIS/BGP 的 Collect:

```go
func (c *ControllerConnector) collectBGP(ctx context.Context) ([]Resource, error) {
    // 1. 先获取设备列表（或从缓存读取）
    devices, err := c.collectDevices(ctx)
    if err != nil { return nil, err }

    var resources []Resource
    for _, dev := range devices {
        vendor := dev.Properties["vendor"].(string)
        peName := dev.Properties["hostname"].(string)

        // 2. 逐设备调用 BGP API
        text, err := c.fetchBGPText(ctx, peName)
        if err != nil {
            slog.Warn("bgp fetch failed", "device", peName, "error", err)
            continue
        }

        // 3. 厂商感知解析
        peers, err := parser.ParseBGPText(vendor, text)
        if err != nil {
            slog.Warn("bgp parse failed", "device", peName, "vendor", vendor, "error", err)
            continue
        }

        for _, peer := range peers {
            resources = append(resources, Resource{Kind: "BGP", Properties: peer})
        }
    }
    return resources, nil
}
```

---

## 6. 新增 Ontology 定义

### 6.1 ontology/vpn.yaml

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: VPN
  labels: [Service, Network]
spec:
  identity:
    stableKeys: [vpn_id]
  uriTemplate: "vpn:{vpn_id}"
  fieldMapping: {}
  normalize: []
  relationFields: {}
  properties:
    vpn_id:
      type: string
      required: true
    name:
      type: string
    svc_type:
      type: string
      enum: [mpls-vpn, vxlan-evpn, vpls, vpws]
    tunnel_type:
      type: string
      enum: [sr-mpls, srv6, mpls]
    topology:
      type: string
      enum: [any-to-any, hub-spoke]
    site_count:
      type: int
    sna_count:
      type: int
    status:
      type: string
      enum: [Up, Down]
      default: "Up"
    create_time:
      type: string
    update_time:
      type: string
```

### 6.2 ontology/bgp.yaml

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: BGP
  labels: [Protocol, Network]
spec:
  identity:
    stableKeys: [bgp_id]
  uriTemplate: "bgp:{bgp_id}"
  fieldMapping: {}
  normalize: []
  relationFields:
    run_on:
      relationType: RUNS_ON
  properties:
    bgp_id:
      type: string
      required: true
    peer_ip:
      type: string
      required: true
    peer_as:
      type: int
    state:
      type: string
      enum: [Established, Connect, Idle, Active, OpenSent, OpenConfirm]
      default: "Connect"
    uptime:
      type: string
    router_id:
      type: string
    local_as:
      type: int
```

### 6.3 ontology/tunnel.yaml

```yaml
apiVersion: twin.io/v1
kind: EntityType
metadata:
  name: Tunnel
  labels: [Service, Network]
spec:
  identity:
    stableKeys: [tunnel_id]
  uriTemplate: "tunnel:{tunnel_id}"
  fieldMapping: {}
  normalize: []
  relationFields:
    for_vpn:
      relationType: TUNNEL_FOR
  properties:
    tunnel_id:
      type: string
      required: true
    name:
      type: string
    tunnel_name:
      type: string
    src_device:
      type: string
    dst_device:
      type: string
    vpn_id:
      type: string
    status:
      type: string
      enum: [Up, Down]
      default: "Down"
    path_count:
      type: int
```

### 6.4 ontology/relations.yaml 补充

在现有 relations.yaml 末尾追加:

```yaml
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: HAS_BGP_PEER
spec:
  source: [Device]
  target: [BGP]
---
apiVersion: twin.io/v1
kind: RelationType
metadata:
  name: TUNNEL_FOR
spec:
  source: [Tunnel]
  target: [VPN]
```

---

## 7. 认证方案

### 7.1 Bearer Token 认证

Controller 使用 Bearer Token 认证（区别于 Netbox 的 Token Auth）:
```
Authorization: Bearer {access_token}
```

**需要增强 `internal/connector/httpclient.go`**:

```go
// 在 applyAuth 方法中新增 "bearer" case
func (c *HTTPClient) applyAuth(req *http.Request) {
    switch c.auth.Type {
    case "token":
        // ... 现有逻辑
    case "basic":
        // ... 现有逻辑
    case "bearer":   // 新增
        token := c.auth.Token
        if token == "" && c.auth.TokenEnv != "" {
            token = os.Getenv(c.auth.TokenEnv)
        }
        req.Header.Set("Authorization", "Bearer "+token)
    }
}
```

### 7.2 Token 自动刷新（可选增强）

如果需要在 Connector 内部管理 Token 生命周期:

```go
type ControllerConnector struct {
    http       *connector.HTTPClient
    name       string
    types      []string
    tokenURL   string
    username   string
    password   string
    deviceID   string
    token      string
    tokenExp   time.Time
}

func (c *ControllerConnector) ensureToken(ctx context.Context) error {
    if time.Now().Before(c.tokenExp.Add(-60 * time.Second)) {
        return nil // Token 还有效（预留 60s 缓冲）
    }
    // 调用 /oauth/token 获取新 Token
    // 更新 c.token 和 c.tokenExp
    // 更新 c.http 的 auth 配置
}
```

---

## 8. connectors.yaml 配置示例

```yaml
connectors:
  # ... 现有 mock 配置保持不变 ...

  # Controller Connector
  - name: controller-1
    type: controller
    config:
      base_url: "http://controller.example.com"
      timeout: "60s"
      token_url: "/oauth/token"
      username: "admin"
      password_env: "CONTROLLER_PASSWORD"
      device_id: "be6d4c50-6476-4f6c-b3ae-2e6613611eed"
    entity_types: [Device, Interface, Link, Alarm, VPN, Tunnel, ISIS, BGP]
    auth:
      type: bearer
      token_env: CONTROLLER_TOKEN
```

---

## 9. 技术要点清单

### 9.1 Bearer Token 认证增强

- 修改文件: `internal/connector/httpclient.go`
- 在 `applyAuth` 中新增 `"bearer"` case
- 建议同时支持静态 Token 和动态 Token 刷新两种模式

### 9.2 两套分页格式适配

- Controller 的分页格式与现有 `HTTPClient.Paginate`（Netbox DRF 风格）**不兼容**
- 需在 `controller.go` 中实现两个私有分页方法: `paginateSpringData` 和 `paginateVPN`
- Spring Data 分页: `content[]` + `total_pages` + `number`（0-based）
- VPN 分页: `content[]` + `total_pages` + `page_num`（1-based）

### 9.3 深度嵌套展平

- **Device**: 3 层嵌套 → `peports.peport-info[].label[]`
- **Tunnel**: 5 层嵌套 → `te-tuples[].explicit-tunnel[].te-path[].explicit-path[].hop`
- 在 `transform.go` 中实现逐层展平，输出扁平 `map[string]any`

### 9.4 kebab-case 转 snake_case

Controller API 响应字段名统一使用 kebab-case（如 `management-ip`、`connect-status`），而 ontology 定义使用 snake_case（如 `management_ip`、`connect_status`）。

建议在 `transform.go` 中实现通用转换工具函数:

```go
// kebabToSnake 将 kebab-case 键名转换为 snake_case
func kebabToSnake(s string) string {
    return strings.ReplaceAll(s, "-", "_")
}

// transformKeys 递归转换 map 中所有键名
func transformKeys(m map[string]any) map[string]any {
    result := make(map[string]any, len(m))
    for k, v := range m {
        result[kebabToSnake(k)] = v
    }
    return result
}
```

### 9.5 ISIS/BGP 的 N+1 调用问题

- ISIS 和 BGP 需逐设备调用 `/restconf/operations/oper-rpc:*`
- 假设 50 台设备，需要 50 次 HTTP 请求
- **建议**: 使用 `errgroup.Group` 并发控制（最大并发数 5），避免串行等待

```go
import "golang.org/x/sync/errgroup"

func (c *ControllerConnector) collectBGP(ctx context.Context) ([]Resource, error) {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(5) // 最大并发 5

    var mu sync.Mutex
    var allResources []Resource

    for _, dev := range c.cachedDevices {
        dev := dev
        g.Go(func() error {
            text, err := c.fetchBGPText(ctx, dev.Name)
            if err != nil { return nil } // 单设备失败不阻断
            peers, err := parser.ParseBGPText(dev.Vendor, text)
            // ... append with mutex
            return nil
        })
    }
    g.Wait()
    return allResources, nil
}
```

### 9.6 Collect 依赖顺序

**Device 必须最先完成**，因为:
- Interface 从 Device 响应中提取
- ISIS/BGP 需要 Device 的 vendor 信息选择解析器
- Link 的 source/destination 关联 Device

SyncService 的 FullSync 按 Connector 的 `entity_types` 列表顺序调用 Collect，因此在 `connectors.yaml` 中 **Device 必须排在首位**:

```yaml
entity_types: [Device, Interface, Link, Alarm, VPN, Tunnel, ISIS, BGP]
```

### 9.7 枚举映射

多个实体的 status/severity 字段在 API 和 ontology 之间需要映射:

| 实体 | API 值 | Ontology 值 |
|------|--------|------------|
| Device.status | UP / DOWN / UNKNOWN | Up / Down / Down |
| Interface.status | UP / DOWN | Up / Down |
| Link.status | UP / DOWN | Up / Down |
| Alarm.severity | CRITICAL / MAJOR / MINOR / WARNING | Critical / Major / Minor / Warning |
| VPN.status | up / down | Up / Down |
| Tunnel.status | COMPLETED / PENDING / FAILED | Up / Down / Down |
| Device.device_type | PE / P / CE | Edge / Core / Access |

建议在 `transform.go` 中集中管理枚举映射:

```go
var statusMap = map[string]string{
    "UP": "Up", "DOWN": "Down", "UNKNOWN": "Down",
    "up": "Up", "down": "Down",
    "COMPLETED": "Up", "PENDING": "Down", "FAILED": "Down",
}
```

### 9.8 注册到 ConnectorFactory

在 `register.go` 中实现 Builder 函数:

```go
package controller

import (
    "time"
    "gitlab.com/pml/network-digital-twin/internal/connector"
)

func Builder() connector.ConnectorBuilder {
    return func(name string, cfg map[string]any, entityTypes []string) (connector.Connector, error) {
        baseURL, _ := cfg["base_url"].(string)
        timeout := 60 * time.Second
        if t, ok := cfg["timeout"].(string); ok {
            timeout, _ = time.ParseDuration(t)
        }
        client := connector.NewHTTPClient(
            connector.WithBaseURL(baseURL),
            connector.WithTimeout(timeout),
            connector.WithRateLimit(10),
            connector.WithAuth(connector.AuthConfig{Type: "bearer"}),
        )
        return NewControllerConnector(name, client, entityTypes, cfg), nil
    }
}
```

在 `cmd/server/main.go` 中注册:

```go
factory.RegisterBuilder("controller", controller.Builder())
```

### 9.9 设备缓存机制

由于 Interface/ISIS/BGP 的 Collect 都依赖 Device 数据，建议实现简单的内存缓存:

```go
type ControllerConnector struct {
    // ... 其他字段
    cachedDevices []DeviceInfo  // 缓存最近一次采集的设备列表
    cacheTime     time.Time
    cacheTTL      time.Duration // 默认 5 分钟
}

type DeviceInfo struct {
    Name   string
    Vendor string
}

func (c *ControllerConnector) getDevices(ctx context.Context) ([]DeviceInfo, error) {
    if time.Since(c.cacheTime) < c.cacheTTL && len(c.cachedDevices) > 0 {
        return c.cachedDevices, nil
    }
    // 调用 Device API 并更新缓存
}
```

---

## 10. 测试策略

### 10.1 单元测试

参照 `internal/connector/netbox/netbox_test.go` 的模式，使用 `httptest.NewServer` 模拟 Controller API:

```go
func TestCollectDevices(t *testing.T) {
    // 1. 构造 mock server 返回 Controller 格式的 JSON
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 根据 URL 路由返回对应 mock 数据
    }))
    defer server.Close()

    // 2. 创建 ControllerConnector 指向 mock server
    client := connector.NewHTTPClient(
        connector.WithBaseURL(server.URL),
    )
    c := NewControllerConnector("test-controller", client, []string{"Device"}, nil)

    // 3. 验证 Collect 输出
    resources, err := c.Collect(context.Background(), "Device")
    require.NoError(t, err)
    assert.NotEmpty(t, resources)
}
```

### 10.2 Mock 数据

在 `testdata/mock_controller/` 目录下准备各实体的 JSON mock 数据文件，数据格式严格遵循 API 文档的响应结构。

### 10.3 文本解析器测试

为 `parser.go` 编写测试用例，覆盖 H3C/ZTE 两种厂商格式:

```go
func TestParseBGPTextH3C(t *testing.T) {
    input := `display bgp peer ipv4
 BGP local router ID: 172.16.11.2
 Local AS number: 137749
 Peer                    AS  MsgRcvd  MsgSent OutQ  PrefRcv Up/Down  State
 172.16.11.4         137749   115005    18053    0      514 0230h43m Established`

    peers, err := ParseBGPText("H3C", input)
    require.NoError(t, err)
    assert.Len(t, peers, 1)
    assert.Equal(t, "172.16.11.4", peers[0]["peer_ip"])
    assert.Equal(t, "Established", peers[0]["state"])
}
```

---

## 11. 开发步骤建议

| 步骤 | 任务 | 预估工时 |
|------|------|---------|
| 1 | 新增 ontology YAML（vpn.yaml / bgp.yaml / tunnel.yaml + relations 补充） | 0.5 天 |
| 2 | 增强 HTTPClient 支持 Bearer Token | 0.5 天 |
| 3 | 实现 register.go + controller.go 骨架（Metadata/Ping/Stream） | 0.5 天 |
| 4 | 实现 collectDevices + transformDevice | 1 天 |
| 5 | 实现 collectInterfaces（从 Device 缓存提取） | 0.5 天 |
| 6 | 实现 collectLinks + transformLink | 0.5 天 |
| 7 | 实现 collectAlarms + transformAlarm | 0.5 天 |
| 8 | 实现 collectVPNs（L3 + L2 分页合并） | 1 天 |
| 9 | 实现 collectTunnels（policy-instance 深度展平） | 1 天 |
| 10 | 实现 parser.go（BGP/ISIS 文本解析器） | 1.5 天 |
| 11 | 实现 collectISIS / collectBGP（N+1 + 并发控制） | 1 天 |
| 12 | 编写单元测试 + mock 数据 | 1.5 天 |
| 13 | 更新 connectors.yaml + main.go 注册 | 0.5 天 |
| 14 | 集成测试 + 端到端验证 | 1 天 |
| **合计** | | **~11 天** |
