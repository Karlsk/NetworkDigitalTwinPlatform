# V1-01: Node.Labels 多标签改造 (BREAKING CHANGE)

**工时**: 2 天
**前置**: 无
**风险等级**: 高（跨包 BREAKING CHANGE）
**Phase**: Phase 1 — 基础迁移

---

## 背景

当前 `assembler.Node` 使用 `Label string`（单标签），无法支持 V1-15 本体继承体系（`extends`）所需的多 Label 语义（如 `:Resource:Device`）。本任务是整个继承机制的前置基础。

---

## Day 1: 核心数据结构 + Neo4j 驱动层改造

### 1. assembler/types.go

将 `Node.Label string` 改为 `Node.Labels []string`：

```go
// MVP
type Node struct {
    Label string
    URI   string
    Props map[string]any
}

// V1
type Node struct {
    Labels []string  // ["Resource", "Device"]，从基类到具体类
    URI    string
    Props  map[string]any
}

// 便捷构造函数保持向后兼容。
func NewNode(label string, uri string, props map[string]any) Node {
    return Node{Labels: []string{label}, URI: uri, Props: props}
}

// MostSpecificLabel 返回最具体的标签（最后一个）。
func (n Node) MostSpecificLabel() string {
    if len(n.Labels) == 0 {
        return ""
    }
    return n.Labels[len(n.Labels)-1]
}
```

### 2. graph/neo4j.go — Cypher 生成改造

- **`groupNodesByLabel` → `groupNodesByLabels`**: 按 `MostSpecificLabel()` 分组（MERGE 需要按最具体 Label 分组）
- **`BulkCreate`**: `CREATE (x:Label ...)` → `CREATE (x:Parent:Child ...)` — 多 Label 拼接
  ```cypher
  UNWIND $batch AS row
  CREATE (x:Resource:Device {_db: $_db})
  SET x += row.props
  ```
- **`Upsert`**: Neo4j `MERGE` 不支持多 Label，因此：
  ```cypher
  MERGE (x:Device {_db: $_db, uri: $uri})
  ON CREATE SET x:Resource   -- 创建时补充父 Label
  SET x += $props
  ```
- **`CloneDB`**: `groupCloneNodesByLabel` 从 `labels(n)` 返回的 `[]string` 正确分组
- **`BuildCypher`**: 同步更新预览生成逻辑

### 3. graph/interface.go

`EnsureIndexes(ctx, labels []string)` — 需为每个 Label（包括父 Label）创建索引：

```cypher
CREATE INDEX idx_resource_db_uri IF NOT EXISTS FOR (n:Resource) ON (n._db, n.uri);
CREATE INDEX idx_device_db_uri IF NOT EXISTS FOR (n:Device) ON (n._db, n.uri);
```

---

## Day 2: 上下游适配

### 4. assembler/assembler.go

`Assemble()` 中节点转换：

```go
// MVP: Node.Label = res.Kind
// V1 (暂时单标签，V1-15 扩展为多标签):
node := assembler.Node{
    Labels: []string{res.Kind},
    URI:    res.URI,
    Props:  res.Properties,
}
```

### 5. snapshot/exporter.go

YAML 导出结构变更：

```go
// MVP
type yamlNodeItem struct {
    Label string `yaml:"label"`
    URI   string `yaml:"uri"`
    Props map[string]any `yaml:"props"`
}

// V1
type yamlNodeItem struct {
    Labels []string `yaml:"labels"`
    URI    string   `yaml:"uri"`
    Props  map[string]any `yaml:"props"`
}
```

### 6. snapshot/importer.go

向后兼容旧格式：

```go
// 兼容逻辑: 如果 YAML 中只有 label 字段（旧格式），转为 []string{label}
type yamlNodeItem struct {
    Label  string   `yaml:"label"`   // 旧格式，兼容读取
    Labels []string `yaml:"labels"`  // 新格式
    URI    string   `yaml:"uri"`
    Props  map[string]any `yaml:"props"`
}

func (item yamlNodeItem) getLabels() []string {
    if len(item.Labels) > 0 {
        return item.Labels
    }
    if item.Label != "" {
        return []string{item.Label}
    }
    return nil
}
```

### 7. mcp/tools.go

Diff 输出等引用 `Node.Label` 的地方全部适配为 `Node.MostSpecificLabel()` 或 `Node.Labels`。

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/assembler/types.go` | 修改 (核心) | `Label` → `Labels []string` + `NewNode()` + `MostSpecificLabel()` |
| `internal/assembler/assembler.go` | 修改 | 节点转换适配 |
| `internal/assembler/assembler_test.go` | 修改 | 测试适配 |
| `internal/assembler/types_test.go` | 修改 | 测试适配 |
| `internal/graph/neo4j.go` | 修改 (核心) | Cypher 生成全面改造 |
| `internal/graph/neo4j_test.go` | 修改 | Cypher 断言更新 |
| `internal/graph/interface.go` | 修改 | EnsureIndexes 参数适配 |
| `internal/snapshot/exporter.go` | 修改 | YAML 导出 `labels` 字段 |
| `internal/snapshot/importer.go` | 修改 | 向后兼容 `label` 旧格式 |
| `internal/snapshot/manager.go` | 修改 | 引用 Label 的地方适配 |
| `internal/snapshot/manager_test.go` | 修改 | 测试适配 |
| `internal/mcp/tools.go` | 修改 | Diff 输出适配 |
| `cmd/server/main.go` | 修改 | EnsureIndexes 调用适配 |
| `cmd/pipeline-demo/main.go` | 修改 | Demo 适配 |

---

## 验收标准

- [x] `go build ./...` 编译通过
- [x] `go test -race ./...` 全部现有测试通过
- [x] `BuildCypher("create", ...)` 输出包含多 Label 格式 `CREATE (x:Parent:Child ...)`
- [x] 旧格式 YAML 快照文件（`label` 字段）仍可正确导入（向后兼容）
- [x] `NewNode(label, uri, props)` 便捷构造函数正常工作
- [x] `Node.MostSpecificLabel()` 返回正确的最具体标签
