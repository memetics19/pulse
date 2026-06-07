-- name: CreateAPIKey :one
INSERT INTO api_keys (name, key_hash, prefix, scopes) VALUES (?, ?, ?, ?) RETURNING *;

-- name: ListAPIKeys :many
SELECT * FROM api_keys WHERE revoked_at IS NULL ORDER BY created_at DESC;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = ? WHERE id = ?;
