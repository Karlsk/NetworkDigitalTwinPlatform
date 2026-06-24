package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"gitlab.com/pml/network-digital-twin/internal/assembler"
	"gitlab.com/pml/network-digital-twin/internal/connector/mock"
	"gitlab.com/pml/network-digital-twin/internal/normalizer"
	"gitlab.com/pml/network-digital-twin/internal/schema"
)

func main() {
	// 1. 加载本体
	ontologyDir := filepath.Join("ontology")
	reg := schema.NewSchemaRegistry()
	if err := reg.Load(ontologyDir); err != nil {
		log.Fatalf("Load ontology: %v", err)
	}
	fmt.Println("=== Schema Registry 加载完成 ===")
	fmt.Printf("  EntityType 数量: %d\n", len(reg.ListEntityTypes()))
	fmt.Printf("  RelationType 数量: %d\n\n", len(reg.ListRelationTypes()))

	// 2. 创建 Mock Connector
	dataDir := filepath.Join("testdata", "mock_netbox")
	conn := mock.NewMockConnector("mock-netbox", dataDir,
		[]string{"Device", "Interface", "ISIS", "Link", "Network_Slice"})

	// 3. 创建 Normalizer + Assembler
	norm := normalizer.NewNormalizer(reg)
	asm := assembler.NewGraphAssembler(reg)

	var allResources []normalizer.NormalizedResource
	ctx := context.Background()

	// 4. 逐实体类型处理
	for _, et := range []string{"Device", "Interface", "ISIS", "Link", "Network_Slice"} {
		resources, err := conn.Collect(ctx, et)
		if err != nil {
			fmt.Printf("  [跳过] %s: %v\n", et, err)
			continue
		}

		fmt.Printf("=== Connector 采集: %s (%d 条) ===\n", et, len(resources))

		for _, res := range resources {
			// 打印原始 Resource（只打印前 2 条的 Properties 摘要）
			nr, err := norm.Normalize(res)
			if err != nil {
				fmt.Printf("  [归一化失败] %s/%s: %v\n", res.Kind, res.ID, err)
				continue
			}

			// 打印归一化结果摘要
			fmt.Printf("  归一化: %s → URI=%s, 属性数=%d\n", res.Kind, nr.URI, len(nr.Properties))
			allResources = append(allResources, *nr)
		}
		fmt.Println()
	}

	// 5. Assemble
	fmt.Println("=== GraphAssembler 组装 ===")
	gm, warnings, err := asm.Assemble(allResources)
	if err != nil {
		log.Fatalf("Assemble: %v", err)
	}

	fmt.Printf("  节点总数: %d\n", len(gm.Nodes))
	fmt.Printf("  关系总数: %d\n", len(gm.Relations))
	fmt.Printf("  警告总数: %d\n\n", len(warnings))

	// 6. 打印所有节点
	fmt.Println("=== 输出: 节点列表 ===")
	for i, n := range gm.Nodes {
		propsJSON, _ := json.Marshal(n.Props)
		fmt.Printf("  [%d] Label=%-15s URI=%-30s Props=%s\n", i+1, n.Label, n.URI, string(propsJSON))
	}

	// 7. 打印所有关系
	fmt.Println("\n=== 输出: 关系列表 ===")
	for i, r := range gm.Relations {
		fmt.Printf("  [%d] %s: %s → %s\n", i+1, r.Type, r.From, r.To)
	}

	// 8. 打印警告
	if len(warnings) > 0 {
		fmt.Println("\n=== 警告: 孤儿边 ===")
		for i, w := range warnings {
			fmt.Printf("  [%d] %s: %s\n", i+1, w.Type, w.Detail)
		}
	} else {
		fmt.Println("\n=== 无孤儿边警告 ===")
	}

	// 9. 提示：当前未写入 Neo4j
	fmt.Println("\n=== ⚠️  当前状态 ===")
	fmt.Println("  以上数据仅在内存中（GraphModel 结构体）")
	fmt.Println("  Neo4j 驱动层 (internal/graph/neo4j.go) 尚未实现")
	fmt.Println("  数据未落盘到 Neo4j 数据库")
	fmt.Println("\n  要写入 Neo4j，需要:")
	fmt.Println("    1. 启动 Neo4j: docker-compose -f deploy/docker-compose.yml up -d")
	fmt.Println("    2. 实现 GraphDB 接口 (internal/graph/neo4j.go)")
	fmt.Println("    3. 在 SyncService 中调用 GraphDB.BulkCreate()")
}
