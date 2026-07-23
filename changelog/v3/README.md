# V3 研发计划 — 拓扑时态历史与回放能力

> 基于 V2 基础设施（Kafka + PostgreSQL + Gin + 可观测性）就绪，V3 聚焦 **拓扑时态历史**：
> 解决 V2 遗留的核心架构缺陷——增量同步 `MERGE` 是破坏性更新，销毁了事件级历史，
> 导致「拓扑回放」「任意时刻重建」「RCA 根因分析」无法实现。
>
> 本计划引入 **事件溯源（Event Sourcing）+ 物化投影** 模式：
> 1. PG 事件表作为权威历史源（append-only WAL）
> 2. Neo4j `default` 降级为物化投影（热查询）
> 3. 每次 FullSync 后自动建快照作为 checkpoint
> 4. 事件分区与快照 TTL 联动压实，PG 在线数据量有界
>
> 该能力是 RCA / Simulation / 时态 Diff 引擎落地的 **前置依赖**（CLAUDE.md 痛点 3）。

**总任务数**: 10 项 (V3-01 ~ V3-10)
**预估工时**: 18.5 人天（基础估算，不含缓冲；加 20% 缓冲后约 23 天）
**关联文档**: [V2 研发计划](../v2/README.md) · [项目规范](../../CLAUDE.md)

---

## 1. 背景与问题

### 1.1 当前架构的致命缺陷

V2 的增量同步直接对 `default` 逻辑 DB 执行 `MERGE ... SET x += n.props`（调用点 [`sync_service.go:221`](../../internal/service/sync_service.go:221)，Cypher 生成 [`neo4j.go:329`](../../internal/graph/neo4j.go:329)），是**破坏性更新**：

```
T1: FullSync        → default = {A, B, C}
T2: 增量 update D   → default = {A, B, C, D}
T3: 增量 update B'  → default = {A, B', C, D}   ← B 的旧值永久丢失
T4: Create("snap")  → 只能拿到 T4 当前态，T2/T3 的中间过程无法重建
```

两次全量之间的细粒度变更在 MERGE 后丢失。（V2 的 FullSync 仅手动触发；V3-10 引入周期调度器后约定 **FullSync 周期 ≤ 1 小时/次**，作为容量与回放窗口的前提——见 §6.2、§4.4。）

### 1.2 现有持久化无法补救

| 存储 | 记录内容 | 能否做事件级回放 |
|------|---------|----------------|
| Neo4j `default` | 当前态 | ❌ 只有最新值 |
| Snapshot YAML | 某时刻状态镜像 | ❌ 快照粒度，非事件粒度 |
| AuditLog | 快照生命周期（create/restore/delete） | ❌ 不记数据变更 |
| SyncLog | 同步运行元数据（type/status/计数） | ❌ 不记单条事件 |
| Kafka topic | 事件流（仅 Kafka 模式） | ⚠️ 天然 WAL，但当前无回放消费路径 |
| Channel | 内存事件 | ❌ 消费后丢弃，零持久化 |

**结论**：当前系统做不了事件级回放，RCA 引擎（V3 预留）无数据基础。

---

## 2. 关键决策

经多方案讨论（见 §3 备选方案否决），确定以下 4 项核心决策：

### 决策 1：不用「双 default」，用「default + 自动快照」

**否决** `default1`（存最新）+ `default2`（存全量基础快照）的双活 DB 方案。理由：
- 与现有 Snapshot 机制重复（快照本就是"已知良好基线"）
- 双活 DB 引入双写一致性负担
- `default2` 要么是死数据（不如 YAML），要么要持续同步（退回双写）

**采用**：保持单一 `default`（live），**每次 FullSync（V3-10 调度器或手动调用）后自动 `Create` 快照**作为 checkpoint。快照已有的 YAML 归档 + 逻辑 DB 懒加载 + LRU 保活机制直接复用。

### 决策 2：当前态放 Neo4j，事件历史放 PG

明确两类数据的存储定位：

| 数据类 | 存储 | 角色 | 读取模式 |
|--------|------|------|---------|
| **当前态** | Neo4j `default` | 物化投影（加速读） | 热查询：MCP query_topology / 分析引擎 / 时态 Diff |
| **事件历史** | PG `topology_events` 分区表 | 权威源（source of truth） | 时间范围扫描、按 uri 查变更链（RCA） |
| **状态 checkpoint** | Snapshot YAML + 逻辑 DB | 回放起点 | 一次性加载 |

> **否决 Redis 作为事件存储**：Redis 按 node 存最新态 = 与 `default` 重复，不增加时序能力；存事件则非其所长（内存成本高、范围查询弱、持久化是附加项）。Redis 仅在后续需要「热拓扑查询缓存」时作为可选只读缓存引入，非本计划核心。

### 决策 3：Snapshot 与 Event 分离（checkpoint + WAL 模式）

**否决** 将事件塞进 snapshot YAML（语义混乱、快照膨胀）。

**采用** 数据库经典的 checkpoint + WAL 模式：
- Snapshot = 状态 checkpoint（现状不变）
- PG 事件表 = checkpoint 之间的 WAL
- 关联方式：快照指针 + 事件时间窗，不物理混合

```
回放时刻 T = 最近快照(T0) + 重放 (T0, T] 区间事件
```

### 决策 4：事件分区与快照 TTL 联动压实

事件表按时间分区，保留窗口与快照 TTL 对齐。**压实规则**：早于「最旧保留快照」的事件是冗余的（无法被任何回放用到），可安全淘汰（前提见 §6.3）。

- 每次 FullSync 后自动建快照
- 旧快照按 `retention_days` 淘汰时，**连带淘汰其之前的事件分区**
- 结果：**PG 在线数据量上限 = 保留窗口内的事件量**（有界）

---

## 3. 备选方案否决记录

| 方案 | 否决理由 |
|------|---------|
| 双 default（default1 最新 + default2 基线） | 与 Snapshot 重复 + 双写一致性负担（决策 1） |
| Redis 按 node 存最新态 | 与 Neo4j default 重复，无时序能力（决策 2） |
| Snapshot YAML 内嵌事件 | 语义混乱、快照膨胀（决策 3） |
| 单 default + event node 关联变化 node（图原生事件溯源） | 取最新拓扑需折叠全事件历史，O(事件数)，热查询性能风险高。仅可作为「近期事件图内辅助索引」用于 RCA 窗口查询，不能作主存储 |
| 纯 Kafka topic 作唯一事件源 | 范围/聚合查询弱，Channel 模式下不可用；需统一抽象。Kafka 退为传输层，PG 为权威源 |

---

## 4. 目标架构

### 4.1 三件套数据模型

```
        ┌──────────────────────────────────────────────┐
        │  PG topology_events（append-only，权威源）    │
        │  · 按天分区，滑动保留                          │
        │  · 所有来源（webhook/kafka）统一落盘；channel 仅作 bus 传输模式   │
        └───────────────┬──────────────────────────────┘
                        │ ① Ingest 同事务写 events + outbox（不直接 Publish）
                        │ ② relay 唯一投递方，轮询 outbox → bus
                        ▼
        ┌──────────────────────────────────────────────┐
        │  Neo4j default（物化投影，非权威）            │
        │  · consumer 消费 bus → Upsert/Delete（MERGE +=）│
        │  · 应用成功后回写 topology_events.applied=true │
        │  · 热查询 / MCP / 分析引擎读这里               │
        │  · 可从 PG 事件完整重建（自愈）                │
        └───────────────┬──────────────────────────────┘
                        │ ③ FullSync 后 hook 建 checkpoint
                        ▼
        ┌──────────────────────────────────────────────┐
        │  Snapshot YAML + 逻辑 DB（状态 checkpoint）   │
        │  · 每次 FullSync（调度器/手动）后自动 Create   │
        │  · 按 TTL 淘汰，连带淘汰其之前事件分区         │
        └──────────────────────────────────────────────┘
```

### 4.2 统一接入与完整增量数据流（EventSink + Outbox）

**核心原则：PG 先写（权威 + outbox，一个事务），再异步经 bus 投影到 default。bus 不承担持久化，只是投影投递通道。**

两条入口（webhook / 外部 kafka）都收敛到 `Ingest`，**绝不直接 `publisher.Publish`**（否则绕过权威 PG，回到双写困境）。

