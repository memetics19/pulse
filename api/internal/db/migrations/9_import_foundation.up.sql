PRAGMA foreign_keys = OFF;

CREATE TABLE monitors_new (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    name                  TEXT NOT NULL,
    url                   TEXT NOT NULL DEFAULT '',
    type                  TEXT NOT NULL CHECK(type IN ('http','https','tcp','ping','dns','ssl','infra','push')),
    interval_seconds      INTEGER NOT NULL DEFAULT 60,
    timeout_seconds       INTEGER NOT NULL DEFAULT 10,
    expected_status       INTEGER,
    keyword_check         TEXT NOT NULL DEFAULT '',
    degraded_threshold_ms INTEGER NOT NULL DEFAULT 500,
    down_threshold_ms     INTEGER NOT NULL DEFAULT 2000,
    is_active             INTEGER NOT NULL DEFAULT 1,
    group_id              INTEGER REFERENCES monitor_groups(id) ON DELETE SET NULL,
    source                TEXT NOT NULL DEFAULT 'internal',
    external_id           TEXT NOT NULL DEFAULT '',
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO monitors_new
SELECT id, name, url, type, interval_seconds, timeout_seconds, expected_status,
       keyword_check, degraded_threshold_ms, down_threshold_ms, is_active,
       group_id, source, external_id, created_at
FROM monitors;

DROP TABLE monitors;
ALTER TABLE monitors_new RENAME TO monitors;

ALTER TABLE monitor_groups ADD COLUMN source TEXT NOT NULL DEFAULT 'internal';
ALTER TABLE monitor_groups ADD COLUMN external_id TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_monitors_source_external
ON monitors(source, external_id) WHERE external_id <> '';

CREATE UNIQUE INDEX idx_groups_source_external
ON monitor_groups(source, external_id) WHERE external_id <> '';

CREATE TABLE push_monitor_tokens (
    monitor_id  INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    prefix      TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at  DATETIME
);

CREATE TABLE import_runs (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    source             TEXT NOT NULL,
    source_version     TEXT NOT NULL,
    input_hash         TEXT NOT NULL,
    idempotency_key    TEXT NOT NULL UNIQUE,
    conflict_policy    TEXT NOT NULL CHECK(conflict_policy IN ('fail','skip','update')),
    status             TEXT NOT NULL CHECK(status IN ('running','completed','failed')),
    plan_hash          TEXT NOT NULL DEFAULT '',
    summary_json       TEXT NOT NULL DEFAULT '{}',
    error_summary      TEXT NOT NULL DEFAULT '',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at       DATETIME
);

PRAGMA foreign_keys = ON;
