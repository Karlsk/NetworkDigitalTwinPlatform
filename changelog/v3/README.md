# V3 研发计划 — 拓扑时态历史与回放能力

> 基于 V2 基础设施（Kafka + PostgreSQL + Gin + 可观测性）就绪，V3 聚焦 **拓扑时态历史**：
> 解决 V2 遗留的核心架构缺陷——增量同步 `MERGE` 是破坏性更新，销毁了事件级历史，
> 导致「拓扑回放」「任意时刻重建」「RCA 根因分析」无法实现。
>
> 本计划引入 **事件溯源（Event Sourcing）+ 物化投影** 模式：
> 1. PG 事件表作为权威历史源（append-only WAL）
> 2. Neo4j `default` 降级为当前态物化投影（热查询）
> 3. 每次 FullSync 后自动建快照作为 checkpoint
> 4. 事件分区与快照 TTL 联动压实，PG 在线数据量有界
>
> 该能力是 RCA / Simulation / 时态 Diff 引擎落地的 **前置依赖**（CLAUDE.md 痛点 3）。

**总任务数**: 9 项 (V3-01 ~ V3-09)
**预估工时**: 18 人天（含缓冲 22 天）
**关联文档**: [V2 研发计划](../v2/README.md) · [项目规范](../../CLAUDE.md)

---

## 1. 背景与问题

### 1.1 当前架构的致命缺陷

V2 的增量同步直接对 `default` 逻辑 DB 执行 `MERGE ... SET x += n.props`（[`sync_service.go:209`](../../internal/service/sync_service.go:209)），是**破坏性更新**：

```
T1: FullSync        → default = {A, B, C}
T2: 增量 update D   → default = {A, B, C, D}
T3: 增量 update B'  → default = {A, B', C, D}   ← B 的旧值永久丢失
T4: Create("snap")  → 只能拿到 T4 当前态，T2/T3 的中间过程无法重建
```

FullSync 周期 ≤ 1 小时/次，两次全量之间的细粒度变更在 MERGE 后丢失。

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

**采用**：保持单一 `default`（live），**每次定时 FullSync 后自动 `Create` 快照**作为 checkpoint。快照已有的 YAML 归档 + 逻辑 DB 懒加载 + LRU 保活机制直接复用。

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

事件表按时间分区，保留窗口与快照 TTL 对齐。**压实规则**：早于「最旧保留快照」的事件是冗余的（无法被任何回放用到），可安全淘汰。

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
        │  · 所有来源（webhook/channel/kafka）统一落盘   │
        └───────────────┬──────────────────────────────┘
                        │ ① 同事务写 outbox
                        │ ② relay 投递（保证不丢）
                        ▼
        ┌──────────────────────────────────────────────┐
        │  Neo4j default（物化当前态，非权威）          │
        │  · 消费事件 → Upsert/Delete（MERGE +=）       │
        │  · 热查询 / MCP / 分析引擎读这里               │
        │  · 可从 PG 事件完整重建（自愈）                │
        └───────────────┬──────────────────────────────┘
                        │ ③ FullSync 后周期 checkpoint
                        ▼
        ┌──────────────────────────────────────────────┐
        │  Snapshot YAML + 逻辑 DB（状态 checkpoint）   │
        │  · 每次定时 FullSync 后自动 Create            │
        │  · 按 TTL 淘汰，连带淘汰其之前事件分区         │
        └──────────────────────────────────────────────┘
```

### 4.2 统一接入（EventSink）

不管 webhook / channel / kafka，所有来源在 `Ingest` 层归一为同一内部事件，**先落 PG（权威），再投递投影**：

```go
type EventSink interface {
    Persist(ctx context.Context, e *TopologyEvent) error
}

