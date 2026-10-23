-- The pruner deletes by timestamp alone. The existing indexes lead with
-- monitor_id or agent_id, so without these every prune pass scanned each
-- table in full. With them, finding the next batch of expired rows is a short
-- range scan, and a pass with nothing to delete costs one index probe.
CREATE INDEX idx_check_results_checked_at ON check_results(checked_at);
CREATE INDEX idx_agent_diagnostics_collected_at ON agent_diagnostics(collected_at);
CREATE INDEX idx_infra_metrics_raw_collected_at ON infra_metrics_raw(collected_at);
CREATE INDEX idx_infra_metrics_1m_bucket_at ON infra_metrics_1m(bucket_at);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
