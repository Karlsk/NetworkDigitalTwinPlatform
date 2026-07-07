# V1-17: MetaCache 实现

**工时**: 1 天
**前置**: V1-14
**风险等级**: 中（并发一致性）
**Phase**: Phase 3 — 缓存审计 + 验收

---

## 背景

当前 `SnapshotManager.List()` 每次调用都对所有 YAML 文件做完整解析（`importFromYAML`），性能随快照数量线性退化。TD-03 要求引入 MetaCache 缓存 SnapshotMeta，避免重复解析。

---

## 实现内容

### 1. SnapshotManager 新增字段

```go
type SnapshotManager struct {
    // ... 现有字段
    metaCache map[string]SnapshotMeta  // 新增: name → meta 缓存
    cacheMu   sync.RWMutex            // 新增: 缓存读写锁
}
```

### 2. 缓存策略

| 操作 | 缓存行为 |
|------|---------|
| `Create()` | 完成后将 meta 写入 cache |
| `Delete()` | 从 cache 删除对应条目 |
| `List()` | 优先从 cache 读取；cache miss 时从 YAML 加载并回填 cache |
| `Restore()` | 不影响 cache |

### 3. List 优化

```go
func (m *SnapshotManager) List(ctx context.Context) ([]SnapshotMeta, error) {
    m.cacheMu.RLock()
    if len(m.metaCache) > 0 {
        // 缓存命中: 直接返回
        result := make([]SnapshotMeta, 0, len(m.metaCache))
        for _, meta := range m.metaCache {
            result = append(result, meta)
        }
        m.cacheMu.RUnlock()
        return result, nil
    }
    m.cacheMu.RUnlock()

    // 缓存未命中: 全量 warm cache
    return m.warmCache(ctx)
}

func (m *SnapshotManager) warmCache(ctx context.Context) ([]SnapshotMeta, error) {
    m.cacheMu.Lock()
    defer m.cacheMu.Unlock()

    // 扫描目录，只解析 meta 文档头
    files, _ := os.ReadDir(m.snapshotDir)
    var result []SnapshotMeta
    for _, f := range files {
        if !strings.HasSuffix(f.Name(), ".yaml") {
            continue
        }
        meta, err := importMetaOnly(filepath.Join(m.snapshotDir, f.Name()))
        if err != nil {
            continue
        }
        m.metaCache[meta.Name] = meta
        result = append(result, meta)
    }
    return result, nil
}
```

### 4. importer.go — importMetaOnly

```go
// importMetaOnly 只解码第一个 YAML 文档（meta），不解码 nodes/rels。
// 性能提升: 从 O(N*全文档) 降为 O(N*meta文档头)。
func importMetaOnly(filePath string) (SnapshotMeta, error) {
    data, err := os.ReadFile(filePath)
    if err != nil {
        return SnapshotMeta{}, err
    }
    // 只解码第一个 YAML 文档
    decoder := yaml.NewDecoder(bytes.NewReader(data))
    var meta SnapshotMeta
    if err := decoder.Decode(&meta); err != nil {
        return SnapshotMeta{}, fmt.Errorf("parse meta from %s: %w", filePath, err)
    }
    return meta, nil
}
```

---

## 涉及文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `internal/snapshot/manager.go` | 修改 | MetaCache + warmCache + List 优化 |
| `internal/snapshot/importer.go` | 修改 | importMetaOnly 新函数 |

---

## 验收标准

- [x] 编译通过
- [x] 第二次 `List()` 调用从缓存返回，不读取 YAML 文件
- [x] `Create` 后 `List` 立即可见新快照
- [x] `Delete` 后 `List` 立即不可见
- [x] 100 个快照文件时 `List()` 性能 < 100ms（首次 warm cache 后 < 1ms）
- [x] 并发 `List()` 调用无 data race
- [x] `go test -race ./internal/snapshot/...` 全部通过