// webhook / kafka consumer / channel consumer 都走这里
func (s *SyncService) Ingest(ctx context.Context, raw events.SyncEvent, source string) error {
    ev := normalizeToTopologyEvent(raw, source)   // 统一 schema
    if err := s.sink.Persist(ctx, ev); err != nil { // ① 先落 PG（权威）
        return err
    }
    return s.publisher.Publish(ctx, raw)            // ② 再投递投影
}
```

### 4.3 一致性模型（Outbox 模式）

**问题**：PG 写成功但 Publish 失败 → 事件丢失。
**方案**：`INSERT INTO topology_events` + `INSERT INTO outbox` 同一 PG 事务；relay 协程轮询 outbox → 投递 bus → 成功后标记已发。

收益：事件永不丢；`default` 可从 PG 完整重建（灾难恢复）。

### 4.4 回放路径（核心新能力）

```go
func Reconstruct(ctx context.Context, T time.Time) (*GraphModel, error) {
    snap := latestSnapshotBefore(T)              // 最近 checkpoint
    model := loadSnapshot(snap)                   // EnsureLoaded / YAML import
    events := queryEvents(ctx, snap.CreatedAt, T) // PG: event_time BETWEEN ...
    for _, ev := range events {                   // 按 event_time 顺序
        applyEvent(model, ev)                     // upsert/delete 原地折叠
    }
    return model
}
```

成本 `O(快照加载 + 窗口内事件数)`。FullSync 每小时一次 → 最大回放窗口 1 小时事件量，有界。

### 4.5 顺序与幂等

- **按 uri 有序**：PG `id` 给全局序；Kafka 模式 partition by uri 保证同节点有序
- **幂等**：`MERGE (_db, uri)` 天然幂等，重投安全；用 `(topic, partition, offset)` 或 PG `id` 去重
- **Schema 演进**：payload 用 JSONB + `schema_version` 字段，向后兼容

---

## 5. PG 事件表 Schema

```sql
CREATE TABLE topology_events (
    id             BIGSERIAL,
    event_time     TIMESTAMPTZ NOT NULL,
    ingest_time    TIMESTAMPTZ NOT NULL DEFAULT now(),
    source         TEXT NOT NULL,        -- webhook|kafka|channel|fullsync
    connector      TEXT NOT NULL,
    entity_type    TEXT NOT NULL,
    action         TEXT NOT NULL,        -- upsert|delete|delete_relation
    uri            TEXT,                 -- 目标节点（RCA 按节点查历史）
    payload        JSONB NOT NULL,       -- 完整事件数据
    schema_version INT NOT NULL DEFAULT 1,
    kafka_topic       TEXT,
    kafka_partition   INT,
    kafka_offset      BIGINT,            -- 溯源 + 重放定位 + 去重
    applied          BOOLEAN NOT NULL DEFAULT false, -- 投影是否已应用
    PRIMARY KEY (id, event_time)
) PARTITION BY RANGE (event_time);

CREATE INDEX ON topology_events (uri, event_time DESC);  -- RCA: 这个节点发生了什么
CREATE INDEX ON topology_events (event_time);            -- 回放: 时间窗内事件
CREATE INDEX ON topology_events (kafka_topic, kafka_partition, kafka_offset); -- 去重

CREATE TABLE event_outbox (
    id          BIGSERIAL PRIMARY KEY,
    event_id    BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    dispatched  BOOLEAN NOT NULL DEFAULT false
);
```

**自动建分区**：后台作业每日预建次日分区；过期分区 `DROP PARTITION`（DDL 级秒删）。

---

## 6. 容量与保留策略

### 6.1 只记真正变更（防爆炸的第一道防线）

**不记** Connector 周期 poll 的全量状态（那是 FullSync 的职责）。**只记**：
- 外部 push（webhook / kafka）的真实变更
- 增量 diff 出的实际变化

### 6.2 量级估算

| 参数 | 假设值 |
|------|--------|
| 峰值变更事件 | 500 事件/秒 |
| 保留窗口 | 7 天 |
| 单事件大小 | ~1 KB |
| 总量 | ~3 亿行 / ~300 GB |
| 按天分区 | 每天 ~43 GB，`DROP PARTITION` 秒级清理 |

若超容量：缩短保留到 3 天 / 冷分区列存压缩 / 对齐 RCA 实际窗口（通常 24–72h 足够）。

### 6.3 压实规则

事件保留窗口 = 最旧保留快照至今。FullSync 后建快照 → 旧快照 TTL 淘汰 → 连带 `DROP` 其之前的事件分区。PG 在线数据严格有界。

---

## 7. 对高级引擎的赋能

| 引擎 | V2 现状 | V3 解锁能力 |
|------|---------|------------|
| **Diff** | 仅比对两个手动快照 | 任意 T1/T2 时态 diff，不依赖是否拍过快照 |
| **Simulation** | 只能从当前态 fork（CloneDB 沙盒） | 从任意历史时刻 fork：「如果 T1 时改了 X 会怎样」 |
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
| **任意时刻 T 重建** | `loadSnapshot(T0) + 重放 (T0,T] 事件` | O(快照加载 + 窗口事件) 有界 | 时态回放、任意 T1/T2 diff |
| **RCA 按节点查变更** | **只查 PG** `WHERE uri=$u AND event_time BETWEEN` | O(该节点事件数) 索引命中 | "故障节点窗口内变了什么" |

> **RCA 不做全图重建**——它是按 uri 的 PG 索引查询（毫秒级）。把 RCA 做成"重建全图再分析"是性能灾难。

### 8.2 缓存分层（图态为主，结果缓存兜底）

| 缓存层 | 存什么 | 工具 | 引入时机 |
|--------|--------|------|---------|
| **图态缓存** | 完整历史快照逻辑 DB（可 Cypher 遍历） | **Neo4j LRU（现有 `maxActive`）** | 已有，V3 更关键 |
| **结果缓存** | `Reconstruct(T)` / diff 的序列化结果 | 进程内 LRU 或 Redis（可选） | V3 后期按需 |
| **查询缓存** | 热点 topology 查询结果 | Redis（可选） | 仅 Neo4j 成瓶颈时 |

> **Redis 不替代图态缓存**：历史态是图形状，RCA/Impact 要在其上做 BFS/DFS 邻居遍历，只能跑 Cypher。Redis 存序列化图会丧失图查询能力，反序列化开销也大。Redis 仅作可选的结果/查询缓存。

### 8.3 快照温度生命周期

```
Create(name)        → 写 YAML + metaCache + PG           【冷】
EnsureLoaded(name)  → YAML import → BulkCreate 进逻辑 DB  【热】
   （由 Restore / Diff / Reconstruct / PreWarm 触发）
