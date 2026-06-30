# V1-14: Diff 测试

**工时**: 1 天
**前置**: V1-12, V1-13
**风险等级**: 低
**Phase**: Phase 2b — 属性级 Diff + 本体继承

---

## 背景

V1-11 定义了属性级变更类型，V1-12/V1-13 分别实现了 LocalDiff 和 Cypher Diff 的属性级对比。本任务编写完整测试用例验证正确性。

---

## 实现内容

### 1. snapshot/manager_test.go — 新增测试

#### LocalDiff 属性级测试

| 测试ID | 场景 | 预期 |
|--------|------|------|
| TC-D01 | 属性新增字段检测 | `ChangedNodes[0].AddedFields` 包含新增字段 |
| TC-D02 | 属性删除字段检测 | `ChangedNodes[0].RemovedFields` 包含删除字段 |
| TC-D03 | 属性修改字段检测（含 int/float64 归一化） | `ChangedNodes[0].ModifiedFields` 正确，int(42) vs float64(42.0) 不误报 |
| TC-D04 | 无变更时 ChangedNodes 为空 | `len(diff.ChangedNodes) == 0` |
| TC-D05 | 多节点同时变更 | 多个 NodeChange 条目 |
| TC-D06 | 关系属性变更 | `ChangedRels` 正确检测 |

#### Cypher Diff 属性级测试（需 Neo4j 集成环境）

| 测试ID | 场景 | 预期 |
|--------|------|------|
| TC-D07 | Cypher Diff 属性级变更检测 | `ChangedNodes` 非空 |
| TC-D08 | Cypher Diff 与 LocalDiff 结果一致性 | 对相同数据两种方法结果一致 |

### 2. compareProps 单元测试

| 场景 | 预期 |
|------|------|
| 两个空 map | added/removed/modified 均为空 |
| a 为空，b 有值 | 全部为 added |
| a 有值，b 为空 | 全部为 removed |
| int(42) vs float64(42.0) | 相等，不出现在 modified |
| int(42) vs float64(43.0) | 出现在 modified |
| string "up" vs string "down" | 出现在 modified |
| bool true vs bool true | 相等 |

### 3. E2E 测试更新 (test/e2e/e2e_test.go)

- 创建快照 A → 修改节点属性 → 创建快照 B → Diff(A, B) 验证属性级变更
- 同时验证 LocalDiff 和 Cypher Diff 结果一致

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/snapshot/manager_test.go` | 修改 | 新增属性级 Diff 测试 |
| `test/e2e/e2e_test.go` | 修改 | E2E Diff 属性级验证 |

---

## 验收标准

- [ ] 全部测试用例通过
- [ ] LocalDiff 属性级对比 6 个测试用例全部通过
- [ ] Cypher Diff 属性级对比通过
- [ ] LocalDiff 和 Cypher Diff 对相同输入产出一致的 ChangedNodes/ChangedRels
- [ ] compareProps 数值归一化测试通过
- [ ] `go test -race ./internal/snapshot/...` 全部通过