```
① 外部推送变更（两种入口）
   ├─ webhook:        POST /api/v1/sync/webhook → SyncService.Ingest(raw, "webhook")
   └─ 外部 kafka:     KafkaDataSourceConsumer 消费外部 topic → SyncService.Ingest(raw, "kafka")

② Ingest 归一化 + 扇出（1 条 SyncEvent → 1..N 行 TopologyEvent，按 uri 拆；见下文 payload schema）

③ PG 事务（权威，先做、原子）────────────────────────────────┐
   BEGIN;                                                     │ 成功才返回
     INSERT INTO topology_events (...) VALUES (...), ...;      │  webhook → 202
     INSERT INTO event_outbox      (event_id) VALUES (:id,...); │  kafka  → commit offset
   COMMIT;                                                    │

④ relay 协程（异步轮询 outbox，唯一投递方）────────────────────┘
   SELECT 未投递 outbox 批量 → 对每条 TopologyEventRow 单独 envelope 投递 bus
   → UPDATE outbox SET dispatched=true
   （投递失败不标 dispatched，下次重试；重复投递靠下游 MERGE 幂等兜底）

⑤ consumer 消费 bus（异步，StartConsumer；消费侧单 goroutine 串行化）
   → 攒一批 N 条 envelope（时间窗/条数触发，保 §4.5 同 uri 到达序）→ 一次 graph 批量 Upsert/DeleteByURIs/DeleteRelations（UNWIND；assembler.Node.Labels 复用 §4.4 entity_type→GetLabels 派生）
   → graph 写成功后 MarkApplied([N event_ids]) 翻转 applied=true（非原子跨库，靠 MERGE 幂等+对账兜底）
```

**EventSink 接口与 Ingest 骨架**（`Ingest` 仅写 PG+outbox，**不直接 Publish**）：

```go
type EventSink interface {
    // Persist 在一个 PG 事务内写入 topology_events + event_outbox；跨表事务封装见下方 WithTx
    Persist(ctx context.Context, rows []TopologyEvent) error
}

// 跨表事务封装（V3-04 交付；repository 包内）
type TxFunc func(ctx context.Context, tx pgx.Tx) error
func WithTx(ctx context.Context, fn TxFunc) error  // pgxpool.BeginTx + defer Rollback

func (s *SyncService) Ingest(ctx context.Context, raw events.SyncEvent, source string) error {
    rows := normalizeToTopologyEvent(raw, source)   // 扇出 1→N 行
    return s.sink.Persist(ctx, rows)                // 仅落 PG（权威）；投递由 relay 负责
}

// 投递粒度（V3-05 决定）：relay 对 outbox 中每一行 TopologyEvent 单独 envelope 投递 bus，
// envelope 必含单值 event_id（来自 outbox.event_id，下游 MarkApplied 用）+ version（滚动发布兼容，见 §4.5）。
// 同一 SyncEvent 扇出后产生 N 条 bus 消息（1 消息 = 1 event_id = 1 行）；EventSink.Persist 无需返回 id，
// relay 从 outbox.event_id 取值注入 envelope。
```

**扇出规则 `normalizeToTopologyEvent`**（保证每行 `uri` 非空，可被 `(uri,event_time)` RCA 索引命中）：

```
1 条 SyncEvent → 1..N 行 TopologyEvent：
  upsert          : Data 每条记录 → 1 行（uri = 归一化后记录的 uri）
  delete          : URIs 每个 → 1 行（批量删除拆分，确保每个被删节点在自己变更链可见）
  delete_relation : 按 From（与 To）关联 → 1 行（uri = From，payload 含 type/from/to）
  relations 归属  : upsert 行只携带 from=该行 uri 的关系（每条关系仅出现在其 from 端行内，避免 N×M 冗余；
                    对端 to-uri 不在本批时由 to 端后续 upsert 补齐，applyEvent 容忍悬空 to）
每行填：event_time（源时间戳优先）、source、connector、entity_type、action、payload(原始 JSONB)、kafka 三元组(若有)
```

**Payload JSONB schema**（V3-04 固化，与 `payload_version: 1` 配套；向后兼容由 `payload_version` 字段承诺）：

```jsonc
// upsert payload（每行一条）
{
  "data":       { ...原始 Resource props（已 fieldMapping）... },
  "relations":  [{"type":"HAS_INTERFACE","from":"<uri>","to":"<uri>","props":{...}?}, ...]  // 可空
}

// delete payload（uri 已在列里，payload 仅做兼容占位）
{}

// delete_relation payload
{"type": "HAS_INTERFACE", "from": "<uri>", "to": "<uri>"}
```

> ⚠ **不允许**：单条事件塞多个 uri 进 JSON 数组、或 `uri=NULL`——会令 RCA 索引漏命中（"节点为何消失"查不到）。
> ⚠ Channel 模式不提供 partition，消费侧必须单 goroutine 串行处理（与现有 `StartConsumer` 实现一致）。

### 4.3 一致性模型（Outbox 模式 + 投影水位）

**解决的问题**：双写困境——"写 PG + 发 bus"无法原子。先 PG 后 bus → bus 失败丢；先 bus 后 PG → PG 失败幻觉。

**方案**（Transactional Outbox）：
- `Ingest` 在**同一 PG 事务**内写 `topology_events` + `event_outbox` → 两行同生共死
- **relay 是唯一投递方**：异步轮询 outbox → `publisher.Publish(bus)` → 成功标 `dispatched=true`；失败重试
- 下游 `MERGE (_db,uri)` 幂等 → relay 重复投递安全

#### `dispatched` 语义（V3-05 强约束，与 `applied` 解耦）

relay 投 bus 成功即标 `dispatched=true`，**不待 consumer 应用结果**。consumer 失败时事件保持 `applied=false`，由对账 / `RebuildDefault` 兜底，**不重置 `dispatched=false`**。

理由：避免下游 consumer 临时故障导致 outbox 重投风暴、阻塞 relay。**幂等分两层，不可混淆**：
- **ingest 去重** = Partial Unique Index `(kafka_topic,kafka_partition,kafka_offset)`（建在 `topology_events`，防外部 Kafka 重复**入库**产生重复行）
- **delivery 去重** = `MERGE (_db,uri)`（relay 重投是 re-Publish 到 bus，**不**重新 INSERT topology_events，故该索引与此无关；重复投递污染 default 仅由 MERGE 兜底）

重复投递对 default 无副作用——但靠的是 MERGE，不是那个索引。把两者并列为"重复投递双层保证"会让 V3-05 实现者误以为索引覆盖投递去重，从而在 bus/consumer 侧放松 MERGE 或去重。

两个标志的关系（独立推进，PG 是唯一权威，default 落后可接受 → 最终一致）：

| 标志 | 写者 | 置位时机 | 失败时 |
|------|------|---------|--------|
| `dispatched` | relay | bus `Publish` 成功 | 保持 `false`，relay 下次重投 |
| `applied` | consumer（经 `MarkApplied`） | default 投影成功 | 保持 `false`，对账 / `RebuildDefault` 兜底 |

> `dispatched=true ∧ applied=false` 是正常态（已投递待应用，或已应用未回写）；`dispatched=false` 才表示 relay 未完成投递。**consumer 失败永不回退 `dispatched`**——这是"先 PG 后 Neo4j"在运维语义上的闭环：PG 落账即权威，Neo4j 落后只触发兜底重建，不触发重投。

#### `applied` 标志写入契约（V3-06 强约束）

- **唯一写入点**：`TopologyEventRepository.MarkApplied(ctx, tx pgx.Tx, ids []int64) error`
  - **consumer** 与 **RebuildDefault** 都必须经此 helper，**禁止**两条路径各自裸写 SQL（避免行为漂移）
- **写者与顺序（非原子跨库）**：consumer（`StartConsumer`）先完成 `default` 投影（graph.Upsert/Delete，Neo4j 自身事务），成功后再 `MarkApplied(envelope.EventIDs)`（PG 自身事务）。**两者不在同一可回滚事务**——`pgx.Tx` 回滚不了 Neo4j 写入（`internal/graph/interface.go` 的 `Upsert` 无 tx 参数）。`default` 与 `applied` 是两个独立存储的最佳努力推进，靠 MERGE 幂等 + 对账收敛。
- **失败语义**：① graph 写失败 → default 未变更，applied 保持 `false`；② graph 写成功、MarkApplied 前失败/崩溃 → default **已含**变更、applied=`false`（"已应用未回写"欠报态），由对账/`RebuildDefault` 经幂等 MERGE 补写 applied=true。两种情况事件已在 PG（先 PG 后 Neo4j），均**不**重投（§4.3 dispatched 语义）。`applied=false` 积压超阈值告警。
- **MarkApplied 幂等（必写死）**：SQL 为 `UPDATE topology_events SET applied=true WHERE id=ANY($1)`，**不带** `WHERE applied=false` 前置条件——对已 `true` 的行是 0-影响 no-op、不报错、不视为失败。relay+consumer 为 at-least-once 语义，重复投递 + 重复 MarkApplied 是预期安全路径（重投去重靠 MERGE，非索引，见 §4.5/§4.3 幂等分层）。
- **event_id 传递**：bus envelope 携带**单值** `event_id`（来自 `outbox.event_id`）+ `version`；consumer 批量聚合 N 条 envelope 后，一次 graph 批量写 + 一次 `MarkApplied([N ids])`（§4.2 ⑤，消解"复数 EventIDs vs 逐行投递"矛盾）。**consumer 投影改用 per-row apply**（复用 §4.4 `applyEvent` 派生 `assembler.Node`，**绕开** `service.SyncEvent`/`toServiceEvent` 转换——EventIDs 不再经 `service.SyncEvent` 透传，断链消失）；故 `events.SyncEvent` **无需**新增 `EventIDs` 字段（event_id 在 envelope 层）。

