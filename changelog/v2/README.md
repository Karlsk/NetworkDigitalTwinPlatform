# V2 研发计划 — 基础设施全面升级

> 基于 V1 全部 22 项任务 + V1.2 全部 4 项任务完成，V2 聚焦 **基础设施升级**：
> 1. Kafka 事件流（替代内存 Channel 缓冲）
> 2. PostgreSQL 元数据存储（替代内存/YAML 元数据）
> 3. Gin HTTP REST API（扩展 MCP 之外的访问通道）
> 4. Prometheus + OpenTelemetry 可观测性
> 5. golangci-lint CI/CD 集成

**总任务数**: 18 项 (V2-01 ~ V2-18)
**预估工时**: 22 人天（含缓冲 26 天）
**架构设计**: 详见 [V1架构设计.md](../../docs/V1架构设计.md) 第 9 节技术债务清单 + [V1扩展方向.md](../../docs/V1扩展方向.md) 第 4-9 章

---

## 版本范围

| V2 纳入 | 推迟到 V3 |
|---------|----------|
| Kafka 事件流（替代内存 Channel） | Impact/RCA/Simulation 分析引擎 |
| PostgreSQL 元数据存储 | Neo4j Enterprise 真多 DB 迁移 |
| Gin HTTP REST API | 语义映射规则外置（按需） |
| Prometheus + OpenTelemetry 可观测性 | |
| golangci-lint CI/CD 集成 | |

### V3 预留说明

以下功能明确推迟到 V3 实现：

- **Impact Engine**: `internal/engine/impact.go` 保持 `ErrNotImplemented` 骨架，V3 实现基于图遍历的 BFS/DFS 影响分析
- **RCA Engine**: `internal/engine/rca.go` 保持 `ErrNotImplemented` 骨架，V3 实现基于图传播的确定性根因分析
- **Simulation Engine**: `internal/engine/simulation.go` 保持 `ErrNotImplemented` 骨架，V3 实现基于快照沙盒的操作仿真
- **Neo4j Enterprise 迁移**: 驱动层 `_db` 属性隔离保持不变，迁移到 Enterprise 时仅改驱动层实现
- **语义映射规则外置**: 当前规则量可控（每个 EntityType 的 normalize 规则 < 10 条），待规则膨胀后再拆分 `mapping_rules.yaml`

---

## 里程碑

| 里程碑 | 目标 | 预计时间 | 验收标准 | 任务范围 |
|--------|------|---------|---------|---------|
| **V2-M1: 消息持久化** | Kafka 替代内存 Channel | 第 1-2 周 | Kafka Producer/Consumer 全链路贯通，进程重启不丢消息 | V2-01 ~ V2-04 |
| **V2-M2: 元数据持久化** | PostgreSQL 替代内存/YAML 元数据 | 第 2-3 周 | 快照/同步/连接器/审计元数据全部持久化，进程重启数据完整 | V2-05 ~ V2-10 |
| **V2-M3: API 服务化** | Gin HTTP API 上线 | 第 3-4 周 | 8 个 REST 端点可用，Swagger 文档自动生成 | V2-11 ~ V2-14 |
| **V2-M4: 可观测 + CI** | Prometheus/OTel + GitHub Actions | 第 4-5 周 | /metrics 可抓取，链路追踪可见，CI 流水线全绿 | V2-15 ~ V2-17 |
| **V2-M5: 验收** | 全量测试 + 文档归档 | 第 5 周 | 15 项验收清单全部通过 | V2-18 |

---

## Phase 1: Kafka 事件流 (V2-01 ~ V2-04)

> 目标：内存 Channel 替换为 Kafka Consumer Group，获得消息持久化 + 重试 + 多消费者并行

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V2-01](V2-01_Kafka依赖与接口抽象.md) | Kafka 依赖引入 + 事件接口抽象 | 1天 | 无 | `internal/events/interface.go` + `go.mod` 新增依赖 |
| [V2-02](V2-02_Kafka事件生产者.md) | Kafka Producer 实现 + HandleWebhook 改造 | 1天 | V2-01 | `internal/events/kafka_producer.go` + SyncService 改造 |
| [V2-03](V2-03_Kafka事件消费者.md) | Kafka Consumer Group 实现 + StartConsumer 改造 | 1.5天 | V2-02 | `internal/events/kafka_consumer.go` + 消费循环改造 |
| [V2-04](V2-04_Kafka集成测试与配置.md) | Kafka 集成测试 + 配置扩展 + Fallback | 1.5天 | V2-03 | testcontainers 测试 + config.yaml 扩展 + Channel Fallback |

