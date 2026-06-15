# I-13: GraphLock 并发保护

## 1. 任务概述

实现 GraphLock（基于 `sync.RWMutex`），为 Restore/FullSync/IncrementalSync 提供写锁互斥，为 MCP Query/Snapshot.Create 提供读锁共享。防止并发写入导致脏图。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 3: 实现阶段 — 同步服务 + 快照管理 |
| 预估工时 | 0.5 天 |
| 前置任务 | I-08 |
| 交付物 | `internal/snapshot/graphlock.go` |

## 2. 详细实现步骤

```go
package snapshot

import "sync"

// GraphLock 图数据库并发保护锁
// Restore/FullSync/IncrementalSync 持有写锁，阻塞其他写操作
// MCP Query/Snapshot.Create 持有读锁，允许并发读
type GraphLock struct {
    mu sync.RWMutex
}

func NewGraphLock() *GraphLock {
    return &GraphLock{}
}

// Lock 排他锁（Restore/FullSync/IncrementalSync 使用）
func (l *GraphLock) Lock()   { l.mu.Lock() }
func (l *GraphLock) Unlock() { l.mu.Unlock() }

// RLock 共享锁（MCP Query/Snapshot.Create 使用）
func (l *GraphLock) RLock()   { l.mu.RLock() }
func (l *GraphLock) RUnlock() { l.mu.RUnlock() }
```

### 使用场景表

| 操作 | 锁类型 | 说明 |
|------|--------|------|
| Restore | 写锁 (Lock) | ClearDB + CloneDB 期间禁止增量写入 |
| FullSync | 写锁 (Lock) | ClearDB + BulkCreate 期间禁止增量写入 |
| IncrementalSync | 写锁 (Lock) | Upsert/DeleteByURIs 期间禁止其他写入 |
| MCP Query | 读锁 (RLock) | 查询期间允许其他读，阻塞写 |
| Snapshot.Create | 读锁 (RLock) | 导出期间允许其他读 |

### 关键约束

- **SyncService 和 SnapshotManager 必须共享同一个 GraphLock 实例**
- 在 `cmd/server/main.go` 中创建一次，通过构造函数注入

## 3. 设计原理

- `sync.RWMutex` 而非 `sync.Mutex`：允许多个读操作并发，只阻塞写操作
- 只有一种锁（GraphLock），避免多锁交叉导致的死锁风险
- 写锁期间增量同步被阻塞，但 Webhook 不丢消息（Channel 缓冲）

## 4. 验收标准

- [ ] 写锁互斥验证（goroutine A 持有 Lock → B 的 Lock 阻塞）
- [ ] 读写锁兼容（多个 RLock 可同时持有）
- [ ] SyncService 和 SnapshotManager 使用同一个 GraphLock 实例

## 5. 注意事项

- 不要在 Lock 内部调用 RLock（Go 的 RWMutex 不支持锁降级）
- 确保 Lock 和 Unlock 成对出现（使用 defer）
- 写锁持有时间应该尽量短（ClearDB + BulkCreate/CloneDB 的时间）

---

# I-14: SyncService.FullSync 全量同步

## 1. 任务概述

实现 SyncService.FullSync：持有写锁 → ClearDB → 遍历所有 Connector 全量拉取 → Normalizer → Assembler → GraphDB.BulkCreate → 释放锁。编排完整的数据同步流水线。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 3: 实现阶段 — 同步服务 + 快照管理 |
| 预估工时 | 1.5 天 |
| 前置任务 | I-03, I-04, I-05, I-06, I-09, I-13 |
| 交付物 | `internal/service/sync_service.go` |

## 2. 详细实现步骤

```go
package service

import (
    "context"
    "log/slog"
    "time"

    "gitlab.com/pml/network-digital-twin/internal/assembler"
    "gitlab.com/pml/network-digital-twin/internal/connector"
    "gitlab.com/pml/network-digital-twin/internal/graph"
    "gitlab.com/pml/network-digital-twin/internal/normalizer"
    "gitlab.com/pml/network-digital-twin/internal/snapshot"
)

type SyncService struct {
    registry   *connector.ConnectorRegistry
    normalizer *normalizer.Normalizer
    assembler  *assembler.GraphAssembler
    graph      graph.GraphDB
    lock       *snapshot.GraphLock
    eventChan  chan SyncEvent
}

func NewSyncService(
    registry *connector.ConnectorRegistry,
    norm *normalizer.Normalizer,
    asm *assembler.GraphAssembler,
    gdb graph.GraphDB,
    lock *snapshot.GraphLock,
    bufferSize int,
) *SyncService {
    return &SyncService{
        registry:   registry,
        normalizer: norm,
        assembler:  asm,
        graph:      gdb,
        lock:       lock,
        eventChan:  make(chan SyncEvent, bufferSize),
    }
}

func (s *SyncService) FullSync(ctx context.Context) (*SyncResult, error) {
    start := time.Now()

    // 1. 持有写锁
    s.lock.Lock()
    defer s.lock.Unlock()

    // 2. ClearDB
    if err := s.graph.ClearDB(ctx, "default"); err != nil {
        return nil, err
    }

    // 3. 全量拉取所有 Connector 的所有实体
    var allResources []connector.Resource
    for _, meta := range s.registry.List() {
        conn, _ := s.registry.Get(meta.Name)
        for _, et := range meta.EntityTypes {
            resources, err := conn.Collect(ctx, et)
            if err != nil {
                slog.Error("collect failed", "connector", meta.Name, "entityType", et, "error", err)
                continue // 单个 Connector 失败不阻断其他
            }
            allResources = append(allResources, resources...)
        }
    }

    // 4. 归一化
    var allNormalized []normalizer.NormalizedResource
    for _, r := range allResources {
        norm, err := s.normalizer.Normalize(r)
        if err != nil {
            slog.Warn("normalize failed", "kind", r.Kind, "id", r.ID, "error", err)
            continue
        }
        allNormalized = append(allNormalized, *norm)
    }

    // 5. 组装图模型
    model, warnings, err := s.assembler.Assemble(allNormalized)
    if err != nil {
        return nil, err
    }

    // 6. 批量写入 Neo4j
    if err := s.graph.BulkCreate(ctx, "default", model.Nodes, model.Relations); err != nil {
        return nil, err
    }

    return &SyncResult{
        NodesCreated:       len(model.Nodes),
        RelationsCreated:  len(model.Relations),
        OrphanEdgesSkipped: len(warnings),
        Warnings:           warnings,
        Duration:           time.Since(start),
    }, nil
}
```

## 3. 设计原理

- **流水线编排**：Connector → Normalizer → Assembler → GraphDB，各层解耦
- **容错策略**：单个 Connector/Normalizer 失败不阻断整个同步，记录日志继续处理
- **写锁保护**：FullSync 期间 IncrementalSync 和 Restore 被阻塞

## 4. 验收标准

- [ ] FullSync Mock 数据后 Neo4j 中存在正确数量的节点（≥20）和关系（≥30）
- [ ] SyncResult 包含正确的统计信息
- [ ] 并发调用 FullSync 时互斥生效

## 5. 注意事项

- Connector.Collect 失败时 log.Error + continue，不 return error
- Normalizer 失败时 log.Warn + continue（可能是数据不合法）
- FullSync 的 defer Unlock 确保异常时也释放锁
