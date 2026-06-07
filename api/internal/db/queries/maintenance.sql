-- name: CreateMaintenance :one
INSERT INTO maintenance_windows (title, description, status, affected_monitor_ids, starts_at, ends_at)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ListMaintenance :many
SELECT * FROM maintenance_windows ORDER BY starts_at DESC;

-- name: ListActiveMaintenance :many
SELECT * FROM maintenance_windows WHERE status = 'in_progress';

-- name: UpdateMaintenanceStatus :exec
UPDATE maintenance_windows SET status = ? WHERE id = ?;

-- name: DeleteMaintenance :exec
DELETE FROM maintenance_windows WHERE id = ?;

-- name: DueToStart :many
SELECT * FROM maintenance_windows WHERE status = 'scheduled' AND starts_at <= ?;

-- name: DueToEnd :many
SELECT * FROM maintenance_windows WHERE status = 'in_progress' AND ends_at <= ?;
