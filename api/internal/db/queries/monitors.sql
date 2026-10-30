-- name: ListMonitors :many
SELECT * FROM monitors ORDER BY created_at ASC;

-- name: ListMonitorsByGroup :many
SELECT * FROM monitors WHERE group_id = ? ORDER BY created_at ASC;

-- name: GetMonitor :one
SELECT * FROM monitors WHERE id = ?;

-- name: CreateMonitor :one
INSERT INTO monitors (name, url, type, interval_seconds, timeout_seconds,
  expected_status, keyword_check, degraded_threshold_ms, down_threshold_ms,
  is_active, group_id, source, external_id, tls_warn_days, tls_critical_days,
  method, headers, body, auth_type, auth_username, auth_secret,
  tls_verify, ca_pem, follow_redirects, accepted_statuses, keyword_mode)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
  ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateMonitor :one
UPDATE monitors SET
  name = ?, url = ?, type = ?, interval_seconds = ?, timeout_seconds = ?,
  expected_status = ?, keyword_check = ?, degraded_threshold_ms = ?,
  down_threshold_ms = ?, is_active = ?, group_id = ?, tls_warn_days = ?,
  tls_critical_days = ?,
  method = ?, headers = ?, body = ?, auth_type = ?, auth_username = ?,
  auth_secret = ?, tls_verify = ?, ca_pem = ?, follow_redirects = ?,
  accepted_statuses = ?, keyword_mode = ?
WHERE id = ?
RETURNING *;

-- name: DeleteMonitor :exec
DELETE FROM monitors WHERE id = ?;

-- name: ListActiveMonitors :many
SELECT * FROM monitors WHERE is_active = 1;

-- name: GetMonitorBySourceExternalID :one
SELECT * FROM monitors WHERE source = ? AND external_id = ?;

-- name: ListMonitorMetrics :many
-- One row per active monitor with its latest check result, for the /metrics
-- scrape. The status and response time are NULL until a monitor has a check.
-- Only names, types and groups are selected: targets (URLs, hosts) must never
-- reach metric labels.
SELECT
  m.name,
  m.type,
  CAST(COALESCE(g.name, '') AS TEXT) AS group_name,
  cr.status AS last_status,
  cr.response_time_ms AS last_response_time_ms
FROM monitors m
LEFT JOIN monitor_groups g ON g.id = m.group_id
LEFT JOIN check_results cr ON cr.id = (
  SELECT c.id FROM check_results c
  WHERE c.monitor_id = m.id
  ORDER BY c.checked_at DESC, c.id DESC
  LIMIT 1
)
WHERE m.is_active = 1
ORDER BY m.id;
