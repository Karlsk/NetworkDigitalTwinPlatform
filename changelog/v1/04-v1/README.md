# V1 研发计划 — 任务汇总

> 基于 MVP 35 项任务全部完成，V1 选定 4 个优先方向：
> 1. 真实数据源 Connector (Netbox/CMDB/Controller)
> 2. TD-03 SnapshotService 缓存/审计
> 3. TD-04 属性级 Diff
> 4. 本体继承机制 (extends)

**总任务数**: 22 项 (V1-01 ~ V1-22)
**预估工时**: 25 人天（含缓冲 30 天）
**架构设计**: 详见 [V1架构设计.md](../../docs/V1架构设计.md)

---

## Phase 1: 基础迁移 (V1-01 ~ V1-05)

> 目标：`Node.Labels` 多标签改造 + Connector 工厂模式 + HTTP 客户端公共层

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1-01](V1-01_Node.Labels多标签改造.md) | Node.Labels 多标签改造 (BREAKING) | 2天 | 无 | assembler/types.go + neo4j.go + snapshot 全链路迁移 |
| [V1-02](V1-02_Connector接口增强.md) | Connector 接口增强 + Ping | 0.5天 | 无 | interface.go 新增 Ping() |
| [V1-03](V1-03_ConnectorFactory模式.md) | ConnectorFactory 模式 | 1天 | V1-02 | factory.go + config.go + connectors.yaml auth段 |
| [V1-04](V1-04_HTTP客户端公共层.md) | HTTP 客户端公共层 | 1天 | V1-02 | httpclient.go (认证/重试/限流/分页) |
| [V1-05](V1-05_迁移测试与回归.md) | 迁移测试 + 回归验证 | 1天 | V1-01, V1-03, V1-04 | go test -race 全量通过 |

- [x] V1-01 编译通过 + 全量测试绿灯
- [x] V1-02 MockConnector 实现 Ping()
- [x] V1-03 ConnectorFactory 从 YAML 配置创建 Mock Connector
- [x] V1-04 HTTPClient Token/Basic Auth + 重试 + 限流
- [x] V1-05 `go test -race ./...` 全部通过，覆盖率 >= 70%

---

## Phase 2a: 真实数据源 Connector (V1-06 ~ V1-10)

> 目标：三个真实 REST API Connector，替换 Mock 数据源

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1-06](V1-06_NetboxConnector.md) | NetboxConnector (Device + Interface) | 2天 | V1-04, V1-03 | connector/netbox/ |
| [V1-07](V1-07_CMDBConnector.md) | CMDBConnector (ISIS + Link + Network_Slice) | 1.5天 | V1-04, V1-03 | connector/cmdb/ |
| [V1-08](V1-08_ControllerConnector.md) | ControllerConnector (Device_Status + Telemetry) | 1.5天 | V1-04, V1-03 | connector/controller/ |
| [V1-09](V1-09_Connector集成测试.md) | Connector 集成测试 (httptest mock server) | 1.5天 | V1-06~V1-08 | 各 connector _test.go |
| [V1-10](V1-10_全链路验证.md) | 全链路验证 (真实 Connector + FullSync) | 1天 | V1-09 | E2E 测试更新 |

- [x] V1-06 NetboxConnector.Collect("Device") + Collect("Interface") 正确
- [x] V1-07 CMDBConnector 三种实体类型均可采集
- [x] V1-08 ControllerConnector 动态状态采集正确
- [x] V1-09 httptest mock server 7 个测试用例全部通过
- [x] V1-10 E2E httptest mock server 通过完整同步流水线

---

## Phase 2b: 属性级 Diff + 本体继承 (V1-11 ~ V1-16)

> 目标：属性级差异对比 + Ontology extends 继承体系

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1-11](V1-11_SnapshotDiff数据结构扩展.md) | SnapshotDiff 数据结构扩展 | 0.5天 | V1-01 | NodeChange/RelChange/FieldChange 类型 |
| [V1-12](V1-12_LocalDiff属性级对比.md) | LocalDiff 属性级对比 | 1天 | V1-11 | LocalDiff 增强 |
| [V1-13](V1-13_CypherDiff属性级对比.md) | Cypher Diff 属性级对比 | 1.5天 | V1-11 | Cypher Diff 增强 + MCP 适配 |
| [V1-14](V1-14_Diff测试.md) | Diff 测试 | 1天 | V1-12, V1-13 | 6 个测试用例 |
| [V1-15](V1-15_EntityTypeSpec_Extends.md) | EntityTypeSpec Extends + 继承合并 | 1.5天 | V1-01 | schema/types.go + registry.go |
| [V1-16](V1-16_Neo4j多Label索引.md) | Neo4j 多 Label + EnsureIndexes | 1天 | V1-15, V1-01 | graph/neo4j.go + assembler.go |

