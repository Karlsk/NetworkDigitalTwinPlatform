# 第二阶段：数据引擎集成（同步服务 + 快照管理 + 并发保护）

> 全量同步 → 增量同步 → 快照管理 → 并发保护 → 数据流验证

## 任务索引

### 实现阶段 — 同步服务 + 快照管理 (Phase 3)

| 任务ID | 任务名称 | 工时 | 文件 |
|--------|---------|------|------|
| I-13 | GraphLock 并发保护 | 0.5天 | [I-13_to_I-14_GraphLock与全量同步.md](I-13_to_I-14_GraphLock与全量同步.md) |
| I-14 | SyncService.FullSync 全量同步 | 1.5天 | [I-13_to_I-14_GraphLock与全量同步.md](I-13_to_I-14_GraphLock与全量同步.md) |
| I-15 | IncrementalSync + Channel + Webhook | 2天 | [I-15_to_I-18_增量同步与快照管理.md](I-15_to_I-18_增量同步与快照管理.md) |
| I-16 | SnapshotManager.Create + List + Delete | 1.5天 | [I-15_to_I-18_增量同步与快照管理.md](I-15_to_I-18_增量同步与快照管理.md) |
| I-17 | EnsureLoaded + Restore + Diff | 1.5天 | [I-15_to_I-18_增量同步与快照管理.md](I-15_to_I-18_增量同步与快照管理.md) |
| I-18 | cleanup 懒加载清理 | 0.5天 | [I-15_to_I-18_增量同步与快照管理.md](I-15_to_I-18_增量同步与快照管理.md) |

### 测试阶段 (Phase 4)

| 任务ID | 任务名称 | 工时 | 文件 |
|--------|---------|------|------|
| T-06 | SyncService 集成测试 | 1.5天 | [T-06_to_T-08_数据引擎测试.md](T-06_to_T-08_数据引擎测试.md) |
| T-07 | SnapshotManager 集成测试 | 1.5天 | [T-06_to_T-08_数据引擎测试.md](T-06_to_T-08_数据引擎测试.md) |
| T-08 | GraphLock 并发测试 | 1天 | [T-06_to_T-08_数据引擎测试.md](T-06_to_T-08_数据引擎测试.md) |

---

## 功能描述

第二阶段基于第一阶段的基础设施，构建完整的数据同步和快照管理能力。核心目标是：

1. **实现同步服务**：编排 Connector → Normalizer → GraphAssembler → GraphDB 的完整数据流
2. **实现全量同步**：定时触发，ClearDB + BulkCreate，持有写锁排他执行
3. **实现增量同步**：Webhook 事件触发，Channel 缓冲防丢消息，单协程消费
4. **实现快照管理**：YAML 归档永久保留 + Neo4j 逻辑 DB 懒加载
5. **实现 GraphLock 并发保护**：Restore/FullSync/IncrementalSync 写锁互斥，防止脏图
6. **验证 ID 稳定性与合并策略**：URI 不可变 + 属性增量合并 + 关系增量更新

本阶段完成后，系统可通过 Mock 数据构建完整的网络拓扑孪生体，并支持快照创建/恢复/对比。

## 文件清单

### Mock 数据体系

| 文件路径 | 用途 |
|----------|------|
| `testdata/mock_netbox/devices.json` | 3 台设备数据（Core01, Edge05, Access03） |
| `testdata/mock_netbox/interfaces.json` | ~18 个接口（每台 6 个，含 Up/Down/AdminDown 状态） |
| `testdata/mock_cmdb/isis.json` | 2-3 个 ISIS 路由协议实例 |
| `testdata/mock_cmdb/links.json` | 2-3 条链路数据 |
| `testdata/golden/expected_topology.yaml` | 期望的完整拓扑输出（Golden File） |
| `testdata/golden/expected_analysis.json` | 期望的节点/关系计数 |

### 同步服务

| 文件路径 | 用途 |
|----------|------|
| `internal/service/sync_service.go` | **全量/增量同步编排**：协调 Connector → Normalizer → GraphAssembler → GraphDB 流水线 |

### 快照系统

