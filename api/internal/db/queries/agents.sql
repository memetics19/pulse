-- name: ListAgents :many
SELECT * FROM infra_agents ORDER BY created_at ASC;

-- name: GetAgent :one
SELECT * FROM infra_agents WHERE id = ?;

-- name: GetAgentByTokenHash :one
SELECT * FROM infra_agents WHERE token_hash = ?;

-- name: CreateAgent :one
INSERT INTO infra_agents (name, host_label, token_hash) VALUES (?, ?, ?) RETURNING *;

-- name: UpdateAgentLastSeen :exec
UPDATE infra_agents SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteAgent :exec
DELETE FROM infra_agents WHERE id = ?;
