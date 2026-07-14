-- name: CreateImportRun :one
INSERT INTO import_runs
  (source, source_version, input_hash, idempotency_key, conflict_policy, status, plan_hash)
VALUES (?, ?, ?, ?, ?, 'running', ?) RETURNING *;

-- name: GetImportRunByIdempotencyKey :one
SELECT * FROM import_runs WHERE idempotency_key = ?;

-- name: CompleteImportRun :one
UPDATE import_runs SET status = 'completed', summary_json = ?, completed_at = CURRENT_TIMESTAMP
WHERE id = ? RETURNING *;

-- name: FailImportRun :one
UPDATE import_runs SET status = 'failed', error_summary = ?, completed_at = CURRENT_TIMESTAMP
WHERE id = ? RETURNING *;