| 文件路径 | 用途 |
|----------|------|
| `internal/snapshot/manager.go` | **快照生命周期管理**：Create/Restore/List/Delete/Diff + EnsureLoaded 懒加载 |
| `internal/snapshot/exporter.go` | **快照导出**：Neo4j default DB → YAML 归档文件 |
| `internal/snapshot/importer.go` | **快照导入**：YAML 归档 → Neo4j 逻辑 DB |
| `internal/snapshot/graphlock.go` | **GraphLock 并发保护**：sync.RWMutex 封装 |

### HTTP Handler

| 文件路径 | 用途 |
|----------|------|
| `internal/handler/sync_handler.go` | 同步 API：`POST /api/v1/sync/full`、`POST /api/v1/sync/webhook` |
| `internal/handler/snapshot_handler.go` | 快照 API：create/list/restore/delete/diff |

### 各文件详细说明

#### `internal/service/sync_service.go` — 同步服务

```go
type SyncService struct {
    registry   *connector.ConnectorRegistry
    normalizer *normalizer.Normalizer
    assembler  *assembler.GraphAssembler  // ⭐ IR 层
    graph      graph.GraphDB
    lock       *snapshot.GraphLock        // ⭐ 与 SnapshotManager 共享同一把锁
    eventChan  chan SyncEvent             // ⭐ 缓冲 channel，防丢消息
}

type SyncResult struct {
    NodesCreated       int
    RelationsCreated int
    OrphanEdgesSkipped int  // ⭐ 孤儿边计数
    Warnings           []assembler.ValidationWarning
    Duration           time.Duration
}

type SyncEvent struct {
    Action      string   // "update", "delete", "delete_relation"
    EntityType string
    Connector   string
    Data        []map[string]any  // update 时的数据
    URIs        []string          // delete 时的 URI 列表
    Relations   []assembler.Relation  // delete_relation 时的关系列表
}

// FullSync: 全量同步 (定时触发，持有写锁)
// 1. lock.Lock()                    ← 排他锁，阻塞增量同步和恢复
// 2. ClearDB("default")
// 3. 所有 Connector.Collect() 获取全量数据
// 4. Normalizer → Assembler → GraphDB.BulkCreate
// 5. lock.Unlock()                  ← 释放锁
func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error)

// IncrementalSync: 增量同步 (事件触发: Webhook，持有写锁)
// 根据 event.Action 分发:
//   "update"           → Normalizer → Assembler → GraphDB.Upsert() (MERGE 节点+关系)
//   "delete"           → GraphDB.DeleteByURIs() (DETACH DELETE 节点)
//   "delete_relation"  → GraphDB.DeleteRelations() (仅删除指定关系)
func (s *SyncService) IncrementalSync(ctx context.Context, event SyncEvent) (*SyncResult, error)

// StartConsumer: 启动消费者协程 (服务启动时调用)
func (s *SyncService) StartConsumer(ctx context.Context) {
    go func() {
        for event := range s.eventChan {
            s.lock.Lock()
            s.processEvent(ctx, event)
            s.lock.Unlock()
        }
    }()
}

// HandleWebhook: Webhook Handler (立即返回 202)
func (s *SyncService) HandleWebhook(event SyncEvent) error {
    select {
    case s.eventChan <- event:
        return nil  // 入队成功，返回 202 Accepted
    default:
        return errors.New("event queue full")  // channel 满，返回 503
    }
}
```

#### `internal/snapshot/manager.go` — 快照管理

