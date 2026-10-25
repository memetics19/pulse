-- name: ListAgents :many
SELECT * FROM infra_agents ORDER BY created_at ASC;

-- name: GetAgent :one
SELECT * FROM infra_agents WHERE id = ?;

-- name: GetAgentByTokenHash :one
SELECT * FROM infra_agents WHERE token_hash = ?;

-- name: CreateAgent :one
INSERT INTO infra_agents (name, host_label, token_hash) VALUES (?, ?, ?) RETURNING *;

-- CreateAgentUniqueName inserts the agent only when no other agent has the
-- same name, ignoring case and surrounding spaces. One statement keeps the
-- check and the insert atomic. It returns no row when the name is taken.
-- name: CreateAgentUniqueName :one
INSERT INTO infra_agents (name, host_label, token_hash)
SELECT sqlc.arg(name), sqlc.arg(host_label), sqlc.arg(token_hash)
WHERE NOT EXISTS (
    SELECT 1 FROM infra_agents
    WHERE lower(trim(infra_agents.name)) = lower(trim(sqlc.arg(name)))
)
RETURNING *;

-- name: UpdateAgentLastSeen :exec
UPDATE infra_agents SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteAgent :exec
DELETE FROM infra_agents WHERE id = ?;
