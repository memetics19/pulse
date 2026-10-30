-- Per-monitor sensitivity. The defaults are the values Pulse used before they
-- were configurable: an incident after 2 down results, auto-resolve after 2
-- answering results, and 1 retry 500ms after a transient check failure.
ALTER TABLE monitors ADD COLUMN failures_before_incident INTEGER NOT NULL DEFAULT 2;
ALTER TABLE monitors ADD COLUMN recoveries_before_resolve INTEGER NOT NULL DEFAULT 2;
ALTER TABLE monitors ADD COLUMN retries INTEGER NOT NULL DEFAULT 1;
ALTER TABLE monitors ADD COLUMN retry_interval_ms INTEGER NOT NULL DEFAULT 500;
