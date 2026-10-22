-- name: ListGroups :many
SELECT * FROM monitor_groups ORDER BY display_order ASC, id ASC;

-- name: GetGroup :one
SELECT * FROM monitor_groups WHERE id = ?;

-- name: CreateGroup :one
INSERT INTO monitor_groups (name, display_order, description) VALUES (?, ?, ?) RETURNING *;

-- name: UpdateGroup :one
UPDATE monitor_groups SET name = ?, display_order = ?, description = ? WHERE id = ? RETURNING *;

-- name: DeleteGroup :exec
DELETE FROM monitor_groups WHERE id = ?;

-- name: GetGroupBySourceExternalID :one
SELECT * FROM monitor_groups WHERE source = ? AND external_id = ?;

-- name: CreateImportedGroup :one
INSERT INTO monitor_groups (name, display_order, description, source, external_id)
VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateImportedGroup :one
UPDATE monitor_groups SET name = ?, display_order = ?, description = ?
WHERE id = ? RETURNING *;