```go
type SnapshotManager struct {
    graph      graph.GraphDB
    lock       *GraphLock  // ⭐ 并发保护
    snapDir    string      // YAML 快照文件存储目录 (永久归档)
    maxActive  int         // Neo4j 中最多保留的逻辑 DB 数量，默认 5
}

// Create: 导出 default DB 为 YAML 归档文件 (不写 Neo4j)
func (m *SnapshotManager) Create(ctx context.Context, name string) (*SnapshotMeta, error)
// 1. lock.RLock()
// 2. 查询 default DB 全量数据
// 3. 序列化为 YAML 文件 → 写入 snapDir/{name}.yaml
// 4. lock.RUnlock()
// 5. 记录元数据 (时间戳、节点数、关系数)

// EnsureLoaded: 确保快照已加载到 Neo4j 逻辑 DB (懒加载)
func (m *SnapshotManager) EnsureLoaded(ctx context.Context, name string) error
// 1. 检查 Neo4j 是否已有名为 name 的逻辑 DB
// 2. 有 → 直接使用
// 3. 没有 → 读取 YAML 文件 → 写入 Neo4j 逻辑 DB
// 4. 触发清理: 如果活跃逻辑 DB 数 > maxActive，清理最久未使用的

// Restore: 将指定快照恢复为 default DB (持有写锁)
func (m *SnapshotManager) Restore(ctx context.Context, name string) error
// 1. lock.Lock()                ← 排他锁，阻塞增量同步
// 2. EnsureLoaded(name)         → 确保快照已在 Neo4j 逻辑 DB 中
// 3. ClearDB("default")
// 4. CloneDB(name, "default")
// 5. lock.Unlock()              ← 释放锁

func (m *SnapshotManager) List(ctx context.Context) ([]SnapshotMeta, error)
func (m *SnapshotManager) Delete(ctx context.Context, name string) error
func (m *SnapshotManager) Diff(ctx context.Context, a, b string) (*SnapshotDiff, error)

// cleanup: 清理 Neo4j 中超过 maxActive 的逻辑 DB (按最近使用时间排序)
func (m *SnapshotManager) cleanup(ctx context.Context) error
```

#### `internal/snapshot/graphlock.go` — 并发保护

```go
type GraphLock struct {
    mu sync.RWMutex
}

// Lock: Restore/FullSync 使用 (排他锁，阻塞增量写入)
func (l *GraphLock) Lock()
func (l *GraphLock) Unlock()

// RLock: 只读查询使用 (共享锁，不阻塞其他读)
func (l *GraphLock) RLock()
func (l *GraphLock) RUnlock()
```

**GraphLock 使用场景**：

| 操作 | 锁类型 | 说明 |
|------|--------|------|
| Restore | 写锁 (Lock) | ClearDB + CloneDB 期间禁止增量写入 |
| FullSync | 写锁 (Lock) | ClearDB + BulkCreate 期间禁止增量写入 |
| IncrementalSync | 写锁 (Lock) | Upsert/DeleteByURIs 期间禁止其他写入 |
| MCP Query | 读锁 (RLock) | 查询期间允许其他读，阻塞写 |
| Snapshot.Create | 读锁 (RLock) | 导出期间允许其他读 |

#### Channel 缓冲防丢消息

```
问题场景:
  Restore 持有 GraphLock 期间 (可能耗时数秒)
  Webhook 请求到达 → 如果阻塞等锁 → 外部系统超时 → 重试 → 消息重复/丢失

解决方案: Webhook → Channel → 消费者协程

  WebhookHandler
      │
      │ 立即写入 channel → 返回 202 Accepted (毫秒级)
      ▼
  SyncEvent (入队)
      │
      ▼
  消费者协程 (单协程，串行消费)
      │
      │ 等待 GraphLock 可用
      ▼
  IncrementalSync()
      │ lock.Lock()
      │ Upsert / DeleteByURIs
      │ lock.Unlock()
```

#### Webhook Payload 格式

```json
{
  "action": "update",
  "entity_type": "Device",
  "connector": "netbox",
  "data": [
    { "serial_number": "SN12345", "hostname": "Router_Core_01", "status": "Down", ... }
  ]
}
```

```json
{
  "action": "delete",
  "entity_type": "Device",
  "connector": "netbox",
  "uris": ["device:SN99999"]
}
```

```json
{
  "action": "delete_relation",
  "entity_type": "Device",
  "connector": "netbox",
  "relations": [
    { "type": "HAS_INTERFACE", "from": "device:SN12345", "to": "iface:SN12345_GE1/0/2" }
  ]
}
```

**Action 语义**：

| Action | 节点处理 | 关系处理 |
|--------|---------|---------|
| `update` | MERGE + SET += | MERGE（新增，不删旧关系） |
| `delete` | DELETE 节点 + DETACH DELETE | 节点删除时自动清理所有关联关系 |
| `delete_relation` | 不涉及 | 仅删除指定关系 |

#### ID 稳定性与合并策略

**1. URI 永久不变性**

URI 由 Schema EntityType 的 `identity.stableKeys` + `uriTemplate` 生成。`stableKeys` 必须选择不可变标识：

