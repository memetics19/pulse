DELETE FROM monitors WHERE type = 'push';
DROP TABLE IF EXISTS push_monitor_tokens;
DROP TABLE IF EXISTS import_runs;
DROP INDEX IF EXISTS idx_monitors_source_external;
DROP INDEX IF EXISTS idx_groups_source_external;

PRAGMA foreign_keys = OFF;
CREATE TABLE monitors_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL CHECK(type IN ('http','tcp','ping','dns','ssl','infra')),
    interval_seconds INTEGER NOT NULL DEFAULT 60,
    timeout_seconds INTEGER NOT NULL DEFAULT 10,
    expected_status INTEGER,
    keyword_check TEXT NOT NULL DEFAULT '',
    degraded_threshold_ms INTEGER NOT NULL DEFAULT 500,
    down_threshold_ms INTEGER NOT NULL DEFAULT 2000,
    is_active INTEGER NOT NULL DEFAULT 1,
    group_id INTEGER REFERENCES monitor_groups(id) ON DELETE SET NULL,
    source TEXT NOT NULL DEFAULT 'internal',
    external_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO monitors_old SELECT * FROM monitors;
DROP TABLE monitors;
ALTER TABLE monitors_old RENAME TO monitors;
PRAGMA foreign_keys = ON;
