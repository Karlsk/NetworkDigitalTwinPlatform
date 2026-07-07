# I-06: GraphAssembler 孤儿边校验

## 1. 任务概述

在 GraphAssembler.Assemble() 方法中增加孤儿边校验：关系推导完成后，检查每条关系的目标节点是否存在于当前批次的 Nodes 中。不存在的关系跳过 + Warn，不阻断同步。

> 注意：此任务的核心逻辑已在 I-05 的 Assemble 方法中实现（阶段 3），本任务聚焦于完善校验逻辑和可观测性。

| 属性 | 值 |
|------|-----|
| 所属阶段 | Phase 2: 实现阶段 — 数据流管线 |
| 预估工时 | 0.5 天 |
| 前置任务 | I-05 |
| 交付物 | `internal/assembler/assembler.go` 补充 |

## 2. 详细实现步骤

### 孤儿边校验逻辑（已在 I-05 的 Assemble 阶段 3 中）

```go
// 阶段 3: 孤儿边校验 — 检查关系目标节点是否存在于 Nodes 中
var validRelations []Relation
for _, rel := range relations {
    if !uriIndex[rel.To] {
        warnings = append(warnings, ValidationWarning{
            Type:   "orphan_edge",
            Detail: fmt.Sprintf("%s: %s → %s", rel.Type, rel.From, rel.To),
        })
        slog.Warn("orphan edge skipped",
            "type", rel.Type, "from", rel.From, "to", rel.To)
        continue
    }
    validRelations = append(validRelations, rel)
}
```

### SyncResult 中的孤儿边统计

**文件**: `internal/service/sync_service.go`

```go
type SyncResult struct {
    NodesCreated       int
    RelationsCreated  int
    OrphanEdgesSkipped int                            // 孤儿边计数 (可观测)
    Warnings           []assembler.ValidationWarning
    Duration           time.Duration
}
```

FullSync/IncrementalSync 返回 SyncResult 时，`OrphanEdgesSkipped = len(warnings)`。

### 场景示例

```
输入:
  Device R1 引用 interfaces: ["iface:SN12345_GE1/0/2"]
  但该 Interface 不在当前批次中（未采集到）

处理:
  校验发现 uriIndex["iface:SN12345_GE1/0/2"] == false
  → log.Warn("orphan edge skipped: HAS_INTERFACE device:SN12345 → iface:SN12345_GE1/0/2")
  → 跳过该关系
  → SyncResult.OrphanEdgesSkipped++

结果:
  该 HAS_INTERFACE 关系不被创建
  Device R1 节点正常创建
  其他节点和关系不受影响
```

## 3. 设计原理

### 为什么跳过而不是报错？

- 一个 Interface 缺失不应该导致整个同步批次失败
- 多数据源场景下，不同 Connector 的采集时机可能不一致
- 下次全量同步时，如果目标节点存在，关系会自动补上

### 为什么是 Warn 而不是 Error？

- 孤儿边是预期内的情况（数据源不完整）
- Error 级别日志会触发告警，孤儿边不应该频繁告警
- 通过 SyncResult.OrphanEdgesSkipped 计数设置告警阈值（如 > 10% 才告警）

### uriIndex 的作用

- 阶段 1 建好所有节点后，将每个 URI 记录到 `uriIndex map[string]bool`
- 阶段 2 推导关系后，用 uriIndex 检查目标节点是否存在
- O(1) 的 map 查找，性能高效

## 4. 验收标准

- [ ] 目标节点不存在的关系被跳过，返回 `ValidationWarning{Type: "orphan_edge"}`
- [ ] 目标节点存在的关系正常生成
- [ ] slog.Warn 输出孤儿边详情（type, from, to）
- [ ] SyncResult.OrphanEdgesSkipped 正确计数
- [ ] 孤儿边不阻断同步（其他节点和关系正常处理）

## 5. 注意事项

- 孤儿边只检查 `rel.To`（目标节点），不检查 `rel.From`（源节点一定存在，因为是当前批次生成的）
- CONNECTS_TO 关系可能指向其他 Device 的 Interface，如果该 Interface 不在当前批次，会触发孤儿边
- 全量同步时孤儿边较少（所有 Connector 的数据都在），增量同步时可能较多
- V1 可考虑"延迟关系建立"机制：先记录孤儿边，目标节点出现后自动补建关系
