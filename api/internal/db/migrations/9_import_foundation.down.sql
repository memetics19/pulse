-- pulse:foreign-keys-off-transaction

CREATE TABLE import_foundation_sequence_state (
    table_name TEXT PRIMARY KEY,
    seq        INTEGER NOT NULL
);

INSERT INTO import_foundation_sequence_state (table_name, seq)
SELECT name, seq
FROM sqlite_sequence
WHERE name IN ('monitors', 'monitor_groups');

CREATE TABLE import_foundation_group_identities (
    group_id    INTEGER PRIMARY KEY,
    source      TEXT NOT NULL,
    external_id TEXT NOT NULL
);

INSERT INTO import_foundation_group_identities (group_id, source, external_id)
SELECT id, source, external_id
FROM monitor_groups
WHERE source <> 'internal' OR external_id <> '';

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

UPDATE sqlite_sequence
SET seq = MAX(seq, (SELECT seq FROM import_foundation_sequence_state
                    WHERE table_name = 'monitors'))
WHERE name = 'monitors'
  AND EXISTS (SELECT 1 FROM import_foundation_sequence_state
              WHERE table_name = 'monitors');

INSERT INTO sqlite_sequence (name, seq)
SELECT 'monitors', seq
FROM import_foundation_sequence_state
WHERE table_name = 'monitors'
  AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'monitors');

UPDATE sqlite_sequence
SET seq = MAX(seq, (SELECT seq FROM import_foundation_sequence_state
                    WHERE table_name = 'monitor_groups'))
WHERE name = 'monitor_groups'
  AND EXISTS (SELECT 1 FROM import_foundation_sequence_state
              WHERE table_name = 'monitor_groups');

INSERT INTO sqlite_sequence (name, seq)
SELECT 'monitor_groups', seq
FROM import_foundation_sequence_state
WHERE table_name = 'monitor_groups'
  AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'monitor_groups');

DROP TABLE import_foundation_sequence_state;
