# 基于本体论的 SRv6 网络数字孪生系统——高阶工程架构与运维规范 (MVP补充方案)

## 1. 本体版本管理与灰度兼容规范

随着网络特性的演进（例如引入全新的 Flex-Algo 或 BIER-TE 协议），本体（Schema）必然会面临变更。为了保障现网数字孪生底座在升级过程中的平滑与稳定，特制定本规范。

### 1.1 Schema 变更的灰度发布策略
由于 Neo4j 配合 neosemantics (n10s) 初始化时使用了 `handleVocabUris: "IGNORE"`，图底座对 Schema 的感知属于弱类型与渐进式，这为灰度发布提供了天然的便利。
1. **非破坏性变更（向前兼容）：** 如仅新增类（Class）、新增属性（Data Property）或新语义关联。属于完全向前兼容，直接在主本体 TTL 中增量追加定义并热加载即可，存量孪生体无需任何改动。
2. **破坏性变更（不兼容变更）：** 如重命名核心语义关系（将 `net:carriedBy` 变更为 `net:transportedBy`）。
   * **过渡期语义双轨：** 在新版本体中同时保留旧关系与新关系，并使用 OWL 标准声明等价属性：`net:transportedBy owl:equivalentProperty net:carriedBy .`。
   * **智能体路由适配：** 更新大模型 Agent (OpenCode) 专家知识库，使其认知到新旧关系在当前版本的等价性。
   * **数据平滑洗数：** 灰度验证通过后，下发全量 Cypher 变动脚本，将旧关系异步收敛并彻底剔除。

### 1.2 历史快照（.ttl）的自适应版本兼容架构
当大模型 Agent 或运维人员调取历史 `.ttl` 快照进行故障复盘或仿真演练时，旧快照通常缺失新版本本体引入的硬性指标属性。本系统采用 **Go语言版本迁移适配层（Migration Adapter）** 来解决跨版本回溯问题。

```go
package twin

import (
	"context"
	"fmt"
	"[github.com/neo4j/neo4j-go-driver/v5/neo4j](https://github.com/neo4j/neo4j-go-driver/v5/neo4j)"
	"log"
)

type SnapshotVersionManager struct {
	driver neo4j.DriverWithContext
}

// LoadLegacySnapshotCompatible 自适应快照兼容加载器
func (m *SnapshotVersionManager) LoadLegacySnapshotCompatible(ctx context.Context, ttlContent string, snapshotVersion string) error {
	session := m.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// 1. 利用 n10s 动态强行载入旧快照（RDF天然向前兼容，新系统未定义的属性会作为普通标签保留，系统不崩溃）
	_, err := session.Run(ctx, `CALL n10s.rdf.import.inline($ttl, "Turtle");`, map[string]interface{}{"ttl": ttlContent})
	if err != nil {
		return fmt.Errorf("快照文本注入失败: %w", err)
	}

	// 2. 版本进化路由：如果注入的是 v1.0 旧快照，而当前孪生沙箱基于 v2.0 本体，则自动应用图谱补丁（Schema Patch）
	if snapshotVersion == "v1.0" {
		log.Println("⚠️ [版本兼容] 检测到 v1.0 历史快照，正在自动应用 v2.0 拓扑层演进补丁...")
		
		// 案例：新版本本体强要求 Interface 必须具备 flex_algo 属性，旧快照没有。在此处使用补丁予以初始化默认值
		patchCypher := `
		MATCH (i:Interface) 
		WHERE i.flex_algo IS NULL
		SET i.flex_algo = "Algo_0"
		`
		_, err = session.Run(ctx, patchCypher, nil)
		if err != nil {
			return fmt.Errorf("应用版本变迁补丁失败: %w", err)
		}
	}
	return nil
}
```
## 2. 基于 SHACL 的语义数据质量治理需求

由于控制器 API 和数据中台在现网复杂环境下极易吐出脏数据（如：接口带宽字段缺失、误报为负数、或者 SRv6 Policy 未关联物理物理接口等），系统必须引入语义级数据质量守门员。本系统采用 W3C 标准的 SHACL（Shapes Constraint Language，形状约束语言） 进行强校验拦截。

