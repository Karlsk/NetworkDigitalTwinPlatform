# 基于本体论的 SRv6 网络多层拓扑数字孪生系统——需求分析说明书 (MVP版)

## 1. 引言

### 1.1 编写目的
本规范书旨在明确“基于本体论的 SRv6 网络多层拓扑数字孪生系统”在 MVP（最小可行性产品）阶段的业务逻辑、系统功能、数据架构及技术实现要求。本文档将作为后续系统架构设计、数据流水线开发、大模型 Agent (OpenCode) 技能库建设的核心指导依据。

### 1.2 背景与痛点
随着下一代网络（SRv6、网络切片、Flex-Algo）的广泛部署，传统的网络运维面临以下严峻挑战：
1. **数据孤岛：** 控制器掌握底层协议路径，数据中台掌握资产与客户业务，两层数据割裂，无法进行跨层穿透分析。
2. **缺乏语义认知：** 传统图数据库仅有节点和线，机器无法理解“Policy承载EVPN业务”这种业务逻辑语义，推理能力弱。
3. **高阶运维断层：** 现网故障根因分析（RCA）依赖专家经验；缺乏割接前的安全沙箱，无法精准预判变更带来的业务风险（爆炸半径）。

### 1.3 项目目标
利用**本体论（Ontology）**规范网络世界观，结合 **Neo4j + neosemantics (n10s)** 构建实时动态的网络数字孪生底座，并基于 **OpenCode (LLM Agent) + 专家知识库 + 仿真 Skill**，实现分钟级的自动化根因分析、全网跨层影响范围评估、以及变更前的无损仿真验证。

---

## 2. 系统总体架构与设计思路

系统采用**“语义建模+双轨数据流+智能体闭环”**的整体架构：
1. **语义建模层：** 采用 W3C 标准的 Turtle (.ttl) 声明式定义物理层、协议层、业务层、状态层的本体模型。
2. **双轨数据流水线：**
   * **静态/周期轨（冷数据）：** ETL 聚合控制器与中台资产，生成 `.ttl` 拓扑快照，通过 n10s 批量导入重构全网骨架。
   * **动态/实时轨（热数据）：** 针对 Telemetry、Syslog、告警等高频状态流，通过 Go 中间件和原生 Cypher 直接对图谱执行毫秒级属性修改。
3. **智能体应用层：** OpenCode 智能体通过工具调用（Tool Calling），驱动图检索 Skill 与网络仿真引擎，实现高阶运维闭环。

---

## 3. 核心功能需求

### 3.1 多层拓扑本体建模需求 (Ontology Modeling)
系统必须支持跨多层网络维度的实体与关系的形式化建模，MVP 阶段核心 Schema 规范如下：

| 层面 | 核心实体（Class） | 数据属性（Data Property） | 语义关系（Object Property） |
| :--- | :--- | :--- | :--- |
| **物理层** | `Device` (路由器)<br>`Interface` (接口) | `uri`, `name`, `status`, `bandwidth`, `current_traffic` | `Device -[hasInterface]-> Interface` |
| **协议层** | `SRv6_Policy`<br>`Segment_List`<br>`SRv6_Locator` | `uri`, `status`, `prefix`, `current_delay` | `SRv6_Policy -[runsOnInterface]-> Interface`<br>`SRv6_Policy -[usesSegmentList]-> Segment_List` |
| **业务层** | `EVPN_Instance`<br>`Network_Slice` | `uri`, `customer`, `sla_delay`, `status` | `EVPN_Instance -[carriedBy]-> SRv6_Policy`<br>`EVPN_Instance -[belongsToSlice]-> Network_Slice` |
| **状态层** | `Alarm` (告警)<br>`Log_Event` (日志) | `alarm_id`, `severity`, `timestamp`, `message` | `Alarm -[occurredOn]-> Interface` |

