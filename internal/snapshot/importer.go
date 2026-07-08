// Package snapshot 实现快照管理。
package snapshot

import (
	"bytes"
	"fmt"
	"os"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gopkg.in/yaml.v3"
)

// yamlNodeItemImport 同时支持新旧 YAML 格式:
// 新格式: labels: ["Resource", "Device"]
// 旧格式: label: Device（向后兼容）
type yamlNodeItemImport struct {
	Label  string   `yaml:"label"`  // 旧格式，兼容读取
	Labels []string `yaml:"labels"` // 新格式
	URI    string   `yaml:"uri"`
	Props  map[string]any `yaml:"props,omitempty"`
}

// getLabels 优先返回 Labels，如果为空则 fallback 到旧的 Label 字段。
func (item yamlNodeItemImport) getLabels() []string {
	if len(item.Labels) > 0 {
		return item.Labels
	}
	if item.Label != "" {
		return []string{item.Label}
	}
	return nil
}

// yamlNodesDocImport 节点文档（导入用，同时支持新旧格式）。
type yamlNodesDocImport struct {
	Kind  string                 `yaml:"kind"`
	Items []yamlNodeItemImport   `yaml:"items"`
}

// importMetaOnly 只解码第一个 YAML 文档（meta），不解码 nodes/rels。
// 性能提升: 从 O(N*全文档) 降为 O(N*meta文档头)。
func importMetaOnly(filePath string) (SnapshotMeta, error) {
	return ImportMetaOnly(filePath)
}

// ImportMetaOnly 解析 YAML 快照文件的第一个文档（元数据），返回 SnapshotMeta。
// V2-10: 导出供 cmd/migrate-data 数据迁移工具使用。
func ImportMetaOnly(filePath string) (SnapshotMeta, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return SnapshotMeta{}, fmt.Errorf("read file for meta: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var metaDoc yamlMetaDoc
	if err := decoder.Decode(&metaDoc); err != nil {
		return SnapshotMeta{}, fmt.Errorf("parse meta from %s: %w", filePath, err)
	}
	return SnapshotMeta{
		Name:      metaDoc.Name,
		CreatedAt: metaDoc.CreatedAt,
		NodeCount: metaDoc.NodeCount,
		RelCount:  metaDoc.RelCount,
		FilePath:  filePath,
	}, nil
}

// importFromYAML 从 YAML 多文档文件读取快照数据。
// 返回节点、关系和元数据。
func importFromYAML(filePath string) (nodes []assembler.Node, rels []assembler.Relation, meta SnapshotMeta, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, SnapshotMeta{}, fmt.Errorf("open yaml file: %w", err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)

	// 文档 1: 元数据
	var metaDoc yamlMetaDoc
	if err := dec.Decode(&metaDoc); err != nil {
		return nil, nil, SnapshotMeta{}, fmt.Errorf("decode meta doc: %w", err)
	}
	meta = SnapshotMeta{
		Name:      metaDoc.Name,
		CreatedAt: metaDoc.CreatedAt,
		NodeCount: metaDoc.NodeCount,
		RelCount:  metaDoc.RelCount,
		FilePath:  filePath,
	}

	// 文档 2: 节点
	var nodesDoc yamlNodesDocImport
	if err := dec.Decode(&nodesDoc); err != nil {
		return nil, nil, SnapshotMeta{}, fmt.Errorf("decode nodes doc: %w", err)
	}
	nodes = make([]assembler.Node, 0, len(nodesDoc.Items))
	for _, item := range nodesDoc.Items {
		nodes = append(nodes, assembler.Node{
			Labels: item.getLabels(),
			URI:    item.URI,
			Props:  item.Props,
		})
	}

	// 文档 3: 关系
	var relsDoc yamlRelsDoc
	if err := dec.Decode(&relsDoc); err != nil {
		return nil, nil, SnapshotMeta{}, fmt.Errorf("decode rels doc: %w", err)
	}
	rels = make([]assembler.Relation, 0, len(relsDoc.Items))
	for _, item := range relsDoc.Items {
		rels = append(rels, assembler.Relation{
			Type:  item.Type,
			From:  item.From,
			To:    item.To,
			Props: item.Props,
		})
	}

	return nodes, rels, meta, nil
}
