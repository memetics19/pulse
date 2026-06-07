CREATE TABLE maintenance_windows (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    title                TEXT NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'scheduled',
    affected_monitor_ids TEXT NOT NULL DEFAULT '[]',
    starts_at            TIMESTAMP NOT NULL,
    ends_at              TIMESTAMP NOT NULL,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
