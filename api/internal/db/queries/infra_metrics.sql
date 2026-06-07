-- name: InsertRawMetric :exec
INSERT INTO infra_metrics_raw (agent_id, collected_at, cpu_percent, mem_percent,
  mem_used_mb, disk_percent, disk_used_gb, net_rx_bytes, net_tx_bytes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListRawMetrics :many
SELECT * FROM infra_metrics_raw
WHERE agent_id = ? AND collected_at >= ?
ORDER BY collected_at ASC;

-- name: PruneRawMetrics :exec
DELETE FROM infra_metrics_raw WHERE collected_at < ?;

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
