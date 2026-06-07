-- name: GetPageByDomain :one
SELECT * FROM status_pages WHERE domain = ?;

-- name: GetDefaultPage :one
SELECT * FROM status_pages WHERE is_default = 1 LIMIT 1;

-- name: ListStatusPages :many
SELECT * FROM status_pages ORDER BY is_default DESC, created_at ASC;

-- name: GetStatusPage :one
SELECT * FROM status_pages WHERE id = ?;

-- name: CreateStatusPage :one
INSERT INTO status_pages (domain, title, published) VALUES (?, ?, ?) RETURNING *;

-- name: UpdateStatusPage :exec
UPDATE status_pages SET domain = ?, title = ?, published = ? WHERE id = ?;

-- name: DeleteStatusPage :exec
DELETE FROM status_pages WHERE id = ? AND is_default = 0;

-- name: ListPageGroupIDs :many
SELECT group_id FROM status_page_groups WHERE status_page_id = ?;

-- name: ClearPageGroups :exec
DELETE FROM status_page_groups WHERE status_page_id = ?;

-- name: AddPageGroup :exec
INSERT INTO status_page_groups (status_page_id, group_id) VALUES (?, ?) ON CONFLICT DO NOTHING;
