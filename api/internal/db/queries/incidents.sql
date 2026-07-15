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
