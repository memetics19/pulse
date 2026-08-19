-- name: InsertAgentDiagnostic :exec
INSERT INTO agent_diagnostics (agent_id, collected_at, payload)
VALUES (?, ?, ?);

-- name: ListAgentDiagnostics :many
SELECT * FROM agent_diagnostics
WHERE agent_id = ?
ORDER BY collected_at DESC
LIMIT ?;

-- name: PruneAgentDiagnostics :exec
DELETE FROM agent_diagnostics WHERE collected_at < ?;
