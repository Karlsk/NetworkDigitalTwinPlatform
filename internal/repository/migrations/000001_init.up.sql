-- V2-05: PostgreSQL Schema DDL — 初始化迁移
-- 包含: snapshots, sync_logs, connector_configs, audit_logs, schema_versions

-- 快照元数据
CREATE TABLE IF NOT EXISTS snapshots (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    node_count  INTEGER NOT NULL DEFAULT 0,
    rel_count   INTEGER NOT NULL DEFAULT 0,
    file_path   VARCHAR(1024) NOT NULL,
    status      VARCHAR(32) NOT NULL DEFAULT 'active',
    metadata    JSONB DEFAULT '{}'
);

CREATE INDEX idx_snapshots_created_at ON snapshots(created_at);
CREATE INDEX idx_snapshots_status ON snapshots(status);

-- 同步历史日志
CREATE TABLE IF NOT EXISTS sync_logs (
    id              BIGSERIAL PRIMARY KEY,
    sync_type       VARCHAR(32) NOT NULL,          -- "full" / "incremental"
    status          VARCHAR(32) NOT NULL,          -- "success" / "failed"
    nodes_created   INTEGER NOT NULL DEFAULT 0,
    relations_created INTEGER NOT NULL DEFAULT 0,
    orphan_edges    INTEGER NOT NULL DEFAULT 0,
    warnings        JSONB DEFAULT '[]',
    error_message   TEXT,
    started_at      TIMESTAMPTZ NOT NULL,
    completed_at    TIMESTAMPTZ,
    duration_ms     BIGINT
);

CREATE INDEX idx_sync_logs_started_at ON sync_logs(started_at);
CREATE INDEX idx_sync_logs_type ON sync_logs(sync_type);

-- 连接器配置
CREATE TABLE IF NOT EXISTS connector_configs (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL UNIQUE,
    type        VARCHAR(64) NOT NULL,              -- "mock" / "netbox" / "controller" / "cmdb"
    config      JSONB NOT NULL DEFAULT '{}',
    entity_types JSONB NOT NULL DEFAULT '[]',
    priority    INTEGER NOT NULL DEFAULT 0,
    status      VARCHAR(32) NOT NULL DEFAULT 'active',
    last_ping   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_connector_configs_type ON connector_configs(type);

-- 审计日志
CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    action      VARCHAR(64) NOT NULL,              -- "create" / "restore" / "delete" / "diff" / "sync"
    snapshot    VARCHAR(255),
    actor       VARCHAR(64) NOT NULL,              -- "mcp" / "webhook" / "system" / "http_api"
    detail      TEXT,
    error       TEXT
);

CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_snapshot ON audit_logs(snapshot);

-- Schema 版本追踪
CREATE TABLE IF NOT EXISTS schema_versions (
    id              BIGSERIAL PRIMARY KEY,
    version         INTEGER NOT NULL UNIQUE,
    entity_types    JSONB NOT NULL DEFAULT '[]',
    relation_types  JSONB NOT NULL DEFAULT '[]',
    applied_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    description     TEXT
);