LRU 淘汰(maxActive) → 逻辑 DB 清掉，YAML 保留              【回冷】
```

> **热不是 Create 的产物，是被查询的副作用。** V3 自动快照（FullSync 后）产出的也是冷 YAML。
> 可选 `snapshot.pre_warm_latest=true`：建快照后立即 EnsureLoaded，让"当前 vs 上次全量"高频 diff 零加载。

### 8.4 diff 路径决策表

| 场景 | 路径 | 成本 |
|------|------|------|
| 快照 vs 快照（都热） | Cypher 跨 `_db` diff | O(图大小) 索引命中 |
| 快照 vs 快照（冷，一次性） | LocalDiff（YAML，Go） | O(图大小) 无加载 |
| 快照 vs 任意 T | Reconstruct(T) 一次，比对快照 | O(重建 T + 比对) |
| 任意 T_a vs T_b（同基础快照） | **Reconstruct(T_a) + 折叠 (T_a,T_b] 事件比对端态** | O(重建 T_a + 窗口事件) |
| 节点变更历史（RCA） | PG 按 uri 查事件链 | O(该节点事件数) |

> **不引入 APOC**：Neo4j CE 无原生图 diff；现有"Cypher 跨 `_db` + Go `compareProps`"是最优解。`apoc.diff.maps` 只比对 map（属性），解决不了图结构 diff，边际收益小于引入依赖的成本。属性级 diff 统一走 `compareProps`（已实现，含数值归一化）。

### 8.5 flapping（抖动）处理

**端态比对天然吸收 flap；flap 次数单独作 RCA 指标。两者不混。**

场景：接口在 (T_a, T_b) 内 `up→down→up→down→up` 多次翻转，但两端都 `up`。
- **端态 diff**（`compareProps(stateA, stateB)`）：status 两端都 `up` → 净零 → **不进 diff**（正确）
- **raw event 列表 ≠ diff**：直接罗列事件会显示假变更。必须"折叠事件成终态，比对端态"

正确算法（任意时刻 diff）：
```
diffArbitrary(T_a, T_b):
    stateA = Reconstruct(T_a)
    stateB = clone(stateA)
    for ev in queryEvents(T_a, T_b] ordered by event_time, id:   # 严格时序
        apply(stateB, ev)
    return compareProps(stateA, stateB)   # 比对端态，非罗列事件
