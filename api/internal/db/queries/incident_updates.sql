-- name: ListIncidentUpdates :many
SELECT * FROM incident_updates WHERE incident_id = ? ORDER BY created_at ASC;

-- name: CreateIncidentUpdate :one
INSERT INTO incident_updates (incident_id, status, message, author)
VALUES (?, ?, ?, ?) RETURNING *;
