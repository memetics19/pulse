CREATE TABLE monitor_groups (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    description   TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE monitors (
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

CREATE TABLE check_results (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    monitor_id       INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    checked_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status           TEXT NOT NULL CHECK(status IN ('up','down','degraded')),
    response_time_ms INTEGER,
    status_code      INTEGER,
    error_message    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_check_results_monitor_checked ON check_results(monitor_id, checked_at DESC);

CREATE TABLE ssl_checks (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    monitor_id        INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    checked_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cert_expiry_date  DATETIME,
    days_until_expiry INTEGER,
    issuer            TEXT NOT NULL DEFAULT '',
    subject           TEXT NOT NULL DEFAULT '',
    is_valid          INTEGER NOT NULL DEFAULT 0,
    error_message     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_ssl_checks_monitor ON ssl_checks(monitor_id, checked_at DESC);

CREATE TABLE infra_agents (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    host_label   TEXT NOT NULL,
    token        TEXT NOT NULL UNIQUE,
    last_seen_at DATETIME,
    is_active    INTEGER NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE infra_metrics_raw (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id     INTEGER NOT NULL REFERENCES infra_agents(id) ON DELETE CASCADE,
    collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    cpu_percent  REAL NOT NULL DEFAULT 0,
    mem_percent  REAL NOT NULL DEFAULT 0,
    mem_used_mb  REAL NOT NULL DEFAULT 0,
    disk_percent REAL NOT NULL DEFAULT 0,
    disk_used_gb REAL NOT NULL DEFAULT 0,
    net_rx_bytes INTEGER NOT NULL DEFAULT 0,
    net_tx_bytes INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_infra_metrics_agent ON infra_metrics_raw(agent_id, collected_at DESC);

CREATE TABLE infra_metrics_1m (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id     INTEGER NOT NULL REFERENCES infra_agents(id) ON DELETE CASCADE,
    bucket_at    DATETIME NOT NULL,
    cpu_percent  REAL NOT NULL DEFAULT 0,
    mem_percent  REAL NOT NULL DEFAULT 0,
    disk_percent REAL NOT NULL DEFAULT 0,
    net_rx_bytes INTEGER NOT NULL DEFAULT 0,
    net_tx_bytes INTEGER NOT NULL DEFAULT 0,
    UNIQUE(agent_id, bucket_at)
);

CREATE TABLE incidents (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    title                TEXT NOT NULL,
    severity             TEXT NOT NULL CHECK(severity IN ('minor','major','critical')),
    status               TEXT NOT NULL CHECK(status IN ('detected','investigating','identified','implementing','monitoring','resolved')),
    affected_monitor_ids TEXT NOT NULL DEFAULT '[]',
    started_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at          DATETIME,
    rca                  TEXT,
    source               TEXT NOT NULL DEFAULT 'internal',
    external_id          TEXT NOT NULL DEFAULT '',
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE incident_updates (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    incident_id INTEGER NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    status      TEXT NOT NULL,
    message     TEXT NOT NULL,
    author      TEXT NOT NULL DEFAULT 'admin',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_incident_updates_incident ON incident_updates(incident_id, created_at DESC);

CREATE TABLE notifications (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    channel     TEXT NOT NULL CHECK(channel IN ('email','slack')),
    config_json TEXT NOT NULL DEFAULT '{}',
    monitor_id  INTEGER REFERENCES monitors(id) ON DELETE CASCADE,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE theme_config (
    id          INTEGER PRIMARY KEY CHECK(id = 1),
    preset      TEXT NOT NULL DEFAULT 'default-light',
    custom_css  TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO theme_config (id, preset) VALUES (1, 'default-light');
