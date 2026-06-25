// Package snapshot 实现快照管理。
package snapshot

import (
	"fmt"
	"os"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gopkg.in/yaml.v3"
)

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
	var nodesDoc yamlNodesDoc
	if err := dec.Decode(&nodesDoc); err != nil {
		return nil, nil, SnapshotMeta{}, fmt.Errorf("decode nodes doc: %w", err)
	}
	nodes = make([]assembler.Node, 0, len(nodesDoc.Items))
	for _, item := range nodesDoc.Items {
		nodes = append(nodes, assembler.Node{
			Label: item.Label,
			URI:   item.URI,
			Props: item.Props,
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
