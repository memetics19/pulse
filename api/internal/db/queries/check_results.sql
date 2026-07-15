-- name: InsertCheckResult :one
INSERT INTO check_results (monitor_id, checked_at, status, response_time_ms, status_code, error_message)
VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ListCheckResults :many
SELECT * FROM check_results
WHERE monitor_id = ? AND checked_at >= ?
ORDER BY checked_at DESC
LIMIT ?;

-- name: LatestCheckResult :one
SELECT * FROM check_results WHERE monitor_id = ? ORDER BY checked_at DESC LIMIT 1;

-- name: UptimePercent :one
-- COUNT(*)=0 (no checks in range) would divide by zero → NULL; COALESCE keeps
-- the result a non-null REAL (100.0 = "no failures observed") so it scans into
-- float64 rather than erroring. NULLIF avoids the divide-by-zero itself.
SELECT
  COALESCE(CAST(SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) AS REAL) / NULLIF(COUNT(*), 0) * 100, 100.0) as uptime_pct
FROM check_results
WHERE monitor_id = ? AND checked_at >= ?;

-- name: PruneCheckResults :exec
DELETE FROM check_results WHERE checked_at < ?;

-- name: LatestStatuses :many
SELECT cr.monitor_id, cr.status
FROM check_results cr
JOIN (SELECT monitor_id, MAX(checked_at) AS mx FROM check_results GROUP BY monitor_id) latest
  ON cr.monitor_id = latest.monitor_id AND cr.checked_at = latest.mx;
