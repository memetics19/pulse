ALTER TABLE monitors DROP COLUMN retry_interval_ms;
ALTER TABLE monitors DROP COLUMN retries;
ALTER TABLE monitors DROP COLUMN recoveries_before_resolve;
ALTER TABLE monitors DROP COLUMN failures_before_incident;