**投影一致性与自动对账**（Outbox 只保证投递不丢，**不保证 default 收敛**；由后台对账作业保证最终一致）：
- 维护「已应用事件 id 高水位」；周期性用 `PG 已落盘量 − 已应用量` 得投影积压指标并告警
- **后台对账作业（V3-08 交付，非可选）**：周期抽样比对 `default` 与 `Reconstruct(currentWatermark)`，发现发散自动触发 `RebuildDefault`（§4.6）——这是 `applied=false`（含 consumer crash 欠报态、滚动发布假阳性）的标准自愈路径，使"最终一致"名副其实，**不依赖人工**

**relay 配置**（V3-05 交付，均可在 §5.1 配置表调整）：`poll_interval`（默认 1s）、`batch_size`（默认 100）、`workers`（默认 2）、`max_retries`、退避；随 ctx 启停，退出时 drain outbox；outbox 积压深度超阈值告警/死信。

### 4.4 回放路径（核心新能力）

```go
// 哨兵错误：归属 internal/temporal 包（与 TemporalService 同包；§8.7 归属决策）
// errors.Is(err, temporal.ErrSnapshotEvicted) 判定
var ErrSnapshotEvicted = errors.New(
    "no snapshot covers requested time: fullsync ttl expired or never ran")

func Reconstruct(ctx context.Context, T time.Time) (*GraphModel, error) {
    snap := latestSnapshotBefore(T)                  // 最近 checkpoint
    if snap == nil {                                 // T 早于最旧保留快照
        return nil, ErrSnapshotEvicted               // 明确错误，不静默返回错误结果
    }
    model := loadSnapshot(snap)                       // EnsureLoaded / YAML import
    // 用快照已吸收到的 event_time 水位作下界（非墙钟 CreatedAt），避免源时钟偏差导致重复/遗漏
    events := queryEvents(ctx, snap.AsOfEventTime, T) // PG: event_time > watermark AND event_time <= T（严格大于下界，见下水位定义）
    for _, ev := range events {                       // 按 (event_time, id) 升序
        applyEvent(model, ev)                         // 见下方伪代码
    }
    return model, nil
}
```

**`AsOfEventTime` 水位定义（V3-08 强约束，修正"水位怎么算"的歧义）**：

快照内容来自 `default` 物化投影，而 `default` 只含**已 applied** 的事件（consumer 应用后才 MarkApplied）。故 `AsOfEventTime` 必须取**已应用事件的最大 event_time**：

```sql
-- Create() 开始时计算水位（非墙钟 now()，非全量 max(event_time)）
SELECT coalesce(max(event_time), '-infinity'::timestamptz) FROM topology_events WHERE applied = true;
```

- **❌ 不能用全量 `max(event_time)`**：会把 applied=false 的未应用事件算进快照"声称吸收"的水位，Reconstruct 用它做下界会**静默跳过**这些未应用事件 → 错误拓扑。
- **❌ 不能用墙钟 `time.Now()`**：两个相隔 5 分钟、期间无事件的快照水位会不同，且与压实边界（§6.3）脱钩。
- **两快相隔无事件 → AsOfEventTime 相等 → 回放结果一致**（回答"5 分钟无事件"场景）。
- **读水位与 dump 一致性**：Create() 读水位与 dump default 必须在同一 `GraphLock` 读锁窗口内，避免 dump 期间新事件被 applied 抬高水位而快照未吸收。
- **FullSync 来源的自动快照**：ClearDB+BulkCreate 天然超越 applied 标志（整批重建 default），水位取 FullSync 完成时刻的 `max(applied event_time)`；其 `origin=fullsync` 元数据标记是压实基线校验的关键（§6.3）。

> Reconstruct 下界为严格 `> watermark`：快照已吸收含 watermark 在内的事件，下界再含它会靠 applyEvent 幂等兜底——精确边界优于隐式幂等。

**`applyEvent` 语义**（Reconstruct 与 §8.5 diffArbitrary 共用；作用于内存 `GraphModel`，幂等）：

```
# model.Nodes 按 uri 索引；model.Relations 按 (type,from,to) 索引；复用 assembler.Node/Relation 结构
applyEvent(model, ev):
    switch ev.action:
    case "upsert":            # 节点按 uri 合并 + 关系 MERGE
        labels := registry.GetLabels(ev.entity_type)   # 多 Label（含 EntityType extends 继承）从 Schema 派生（现码 internal/schema/registry.go），不进 payload
        mergeOrCreateNode(model, ev.uri, labels, ev.payload.data)  # 存在则 props +=（Label 不变），不存在则新增
        for r in ev.payload.relations: mergeRel(model, r)                      # 按 (type,from,to) 存在更新/不存在新增
    case "delete":            # 删节点 + 级联出入关系（等价 Neo4j DETACH DELETE）
        removeNodeAndEdges(model, ev.uri)
    case "delete_relation":   # 仅按 (type,from,to) 删关系，不删节点；找不到也安全 no-op
        removeRel(model, ev.payload.type, ev.payload.from, ev.payload.to)
    # 幂等：重复应用同一事件安全（upsert=MERGE，delete/remove 重复无副作用）
# Label 来源（修正 §4.4↔§4.2 矛盾）：不从 payload 取（§4.2 upsert payload 只有 data/relations，无 labels 字段是有意为之），
# 由 ev.entity_type（topology_events NOT NULL 列）经 registry.GetLabels 解析多 Label（含 extends 继承；未知类型降级为单标签 [entity_type]）。
# schema 演进新增 Label 时，mergeOrCreateNode 不改已存在节点的 Label（Label 视为创建期属性）。
# re-creation-after-delete 语义（决策）：delete 后同 uri 再 upsert = 全新态，不继承历史关系；
# 跨节点入边（A→B 中 B 被 delete 再重建）不会自动恢复——A→B 须由 A 的后续 upsert 重新断言。
# 即"关系仅在源端 upsert 时生效"的最终一致语义；端态 diff 在该边角可能漏报跨节点入边（已知取舍）。
```

**成本（二维，需明确）**：`O(快照加载 + 窗口事件数)`，其中：
- **冷快照加载 = O(图大小)**（YAML import + BulkCreate 进逻辑 DB）——大图上是主要开销
- **窗口事件 = O(窗口内事件数)**

> **"回放窗口 ≤1h 有界"是条件性结论**：依赖 V3-10 调度器每小时成功执行 FullSync 产 checkpoint。FullSync 停摆 → 回放窗口无界增长 + checkpoint 最终被 TTL 耗尽（§6.3）。故「定时 FullSync 健康」列为 SLO（§12 验收 8/§13 风险）。冷大图场景应预热（§8.3）或对超大图拒绝任意 T 重建、降级到快照点 diff。

### 4.5 顺序与幂等

- **按 uri 有序（V3 需新增能力，非现状）**：目标是对同一 uri 的事件严格有序（flap/RCA 变更链时序正确的前提）。
  - PG 模式：`id` 给全局到达序（按 `ingest_time`），但**不保证业务序**（源时钟偏差）
  - Kafka 模式：**当前生产者不设 Key（round-robin），且批量 SyncEvent 无单一 uri 可分区**——partition-by-uri 目前**不可实现**。V3-04/05 需交付：(a) 批量事件先拆成单 uri 行；(b) 生产者按 uri 设 Key + HashPartitioner。**落地前，所有"同 uri 严格有序"结论（flap、RCA 变更链）按"尽力而为"对待。**
- **Channel 模式消费侧**：单 goroutine 串行处理（现有 `StartConsumer` 行为），由 consumer 侧的串行化兜底事件顺序
- **幂等（两层，勿混）**：**delivery 去重** = `MERGE (_db, uri)` 天然幂等，relay 重投对 default 安全（重投不重新入库，与索引无关）；**ingest 去重** = `(topic,partition,offset)` 部分唯一索引（防外部 Kafka 重复**入库**，§5）
- **Schema 演进**：payload 用 JSONB + `payload_version` 字段（注意：与 V2 ontology 的 `schema_versions` 表区分，避免撞名），向后兼容
- **envelope 版本与滚动发布**：bus envelope 带 `version` 字段；consumer 见到高于自身版本的 envelope 时 **fail-fast 告警**（**不**静默吞字段——否则旧 consumer 漏 MarkApplied 致 applied 假阳性，§4.3 积压指标失真）。relay/consumer/MarkApplied 作为原子单元同批上线，禁止跨版本混跑；发布窗口内的 applied 假阳性由对账作业（§4.3）补偿

### 4.6 自愈重建 default（RebuildDefault）