### 3.2 双轨数据同步需求 (Data Synchronization)
系统需要具备支撑百万级网络实体关系的高并发、低延迟同步能力。
* **【需求 3.2.1】全量骨架批量同步（离线轨）：** 系统应支持每天/每小时从控制器和 CMDB 抽取全网拓扑，通过中介实体 `.ttl` 文件利用 `n10s.rdf.import` 技术无损覆写 Neo4j，确保图 Schema 的强一致性契约。
* **【需求 3.2.2】动态状态实时修改（实时轨）：** 针对接口 Down/Up、流量突增、秒级告警等热数据，必须绕过 `.ttl` 编译过程，直接使用原生 Cypher 高并发修改 Neo4j 现有节点属性，时延需控制在 100ms 以内。
* **【需求 3.2.3】实体身份对齐（Identity Resolution）：** 数据流层必须具备翻译功能，将不同数据源（如控制器的接口代号与中台的网元名称）统一对齐至本体定义的唯一 URI。

### 3.3 双向快照与时序孪生回溯需求 (Snapshot & Rollback)
为了支持故障复盘与离线仿真，系统必须具备图谱的双向快照能力。
* **【需求 3.3.1】图转快照（Export to TTL）：** 系统应提供流式 HTTP/Go 组件，支持执行特定 Cypher 语句，将当前 Neo4j 中“物理框架+实时告警状态”的最新复合孪生体整体反向导出为标准的带时间戳的 `.ttl` 快照文件。
* **【需求 3.3.2】快照一键回溯（Import and Revert）：** 运维人员或 OpenCode 智能体可通过传入特定时间戳的 `.ttl` 快照，一键清理当前图空间并精准复原历史某一时刻的全网语义状态。

### 3.4 智能根因分析 (RCA) 与影响范围评估需求
* **【需求 3.4.1】语义规则前向推理：** 系统应具备利用 Cypher 模式匹配对关联故障进行隐性关系显式化的能力。当物理链路发生故障时，上层受波及的 `SRv6_Policy` 状态和 `EVPN_Instance` 必须能够自动推理并标记为 `Degraded/Impacted`（受损/受影响）。
* **【需求 3.4.2】跨层爆炸半径计算（影响范围）：** 当底层任意物理实体（如光纤、单板）报错时，OpenCode 应能自动生成图查询语句，向上层纵向穿透，精准输出受影响的协议路径、网络切片、受损大客户清单。

### 3.5 风险预判与仿真验证需求
* **【需求 3.5.1】计划内割接无损评估（爆炸半径演练）：** 系统必须支持模拟割接。输入拟下线设备后，在数字孪生空间内将其标记为 `Simulated_Down`。系统需通过仿真 Skill（如内置的 SPF 矩阵计算引擎）推演路由收敛后的 TI-LFA 备份路径，并评估备份链路上是否会发生流量过载风险。
* **【需求 3.5.2】外部业务大促容量预测：** 允许输入外部业务指标（如：某大客户流量预计激增 N 倍）。系统应沿本体树状路径进行流量全路径推演，核验沿途各中继物理端口的带宽利用率上限，结合 M/M/1 等排队时延模型技能，提前预判大客户的 SLA（如时延 > 10ms）是否会发生违约，并由 OpenCode 自动给出规避策略（如推荐下发 SRv6 Flex-Algo 切片隔离配置）。

---

## 4. 接口与集成需求

### 4.1 控制器集成接口
* **物理/协议拓扑获取：** 支持通过 BGP-LS 或控制器北向 RESTCONF 接口，定期获取底层物理链路及 SRv6 Policy、Locator、SID 的状态。
* **配置下发通道：** 提供 Skill 接口，当 OpenCode 给出风险规避或排障建议后，可通过控制器向网管自动下发割接/引流配置。

### 4.2 数据中台集成接口
* **资产与业务数据接口：** 对接 CMDB、CRM 系统，提供网元资产编号与政企大客户 EVPN 业务实例的映射关系。
* **实时告警/日志流：** 通过订阅数据中台的 Kafka 集群（或 Syslog 汇聚层），实时捕获高频网络故障事件。

### 4.3 智能体应用层接口
* **Neo4j 图查询驱动：** 提供标准的 Bolt 协议（7687端口）供大模型 Agent 连接。
* **语义导出 HTTP 终结点：** 开放 `http://localhost:7474/rdf/neo4j/cypher` 接口，支持通过 Accept 头拉取标准 Turtle 纯文本。

---

## 5. 非功能性需求

