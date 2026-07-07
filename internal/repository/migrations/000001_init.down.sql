-- V2-05: PostgreSQL Schema DDL — 回滚迁移
-- 按依赖逆序 DROP 5 张表

DROP TABLE IF EXISTS schema_versions;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS connector_configs;
DROP TABLE IF EXISTS sync_logs;
DROP TABLE IF EXISTS snapshots;