```go
func RebuildDefault(ctx context.Context) error {
    s.lock.Lock(); defer s.lock.Unlock()
    graph.ClearDB(ctx, "default")                     // 清空物化投影
    snap := latestSnapshot()                          // 最近 checkpoint
    if snap == nil { return ErrSnapshotEvicted }
    model := loadSnapshot(snap)
    if err := graph.BulkCreate(ctx, "default", model.Nodes, model.Relations); err != nil {
        return err
    }
    // 重放其后全部事件（批处理 500/批）：每批先写 default（Neo4j），再 MarkApplied（PG）——非原子跨库，同 §4.3
    const batchSize = 500
    upperBound, _ := repo.MaxEventID(ctx)            // 冻结重放上界，避免重建期间 Ingest 并发插入导致 offset 漂移
    var lastID int64 = 0
    for {
        batch, err := queryEventsKeyset(ctx, snap.AsOfEventTime, upperBound, lastID, batchSize) // WHERE id > $last AND id <= $upper ORDER BY id LIMIT
        if err != nil { return err }
        if len(batch) == 0 { break }
        lastID = batch[len(batch)-1].ID
        for _, ev := range batch {                    // ① 内存 model（供后续批依赖）
            applyEvent(model, ev)
        }
        if err := applyBatchToDefault(ctx, graph, batch); err != nil {  // ② 写 default（增量 Upsert/Delete，非 BulkCreate）
            return err
        }
        if err := WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error { // ③ 翻 applied（PG 自身事务；与 ② 非原子，靠 MERGE 幂等兜底）
            return repo.MarkApplied(ctx, tx, idsOf(batch))
        }); err != nil { return err }
    }
    return nil
}
```

形态：`cmd` 子命令 或 MCP 工具；批处理（UNWIND 批量，500/批）。**所有 `applied=true` 写入统一经 `MarkApplied` helper**，与 consumer 共用。`applyBatchToDefault(ctx, graph, batch)` 按 action 调 `graph.Upsert` / `graph.DeleteByURIs` / `graph.DeleteRelations`（增量方法，与步骤 2 载入基线的 `BulkCreate` 区分；与 §4.4 `applyEvent` 仅作用于内存 model 不同）；`Upsert` 所需的 `assembler.Node.Labels` 同样复用 §4.4 的 `registry.GetLabels(entity_type)` 派生。证明 `default` 非权威、可随时从 PG 再生。

---

## 5. PG 事件表 Schema

```sql
CREATE TABLE topology_events (
    id             BIGSERIAL,
    event_time     TIMESTAMPTZ NOT NULL,            -- 业务发生时间（RCA/回放/flap 用）
    ingest_time    TIMESTAMPTZ NOT NULL DEFAULT now(),-- 入库时间（投递/去重/延迟监控用）
    source         TEXT NOT NULL,                   -- webhook|kafka（数据起源；channel 是 EventBus 传输模式非来源，见 §6.1）
    connector      TEXT NOT NULL,
    entity_type    TEXT NOT NULL,
    action         TEXT NOT NULL,                   -- upsert|delete|delete_relation
    uri            TEXT,                            -- 目标节点；扇出后每行单值非空（§4.2）
    payload        JSONB NOT NULL,                  -- 完整事件数据；schema 见 §4.2
    payload_version INT NOT NULL DEFAULT 1,         -- payload 结构版本（非 ontology schema_versions）
    kafka_topic       TEXT,
    kafka_partition   INT,
    kafka_offset      BIGINT,                       -- 溯源 + 重放定位 + 去重
    applied        BOOLEAN NOT NULL DEFAULT false,  -- 投影(default)是否已应用；由 MarkApplied helper 唯一写入（§4.3）
    PRIMARY KEY (id, event_time)                    -- 分区表要求：主键须含分区键
) PARTITION BY RANGE (event_time);

CREATE INDEX idx_topology_events_uri_time ON topology_events (uri, event_time DESC);           -- RCA: 这个节点发生了什么
CREATE INDEX idx_topology_events_time     ON topology_events (event_time);                     -- 回放: 时间窗内事件
CREATE UNIQUE INDEX idx_topology_events_kafka_dedup                                         -- DB 层强制 Kafka 去重
    ON topology_events (kafka_topic, kafka_partition, kafka_offset) WHERE kafka_topic IS NOT NULL;

CREATE TABLE event_outbox (
    id          BIGSERIAL PRIMARY KEY,
    event_id    BIGINT NOT NULL,                    -- 应用层逻辑引用（不强制 FK，便于淘汰旧分区时保留 outbox 投递）
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched  BOOLEAN NOT NULL DEFAULT false
);
CREATE INDEX idx_event_outbox_pending ON event_outbox (created_at) WHERE dispatched = false;

-- 快照元数据：扩列 AsOfEventTime + origin（V3-08 用；migration 在 V3-01/03 中加入）
-- 若 existing snapshots 表已存在生产数据，ALTER TABLE ADD COLUMN ... NULL；新创建时 NOT NULL DEFAULT 'manual'
ALTER TABLE snapshots ADD COLUMN as_of_event_time TIMESTAMPTZ;
ALTER TABLE snapshots ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual';  -- fullsync|manual，压实基线校验用（§6.3）
```

**分区自动维护**（V3-01 建次日预建作业 / V3-03 过期 DROP）：
- 预建次日分区：`CREATE TABLE topology_events_YYYYMMDD PARTITION OF topology_events FOR VALUES FROM (...) TO (...)`
- `DEFAULT` 分区兜底（漏建时不报错但告警，需人工迁移）
- 过期分区 `DROP PARTITION`（DDL 级秒删，不产生大量 WAL）

### 5.1 新增配置项（须同步 `internal/config/config.go` SnapshotConfig / 新增 EventConfig）

| 配置 key | 类型 | 默认 | 环境变量 | 说明 |
|---------|------|------|---------|------|
| `snapshot.pre_warm_latest` | bool | false | `SNAPSHOT_PRE_WARM_LATEST` | 建快照后立即 EnsureLoaded（§8.3） |
| `snapshot.retention_days` | int | 7 | `SNAPSHOT_RETENTION_DAYS` | 快照 TTL；与事件保留窗口对齐 |
| `snapshot.auto_create_on_fullsync` | bool | true | `SNAPSHOT_AUTO_CREATE` | FullSync 后是否自动建快照（V3-08 开关） |
| `event.fullsync_interval` | duration | 1h | `EVENT_FULLSYNC_INTERVAL` | V3-10 调度器周期（0 = 禁用） |
| `outbox.poll_interval` | duration | 1s | `OUTBOX_POLL_INTERVAL` | relay 轮询间隔 |
| `outbox.batch_size` | int | 100 | `OUTBOX_BATCH_SIZE` | relay 单批投递量 |
| `outbox.workers` | int | 2 | `OUTBOX_WORKERS` | relay 并发 |
| `outbox.max_retries` | int | 5 | `OUTBOX_MAX_RETRIES` | 最大重试 |
| `event.compaction_grace_days` | int | 1 | `EVENT_COMPACTION_GRACE_DAYS` | 压实淘汰旧分区前的 grace，避免 race（§6.3） |
| `result_cache.enabled` | bool | false | `RESULT_CACHE_ENABLED` | Reconstruct/Diff 结果缓存开关（§8.2） |
| `result_cache.size` | int | 100 | `RESULT_CACHE_SIZE` | 结果缓存 LRU 容量 |
| `result_cache.ttl` | duration | 5m | `RESULT_CACHE_TTL` | 结果缓存 TTL（须 ≤ 保留窗口；压实时按覆盖区间失效，避免返回已淘汰时刻的陈旧结果） |

> 注：事件保留窗口由 `snapshot.retention_days` + 压实联动决定（§6.3，`oldest.AsOfEventTime`），**无独立 `event.retention_days`**（避免与快照 TTL 冲突的死配置）。

---

## 6. 容量与保留策略

### 6.1 只记真正变更（防爆炸的第一道防线）

**不记** Connector 周期 poll 的全量状态（那是 FullSync 的职责）。**只记**：
- 外部 push（webhook / kafka）的真实变更
- 增量 diff 出的实际变化

> `topology_events.source` 枚举为 `webhook|kafka`（数据**起源**），**不含 `fullsync`**——FullSync 走 `ClearDB + BulkCreate` 整批替换 default，不产事件行；它通过"建 checkpoint 快照"间接为回放提供基线（§6.4 cutover 例外）。**`channel` 不是来源**，而是 EventBus=Channel 时的总线传输模式（与 Kafka 总线模式并列）；无论哪种总线模式，事件都经 webhook/kafka 两入口收敛到 `Ingest` 落 PG（§4.2），Channel 总线模式下事件同样落 PG 才有历史。

### 6.2 量级估算

| 参数 | 假设值 |
|------|--------|
| 峰值变更事件 | 500 事件/秒 |
| 保留窗口 | 7 天 |
| 单事件大小 | ~1 KB |
| 总量 | ~3 亿行 / ~300 GB |
| 按天分区 | 每天 ~43 GB，`DROP PARTITION` 秒级清理 |

若超容量：缩短保留到 3 天 / 冷分区列存压缩 / 对齐 RCA 实际窗口（通常 24–72h 足够）。

### 6.3 压实规则（条件性）

事件保留窗口 = 最旧保留快照至今。FullSync 后建快照 → 旧快照 TTL 淘汰 → 连带 `DROP` 其之前的事件分区。PG 在线数据**有界**（前提：applied 积压可收敛——调度器正常产 `origin=fullsync` 快照 + applied 积压告警 + RebuildDefault 兜底；调度器停摆且 applied 持续积压的双故障下分区可能暂留，见失效场景）。

