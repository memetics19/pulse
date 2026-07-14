PRAGMA foreign_keys = OFF;

CREATE TABLE monitor_groups_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    description   TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO monitor_groups_old (id, name, display_order, description, created_at)
SELECT id, name, display_order, description, created_at
FROM monitor_groups;

CREATE TABLE monitors_old (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    name                  TEXT NOT NULL,
    url                   TEXT NOT NULL DEFAULT '',
    type                  TEXT NOT NULL CHECK(type IN ('http','tcp','ping','dns','ssl','infra')),
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

INSERT INTO monitors_old
SELECT id, name, url, CASE type WHEN 'https' THEN 'http' ELSE type END,
       interval_seconds, timeout_seconds, expected_status, keyword_check,
       degraded_threshold_ms, down_threshold_ms, is_active, group_id, source,
       external_id, created_at
FROM monitors
WHERE type <> 'push';

DELETE FROM check_results
WHERE monitor_id IN (SELECT id FROM monitors WHERE type = 'push');
DELETE FROM ssl_checks
WHERE monitor_id IN (SELECT id FROM monitors WHERE type = 'push');
DELETE FROM notifications
WHERE monitor_id IN (SELECT id FROM monitors WHERE type = 'push');

DROP TABLE push_monitor_tokens;
DROP TABLE import_runs;
DROP INDEX idx_monitors_source_external;
DROP INDEX idx_groups_source_external;

DROP TABLE monitors;
ALTER TABLE monitors_old RENAME TO monitors;
DROP TABLE monitor_groups;
ALTER TABLE monitor_groups_old RENAME TO monitor_groups;

PRAGMA foreign_keys = ON;