### 2.1 编写 SHACL 强校验契约文件 (srv6_shacl_shapes.ttl)
```ttl
@prefix sh: [http://www.w3.org/ns/shacl#](http://www.w3.org/ns/shacl#) .
@prefix xsd: [http://www.w3.org/2001/XMLSchema#](http://www.w3.org/2001/XMLSchema#) .
@prefix net: [http://srv6-twin.org/ontology/net#](http://srv6-twin.org/ontology/net#) .

# 1. 物理层接口核心数据合法性约束形状
net:InterfaceShape
    a sh:NodeShape ;
    sh:targetClass net:Interface ; # 校验的目标类
    
    # 约束 A：必须有且仅有一个状态属性，且数据类型必须是 String
    sh:property [
        sh:path net:status ;
        sh:minCount 1 ;
        sh:maxCount 1 ;
        sh:datatype xsd:string ;
    ] ;
    
    # 约束 B：物理链路带宽必须是正整数，硬性过滤负数和 0
    sh:property [
        sh:path net:bandwidth ;
        sh:datatype xsd:long ;
        sh:minExclusive 0 ; 
    ] .

# 2. 协议层 SRv6 Policy 跨层编织关联性约束形状
net:SRv6PolicyShape
    a sh:NodeShape ;
    sh:targetClass net:SRv6_Policy ;
    
    # 约束：任意一条 SRv6 Policy 必须至少绑定在一个底层的物理接口上，否则视为“协议孤岛”脏数据
    sh:property [
        sh:path net:runsOnInterface ;
        sh:minCount 1 ;
    ] .
```

### 2.2 在 Go 适配流水线中集成 n10s-SHACL 动态验证
```go
package twin

import (
	"context"
	"fmt"
	"[github.com/neo4j/neo4j-go-driver/v5/neo4j](https://github.com/neo4j/neo4j-go-driver/v5/neo4j)"
)

type DataQualityValidator struct {
	driver neo4j.DriverWithContext
}

// ValidateGraphQuality 在智能体仿真或割接审批前触发强校验
func (v *DataQualityValidator) ValidateGraphQuality(ctx context.Context) (bool, error) {
	session := v.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// 调用 n10s 内置的验证中间件进行图谱扫描
	query := `
	CALL n10s.validation.shacl.validate() 
	YIELD valid, txCount, errorLog
	RETURN valid, errorLog;
	`
	result, err := session.Run(ctx, query, nil)
	if err != nil {
		return false, err
	}

	if result.Next(ctx) {
		record := result.Record()
		isValid, _ := record.Get("valid")
		errorLog, _ := record.Get("errorLog")

		if !isValid.(bool) {
			fmt.Printf("❌ [数据质量告警] 数字孪生底座未能通过 SHACL 契约校验！阻断变更。错误详情: %v\n", errorLog)
			return false, nil
		}
	}
	return true, nil
}
```

## 3. 元数据驱动架构（面向新业务实体的 0 代码变更设计）
### 3.1 核心解耦思路

现网生产实际中，网络控制器和中台不可能原生提供 .ttl 文本，通常提供的是普通的 HTTP JSON 或 RESTCONF 数据。

如果将适配器（静态轨与动态轨）中的字段和业务类型写死，那么每当引入全新的业务实体（如 Flex-Algo），适配器系统就必须面临重新修改结构体、重新编译上线的窘境。

为了实现“代码 0 修改，只打配置补丁”的高扩展性目标，系统彻底抛弃了硬编码硬映射模型，全面升级为元数据驱动架构（Metadata-driven Architecture）：

- 所有的 JSON 数据全部打散降维为全通用的 map[string]interface{}。

- 将映射逻辑外置到规则配置文件 mappings.yaml。

外置映射规则文件补丁 (mappings.yaml)

```yaml
# 现有的 Interface 映射规则不变...
- target_class: "net:Interface"
  json_source: "controller_srv6_api"
  uri_template: "net:{device_ip}_{if_name}"
  properties:
    - predicate: "net:status"
      json_key: "if_status"
      datatype: "string"

# 【动态追加的补丁】全新引入的 Flex-Algo 业务实体映射规则
- target_class: "net:FlexAlgo_Slice"
  json_source: "controller_flexalgo_api" # 新增的控制器API源
  uri_template: "net:Algo_{algo_id}"     # 动态拼接URI：net:Algo_128
  properties:
    - predicate: "net:algo_id"
      json_key: "algo_id"
      datatype: "int"
    - predicate: "net:metric_type"
      json_key: "metric_type" # 例如: "Delay" 或 "IGP"
      datatype: "string"
  relationships:
    - predicate: "net:bindsToInterface"
      target_uri_template: "net:{device_ip}_{bound_if}" # 动态跨层织网
```

- 业务演进时，只需向配置中心推送几行 YAML 补丁，整个流处理与图数据库管道即可原地完成动态升级。

### 3.2 静态轨（冷数据）：元数据驱动的通用转化引擎

静态轨负责并发抓取中台与控制器的异构 JSON，根据 YAML 补丁规则，在 Go 内存中流式将其组装成 Turtle。