```yaml
# ✅ 正确: 序列号不可变，URI 永久有效
spec:
  identity:
    stableKeys: [serial_number]
  uriTemplate: "device:{serial_number}"

# ❌ 错误: hostname 会变，设备改名后 URI 失效
spec:
  identity:
    stableKeys: [hostname]
  uriTemplate: "device:{hostname}"
```

**2. 属性合并策略 (增量合并)**

```cypher
MERGE (d:Device {_db: $_db, uri: $uri})
SET d += $props  -- 只更新传入的属性，未传入的属性保持不变
```

**3. 关系合并策略 (增量更新)**

```cypher
-- 新增关系: MERGE (幂等，已存在则跳过)
MERGE (a {_db: $_db, uri: $from})-[:HAS_INTERFACE]->(b {_db: $_db, uri: $to})

-- 删除关系: 仅当收到明确删除事件时
MATCH (a {_db: $_db, uri: $from})-[r:HAS_INTERFACE]->(b {_db: $_db, uri: $to})
DELETE r
```

**4. 节点防重 (MERGE 复合键)**

| 机制 | 策略 | Neo4j 实现 |
|------|------|-----------|
| URI 永久不变 | stableKeys 选不可变标识 | `uriTemplate` 生成 |
| 属性合并 | 增量合并 (SET +=) | `MERGE + SET d += $props` |
| 关系合并 | 增量更新 (MERGE) | `MERGE` 新增关系，`delete_relation` 事件明确删除 |
| 节点防重 | MERGE 基于复合键 | `MERGE (n {_db, uri})` |

## 测试内容

### 单元测试

| 测试文件 | 测试范围 | 测试数据 |
|----------|----------|----------|
| `internal/service/sync_service_test.go` | 全量/增量同步编排 | Mock Connector 数据 |
| `internal/snapshot/manager_test.go` | 快照管理 | 内存文件系统 |
| `internal/snapshot/graphlock_test.go` | GraphLock 并发保护 | 并发 goroutine |

### 集成测试

| 测试文件 | 测试范围 | 测试方法 |
|----------|----------|----------|
| `internal/service/sync_test.go` | 全量同步 ETL | testcontainers-go → FullSync → 验证节点/关系数 |
| `internal/service/webhook_test.go` | 增量同步 | 发送 Webhook → 验证数据变更 |
| `internal/snapshot/snapshot_test.go` | 快照创建/恢复 | Create → 修改数据 → Restore → 验证一致 |
| `internal/snapshot/concurrent_test.go` | 并发保护 | Restore 期间发送 Webhook → 验证消息不丢失 |

#### 全量同步测试用例

```go
// TC-SYNC-01: 全量导入
Step:  FullSync()
Check: 返回 SyncResult{NodesCreated: 20+, RelationsCreated: 25+}
       Neo4j 中存在 3 个 Device、12+ 个 Interface、3+ 个 ISIS

// TC-SYNC-02: 孤儿边检测
Step:  注入引用不存在 Interface 的 Device
Check: SyncResult.OrphanEdgesSkipped > 0，同步继续不中断

// TC-SYNC-03: 身份对齐验证
Step:  Mock 数据含 serial_number="SN12345"
Check: Neo4j 中节点 uri = "device:SN12345"
```

#### 增量同步测试用例

```go
// TC-WEBHOOK-01: update 事件
Step:  发送 Webhook {action: "update", data: [{serial_number: "SN12345", status: "Down"}]}
Check: Neo4j 查询 device:SN12345.status = "Down"，其他属性保留

// TC-WEBHOOK-02: delete 事件
Step:  发送 Webhook {action: "delete", uris: ["device:SN99999"]}
Check: Neo4j 查询 device:SN99999 不存在，关联关系已清理

// TC-WEBHOOK-03: delete_relation 事件
Step:  发送 Webhook {action: "delete_relation", relations: [{type: "HAS_INTERFACE", ...}]}
Check: 指定关系已删除，节点仍存在

// TC-WEBHOOK-04: Channel 缓冲防丢消息
Step:  Restore 期间发送 10 个 Webhook
Check: Restore 完成后，10 个事件全部被处理，无丢失
```

#### 快照系统测试用例