**两个前提（必须显式满足，否则压实会破坏回放正确性）**：
1. 只重建 `T ≥ 最旧保留快照.CreatedAt` 的时刻
2. 待淘汰区间的事件必须已被 default 吸收：基线快照为 **FullSync 来源**（`origin=fullsync`，其 ClearDB+BulkCreate 天然超越 applied 标志），**或**待淘汰分区内无 `applied=false` 行。**仅"快照记录了 AsOfEventTime"不够**——手动 `Create` 快照读的是 default 当前态（只含 applied 事件），其 AsOfEventTime 之前可能仍有 applied=false 的积压事件；若直接 DROP，这些事件从权威源 PG **永久丢失**（default 拿不到、RebuildDefault 重放不了、快照也没吸收）→ source-of-truth 被静默腐蚀。

**失效场景**：FullSync 长期停摆 → 不产新 checkpoint → 旧 checkpoint 随 TTL 淘汰 → 早期事件也已压实 → `Reconstruct(T_早期)` 无可用 checkpoint。此时须返回 **`ErrSnapshotEvicted`**（明确错误，不静默返回错误结果）。「定时 FullSync 健康」列为 SLO（§12/§13）。

#### 联动算法（V3-03 落实细节）

1. **触发**：每次 `SnapshotManager.cleanupExpired(ctx)` 执行成功后，对当前最旧保留快照 `oldest`：
   - 读 `oldest.AsOfEventTime`，将 `event_time < oldest.AsOfEventTime - grace` 的所有 `topology_events` 分区加入待淘汰列表
   - **DROP 前校验（防 applied 积压丢失，前提 2）**：对每个待淘汰分区，若存在 `applied=false` 行且 `oldest.origin != 'fullsync'` → **跳过该分区并告警**（这些事件尚未进 default/快照，DROP 即永久丢失权威源数据）
   - 跳过仍含 `event_outbox.event_id` 引用且 `dispatched=false` 的分区（避免 relay 引用已删除 event_id；`dispatched=true` 的 outbox 行不阻止压实——事件已投递，default 侧由 MERGE/快照保证）
2. **grace**：`event.compaction_grace_days`（默认 1）—— 分区的最大 event_time 早于 `oldest.AsOfEventTime - grace` 才删，避免 Reconstruct 进程并发用中
3. **DROP**：`DROP TABLE topology_events_YYYYMMDD;`（DDL 级秒删）
4. **失败回滚**：单分区 DROP 失败仅记错误日志，不影响其他分区与快照淘汰
5. **outbox 与分区淘汰**：`event_outbox` 不引用 event_id 物理 FK。旧分区被 DROP 后：`dispatched=true` 的 outbox 行无影响（事件已投递，default 侧由 MERGE/快照保证）；**`dispatched=false` 的 outbox 行因事件行已物理消失，relay 无法重建 envelope，是无法自愈的孤儿**——故 step 1 的跳过规则对这些分区生效（`dispatched=false` 不 DROP）。其余孤儿由 outbox 自身的过期清理回收（V3-05 范围外，本计划不强制）。

### 6.4 迁移 / Cutover（V2 → V3）

V2 在 cutover 前的 `default` 存量节点不在 `topology_events` 中。cutover 步骤：
1. cutover 时对 `default` 做一次**基线快照 B0**（吸收全部存量态），`origin='fullsync'`（完整基线，压实时超越 applied 标志，§6.3）；其 `AsOfEventTime` 遵循 §4.4 规则 = `max(applied=true 的 event_time)`——cutover 时 `topology_events` 尚无 applied 事件，故为 `-infinity`（语义"B0 + 其后全部事件"，**非**墙钟 cutover 时刻）
2. `Reconstruct` / `RebuildDefault` 统一以「最近快照 + 其后事件」为源，**不假定事件覆盖全部历史**
3. `Reconstruct(T)`：以 B0 为基线重放其后事件；若 T 早于最旧保留快照 → `ErrSnapshotEvicted`

---

## 7. 对高级引擎的赋能

| 引擎 | V2 现状 | V3 解锁能力 |
|------|---------|------------|
| **Diff** | 仅比对两个手动快照 | 任意 T1/T2 时态 diff，不依赖是否拍过快照 |
| **Simulation** | 未实现（`ErrNotImplemented` 骨架，未被任何 service 引用） | 从任意历史时刻 fork：「如果 T1 时改了 X 会怎样」（沙盒 `CloneDB` 已具备，补历史基态即可） |
| **RCA** | **基本做不了**（不知故障前变了什么） | `queryEvents(uri, [t-window, t])` 直接拿变更链——痛点 3 核心依赖 |

> 注：`internal/engine/`（rca/impact/simulation）目前是 `ErrNotImplemented` 骨架，未被任何 service 引用。本计划是它们落地的前置条件。

---

## 8. 读路径、缓存与 diff 设计

> 本节固化几轮设计讨论的结论：读路径分化、缓存分层、快照温度、diff 路径与策略、flapping 处理、service 编排。
> 核心原则——**绝不把所有查询塞进"全图重建"**。

### 8.1 读路径分化（按查询类型走不同路径）

不同查询成本差几个数量级，必须分化：

| 查询类型 | 读路径 | 成本 | 典型场景 |
|---------|--------|------|---------|
| **当前态** | 直接读 Neo4j `default` | O(1) 已物化 | MCP query_topology、监控大盘 |
| **快照点 vs 快照点** | Cypher 跨 `_db` set-diff（已热） | O(图大小) 索引命中 | "上周 vs 本周"对比 |
| **任意时刻 T 重建** | `loadSnapshot(T0) + 重放 (T0,T] 事件` | O(图大小 + 窗口事件)，冷快照含 O(图大小) 加载 | 时态回放、任意 T1/T2 diff |
| **RCA 按节点查变更** | **只查 PG** `WHERE uri=$u AND event_time BETWEEN` | O(该节点事件数) 索引命中 | "故障节点窗口内变了什么" |

> **RCA 不做全图重建**——它是按 uri 的 PG 索引查询（毫秒级）。把 RCA 做成"重建全图再分析"是性能灾难。

### 8.2 缓存分层（图态为主，结果缓存兜底）

| 缓存层 | 存什么 | 工具 | 引入时机 |
|--------|--------|------|---------|
| **图态缓存** | 完整历史快照逻辑 DB（可 Cypher 遍历） | **Neo4j LRU（现有 `maxActive`）** | 已有，V3 更关键 |
| **结果缓存** | `Reconstruct(T)` / diff 的序列化结果（key = `Reconstruct(T)` 或 `Diff(a,b)` 字符串） | 进程内 LRU（`lru.Cache[string, []byte]`，容量 100，TTL 5min）或 Redis（可选） | V3 后期按需启用，开关 `result_cache.enabled` |
| **查询缓存** | 热点 topology 查询结果 | Redis（可选） | 仅 Neo4j 成瓶颈时 |

> **Redis 不替代图态缓存**：历史态是图形状，RCA/Impact 要在其上做 BFS/DFS 邻居遍历，只能跑 Cypher。Redis 存序列化图会丧失图查询能力，反序列化开销也大。Redis 仅作可选的结果/查询缓存。

### 8.3 快照温度生命周期

```
Create(name)        → 写 YAML + metaCache + PG(快照元数据 snapshots 表 / 审计 dual-write)  【冷】
EnsureLoaded(name)  → YAML import → BulkCreate 进逻辑 DB                                    【热】
   （由 Restore / Diff / Reconstruct / PreWarm 触发）
LRU 淘汰(maxActive) → 逻辑 DB 清掉，YAML 保留                                                【回冷】
```

> **热不是 Create 的产物，是被查询的副作用。** V3 自动快照（FullSync 后）产出的也是冷 YAML。
> 可选 `snapshot.pre_warm_latest=true`（§5.1）：建快照后立即 EnsureLoaded，让"当前 vs 上次全量"高频 diff 零加载。
> （注：`Create` 的 PG 写指 V2 既有的快照元数据/审计 dual-write，非 `topology_events`。）
>
> **hook 错误短路（V3-08）**：`WithPostFullSyncHook` 内 `Create` 失败必须短路、跳过 `PreWarm`，且**不得回退导致 FullSync 失败**（FullSync 已成功；快照/预热是尽力而为的 checkpoint）。`Create` 失败须触发**独立告警**（区别于 FullSync 失败）——它是回放有界性的隐性威胁（本轮无 checkpoint → 回放窗口增长）。`EnsureLoaded` 对不存在的快照返回明确哨兵错误（如 `ErrSnapshotNotFound`），不透传文件系统错误，便于上层区分。

### 8.4 diff 路径决策表