```go
package main

import (
	"bytes"
	"fmt"
	"log"
	"strings"
)

type PropertyRule struct {
	Predicate string `yaml:"predicate"`
	JsonKey   string `yaml:"json_key"`
	DataType  string `yaml:"datatype"`
}

type RelationshipRule struct {
	Predicate         string `yaml:"predicate"`
	TargetURITemplate string `yaml:"target_uri_template"`
}

// MappingRule 元数据驱动的核心映射规则契约
type MappingRule struct {
	TargetClass   string             `yaml:"target_class"`
	URITemplate   string             `yaml:"uri_template"`
	Properties    []PropertyRule     `yaml:"properties"`
	Relationships []RelationshipRule `yaml:"relationships"`
}

type UniversalMappingEngine struct {
	Rules []MappingRule
}

// ExecuteMapping 将任意未知领域的 map 结构数据动态翻译为标准 Turtle
func (e *UniversalMappingEngine) ExecuteMapping(rawData map[string]interface{}) string {
	var buf bytes.Buffer

	// 0. 输出 W3C 标准命名空间前缀声明（n10s 解析必须）
	buf.WriteString("@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n")
	buf.WriteString("@prefix net: <http://srv6-twin.org/ontology/net#> .\n\n")

	for _, rule := range e.Rules {
		// 1. 动态渲染当前实体的唯一标识 URI
		currentURI := rule.URITemplate
		for k, v := range rawData {
			placeholder := fmt.Sprintf("{%s}", k)
			currentURI = strings.ReplaceAll(strings.ReplaceAll(currentURI, placeholder, fmt.Sprintf("%v", v)), "/", "_")
		}

		// 2. 生成类型断言 (rdf:type)
		buf.WriteString(fmt.Sprintf("%s rdf:type %s .\n", currentURI, rule.TargetClass))

		// 3. 动态属性抽取与三元组映射
		for _, prop := range rule.Properties {
			if val, exists := rawData[prop.JsonKey]; exists {
				if prop.DataType == "string" {
					buf.WriteString(fmt.Sprintf("%s %s \"%v\" .\n", currentURI, prop.Predicate, val))
				} else {
					buf.WriteString(fmt.Sprintf("%s %s %v .\n", currentURI, prop.Predicate, val))
				}
			}
		}

		// 4. 动态多层跨层织网（Object Properties）
		for _, rel := range rule.Relationships {
			targetURI := rel.TargetURITemplate
			for k, v := range rawData {
				placeholder := fmt.Sprintf("{%s}", k)
				targetURI = strings.ReplaceAll(strings.ReplaceAll(targetURI, placeholder, fmt.Sprintf("%v", v)), "/", "_")
			}
			buf.WriteString(fmt.Sprintf("%s %s %s .\n", currentURI, rel.Predicate, targetURI))
		}
	}
	return buf.String()
}

func main() {
	// 【动态配置补丁】现网突发引入了 Flex-Algo 全新业务实体，这是热加载进来的规则补丁，0代码修改
	flexAlgoPatchRule := MappingRule{
		TargetClass: "net:FlexAlgo_Slice",
		URITemplate: "net:Algo_{algo_id}",
		Properties: []PropertyRule{
			{Predicate: "net:algo_id", JsonKey: "algo_id", DataType: "int"},
			{Predicate: "net:metric_type", JsonKey: "metric_type", DataType: "string"},
		},
		Relationships: []RelationshipRule{
			{Predicate: "net:bindsToInterface", TargetURITemplate: "net:{device_ip}_{bound_if}"},
		},
	}

	engine := &UniversalMappingEngine{Rules: []MappingRule{flexAlgoPatchRule}}

	// 模拟全新控制器接口吐出的 JSON 流 (Go 适配器中完全没有其对应的结构体)
	newFlexAlgoJSON := map[string]interface{}{
		"algo_id":     128,
		"metric_type": "Min_Delay",
		"device_ip":   "10.1.1.5",
		"bound_if":    "GigabitEthernet1/0",
	}

	ttlOutput := engine.ExecuteMapping(newFlexAlgoJSON)
	log.Println("🎉 [元数据引擎] 静态轨0修改，仅通过规则补丁成功完成全新网络实体转换：")
	fmt.Println(ttlOutput)
}
```

### 3.3 动态轨（热数据）：元数据驱动的增量属性合并引擎

动态轨同样拒绝写死字段的 Cypher。利用 APOC 的动态节点合并过程与 Cypher 的 += 增量属性覆盖特性，构建全通用的高频更新引擎。