```go
// TC-SNAP-01: 创建快照
Step:  FullSync() → Create("snap_001")
Check: YAML 文件写入 snapDir/snap_001.yaml，文件非空

// TC-SNAP-02: 快照恢复
Step:  FullSync() → Create("snap_001") → 修改数据 → Restore("snap_001")
Check: Neo4j 数据恢复到 snap_001 的状态

// TC-SNAP-03: 快照列表
Step:  Create("snap_001") + Create("snap_002") → List()
Check: 返回 2 个 SnapshotMeta，含时间戳和节点数

// TC-SNAP-04: 快照对比
Step:  Create("snap_001") → 修改数据 → Create("snap_002") → Diff("snap_001", "snap_002")
Check: 返回节点/关系差异列表

// TC-SNAP-05: EnsureLoaded 懒加载
Step:  查询 snap_001 → EnsureLoaded() → 再次查询
Check: 第一次从 YAML 加载，第二次直接使用 Neo4j 逻辑 DB

// TC-SNAP-06: YAML 归档永久保留
Step:  Delete("snap_001")
Check: Neo4j 逻辑 DB 被清理，但 YAML 文件仍保留
```

#### 并发保护测试用例

```go
// TC-LOCK-01: 写锁互斥
Step:  启动 goroutine A 持有写锁 → 启动 goroutine B 请求写锁
Check: B 阻塞等待 A 释放锁

// TC-LOCK-02: 读写锁兼容
Step:  启动多个 goroutine 持有读锁 → 启动 goroutine 请求写锁
Check: 所有读锁完成后写锁才获得

// TC-LOCK-03: Restore 期间 Webhook 不丢失
Step:  启动 Restore (持有写锁) → 发送 5 个 Webhook → 等待 Restore 完成
Check: 5 个 Webhook 事件全部被处理
```

## 验收标准

### 功能验收

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| A-01 | Mock 数据全量导入 | `POST /api/v1/sync/full` | 返回 SyncResult，节点数 ≥ 20，关系数 ≥ 30 |
| A-02 | 孤儿边检测 | 注入引用不存在节点的数据 | OrphanEdgesSkipped > 0，同步继续 |
| A-03 | update 事件处理 | `POST /api/v1/sync/webhook` {action: "update"} | 属性增量合并正确 |
| A-04 | delete 事件处理 | `POST /api/v1/sync/webhook` {action: "delete"} | 节点 + 关联关系删除 |
| A-05 | delete_relation 事件处理 | `POST /api/v1/sync/webhook` {action: "delete_relation"} | 仅指定关系删除 |
| A-06 | 快照创建 | `POST /api/v1/snapshot/create` | YAML 文件生成，含节点/关系数 |
| A-07 | 快照恢复 | `POST /api/v1/snapshot/restore` | 数据恢复到快照状态 |
| A-08 | 快照列表 | `GET /api/v1/snapshot/list` | 返回所有 YAML 归档元数据 |
| A-09 | 快照对比 | `GET /api/v1/snapshot/diff?a=snap1&b=snap2` | 返回差异列表 |
| A-10 | YAML 归档永久保留 | `DELETE /api/v1/snapshot/snap1` | YAML 文件保留，Neo4j 逻辑 DB 清理 |
| A-11 | Channel 缓冲防丢 | Restore 期间发送 Webhook | 消息不丢失，Restore 完成后全部处理 |
| A-12 | URI 不可变性 | 修改 hostname/IP | URI 保持不变，关系和快照历史有效 |

### 质量验收

| 序号 | 验收项 | 验证方法 | 通过标准 |
|------|--------|----------|----------|
| Q-01 | 单元测试通过 | `go test ./internal/...` | 0 failures |
| Q-02 | 集成测试通过 | `go test -tags=integration ./...` | 0 failures |
| Q-03 | 代码覆盖率 | `go test -cover ./internal/...` | ≥ 75% |
| Q-04 | Lint 通过 | `golangci-lint run` | 无 Error |

### 验收门禁

**以下条件全部满足后，方可进入第三阶段：**

- [ ] A-01~A-12：功能验收全部通过
- [ ] Q-01~Q-04：质量验收全部通过
- [ ] GraphLock 并发测试通过（无死锁、无数据竞争）
- [ ] Channel 缓冲测试通过（无消息丢失）
