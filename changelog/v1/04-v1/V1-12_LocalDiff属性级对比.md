# V1-12: LocalDiff 属性级对比

**工时**: 1 天
**前置**: V1-11
**风险等级**: 低
**Phase**: Phase 2b — 属性级 Diff + 本体继承

---

## 背景

当前 `LocalDiff` 方法（纯内存 Go map 差集，不依赖 Neo4j）只检测 URI 增删。本任务增强其属性级对比能力：对 URI 交集中的节点做 Props 逐字段对比。

---

## 实现内容

### snapshot/manager.go — LocalDiff 方法增强

在现有 LocalDiff 逻辑基础上追加属性级对比：

```go
func (m *SnapshotManager) LocalDiff(a, b string) (*SnapshotDiff, error) {
    // === 现有逻辑保持不变 ===
    // 1. 加载快照 a 和 b 的 YAML 数据
    // 2. 构建 aNodeURIs / bNodeURIs map
    // 3. 差集计算 → AddedNodes / RemovedNodes

    // === 新增: 属性级对比 ===
    // 4. 对 URI 交集中的节点调用 compareProps
    for uri, bNode := range bNodeMap {
        if aNode, ok := aNodeMap[uri]; ok {
            added, removed, modified := compareProps(aNode.Props, bNode.Props)
            if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
                diff.ChangedNodes = append(diff.ChangedNodes, NodeChange{
                    URI:            uri,
                    Label:          bNode.MostSpecificLabel(),
                    AddedFields:    added,
                    RemovedFields:  removed,
                    ModifiedFields: modified,
                })
            }
        }
    }

    // 5. 同理处理关系的 Props（通常为空，但框架需支持）
    for key, bRel := range bRelMap {
        if aRel, ok := aRelMap[key]; ok {
            added, removed, modified := compareProps(aRel.Props, bRel.Props)
            if len(added) > 0 || len(removed) > 0 || len(modified) > 0 {
                diff.ChangedRels = append(diff.ChangedRels, RelChange{
                    Type:           bRel.Type,
                    From:           bRel.From,
                    To:             bRel.To,
                    AddedFields:    added,
                    RemovedFields:  removed,
                    ModifiedFields: modified,
                })
            }
        }
    }

    return diff, nil
}
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/snapshot/manager.go` | 修改 | LocalDiff 增强属性级对比 |

---

## 验收标准

- [x] 编译通过
- [x] LocalDiff 对相同 URI 的节点正确报告 AddedFields
- [x] LocalDiff 对相同 URI 的节点正确报告 RemovedFields
- [x] LocalDiff 对相同 URI 的节点正确报告 ModifiedFields（含 int/float64 归一化）
- [x] 无属性差异的节点不出现在 ChangedNodes 中
- [x] 现有 AddedNodes/RemovedNodes 逻辑不受影响
