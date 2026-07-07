# V1.2 研发计划 — Controller API 统一适配与解耦

> 基于 V1 全部 22 项任务完成，V1.2 聚焦 **Controller 连接器的 API 调用层解耦**，
> 使得 Connector（拓扑同步）和 MCP（按需查询）共享同一套 API 适配层，
> 释放 Controller 全部 API 能力（监控、配置、切片、SR-TE、确定性网络）。

**总任务数**: 4 项 (V1.2-01 ~ V1.2-04)
**预估工时**: 8 人天（含缓冲 10 天）
**参考文档**: [骨干网操作系统API接口规范v1.5](../../docs/骨干网操作系统API接口规范v1.5%20(1).docx)

---

## 核心目标

| # | 目标 | 说明 |
|---|------|------|
| 1 | **统一 API 适配层** | 提取 `ControllerClient` 封装所有 REST API 调用，Token 自动管理 |
| 2 | **能力接口抽象** | 定义 `MonitorQuerier` / `DeviceOperator` 等能力接口，Connector 按需实现 |
| 3 | **MCP 按需调用** | 新增 MCP 工具，Agent 可直接查询监控、设备配置、切片状态 |
| 4 | **渐进式 API 补全** | 按 API 文档逐步覆盖监控/日志/切片/SR-TE/确定性网络等接口 |

---

## 改造前后架构对比

### 改造前（V1 现状）

```
Controller API → ControllerConnector.Collect() → Resource → Normalizer → Assembler → Neo4j
                                                                                     ↓
                                                                    MCP query_topology → Neo4j 查询
```

- `ControllerConnector` 847 行代码混合 Token 管理 + API 调用 + Collect + Transform
- 只有 8 种实体类型的拓扑采集路径被实现
- API 文档中 ~50 个接口，仅覆盖 ~10 个（拓扑 + ISIS/BGP）
- MCP 无法直接调用 Controller API（只能查询已入图数据）

### 改造后（V1.2 目标）

```
路径1 拓扑同步（不变）:
  ControllerClient.FetchXxx() → ControllerConnector.Collect() → Resource → ... → Neo4j

路径2 按需查询（新增）:
  MCP query_monitor      → DeviceService → ControllerClient.FetchDeviceMetrics() → 直接返回
  MCP query_device_info  → DeviceService → ControllerClient.FetchISISNeighbors() → 直接返回
  MCP query_topology_live→ DeviceService → ControllerClient.FetchTopology()      → 直接返回
```

---

## Phase 1: ControllerClient 提取 (V1.2-01)

> 目标：将现有 `fetchXxxRaw()` 方法从 `ControllerConnector` 提取到独立的 `ControllerClient`

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1.2-01](V1.2-01_ControllerClient提取.md) | ControllerClient 统一 API 适配层 | 2天 | 无 | client.go + api_topology.go + api_config.go + 重构 controller.go |

- [ ] V1.2-01 `ControllerClient` 封装 Token 管理 + 所有现有 fetch 方法
- [ ] V1.2-01 `ControllerConnector.Collect()` 变为薄封装：`client.FetchDevices()` → `transformDevice()`
- [ ] V1.2-01 全部现有测试 `go test -race ./...` 通过，零功能变化

---

## Phase 2: Capability 接口定义 (V1.2-02)

> 目标：定义能力接口，让不同 Connector 按需实现

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1.2-02](V1.2-02_Capability接口定义.md) | Capability 能力接口 + ControllerConnector 实现 | 2天 | V1.2-01 | capability.go + controller_monitor.go + controller_operator.go |

- [ ] V1.2-02 `MonitorQuerier` 接口定义 + ControllerConnector 实现
- [ ] V1.2-02 `DeviceOperator` 接口定义 + ControllerConnector 实现
- [ ] V1.2-02 编译时接口满足检查 `var _ MonitorQuerier = (*ControllerConnector)(nil)`

---

## Phase 3: DeviceService + MCP 工具 (V1.2-03)

