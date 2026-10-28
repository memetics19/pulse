-- name: ListIncidents :many
SELECT * FROM incidents ORDER BY started_at DESC;

-- name: ListActiveIncidents :many
SELECT * FROM incidents WHERE status != 'resolved' ORDER BY started_at DESC;

-- name: GetIncident :one
SELECT * FROM incidents WHERE id = ?;

-- name: CreateIncident :one
INSERT INTO incidents (title, severity, status, affected_monitor_ids, started_at, source, external_id)
VALUES (?, ?, 'detected', ?, ?, ?, ?) RETURNING *;

-- name: CreateAutoIncident :one
INSERT INTO incidents (title, severity, status, affected_monitor_ids, started_at, source, external_id)
VALUES (?, 'major', 'detected', ?, ?, 'monitor', ?)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: UpdateIncidentStatus :one
UPDATE incidents SET status = ?, resolved_at = ?, rca = ? WHERE id = ? RETURNING *;

-- name: UpdateIncidentRCA :one
UPDATE incidents SET rca = ? WHERE id = ? RETURNING *;

-- name: DeleteIncident :exec
DELETE FROM incidents WHERE id = ?;

-- name: CountOpenIncidents :one
SELECT COUNT(*) FROM incidents WHERE status != 'resolved';

-- name: ListAutoResolvableIncidents :many
-- Open auto-incidents for one monitor that no person has touched yet: still
-- detected, no timeline updates, and covering only that monitor. The
-- status != 'resolved' term repeats the partial index condition so SQLite can
-- use idx_incidents_active_auto_monitor.
SELECT * FROM incidents
WHERE source = 'monitor'
  AND external_id = sqlc.arg(external_id)
  AND affected_monitor_ids = sqlc.arg(affected_monitor_ids)
  AND status != 'resolved'
  AND status = 'detected'
  AND NOT EXISTS (SELECT 1 FROM incident_updates u WHERE u.incident_id = incidents.id)
ORDER BY id;
