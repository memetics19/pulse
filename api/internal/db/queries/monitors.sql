-- name: ListMonitors :many
SELECT * FROM monitors ORDER BY created_at ASC;

-- name: ListMonitorsByGroup :many
SELECT * FROM monitors WHERE group_id = ? ORDER BY created_at ASC;

-- name: GetMonitor :one
SELECT * FROM monitors WHERE id = ?;

-- name: CreateMonitor :one
INSERT INTO monitors (name, url, type, interval_seconds, timeout_seconds,
  expected_status, keyword_check, degraded_threshold_ms, down_threshold_ms,
  is_active, group_id, source, external_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateMonitor :one
UPDATE monitors SET
  name = ?, url = ?, type = ?, interval_seconds = ?, timeout_seconds = ?,
  expected_status = ?, keyword_check = ?, degraded_threshold_ms = ?,
  down_threshold_ms = ?, is_active = ?, group_id = ?
WHERE id = ?
RETURNING *;

-- name: DeleteMonitor :exec
DELETE FROM monitors WHERE id = ?;

-- name: ListActiveMonitors :many
SELECT * FROM monitors WHERE is_active = 1;