| 场景 | 路径 | 成本 |
|------|------|------|
| 快照 vs 快照（都热） | Cypher 跨 `_db` diff | O(图大小) 索引命中 |
| 快照 vs 快照（冷，一次性） | LocalDiff（YAML，Go） | O(n+m) 哈希表，无加载 |
| 快照 vs 任意 T | 调用方命名快照；Reconstruct(T) 一次（内部自选 `latestSnapshotBefore(T)`），比对快照 | O(重建 T + 比对) |
| 任意 T_a vs T_b（同基础快照） | **Reconstruct(T_a) + 折叠 (T_a,T_b] 事件比对端态** | O(重建 T_a + 窗口事件) |
| 节点变更历史（RCA） | PG 按 uri 查事件链 | O(该节点事件数) |

> **不引入 APOC**：Neo4j CE 无原生图 diff；现有"Cypher 跨 `_db` + Go `compareProps`"是最优解。`apoc.diff.maps` 只比对 map（属性），解决不了图结构 diff，边际收益小于引入依赖的成本。属性级 diff 统一走 `compareProps`（已实现，含数值归一化）。

### 8.5 flapping（抖动）处理

**端态比对天然吸收 flap；flap 次数单独作 RCA 指标。两者不混。**

场景：接口在 (T_a, T_b) 内 `up→down→up→down→up` 多次翻转，但两端都 `up`。
- **端态 diff**（`compareProps(stateA, stateB)`）：status 两端都 `up` → 净零 → **不进 diff**（正确）
- **raw event 列表 ≠ diff**：直接罗列事件会显示假变更。必须"折叠事件成终态，比对端态"

正确算法（任意时刻 diff，`applyEvent` 即 §4.4）：

```
diffArbitrary(T_a, T_b):
    stateA = Reconstruct(T_a)
    stateB = clone(stateA)
    for ev in queryEvents(T_a, T_b] ordered by (event_time, id):   # 严格时序
        applyEvent(stateB, ev)
    return compareProps(stateA, stateB)   # 比对端态，非罗列事件
```

两个独立产出：

| 产出 | 含义 | 用途 |
|------|------|------|
| `Diff(T_a, T_b)` | 端态净变化（flap 折叠） | "两时刻有何不同" |
| `FlapCount(uri, window)` / `EventLog` | 原始事件 / 翻转次数 | RCA 征兆（不稳信号） |

**`FlapCount` 形式化**（V3-08 交付）：
- 节点签名：`FlapCount(uri string, field string, window time.Range) int`（`field` 如 `status`/`admin_status`，默认观测字段集合可配）
- 基线语义：首个事件的值与 **window.start 处的重建态**（覆盖 window.start 的快照）比较——若不同计 1 次翻转（避免漏计"基线→首事件"）；`field` 在某事件中缺失则**跳过该事件**（不产生翻转计数）
- 算法：按 `(event_time, id)` 升序取该 uri 在 window 内、针对 `field` 的事件，计数相邻值不相等的翻转次数（`a→b→a` 计 2 次翻转）
- 关系 flap：独立签名 `FlapCountRel(relType, from, to string, window time.Range) int`——按 (type,from,to) 存在↔不存在翻转计数（关系无 `field` 概念，与节点 flap 解耦）
- 去抖阈值（可选）：窗口内翻转间隔 < 阈值才计为 flap
- 暴露：service 方法 + MCP 工具
- ⚠ 正确性依赖 §4.5 的 partition-by-uri 落地（落地前为"尽力而为"）

规则：
- **存储不压缩**：保留窗口内全部原始事件，flap 查询时算；入库时合并会丢 RCA 信息
- **严格有序**：折叠按 `event_time` 升序，同时间戳同 uri 用 `id` 做 tiebreaker（高频遥测现实问题）

### 8.6 diff 策略：Cypher 主路径（A 方案）+ Local 降级

经 A/B 对比，**采用 A 为骨架**：Cypher 为主路径（已热跳加载、冷加载一次），LocalDiff 作显式选项和降级兜底。

| 维度 | A 加载优先（采用） | B Local 优先（否决作默认） |
|------|-------------------|--------------------------|
| 大图性能 | ✅ Cypher 走 `(_db,uri)` 索引 | ❌ 需全量构建哈希表 O(n+m)，无 Cypher 索引 |
| 重复 diff | ✅ 加载一次后持续快 | 需显式预热才快 |
| 语义一致性 | ✅ 单主路径 | ❌ Cypher/Local 双路径须同步 |
| 解耦 / 韧性 | ❌ 依赖 Neo4j | ✅ 离线可用 |
| 一次性冷 diff | ❌ 双倍 import | ✅ 零加载 |

选 A 的理由：本系统核心是高级运维分析，diff 是 RCA/Simulation 的**高频**底层操作，重复调用是常态；大图下索引 Cypher 优于无索引 Go。B 的韧性优势用 `strategy=local` 显式覆盖即可，不必当默认。

### 8.7 service 层编排（机制与策略分离）

**机制留下层**（`SnapshotManager`）：`Diff`（Cypher）+ `LocalDiff`（Go）保留，只管"给定两快照怎么 diff"，不关心策略与健康度。
**策略上 service**：编排方法集中处理策略选择 + 降级 + 指标 + 预热。Handler/MCP 完全不感知 Neo4j 健康。

**归属**：
- 新建 **`TemporalService`**（`internal/temporal` 包，含 `ErrSnapshotEvicted`）承载 `Reconstruct` / `diffArbitrary` / `FlapCount` / `MarkApplied` helper 调用
- `SnapshotService` 增加 `Diff(DiffOptions)` 编排 + `PreWarm`
- `SyncService` 增加 `WithPostFullSyncHook` 钩子（V3-08 自动快照）+ `Ingest`（V3-04）+ `MarkApplied` 调用（V3-06）
- **自动快照（`WithPostFullSyncHook` / cutover B0）必须写 `origin='fullsync'`**（压实超越 applied 标志的前提，§6.3）；手动 `Create` 默认 `origin='manual'`

```go
type DiffStrategy int
const (
    DiffStrategyAuto   DiffStrategy = iota // 默认：Cypher（已热跳加载，冷加载）
    DiffStrategyCypher                     // 强制 Cypher，失败不降级（诊断用）
    DiffStrategyLocal                      // 强制 LocalDiff（离线/不碰 Neo4j）
)

type DiffOptions struct {
    Strategy  DiffStrategy
    NoDegrade bool
}

func (s *SnapshotService) Diff(ctx context.Context, a, b string, opts DiffOptions) (*snapshot.SnapshotDiff, error) {
    if opts.Strategy == DiffStrategyLocal {
        return s.manager.LocalDiff(a, b)              // 纯文件，不碰 Neo4j
    }
    diff, err := s.manager.Diff(ctx, a, b)            // 内部已 EnsureLoaded
    if err == nil {
        s.metrics.DiffHit("cypher")
        return diff, nil
    }
    if isNeo4jUnavailable(err) && !opts.NoDegrade && opts.Strategy == DiffStrategyAuto {
        slog.Warn("neo4j diff failed, degrading to local", "a", a, "b", b, "error", err)
        s.metrics.DiffDegrade("local")
        return s.manager.LocalDiff(a, b)
    }
    return nil, err
}

func (s *SnapshotService) PreWarm(ctx context.Context, name string) error {
    return s.manager.EnsureLoaded(ctx, name)          // 主动加载进逻辑 DB
}

// 时态 diff（时间参数，补 §8.4「快照 vs 任意T」/「任意 Ta vs Tb」场景；Diff(a,b string) 无法表达）
func (s *SnapshotService) DiffAt(ctx context.Context, snap string, T time.Time, opts DiffOptions) (*snapshot.SnapshotDiff, error)  // 命名快照 vs Reconstruct(T)
func (s *SnapshotService) DiffRange(ctx context.Context, Ta, Tb time.Time, opts DiffOptions) (*snapshot.SnapshotDiff, error)       // 任意 Ta vs Tb（§8.5 diffArbitrary）
```

**Neo4j 健康判定**（`isNeo4jUnavailable`）：连接错误 / 驱动关闭 / 瞬态 ServiceUnavailable → 降级；**`ctx.DeadlineExceeded` / `context.Canceled` → 不降级、fail-fast**（ctx 已死，降级到 LocalDiff 会因同一 ctx 立即失败且掩盖真问题，如大图慢查询）；Cypher 语法 / 数据错误 → **不降级**（真 bug，降级会掩盖）。
> **实现前必须验证 `neo4j-go-driver/v5` 实际导出的连接/瞬态错误类型**（如 `neo4j.ConnectivityError` 等），用 `errors.As` 匹配——若类型名对不上，会导致降级条件永不成立（Neo4j 真挂了也走不到 LocalDiff）。在 V3-08 验收 #12 中需专门跑注入测试。

---

## 9. 任务分解

### Phase 1: 事件存储（V3-01 ~ V3-03）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-01 | PG 事件表 Schema + 分区 + 迁移 | 2天 | 无 | `migrations/000002_topology_events.up.sql`（表+首个分区+DEFAULT 分区）+ `000003_snapshots_add_as_of_origin.up.sql`（快照加 `as_of_event_time` + `origin` 两列）+ **次日分区预建作业** |
| V3-02 | TopologyEventRepository CRUD + 时间窗/uri 查询 + 分页查询 | 1.5天 | V3-01 | `internal/repository/topology_event_repo.go` |
| V3-03 | 保留策略 + 压实作业（与快照 TTL 联动） | 1.5天 | V3-01, V3-02 | **过期分区 DROP** + 联动淘汰逻辑（§6.3 算法）；压实淘汰时 T 越界返回明确错误（`ErrSnapshotEvicted` 哨兵随 `internal/temporal` 包在 V3-08 落地，此处用临时占位错误） |

