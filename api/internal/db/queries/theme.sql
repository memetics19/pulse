-- name: GetTheme :one
SELECT * FROM theme_config WHERE id = 1;

-- name: UpsertTheme :one
INSERT INTO theme_config (id, preset, custom_css, config_json, updated_at)
VALUES (1, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET
  preset = excluded.preset,
  custom_css = excluded.custom_css,
  config_json = excluded.config_json,
  updated_at = excluded.updated_at
RETURNING *;