- [ ] V2-01 `EventPublisher` / `EventConsumer` 接口编译通过
- [ ] V2-02 `HandleWebhook` 通过 Kafka Producer 发送消息成功
- [ ] V2-03 Kafka Consumer Group 消费消息并调用 `IncrementalSync` 成功
- [ ] V2-04 进程重启后 Kafka 中的消息仍可被消费，Channel Fallback 正常工作

---

## Phase 2: PostgreSQL 元数据存储 (V2-05 ~ V2-10)

> 目标：引入 PostgreSQL 存储结构化元数据，替代内存/YAML 的脆弱存储

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V2-05](V2-05_PostgreSQL依赖与Schema设计.md) | PostgreSQL 依赖引入 + Schema DDL + 迁移工具 | 1.5天 | 无 | `migrations/` 目录 + `internal/repository/pg.go` |
| [V2-06](V2-06_SnapshotRepository.md) | SnapshotRepository 快照元数据 CRUD | 1.5天 | V2-05 | `internal/repository/snapshot_repo.go` |
| [V2-07](V2-07_SyncLogRepository.md) | SyncLogRepository 同步历史 + SyncService 集成 | 1天 | V2-05 | `internal/repository/sync_log_repo.go` |
| [V2-08](V2-08_ConnectorConfigRepository.md) | ConnectorConfigRepository + Schema 版本追踪 | 1天 | V2-05 | `internal/repository/connector_repo.go` |
| [V2-09](V2-09_AuditLog持久化.md) | AuditLog 从内存 FIFO 迁移到 PostgreSQL | 1天 | V2-05 | `internal/repository/audit_repo.go` + AuditLog 改造 |
| [V2-10](V2-10_PostgreSQL集成测试与迁移.md) | PostgreSQL 集成测试 + docker-compose + 数据迁移 | 1天 | V2-06~V2-09 | testcontainers 测试 + docker-compose 更新 |

- [ ] V2-05 `migrate up` 成功创建 4 张表
- [ ] V2-06 SnapshotRepository CRUD 操作正确
- [ ] V2-07 SyncLog 记录 FullSync/IncrementalSync 结果
- [ ] V2-08 ConnectorConfig CRUD + SchemaVersion 记录
- [ ] V2-09 AuditLog 持久化到 PostgreSQL，进程重启后审计记录完整
- [ ] V2-10 docker-compose 启动 PostgreSQL，集成测试全部通过

---

## Phase 3: Gin HTTP API (V2-11 ~ V2-14)

> 目标：Gin 框架暴露 REST API，与 MCP Server 并行提供服务

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V2-11](V2-11_Gin框架与路由骨架.md) | Gin 框架引入 + 路由注册 + 启动集成 | 1天 | V2-10 | `internal/api/server.go` + `cmd/server/main.go` 改造 |
| [V2-12](V2-12_Sync与Snapshot端点.md) | POST/GET sync + snapshot CRUD 端点 | 1.5天 | V2-11, V2-10 | `internal/api/handlers/sync.go` + `snapshot.go` |
| [V2-13](V2-13_Topology与Device端点.md) | GET topology + device + monitor + health 端点 | 1.5天 | V2-11 | `internal/api/handlers/topology.go` + `device.go` |
| [V2-14](V2-14_API中间件与文档.md) | CORS/RequestID/Logging 中间件 + Swagger 文档 | 1天 | V2-12, V2-13 | 中间件 + Swagger JSON 自动生成 |

- [ ] V2-11 Gin Server 启动，`GET /api/v1/health` 返回 200
- [ ] V2-12 `POST /api/v1/sync` 触发全量同步，`GET/POST/DELETE /api/v1/snapshot` CRUD 正确
- [ ] V2-13 `GET /api/v1/topology` + device + monitor 端点返回正确数据
- [ ] V2-14 Swagger UI 可访问，CORS/RequestID 中间件工作正常

---

## Phase 4: 可观测性 + CI/CD (V2-15 ~ V2-17)