- [ ] V3-01 `migrate up` 建表 + 首个分区；次日预建作业工作；DEFAULT 分区有数据时告警
- [ ] V3-02 按 uri / 时间窗查询正确；分页查询支持 RebuildDefault 批处理
- [ ] V3-03 旧快照淘汰时连带淘汰其之前事件分区（grace 内不删；applied 积压/origin 校验生效）

> V3-01 只做「建 + 次日预建」；V3-03 只做「过期 DROP + 联动」——边界分明。

### Phase 2: 统一接入与一致性（V3-04 ~ V3-05）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-04 | EventSink + Ingest + 扇出 + payload schema + 事务封装 | 2天 | V3-02 | `SyncService.Ingest` + `normalizeToTopologyEvent` 扇出（1→N 行）+ payload JSONB schema 文档化 + `WithTx(ctx, fn)` 跨表事务封装；webhook/kafka 两入口改走 Ingest（event_id 由 outbox.event_id 注入 envelope，`events.SyncEvent` 不增字段） |
| V3-05 | Outbox + relay + partition-by-uri + 幂等去重 | 2天 | V3-04 | `event_outbox` + relay（唯一投递方，含 §5.1 配置；标 `dispatched=true` on Publish 成功、不待 applied）+ 生产者按 uri 设 Key/HashPartitioner + 批量事件拆单 uri 行 + outbox→bus envelope 含单值 `event_id` + `version` |

- [ ] V3-04 两入口都走 Ingest；批量 delete 的 N 个 uri 各落 1 行（RCA 可查）；`WithTx` 包裹 topology_events + event_outbox 同事务
- [ ] V3-05 PG 写成功后 bus 投递失败 → relay 重投成功，事件不丢；同 uri 事件同分区有序；bus envelope 含单值 `event_id` + `version`；relay 标 `dispatched=true` 不待 applied

### Phase 3: 投影与自愈（V3-06 ~ V3-07）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-06 | main.go 接 StartConsumer + default 降为物化投影 + MarkApplied | 1天 | V3-05 | 增量链路接通（修复 V2 缺口）；consumer 改 per-row apply（复用 §4.4 applyEvent 派生 assembler.Node，**绕开** `service.SyncEvent`/`toServiceEvent`）+ 批量聚合 N envelope 一次 graph 写 + `MarkApplied([N ids])`（非原子跨库，幂等兜底）；投影积压水位指标 |
| V3-07 | RebuildDefault（自愈/灾备） | 1.5天 | V3-06 | `cmd` 子命令/MCP 工具：`ClearDB(default) + loadSnapshot(最近) + 重放其后事件`（复用 §4.6，500/批，MarkApplied 共用） |

- [ ] V3-06 webhook → PG → bus → default 全链路贯通；consumer per-row apply + 批量；`applied` 正确翻转（仅经 MarkApplied helper）；consumer 失败时 `dispatched` 不被回退
- [ ] V3-07 清空 default 后能从 PG 事件完整重建；应用层与 V3-06 共用 `MarkApplied`

### Phase 4: 回放、diff 编排与自动快照（V3-08）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-08 | Reconstruct + diffArbitrary + Diff 编排 + FlapCount + 自动快照 + PreWarm + 对账作业 | 4天 | V3-03, V3-07 | `TemporalService`（Reconstruct/diffArbitrary/FlapCount，含 `applyEvent` §4.4 + watermark 下界 + `ErrSnapshotEvicted`）；`SnapshotService.Diff(DiffOptions)` 编排+降级+`PreWarm`；`SyncService.WithPostFullSyncHook` + `snapshot.auto_create_on_fullsync` 默认 true，命名 `auto-{RFC3339}`，**自动快照写 `origin='fullsync'`**（§6.3 压实解锁前提；手动 `Create` 默认 `origin='manual'`）；**后台对账作业**（周期抽样 default vs Reconstruct(watermark)，发散自动触发 RebuildDefault，§4.3） |

- [ ] V3-08a `Reconstruct(T)` 任意历史时刻拓扑正确；T 早于最旧快照返回 `ErrSnapshotEvicted`
- [ ] V3-08b 任意 T1/T2 diff：flap 窗口内净零、FlapCount 正确
- [ ] V3-08c Diff `Auto` 在 Neo4j 不可用时降级 Local 成功；`PreWarm` 后冷快照零加载 diff；isNeo4jUnavailable 类型注入测试通过
- [ ] V3-08d FullSync（调度器/手动）后自动产生 checkpoint 快照（自动建快照开关开启时），记录 `AsOfEventTime` 水位
- [ ] V3-08e 后台对账作业：注入 applied 欠报态（graph 成功、MarkApplied 失败）后，对账自动发现发散并触发 RebuildDefault 补齐，无需人工

### Phase 5: 调度器（V3-10）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-10 | 周期 FullSync 调度器 | 1.5天 | V3-06 | 后台 ticker（`event.fullsync_interval`，默认 1h，0 禁用）触发 FullSync；**非阻塞 TryLock**——锁被 IncrementalSync/上一轮 FullSync 占用时**跳过本次 tick 并告警**（不排队、不无界阻塞，避免与 consumer 死锁互等）；失败重试 + SLO 告警 |

- [ ] V3-10 每 `fullsync_interval` 自动触发 FullSync 并建 checkpoint；锁占用时跳过+告警；连续失败 N 次告警

### Phase 6: 验收（V3-09）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-09 | 集成测试 + 容量验证 + 文档归档 | 1.5天 | V3-01~V3-08, V3-10 | 验收报告 + CLAUDE.md 更新 |

- [ ] V3-09 全部验收清单通过

---

## 10. 任务依赖关系

```
Phase 1 (事件存储)
V3-01 (Schema+预建) → V3-02 (Repo+分页) → V3-03 (压实+DROP+联动)

Phase 2 (统一接入)
V3-02 → V3-04 (EventSink+Ingest+扇出+WithTx) → V3-05 (Outbox+relay+partition-by-uri)

Phase 3 (投影)
V3-05 → V3-06 (StartConsumer+MarkApplied) → V3-07 (RebuildDefault)

Phase 4 (回放+diff)
V3-03 + V3-07 → V3-08 (Reconstruct+diff编排+FlapCount+自动快照+TemporalService)

Phase 5 (调度器)
V3-06 → V3-10 (周期 FullSync)        # 与 Phase 4 可并行

Phase 6 (验收)
V3-01~V3-08, V3-10 → V3-09
```

---

## 11. 工时汇总

| Phase | 任务范围 | 任务数 | 预估工时 |
|-------|----------|--------|---------|
| Phase 1: 事件存储 | V3-01 ~ V3-03 | 3 | 5 天 |
| Phase 2: 统一接入 | V3-04 ~ V3-05 | 2 | 4 天 |
| Phase 3: 投影自愈 | V3-06 ~ V3-07 | 2 | 2.5 天 |
| Phase 4: 回放+diff | V3-08 | 1 | 4 天 |
| Phase 5: 调度器 | V3-10 | 1 | 1.5 天 |
| Phase 6: 验收 | V3-09 | 1 | 1.5 天 |
| **合计** | | **10** | **18.5 天** |

**风险缓冲**：建议增加 20%，总计 **~23 天** (1人) / **~12.5 天** (2人并行)。

---

## 12. 验收清单

