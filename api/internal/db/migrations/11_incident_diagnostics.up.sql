CREATE TABLE incident_diagnostics (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    -- Optional: an on-demand bundle (pulse-agent --diagnose) belongs to a
    -- host at a moment in time, not necessarily to an incident.
    incident_id  INTEGER REFERENCES incidents(id) ON DELETE CASCADE,
    agent_id     INTEGER NOT NULL REFERENCES infra_agents(id) ON DELETE CASCADE,
    collected_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    payload      TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_incident_diagnostics_incident
ON incident_diagnostics(incident_id, collected_at DESC);
