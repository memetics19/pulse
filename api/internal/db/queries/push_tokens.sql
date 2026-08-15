-- name: GetPushTokenByHash :one
SELECT * FROM push_monitor_tokens WHERE token_hash = ?;

-- name: GetPushTokenByMonitor :one
SELECT * FROM push_monitor_tokens WHERE monitor_id = ?;

-- name: UpsertPushToken :one
INSERT INTO push_monitor_tokens (monitor_id, token_hash, prefix)
VALUES (?, ?, ?)
ON CONFLICT(monitor_id) DO UPDATE SET
  token_hash = excluded.token_hash,
  prefix = excluded.prefix,
  rotated_at = CURRENT_TIMESTAMP
RETURNING *;
