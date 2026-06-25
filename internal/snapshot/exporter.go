// Package snapshot 实现快照管理。
package snapshot

import (
	"fmt"
	"os"
	"time"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gopkg.in/yaml.v3"
)

// YAML 多文档中间结构体。
type yamlMetaDoc struct {
	Kind      string    `yaml:"kind"`
	Name      string    `yaml:"name"`
	CreatedAt time.Time `yaml:"created_at"`
	NodeCount int       `yaml:"node_count"`
	RelCount  int       `yaml:"rel_count"`
}

type yamlNodeItem struct {
	Label string         `yaml:"label"`
	URI   string         `yaml:"uri"`
	Props map[string]any `yaml:"props,omitempty"`
}

type yamlNodesDoc struct {
	Kind  string         `yaml:"kind"`
	Items []yamlNodeItem `yaml:"items"`
}

type yamlRelItem struct {
	Type  string         `yaml:"type"`
	From  string         `yaml:"from"`
	To    string         `yaml:"to"`
	Props map[string]any `yaml:"props,omitempty"`
}

type yamlRelsDoc struct {
	Kind  string        `yaml:"kind"`
	Items []yamlRelItem `yaml:"items"`
}

// exportToYAML 将快照数据导出为 YAML 多文档文件。
// 文件包含三个 YAML 文档: SnapshotMeta → Nodes → Relations。
func exportToYAML(filePath string, meta SnapshotMeta, nodes []assembler.Node, rels []assembler.Relation) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create yaml file: %w", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	defer enc.Close()

	// 文档 1: 元数据
	metaDoc := yamlMetaDoc{
		Kind:      "SnapshotMeta",
		Name:      meta.Name,
		CreatedAt: meta.CreatedAt,
		NodeCount: meta.NodeCount,
		RelCount:  meta.RelCount,
	}
	if err := enc.Encode(metaDoc); err != nil {
		return fmt.Errorf("encode meta doc: %w", err)
	}

	// 文档 2: 节点
	nodeItems := make([]yamlNodeItem, 0, len(nodes))
	for _, n := range nodes {
		nodeItems = append(nodeItems, yamlNodeItem{
			Label: n.Label,
			URI:   n.URI,
			Props: n.Props,
		})
	}
	nodesDoc := yamlNodesDoc{Kind: "Nodes", Items: nodeItems}
	if err := enc.Encode(nodesDoc); err != nil {
		return fmt.Errorf("encode nodes doc: %w", err)
	}

	// 文档 3: 关系
	relItems := make([]yamlRelItem, 0, len(rels))
	for _, r := range rels {
		relItems = append(relItems, yamlRelItem{
			Type:  r.Type,
			From:  r.From,
			To:    r.To,
			Props: r.Props,
		})
	}
	relsDoc := yamlRelsDoc{Kind: "Relations", Items: relItems}
	if err := enc.Encode(relsDoc); err != nil {
		return fmt.Errorf("encode rels doc: %w", err)
	}

	return nil
}
