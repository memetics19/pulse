DROP INDEX idx_incidents_active_auto_monitor;

DROP INDEX idx_check_results_monitor_checked;
CREATE INDEX idx_check_results_monitor_checked
ON check_results(monitor_id, checked_at DESC);