```go
package twin

import (
	"context"
	"[github.com/neo4j/neo4j-go-driver/v5/neo4j](https://github.com/neo4j/neo4j-go-driver/v5/neo4j)"
	"time"
)

// UniversalEventPayload 全通用松散耦合动态数据包
type UniversalEventPayload struct {
	TargetLabel  string                 `json:"target_label"` // 新增实体的Label
	TargetURI    string                 `json:"target_uri"`   
	DynamicProps map[string]interface{} `json:"props"`        // 动态增量指标键值对
}

type UniversalDynamicTrack struct {
	driver neo4j.DriverWithContext
}

// UpdateTwinEntityDynamically 通用动态流原子写入函数
func (t *UniversalDynamicTrack) UpdateTwinEntityDynamically(ctx context.Context, event UniversalEventPayload) error {
	session := t.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// 核心设计：动态打标签 + Map增量揉入合并。彻底与业务网络字段解耦
	dynamicCypher := `
	CALL apoc.merge.node($label, {uri: $uri}) 
	YIELD node
	SET node += $properties, node.last_updated_ts = $ts
	RETURN node.uri;
	`

	_, err := session.Run(ctx, dynamicCypher, map[string]interface{}{
		"label":      []string{event.TargetLabel},
		"uri":        event.TargetURI,
		"properties": event.DynamicProps, // 动态属性图补丁
		"ts":         time.Now().Format(time.RFC3339),
	})
	
	return err
}
```
## 总结：真正的生产级全闭环方案

通过将静态轨的代码演进为“元数据驱动架构”，你关心的可扩展性问题彻底得到解决：

1. 静态轨（冷数据）： 只要修改外置的 YAML 映射规则，Go 微服务无需重启、无需重新编译，来了解析新实体的能力，输出标准的 .ttl 字符串。

2. 图底座（Neo4j + n10s）： 因为 n10s 解析 .ttl 时也是根据文本动态生成的，所以当包含 net:FlexAlgo_Slice 的文本投喂进去时，Neo4j 会在底层自动创建这个全新的 Label，完全不需要提前去改图数据库的 Schema。

3. 大模型层（OpenCode）： 新实体上线后，只需要在 Agent 的“专家知识库”里增加一行对新实体的 Prompt 描述。大模型就能立马理解这个新概念，并在 RCA 诊断时自动生成包含 MATCH (f:FlexAlgo_Slice) 的高阶 Cypher。

这就实现了从 网络控制器数据源 -> Go 中间件 -> Neo4j 图孪生体 -> OpenCode 智能体 的全链路免编译、动态热插拔灰度升级。

---

## 6. MVP 阶段测试验收基线

为平衡 MVP 阶段"快速验证"与"生产可演进"的双重目标，测试策略采用"**需求阶段定义基线、实现阶段补充细节**"的分层策略。

### 6.1 必须在需求阶段定义的测试基线

| 测试类型 | 范围 | 验收标准 |
|----------|------|----------|
| **本体一致性测试** | SRv6 三层本体的 SHACL 形状覆盖度 | 所有核心类（Device/Interface/SRv6_Policy/EVPN_Instance/Network_Slice/Alarm）均需有对应的 NodeShape |
| **SHACL 校验自身的单元测试** | 反向测试集 | 故意注入脏数据（带宽为负数、Policy 孤立无接口绑定、Interface 缺失状态字段），验证约束 100% 拦截 |
| **核心 Cypher 查询基线** | 影响分析/根因追溯/路径查询 | 至少 3 个核心查询用例，每个用例需明确输入图状态、预期输出节点集合 |
| **TTL 快照正反一致性** | 导出 → 导入循环 | 同源 TTL 经过 n10s 导出再导入后，节点数、关系数、关键属性值完全一致 |

### 6.2 可在实现阶段补充的测试细节

| 测试类型 | 说明 |
|----------|------|
| Go 适配器单元测试 | mappings.yaml 各规则的转换正确性 |
| 端到端集成测试 | Neo4j + n10s + Go 中间件 + Kafka 全链路 |
| 性能压测 | 万级网元 5s 重构、100ms 端到端延迟等指标验证 |
| 故障注入测试 | Neo4j 宕机、Kafka 积压、网络分区等异常场景降级 |
| 大模型 Cypher 生成准确率评测 | OpenCode 在测试集上的 Cypher 生成正确率（目标 > 95%） |

### 6.3 MVP 阶段必交付的测试资产

- [ ] `srv6_shacl_shapes.ttl` + 反向测试用 TTL 样本集
- [ ] 3 个核心 Cypher 查询的 Golden Result（标准答案）JSON
- [ ] 一个端到端冒烟测试脚本：导入 13 节点示例 → 执行 3 个查询 → 验证结果

这就实现了从 网络控制器数据源 -> Go 中间件 -> Neo4j 图孪生体 -> OpenCode 智能体 的全链路免编译、动态热插拔灰度升级。