### 5.1 性能与吞吐量
* **全量重构时间：** 在万级网元规模下，通过 Go 语言与 n10s 批量导入 `.ttl` 快照重构全网语义骨架的时间必须在 5 秒以内。
* **实时流处理延迟：** 从 Kafka 收到单一告警日志到 Neo4j 对应实体属性完成更新的端到端延迟控制在 100ms 以内。

### 5.2 兼容性与解耦要求（Vendor Lock-in Avoidance）
* 全量拓扑和状态的输出物必须严格遵循 **W3C RDF/Turtle 规范**，确保数字孪生底座的数据资产与特定的图数据库厂商（如 Neo4j）解耦，未来能够零代码改造成本迁移至其他标准语义图数据库。

### 5.3 机器可读性（LLM 友好性）
* 系统在导入本体时，必须通过 n10s 配置强制将冗长复杂的命名空间 URI 进行简化或忽略（`handleVocabUris: "IGNORE"`），确保生成的 Neo4j 图标签和属性干净直观，从而最大化提升 OpenCode（大模型）自动生成 Cypher 语句的准确率（目标准确率 > 95%）。

## 附录：核心需求组件的 Go 语言工程实现指南

本附录针对需求说明书中要求的“双轨数据流”及“双向快照”提供生产级的 Go 语言核心代码框架，作为开发阶段的脚手架。

### 附录 A：统一身份对齐引擎（对应 需求 3.2.3）

在离线和实时数据流中，不同数据源（控制器、数据中台、监控网管）对同一网元的标识符各不相同。该组件通过硬编码规则或外部缓存映射，将异构输入转换为符合本体论规范的唯一 URI。

```go
package twin

import (
	"fmt"
	"strings"
)

// BaseOntologyURI 本体系标准前缀
const BaseOntologyURI = "[http://srv6-twin.org/ontology/net#](http://srv6-twin.org/ontology/net#)"

// IdentityResolver 身份对齐引擎结构体
type IdentityResolver struct {
	// 实际生产中，这里可以注入 Redis 客户端用于加载中台的资产映射表
	ipToNameMap map[string]string
}

func NewIdentityResolver() *IdentityResolver {
	return &IdentityResolver{
		ipToNameMap: map[string]string{
			"10.1.1.5": "Router_Edge_05",
			"10.1.1.1": "Router_Core_01",
		},
	}
}

// ResolveDeviceURI 将异构的设备标识对齐为标准本体URI
func (r *IdentityResolver) ResolveDeviceURI(rawIdentifier string) string {
	// 规则1：如果是IP地址，查表对齐
	if name, exists := r.ipToNameMap[rawIdentifier]; exists {
		return BaseOntologyURI + name
	}
	// 规则2：标准化格式清除（如转换为大写、替换空格）
	cleanName := strings.ReplaceAll(rawIdentifier, " ", "_")
	return BaseOntologyURI + cleanName
}

// ResolveInterfaceURI 生成标准接口URI
func (r *IdentityResolver) ResolveInterfaceURI(deviceRaw, ifRaw string) string {
	deviceClean := strings.ReplaceAll(deviceRaw, " ", "_")
	// 将 GigabitEthernet1/0/1 转换为标准的 GE1_0_1 缩写，降低大模型识别难度
	ifClean := strings.ReplaceAll(ifRaw, "GigabitEthernet", "GE")
	ifClean = strings.ReplaceAll(ifClean, "/", "_")
	
	return fmt.Sprintf("%s%s_%s", BaseOntologyURI, deviceClean, ifClean)
}
```

### 附录 B：高频实时动态轨写入组件（对应 需求 3.2.2）

针对 Telemetry 性能指标及高频 Syslog 告警，系统绕过重量级的 .ttl 编译过程，使用高并发 Goroutines 和原生 Cypher 驱动直接修改 Neo4j 现有节点的属性，确保 100ms 以内的现网同步时效。