> 目标：Prometheus 指标暴露 + OpenTelemetry 分布式追踪 + GitHub Actions 自动化

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V2-15](V2-15_Prometheus指标集成.md) | Prometheus 指标定义 + Gin middleware + /metrics 端点 | 1.5天 | V2-11 | `internal/metrics/` + Gin 中间件 |
| [V2-16](V2-16_OpenTelemetry分布式追踪.md) | OTel SDK 集成 + span 注入 + 关键路径追踪 | 1天 | V2-15 | `internal/tracing/` + 全链路追踪 |
| [V2-17](V2-17_CI_CD流水线.md) | GitHub Actions: lint + test + coverage + build | 1天 | 无（独立并行） | `.github/workflows/ci.yml` |

- [ ] V2-15 `GET /metrics` 返回 Prometheus 格式指标
- [ ] V2-16 Jaeger/Tempo 可见完整请求链路追踪
- [ ] V2-17 push 触发 CI，lint + test + coverage 全绿

---

## Phase 5: 验收 (V2-18)

> 目标：全量回归测试 + 验收清单 + 文档归档

| 任务ID | 任务名称 | 工时 | 前置 | 交付物 |
|--------|---------|------|------|--------|
| [V2-18](V2-18_V2全量测试与验收.md) | V2 全量测试 + 验收 + 文档归档 | 1.5天 | V2-01~V2-17 | 验收报告 + AGENTS.md 更新 |

- [ ] V2-18 15 项验收清单全部通过

---

## 新增依赖

| 依赖 | 用途 | 引入任务 |
|------|------|---------|
| `github.com/IBM/sarama` | Kafka 客户端（Producer + Consumer Group） | V2-01 |
| `github.com/jackc/pgx/v5` | PostgreSQL 驱动（连接池） | V2-05 |
| `github.com/golang-migrate/migrate/v4` | DB Schema 版本化迁移 | V2-05 |
| `github.com/gin-gonic/gin` | HTTP REST API 框架 | V2-11 |
| `github.com/prometheus/client_golang` | Prometheus 指标采集 | V2-15 |
| `go.opentelemetry.io/otel` | OpenTelemetry 分布式追踪 SDK | V2-16 |
| `github.com/testcontainers/testcontainers-go` | 集成测试容器（Kafka/PostgreSQL） | V2-04, V2-10 |

---

## 任务依赖关系图

```
Phase 1 (Kafka 事件流)
V2-01 (接口抽象) → V2-02 (Producer) → V2-03 (Consumer) → V2-04 (集成测试)

Phase 2 (PostgreSQL 元数据)
V2-05 (Schema) → V2-06 (SnapshotRepo) ─┐
                V2-07 (SyncLogRepo) ───┤
                V2-08 (ConnectorRepo) ─┤→ V2-10 (集成测试)
                V2-09 (AuditRepo) ─────┘

Phase 3 (Gin HTTP API)
V2-11 (骨架) → V2-12 (Sync/Snap) → V2-13 (Topology/Device) → V2-14 (中间件)
V2-10 (PostgreSQL就绪) ─→ V2-12~V2-13 (端点使用 PG Repository)

Phase 4 (可观测性 + CI/CD)
V2-11 → V2-15 (Prometheus) → V2-16 (OTel)
V2-17 (CI/CD，独立并行)

Phase 5 (验收)
V2-01~V2-17 → V2-18 (全量测试 + 验收)
```

**2 人并行方案**（推荐）：

```
Week 1        Week 2        Week 3        Week 4        Week 5
├─────────────┼─────────────┼─────────────┼─────────────┤
│ Person A    │ Person A    │ Person A    │ Person A    │
│ V2-01~V2-02 │ V2-05~V2-06 │ V2-11~V2-12 │ V2-15~V2-16 │
│ V2-03~V2-04 │ V2-07~V2-08 │ V2-13~V2-14 │ V2-18       │
│             │              │              │              │
│ Person B    │ Person B    │ Person B    │ Person B    │
│ V2-05       │ V2-09~V2-10 │ V2-17 (CI)  │             │
│ (PG Schema) │              │              │             │
├─────────────┼─────────────┼─────────────┼─────────────┤
  V2-M1         V2-M2         V2-M3         V2-M4+M5
```

---

## 工时汇总