```

两个独立产出：

| 产出 | 含义 | 用途 |
|------|------|------|
| `Diff(T_a, T_b)` | 端态净变化（flap 折叠） | "两时刻有何不同" |
| `FlapCount(uri, window)` / `EventLog` | 原始事件 / 翻转次数 | RCA 征兆（不稳信号） |

规则：
- **存储不压缩**：保留窗口内全部原始事件，flap 查询时算；入库时合并会丢 RCA 信息
- **严格有序**：折叠按 `event_time` 升序，同时间戳同 uri 用 `id` 做 tiebreaker（高频遥测现实问题）

### 8.6 diff 策略：Cypher 主路径（A 方案）+ Local 降级

经 A/B 对比，**采用 A 为骨架**：Cypher 为主路径（已热跳加载、冷加载一次），LocalDiff 作显式选项和降级兜底。

| 维度 | A 加载优先（采用） | B Local 优先（否决作默认） |
|------|-------------------|--------------------------|
| 大图性能 | ✅ Cypher 走 `(_db,uri)` 索引 | ❌ Go 无索引全量扫 |
| 重复 diff | ✅ 加载一次后持续快 | 需显式预热才快 |
| 语义一致性 | ✅ 单主路径 | ❌ Cypher/Local 双路径须同步 |
| 解耦 / 韧性 | ❌ 依赖 Neo4j | ✅ 离线可用 |
| 一次性冷 diff | ❌ 双倍 import | ✅ 零加载 |

选 A 的理由：本系统核心是高级运维分析，diff 是 RCA/Simulation 的**高频**底层操作，重复调用是常态；大图下索引 Cypher 优于无索引 Go。B 的韧性优势用 `strategy=local` 显式覆盖即可，不必当默认。

### 8.7 service 层编排（机制与策略分离）

**机制留下层**（`SnapshotManager`）：`Diff`（Cypher）+ `LocalDiff`（Go）保留，只管"给定两快照怎么 diff"，不关心策略与健康度。
**策略上 service**：编排方法集中处理策略选择 + 降级 + 指标 + 预热。Handler/MCP 完全不感知 Neo4j 健康。

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
```

**Neo4j 健康判定**（`isNeo4jUnavailable`）：连接错误 / 超时 / 驱动关闭 → 降级；Cypher 语法 / 数据错误 → **不降级**（真 bug，降级会掩盖）。用 `errors.As` 匹配驱动哨兵类型（`ConnectivityError` / `TransientError`）。

---

## 9. 任务分解

### Phase 1: 事件存储（V3-01 ~ V3-03）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-01 | PG 事件表 Schema + 分区 + 迁移 | 2天 | 无 | `migrations/` + 分区自动维护作业 |
| V3-02 | TopologyEventRepository CRUD + 时间窗查询 | 1.5天 | V3-01 | `internal/repository/topology_event_repo.go` |
| V3-03 | 保留策略 + 压实作业（与快照 TTL 联动） | 1.5天 | V3-01, V3-02 | 压实后台任务 + 联动淘汰逻辑 |

- [ ] V3-01 `migrate up` 建表 + 首个分区，自动建分区作业工作
- [ ] V3-02 按 uri / 时间窗查询正确
- [ ] V3-03 旧快照淘汰时连带淘汰事件分区，PG 在线数据有界

### Phase 2: 统一接入与一致性（V3-04 ~ V3-05）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-04 | EventSink 抽象 + Ingest 统一入口 | 2天 | V3-02 | `SyncService.Ingest` 改造，webhook/channel/kafka 统一 |
| V3-05 | Outbox 模式 + relay 协程 | 2天 | V3-04 | `event_outbox` + relay 投递 + 幂等去重 |

- [ ] V3-04 三种来源都走 Ingest，事件全部落 PG
- [ ] V3-05 PG 写成功但 Publish 失败时，relay 重投成功，事件不丢

### Phase 3: 投影与自愈（V3-06 ~ V3-07）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-06 | main.go 接上 StartConsumer + default 投影降级为物化视图 | 1天 | V3-05 | 增量链路接通（修复 V2 接线缺口） |
| V3-07 | 从 PG 事件重建 default（自愈/灾备） | 1.5天 | V3-06 | `RebuildDefault` 工具/命令 |

- [ ] V3-06 webhook → PG → default 全链路贯通
- [ ] V3-07 清空 default 后能从 PG 事件完整重建

### Phase 4: 回放与自动快照（V3-08）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-08 | Reconstruct 时态查询 + FullSync 自动快照 | 2.5天 | V3-03, V3-07 | `Reconstruct(T)` 服务 + FullSync 后自动 Create |

- [ ] V3-08 任意时刻 T 拓扑可重建；FullSync 后自动产生 checkpoint 快照

### Phase 5: 验收（V3-09）

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| V3-09 | 集成测试 + 容量验证 + 文档归档 | 1.5天 | V3-01~V3-08 | 验收报告 + CLAUDE.md 更新 |

- [ ] V3-09 全部验收清单通过

---

## 10. 任务依赖关系

