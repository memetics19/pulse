-- A diagnostic bundle is evidence a specific agent collected about its own
-- host. It deliberately carries no incident link: an agent cannot be
-- authorized to claim an arbitrary incident, and there is no server-owned
-- association to validate one against yet.
CREATE TABLE agent_diagnostics (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    agent_id     INTEGER NOT NULL REFERENCES infra_agents(id) ON DELETE CASCADE,
    collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    payload      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_agent_diagnostics_agent ON agent_diagnostics(agent_id, collected_at DESC);