- [x] V1-11 NodeChange/RelChange 类型可被其他包引用
- [x] V1-12 LocalDiff 正确报告 AddedFields/RemovedFields/ModifiedFields
- [x] V1-13 Cypher Diff 与 LocalDiff 结果一致
- [x] V1-14 全部 Diff 测试用例通过
- [x] V1-15 Device extends Resource 正确继承 Properties，循环继承被检测
- [x] V1-16 Neo4j `MATCH (n:Resource)` 覆盖所有子类型

---

## Phase 3: 缓存审计 + 验收 (V1-17 ~ V1-22)

> 目标：SnapshotService 性能优化 + 审计能力 + V1 全量验收

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V1-17](V1-17_MetaCache实现.md) | MetaCache 实现 | 1天 | V1-14 | SnapshotManager 缓存层 |
| [V1-18](V1-18_AuditLog审计日志.md) | AuditLog 审计日志 | 1天 | V1-17 | snapshot/audit.go |
| [V1-19](V1-19_SnapshotService_List优化.md) | SnapshotService List 优化 | 0.5天 | V1-17 | snapshot_service.go |
| [V1-20](V1-20_快照TTL保留策略.md) | 快照 TTL/保留策略 (可选) | 0.5天 | V1-17 | retentionDays 配置 |
| [V1-21](V1-21_V1全量集成测试.md) | V1 全量集成测试 | 1.5天 | V1-01~V1-20 | 全模块测试更新 |
| [V1-22](V1-22_V1验收与文档归档.md) | V1 验收 + 文档归档 | 1天 | V1-21 | 验收报告 + changelog |

- [x] V1-17 第二次 List() 从缓存返回，不读取 YAML
- [x] V1-18 Create/Restore/Delete 操作均记录审计日志
- [x] V1-19 MCP query_snapshot list 第二次调用明显快于首次
- [x] V1-20 retentionDays > 0 时超期快照自动清理
- [x] V1-21 go test -race 全部通过，覆盖率 >= 70%
- [x] V1-22 15 项验收清单全部通过

---

## V1 验收清单

| # | 验收项 | 验证方法 | 通过标准 |
|---|--------|----------|---------|
| 1 | 编译通过 | `go build ./...` | 无错误 |
| 2 | Lint 通过 | `golangci-lint run` | 无 Error |
| 3 | 单元测试全部通过 | `go test ./...` | 0 failures |
| 4 | Race 检测 | `go test -race ./...` | 无 data race |
| 5 | 覆盖率 | `go test -cover ./...` | >= 70% |
| 6 | NetboxConnector 全量同步 | E2E httptest | 节点/关系正确 |
| 7 | CMDBConnector 全量同步 | E2E httptest | ISIS/Link/Slice 正确 |
| 8 | 属性级 Diff (LocalDiff) | 单元测试 | ChangedNodes 正确 |
| 9 | 属性级 Diff (Cypher Diff) | 集成测试 | 与 LocalDiff 一致 |
| 10 | Ontology 继承 | schema 测试 | Properties 正确合并 |
| 11 | Neo4j 多 Label | E2E 查询 | MATCH (n:Resource) 覆盖子类型 |
| 12 | MetaCache 性能 | 基准测试 | List < 1ms (缓存命中) |
| 13 | AuditLog | 功能测试 | 操作审计记录完整 |
| 14 | ConnectorFactory | 配置切换测试 | YAML 切换无代码改动 |
| 15 | 向后兼容 | 旧 YAML/Old format | 旧格式正常加载 |

---

## 任务依赖关系图

```
Phase 1 (基础迁移)
V1-01 (Node.Labels) ─────────────────────────────────┐
V1-02 (Ping) ──┬──→ V1-03 (Factory) ──┐              │
               └──→ V1-04 (HTTPClient)─┤              │
                                       ↓              │
Phase 2a (真实Connector)              V1-05 (回归)    │
V1-03+V1-04 → V1-06 (Netbox) ──┐                     │
V1-03+V1-04 → V1-07 (CMDB) ────┤→ V1-09 → V1-10    │
V1-03+V1-04 → V1-08 (Controller)┘                     │
                                                      │
Phase 2b (Diff + 继承)                                │
V1-01 → V1-11 (Diff类型) → V1-12 (LocalDiff) ─┐      │
                        → V1-13 (CypherDiff) ──┤→ V1-14
V1-01 → V1-15 (Extends) → V1-16 (多Label) ←───┘

Phase 3 (缓存审计 + 验收)
V1-14 → V1-17 (MetaCache) → V1-18 (AuditLog)
                          → V1-19 (List优化)
                          → V1-20 (TTL, 可选)
V1-01~V1-20 → V1-21 (全量测试) → V1-22 (验收)
```