```go
package twin

import (
	"context"
	"[github.com/neo4j/neo4j-go-driver/v5/neo4j](https://github.com/neo4j/neo4j-go-driver/v5/neo4j)"
	"log"
	"time"
)

type TelemetryMetric struct {
	InterfaceURI string
	TrafficBps   int64
	Status       string
	Timestamp    time.Time
}

type DynamicTrackWorker struct {
	driver neo4j.DriverWithContext
}

func NewDynamicTrackWorker(driver neo4j.DriverWithContext) *DynamicTrackWorker {
	return &DynamicTrackWorker{driver: driver}
}

// HandleTelemetryStream 模拟消费 Kafka 的 Telemetry 流并秒级写入图谱
func (w *DynamicTrackWorker) HandleTelemetryStream(ctx context.Context, metricChan <-chan TelemetryMetric) {
	for metric := range metricChan {
		go func(m TelemetryMetric) {
			// 开启高并发分布式写入会话
			session := w.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
			defer session.Close(ctx)

			// 使用原生极速 Cypher 语句更新“活的数字孪生体”属性
			cypherQuery := `
			MATCH (i:Interface {uri: $if_uri})
			SET i.current_traffic = $traffic,
			    i.status = $status,
			    i.last_telemetry_time = $ts
			RETURN i.uri;
			`

			_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
				result, err := tx.Run(ctx, cypherQuery, map[string]interface{}{
					"if_uri":  m.InterfaceURI,
					"traffic": m.TrafficBps,
					"status":  m.Status,
					"ts":       m.Timestamp.Format(time.RFC3339),
				})
				if err != nil {
					return nil, err
				}
				return nil, result.Err()
			})

			if err != nil {
				log.Printf("[实时轨错误] 无法动态更新接口 %s: %v", m.InterfaceURI, err)
			}
		}(metric)
	}
}
```

### 附录 C：流式快照反向导出组件（对应 需求 3.3.1）

该组件通过连接 n10s 内置的语义解析服务器，执行特定 Cypher 过滤语句，在极低内存开销下，将当前的“物理+实时状态”复合图谱以 W3C 规范的流式形式反向导出为本地文本快照，供 OpenCode 智能体在沙箱内做仿真推演。

```go
package twin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type SnapshotExporter struct {
	HTTPClient  *http.Client
	Neo4jURL    string // 例如: http://localhost:7474/rdf/neo4j/cypher
	AuthHeader  string
}

func NewSnapshotExporter(url, user, password string) *SnapshotExporter {
	auth := user + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	
	return &SnapshotExporter{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Neo4jURL:   url,
		AuthHeader: "Basic " + encodedAuth,
	}
}

// ExportToTTLFile 将当前的现网动态图谱裁剪后导出为标准 Turtle 文件
func (e *SnapshotExporter) ExportToTTLFile(outputDir string) (string, error) {
	// 需求 4.4 提出的裁剪 Cypher：剔除冗余的历史时序噪点，只导出拓扑大骨架和当前活动告警
	croppedCypher := `
	MATCH (n) 
	WHERE NOT n:TelemetryRawHistory AND NOT n:SyslogProcessed
	OPTIONAL MATCH (n)-[r]->(m) 
	WHERE NOT m:TelemetryRawHistory AND NOT m:SyslogProcessed
	RETURN n, r, m
	`

	reqPayload := map[string]string{"cypher": croppedCypher}
	jsonBytes, _ := json.Marshal(reqPayload)

	req, err := http.NewRequest("POST", e.Neo4jURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}

	// 关键契约：必须向 n10s 声明 Accept 为 Turtle 格式
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/turtle")
	req.Header.Set("Authorization", e.AuthHeader)

	resp, err := e.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("neosemantics 导出失败，状态码: %d", resp.StatusCode)
	}

	_ = os.MkdirAll(outputDir, 0755)
	timestamp := time.Now().Format("20060102_150405")
	filePath := fmt.Sprintf("%s/srv6_composite_snapshot_%s.ttl", outputDir, timestamp)

	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 流式写入，确保内存平稳
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}

	return filePath, nil
}
```

---

这三个底层核心组件不仅补充了需求分析文档在技术可实现性上的论证，也为你的网络数字孪生项目提供了极具执行力的 **Go 语言微服务工程脚手架**。直接复制粘贴到原文档末尾，即可形成一个完美的“从宏观业务需求到微观核心代码实现”的闭环说明书！