> 目标：Service 层编排 + MCP 层暴露按需查询工具

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1.2-03](V1.2-03_DeviceService与MCP工具.md) | DeviceService 编排 + 3 个 MCP 新工具 | 2天 | V1.2-02 | service/device_service.go + mcp 工具注册 |

- [ ] V1.2-03 `DeviceService` 编排层，类型断言检查 Connector 能力
- [ ] V1.2-03 MCP `query_monitor` 工具注册 + handler
- [ ] V1.2-03 MCP `query_device_info` 工具注册 + handler
- [ ] V1.2-03 MCP `query_topology_live` 工具注册 + handler

---

## Phase 4: API 扩展补全 (V1.2-04)

> 目标：按 API 文档逐步覆盖监控/日志/切片/SR-TE/确定性网络

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1.2-04](V1.2-04_API扩展补全.md) | 监控/日志/切片/SR-TE/确定性网络 API 补全 | 2天 | V1.2-03 | api_monitor.go + api_slice.go + api_srte.go + api_detnet.go |

- [ ] V1.2-04 监控 API 全覆盖（设备/端口/VPN/Tunnel 指标 + 告警 + 日志）
- [ ] V1.2-04 切片管理 API（FlexE Group/Client + Sub-interface + SRv6 Slice CRUD）
- [ ] V1.2-04 SR-TE 路径 API + 确定性网络 DetNet API
- [ ] V1.2-04 全量 `go test -race ./...` 通过

---

## 任务依赖关系图

```
V1.2-01 (ControllerClient 提取)
    │
    ▼
V1.2-02 (Capability 接口)
    │
    ▼
V1.2-03 (DeviceService + MCP)
    │
    ▼
V1.2-04 (API 扩展补全)
```

全部串行依赖，每个 Phase 在前一个完成后开始。

---

## API 覆盖矩阵（基于 API 文档 v1.5）

| API 大类 | 文档章节 | 接口数 | V1 已覆盖 | V1.2 新增 | 对应任务 |
|---------|---------|-------|----------|----------|---------|
| 账号管理 | 1.x | ~7 | Token | — (内部使用) | V1.2-01 |
| 网络资源 | 2.x | ~10 | 8 (Collect) | POP 分页等 | V1.2-01 |
| 设备配置 (Restconf) | 5.x | ~5 | 2 (ISIS/BGP) | VPN-config/Current-config/Global-route | V1.2-02 |
| 监控 | 6.x | ~12 | 1 (Alarm) | 设备/端口/VPN/Tunnel 指标 + 日志 | V1.2-04 |
| SR-TE | 3.x/4.x | ~4 | 0 | 路径查询/计算 | V1.2-04 |
| 确定性网络 | 7.x | ~6 | 0 | DetNet CRUD + OAM | V1.2-04 |
| 切片管理 | 8.x | ~17 | 0 | FlexE/Sub-interface/SRv6 Slice CRUD | V1.2-04 |
| **合计** | | **~61** | **~11** | **~30+** | |

---

## V1.2 验收清单

| # | 验收项 | 验证方法 | 通过标准 |
|---|--------|----------|---------|
| 1 | 编译通过 | `go build ./...` | 无错误 |
| 2 | Lint 通过 | `golangci-lint run` | 无 Error |
| 3 | 单元测试 | `go test ./...` | 0 failures |
| 4 | Race 检测 | `go test -race ./...` | 无 data race |
| 5 | ControllerClient 单元测试 | client_test.go | Token 刷新 + 各 fetch 方法 |
| 6 | Capability 接口实现 | 编译时检查 | `var _ MonitorQuerier = (*ControllerConnector)(nil)` |
| 7 | MCP query_monitor | 端到端测试 | 返回设备/端口监控数据 |
| 8 | MCP query_device_info | 端到端测试 | 返回 ISIS/BGP 邻居实时数据 |
| 9 | MCP query_topology_live | 端到端测试 | 返回实时拓扑（不依赖 Neo4j） |
| 10 | 向后兼容 | V1 全量测试 | V1 所有测试继续通过 |
