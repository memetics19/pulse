-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: CreateUser :one
INSERT INTO users (username, password_hash, name, email) VALUES (?, ?, ?, ?) RETURNING *;

-- name: CreateFirstUser :one
-- Inserts the user only when the table is empty. It is one statement, so
-- SQLite takes the write lock before the NOT EXISTS check runs: across
-- connections and processes at most one first user can ever be created. A
-- loser gets sql.ErrNoRows.
INSERT INTO users (username, password_hash, name, email)
SELECT CAST(sqlc.arg(username) AS TEXT), CAST(sqlc.arg(password_hash) AS TEXT),
       CAST(sqlc.arg(name) AS TEXT), CAST(sqlc.arg(email) AS TEXT)
WHERE NOT EXISTS (SELECT 1 FROM users)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = ?;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = ?;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = ? WHERE username = ?;

-- name: CreateSession :exec
INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?);

-- name: GetSession :one
SELECT s.token, s.user_id, s.expires_at, u.username
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token = ?;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = ?;

-- DeleteExpiredSessions removes sessions that expired before the cutoff. The
-- cutoff is bound from Go so it is formatted exactly like expires_at, which Go
-- also writes. Comparing against CURRENT_TIMESTAMP would mix formats.
-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE rowid IN (
  SELECT old.rowid FROM sessions AS old WHERE old.expires_at < sqlc.arg(cutoff)
  LIMIT sqlc.arg(batch_size)
);

-- name: SetUserTOTP :exec
UPDATE users SET totp_secret = ?, totp_enabled = ? WHERE id = ?;
