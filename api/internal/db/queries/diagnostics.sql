-- name: InsertIncidentDiagnostic :exec
INSERT INTO incident_diagnostics (incident_id, agent_id, collected_at, payload)
VALUES (?, ?, ?, ?);

-- name: ListIncidentDiagnostics :many
SELECT * FROM incident_diagnostics
WHERE incident_id = ?
ORDER BY collected_at DESC;

-- name: ListAgentDiagnostics :many
SELECT * FROM incident_diagnostics
WHERE agent_id = ?
ORDER BY collected_at DESC
LIMIT ?;

-- name: PruneIncidentDiagnostics :exec
DELETE FROM incident_diagnostics WHERE collected_at < ?;
