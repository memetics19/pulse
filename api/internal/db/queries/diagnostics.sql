-- name: InsertAgentDiagnostic :exec
INSERT INTO agent_diagnostics (agent_id, collected_at, payload)
VALUES (?, ?, ?);

-- name: ListAgentDiagnostics :many
SELECT * FROM agent_diagnostics
WHERE agent_id = ?
ORDER BY collected_at DESC
LIMIT ?;

-- name: PruneAgentDiagnostics :execrows
DELETE FROM agent_diagnostics WHERE id IN (
  SELECT old.id FROM agent_diagnostics AS old WHERE old.collected_at < sqlc.arg(cutoff)
  LIMIT sqlc.arg(batch_size)
);