| # | 验收项 | 验证方法 | 通过标准 |
|---|--------|----------|---------|
| 1 | 编译 + lint | `go build ./... && golangci-lint run` | 无错误 |
| 2 | 事件持久化 | 发 webhook 事件 | PG `topology_events` 有记录 |
| 3 | 统一接入 + 扇出 | webhook / kafka 两来源 × 两总线模式（Channel/Kafka）；批量 delete N uri | 都落 PG；每个 uri 各 1 行（RCA 可查）；Channel 总线模式下事件同样落 PG |
| 4 | Outbox 不丢 | PG 写成功后 Publish 失败 | relay 重投成功，default 最终一致 |
| 5 | 增量链路接通 | 发 webhook 事件 | default 物化投影更新；`applied` 翻转（修复 V2 缺口） |
| 6 | 自愈重建 | 清空 default 后触发 Rebuild | 从最近快照 + 其后事件完整还原 |
| 7 | 时态回放 | `Reconstruct(T)` | 任意历史时刻拓扑正确；T 早于最旧快照返回 `ErrSnapshotEvicted` |
| 8 | FullSync SLO + 自动快照 | 调度器周期触发；连续失败模拟 | 每 `fullsync_interval` 自动产 checkpoint；连续失败 N 次告警；hook 关闭时（`auto_create_on_fullsync=false`）不产快照 |
| 9 | 压实有界 | 跑过 TTL 周期 | 旧快照 + 其前事件分区连带淘汰，PG 在线数据不膨胀；grace 内分区保留 |
| 10 | 性能（分热/冷） | 回放 P95 + 投影延迟 | 热快照回放 P95 < 2s；冷快照回放 P95 < SLA（含 import）；投影延迟 < 5s |
| 11 | 任意 T1/T2 diff | flap 场景 | 端态净零 diff 正确；`FlapCount` 正确 |
| 12 | Diff 降级 | Neo4j 不可用（Auto）；**isNeo4jUnavailable 类型注入测试** | 降级 LocalDiff 成功；`NoDegrade` 时不降级；类型断言匹配正确（否则该验收失败） |
| 13 | PreWarm | 建快照后预热 | 冷快照零加载 diff |
| 14 | RCA 就绪 | 按节点查变更链 | `queryEvents(uri, window)` / `FlapCount` 返回正确结果 |
| 15 | 结果缓存（如启用） | 重复调用 `Reconstruct(T)` / `Diff(a,b)` 同参数 | 第二次命中进程内 LRU 不重建；`result_cache.enabled=false` 时直通 |
| 16 | 向后兼容 | MCP 工具 + 现有 API | 功能不受影响 |
| 17 | 自动对账自愈 | 注入 applied 欠报态（graph 成功、MarkApplied 失败/崩溃） | 对账作业周期内发现 default 与 Reconstruct(watermark) 发散，自动触发 RebuildDefault 补齐，applied 最终翻转，无需人工 |

---

## 13. 风险评估

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|---------|
| **PG 数据丢失（卷损坏/实例丢失）→ 事件历史永久消失** | 低 | **灾难** | §15 备份/DR：定期基础备份 + WAL 归档实现 PITR + 至少一个流复制副本；快照 YAML 卷独立备份；明确 RTO/RPO |
| **PG 单点（单机 docker-compose，单宿主）→ 整宿主失效 = 历史全丢** | 中（部署态） | 高 | §16 部署：PG 必须独立部署 + 主备/自动故障切换；Neo4j 同考虑；定义 SPOF 边界与 RTO |
| 事件表爆炸 | 高 | 高 | 只记变更（非轮询）+ 分区 + 压实联动（§6） |
| FullSync 停摆 → 回放窗口无界 + checkpoint 耗尽 | 中 | 高 | V3-10 调度器 + SLO 监控告警；`ErrSnapshotEvicted` 明确报错；§6.3 前提声明 |
| Outbox relay 成为瓶颈 | 中 | 中 | 批量投递 + 并发 relay（§5.1 配置）；监控 outbox 积压深度 |
| 双写发散（PG 与 default） | 中 | 高 | Outbox + `applied` 水位 + **后台对账作业自动检测发散并触发 RebuildDefault**（§4.3，非人工）+ `RebuildDefault` 自愈 |
| 事件乱序（partition-by-uri 未落地） | 高 | 中 | V3-05 交付 partition-by-uri；落地前 flap/RCA 时序按"尽力而为"，依赖幂等 MERGE 兜底 |
| 回放计算量（冷大图） | 中 | 中 | 预热（§8.3）；超大图拒绝任意 T 重建、降级快照点 diff；验收分热/冷（§12-10） |
| 迁移期数据缺口 | 中 | 中 | §6.4 cutover 基线快照 B0；不假定事件覆盖全部历史 |
| Kafka 去重失效 | 低 | 中 | 部分唯一索引（§5）+ 应用层 offset 校验 |

---

## 14. 决策清单（已拍板）

| 决策点 | 结论 |
|--------|------|
| 事件存储 | PG 分区表（权威源） |
| default 角色 | 物化投影（非权威，可重建） |
| 双 default | **不采用**，用 default + 自动快照 |
| Redis | 暂不引入（仅作可选读缓存） |
| 事件保留窗口 | 3–7 天（对齐 RCA 窗口，按容量测算调整） |
| Outbox | 采用（生产必备）；relay 为唯一投递方，Ingest 不直接 Publish |
| 事件入库范围 | 仅真实变更，排除周期 poll；`source` 不含 fullsync |
| 读路径 | 按查询类型分化；RCA 走 PG 索引，不做全图重建 |
| 缓存分层 | Neo4j LRU 图态缓存为主 + 结果缓存可选；Redis 不替代图态 |
| diff 策略 | Cypher 主路径（A 方案）；LocalDiff 作降级 / 显式 `strategy=local` |
| diff 归属 | 新建 `TemporalService`（`internal/temporal` 包）承载 Reconstruct/diffArbitrary/FlapCount |
| `applied` 写入契约 | 唯一经 `MarkApplied(ctx, tx, ids)` helper；consumer 与 RebuildDefault 共用，禁止裸写 |
| dispatched/applied 解耦 | relay Publish 成功即标 `dispatched=true`、不待 consumer；consumer 失败保持 `applied=false`、**永不回退 `dispatched`**；PG 唯一权威，default 落后由对账作业/RebuildDefault 兜底（§4.3） |
| flapping | 端态比对吸收 flap；`FlapCount` 作独立 RCA 指标；存储不压缩 |
| APOC | 不引入（现有 Cypher + Go `compareProps` 足够） |
| 快照预热 | 可选 `snapshot.pre_warm_latest`（§5.1） |
| 自动快照 | `snapshot.auto_create_on_fullsync` 默认 true；命名 `auto-{RFC3339}`；通过 `WithPostFullSyncHook` 触发 |
| 调度器 | V3-10 周期 FullSync（默认 1h），作为回放有界性前提 |
| payload 版本 | 字段名 `payload_version`（与 ontology `schema_versions` 区分）；schema 在 §4.2 |
| 回放下界 | 快照 `AsOfEventTime` 水位（非墙钟 CreatedAt）= `max(applied=true 的 event_time)`；严格 `> watermark` |
| 压实基线 | 待淘汰分区须 `applied` 全 true 或基线快照 `origin=fullsync`（§6.3）；`origin` 列区分 fullsync/manual |
| 压实 grace | `event.compaction_grace_days` 默认 1，避免 race |
| 哨兵错误 | `ErrSnapshotEvicted` 归属 `internal/temporal` |

---

## 15. PG 备份与容灾（强制项）

**PG 是事件历史的唯一权威源，PG 丢失 = 历史、RCA、回放能力全部永久消失**。本节是 V3 上线前**必须**满足的运维基线。

| 项 | 要求 | 验收 |
|----|------|------|
| **基础备份** | 每日全量 `pg_dump` 或 `pg_basebackup`，至少保留 7 天滚动 | V3-09 中跑一次完整 backup/restore 演练 |
| **WAL 归档 + PITR** | 开启 `archive_mode=on`，WAL 流到独立归档目录；可在任意时间点恢复 | 注入"误删 30 分钟前数据"场景，验证 `pg_restore --target-time` 成功 |
| **流复制副本** | 至少 1 个异步 hot-standby；同步副本用于 RPO≈0 场景 | 注入"primary 挂"场景，副本 30s 内可升主 |
| **快照 YAML 卷** | 与 PG 独立备份（不同故障域），至少 7 天滚动 | 注入 PG 卷损坏 + YAML 卷完好场景，可从 YAML + PG 事件副本恢复 |
| **RTO / RPO 目标** | RTO ≤ 30 分钟；RPO ≤ 5 分钟（WAL 归档可达） | runbook 中显式声明 |
| **定期演练** | 季度一次完整故障切换 + 数据恢复演练 | 演练报告归档 |

> 「事件不可重建」的真正兜底不是 RebuildDefault（它只能从 PG 重建 default，不能重建 PG 自身），而是 §15 上述备份链路。SRE 团队须将 PG 不可恢复列为 P0 级别事件。

---

## 16. 部署与 SPOF 边界

CLAUDE.md 当前部署为单机 docker-compose（neo4j + kafka + postgres + app 四服务同宿主）。V3 把 PG 提升为权威源后，**单机部署使 PG 成为整条历史链路的单点故障**。

**强制要求**（V3 上线前）：

| 项 | 状态 | 备注 |
|----|------|------|
| PG 独立部署（脱离 docker-compose 单一宿主） | 待办 | 推荐托管 PG（RDS/Cloud SQL）或独立 VM |
| PG 主备 + 自动故障切换 | 待办 | Patroni / 云厂商托管方案 |
| Neo4j 独立部署（推荐独立宿主） | 待办 | 当前 MVP 可暂缓但需纳入 roadmap |
| 多可用区 / 多区域容灾 | 待办 | 视业务 RTO/RPO 决定；至少同城双活 |
| 监控：PG 复制延迟、磁盘空间、WAL 积压 | 待办 | 接入 Prometheus（V2-15） |
| 告警：复制断流、磁盘满、WAL 归档失败 | 待办 | P1 告警 |

> §13 风险表的"PG 单点"行即对应此处。当前 `deploy/docker-compose.yml` 不满足 V3 上线条件；需先完成部署拓扑升级，再上线 V3。