```
Phase 1 (事件存储)
V3-01 (Schema) → V3-02 (Repo) → V3-03 (压实)

Phase 2 (统一接入)
V3-02 → V3-04 (EventSink) → V3-05 (Outbox)

Phase 3 (投影)
V3-05 → V3-06 (StartConsumer) → V3-07 (自愈)

Phase 4 (回放)
V3-03 + V3-07 → V3-08 (Reconstruct + 自动快照)

Phase 5 (验收)
V3-01~V3-08 → V3-09
```

---

## 11. 工时汇总

| Phase | 任务范围 | 任务数 | 预估工时 |
|-------|----------|--------|---------|
| Phase 1: 事件存储 | V3-01 ~ V3-03 | 3 | 5 天 |
| Phase 2: 统一接入 | V3-04 ~ V3-05 | 2 | 4 天 |
| Phase 3: 投影自愈 | V3-06 ~ V3-07 | 2 | 2.5 天 |
| Phase 4: 回放 | V3-08 | 1 | 2.5 天 |
| Phase 5: 验收 | V3-09 | 1 | 1.5 天 |
| **合计** | | **9** | **15.5 天** |

**风险缓冲**：建议增加 20%，总计 **~18 天** (1人) / **~10 天** (2人并行)。

---

## 12. 验收清单

| # | 验收项 | 验证方法 | 通过标准 |
|---|--------|----------|---------|
| 1 | 编译 + lint | `go build ./... && golangci-lint run` | 无错误 |
| 2 | 事件持久化 | 发 webhook 事件 | PG `topology_events` 有记录 |
| 3 | 统一接入 | webhook / channel / kafka 三来源 | 都落 PG，下游一致 |
| 4 | Outbox 不丢 | PG 写成功后 Publish 失败 | relay 重投成功，default 最终一致 |
| 5 | 增量链路接通 | 发 webhook 事件 | default 物化视图更新（修复 V2 缺口） |
| 6 | 自愈重建 | 清空 default 后触发 Rebuild | 从 PG 事件完整还原 |
| 7 | 时态回放 | `Reconstruct(T)` | 任意历史时刻拓扑正确 |
| 8 | 自动快照 | 定时 FullSync 后 | 自动产生 checkpoint 快照 |
| 9 | 压实有界 | 跑过 TTL 周期 | 旧快照 + 其前事件分区连带淘汰，PG 在线数据不膨胀 |
| 10 | 性能 | 回放 P95 + 投影延迟 | 回放窗口 1h 内 P95 < 2s；投影延迟 < 5s |
| 11 | 向后兼容 | MCP 工具 + 现有 API | 功能不受影响 |
| 12 | RCA 就绪 | 按节点查变更链 | `queryEvents(uri, window)` 返回正确变更序列 |

---

## 13. 风险评估

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|---------|
| 事件表爆炸 | 高 | 高 | 只记变更（非轮询）+ 分区 + 压实联动（§6） |
| Outbox relay 成为瓶颈 | 中 | 中 | 批量投递 + 并发 relay；监控 outbox 积压深度 |
| 双写一致性（PG 与 default） | 中 | 高 | Outbox 模式 + default 可从 PG 重建（自愈） |
| 事件乱序 | 中 | 中 | 按 uri 分区有序 + MERGE 幂等兜底 |
| 回放计算量 | 低 | 中 | checkpoint + WAL 窗口有界（1h）；超窗走快照 diff |
| 迁移期数据缺口 | 中 | 中 | V3-06 先接通增量链路；灰度开启事件落盘 |

---

## 14. 决策清单（已拍板）

| 决策点 | 结论 |
|--------|------|
| 事件存储 | PG 分区表（权威源） |
| default 角色 | 物化投影（非权威，可重建） |
| 双 default | **不采用**，用 default + 自动快照 |
| Redis | 暂不引入（仅作可选读缓存） |
| 事件保留窗口 | 3–7 天（对齐 RCA 窗口，按容量测算调整） |
| Outbox | 采用（生产必备） |
| 事件入库范围 | 仅真实变更，排除周期 poll |
| 读路径 | 按查询类型分化；RCA 走 PG 索引，不做全图重建 |
| 缓存分层 | Neo4j LRU 图态缓存为主 + 结果缓存可选；Redis 不替代图态 |
| diff 策略 | Cypher 主路径（A 方案）；LocalDiff 作降级 / 显式 `strategy=local` |
| flapping | 端态比对吸收 flap；flap 次数作独立 RCA 指标；存储不压缩 |
| APOC | 不引入（现有 Cypher + Go `compareProps` 足够） |
| 快照预热 | 可选 `snapshot.pre_warm_latest` 预热最新快照 |
