-- name: ListNotifications :many
SELECT * FROM notifications ORDER BY id ASC;

-- name: GetNotification :one
SELECT * FROM notifications WHERE id = ?;

-- name: CreateNotification :one
INSERT INTO notifications (channel, config_json, monitor_id) VALUES (?, ?, ?) RETURNING *;

-- name: UpdateNotification :one
UPDATE notifications SET channel = ?, config_json = ?, monitor_id = ? WHERE id = ? RETURNING *;

-- name: DeleteNotification :exec
DELETE FROM notifications WHERE id = ?;

-- name: ListGlobalNotifications :many
SELECT * FROM notifications WHERE monitor_id IS NULL;
