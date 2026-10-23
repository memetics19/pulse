-- name: InsertRawMetric :exec
INSERT INTO infra_metrics_raw (agent_id, collected_at, cpu_percent, mem_percent,
  mem_used_mb, disk_percent, disk_used_gb, net_rx_bytes, net_tx_bytes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListRawMetrics :many
SELECT * FROM infra_metrics_raw
WHERE agent_id = ? AND collected_at >= ?
ORDER BY collected_at ASC;

-- MaxRawMetricID is the rollup watermark: the newest raw row that exists now.
-- name: MaxRawMetricID :one
SELECT CAST(COALESCE(MAX(id), 0) AS INTEGER) AS max_id FROM infra_metrics_raw;

-- ListRawMetricsAfterID returns the raw rows inserted since the last rollup
-- (ids are AUTOINCREMENT, so they only grow). The collected_at bound matches
-- the window ListRawMetrics uses for a full rollup.
-- name: ListRawMetricsAfterID :many
SELECT agent_id, collected_at FROM infra_metrics_raw
WHERE id > sqlc.arg(after_id) AND id <= sqlc.arg(through_id)
  AND collected_at >= sqlc.arg(since)
ORDER BY id ASC;

-- name: PruneRawMetrics :execrows
DELETE FROM infra_metrics_raw WHERE id IN (
  SELECT old.id FROM infra_metrics_raw AS old WHERE old.collected_at < sqlc.arg(cutoff)
  LIMIT sqlc.arg(batch_size)
);

-- name: Prune1mMetrics :execrows
DELETE FROM infra_metrics_1m WHERE id IN (
  SELECT old.id FROM infra_metrics_1m AS old WHERE old.bucket_at < sqlc.arg(cutoff)
  LIMIT sqlc.arg(batch_size)
);

-- name: Upsert1mMetric :exec
INSERT INTO infra_metrics_1m (agent_id, bucket_at, cpu_percent, mem_percent,
  disk_percent, net_rx_bytes, net_tx_bytes)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(agent_id, bucket_at) DO UPDATE SET
  cpu_percent = excluded.cpu_percent,
  mem_percent = excluded.mem_percent,
  disk_percent = excluded.disk_percent,
  net_rx_bytes = excluded.net_rx_bytes,
  net_tx_bytes = excluded.net_tx_bytes;

-- name: List1mMetrics :many
SELECT * FROM infra_metrics_1m
WHERE agent_id = ? AND bucket_at >= ?
ORDER BY bucket_at ASC;