| Phase | 任务范围 | 任务数 | 预估工时 |
|-------|----------|--------|---------|
| Phase 1: Kafka 事件流 | V2-01 ~ V2-04 | 4 | 5 天 |
| Phase 2: PostgreSQL 元数据 | V2-05 ~ V2-10 | 6 | 7 天 |
| Phase 3: Gin HTTP API | V2-11 ~ V2-14 | 4 | 5 天 |
| Phase 4: 可观测性 + CI/CD | V2-15 ~ V2-17 | 3 | 3.5 天 |
| Phase 5: 验收 | V2-18 | 1 | 1.5 天 |
| **合计** | | **18** | **22 天** |

**风险缓冲**: 建议增加 20%，总计 **26 天** (1人) / **15 天** (2人并行)。

---

## V2 验收清单

| # | 验收项 | 验证方法 | 通过标准 |
|---|--------|----------|---------|
| 1 | 编译通过 | `go build ./...` | 无错误 |
| 2 | Lint 通过 | `golangci-lint run` | 无 Error |
| 3 | 单元测试全部通过 | `go test ./...` | 0 failures |
| 4 | Race 检测 | `go test -race ./...` | 无 data race |
| 5 | 覆盖率 | `go test -cover ./...` | >= 70% |
| 6 | Kafka 消息持久化 | 杀进程重启 | 消息不丢失，Consumer 继续消费 |
| 7 | Kafka Fallback | Kafka 不可用 | 自动降级到内存 Channel |
| 8 | PostgreSQL 元数据 | CRUD 测试 | 快照/同步/连接器/审计数据正确持久化 |
| 9 | 审计日志持久化 | 进程重启 | AuditLog 重启后历史记录可查 |
| 10 | HTTP API 可用性 | curl/httptest | 8 个端点返回正确状态码和数据 |
| 11 | Swagger 文档 | 浏览器访问 | Swagger UI 可访问，所有端点有文档 |
| 12 | Prometheus 指标 | `curl /metrics` | 返回 Prometheus 格式，关键指标可见 |
| 13 | 分布式追踪 | Jaeger UI | 可见完整请求链路（HTTP→Service→GraphDB） |
| 14 | CI/CD 流水线 | GitHub Actions | push 触发 lint+test+build 全绿 |
| 15 | 向后兼容 | MCP Server | MCP 工具功能不受影响，MCP 与 HTTP API 并行 |

---

## V1 技术债偿还清单

| 债务项 | V1 状态 | V2 解决方案 | V2 任务 |
|--------|---------|------------|--------|
| `Connector.Stream()` 未实现 | `ErrNotImplemented` | Kafka Consumer 适配 | V2-03 |
| 内存 Channel 缓冲 | 进程重启丢失 | Kafka 持久化 + Fallback | V2-01~V2-04 |
| 无 HTTP REST API | 仅 MCP | Gin HTTP API | V2-11~V2-14 |
| 无 PostgreSQL 元数据 | YAML + 内存 | PostgreSQL 持久化 | V2-05~V2-10 |
| AuditLog 内存存储 | FIFO 淘汰 | PostgreSQL 持久化 | V2-09 |
| golangci-lint CI 集成 | 本地手动运行 | GitHub Actions 自动化 | V2-17 |
| 无可观测性 | 仅 slog 日志 | Prometheus + OpenTelemetry | V2-15~V2-16 |

---

## 风险评估

| 风险 | 概率 | 影响 | 应对措施 |
|------|------|------|---------|
| Kafka 集群不可用导致服务无法启动 | 高 | 高 | V2-04 实现 Channel Fallback：Kafka 不可用时自动降级到内存 Channel |
| PostgreSQL 连接失败影响核心功能 | 中 | 高 | Repository 层抽象接口，支持 InMemory 实现作为 Fallback |
| Gin 与现有 MCP Server 端口冲突 | 低 | 中 | Gin 监听不同端口（如 8081），或通过统一路由复用同一端口 |
| Kafka Consumer Group Rebalance 导致消息延迟 | 中 | 低 | 合理配置 Session.Timeout / Heartbeat.Interval，消费幂等设计 |
| 迁移数据丢失 | 低 | 高 | V2-10 提供数据迁移工具，迁移前自动备份，迁移后校验一致性 |
| OTel 性能开销 | 低 | 低 | 采样率可配（默认 10%），生产环境按需调整 |
