-- name: GetSetting :one
SELECT value FROM app_settings WHERE key = ?;

-- name: SetSetting :exec
INSERT INTO app_settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;
