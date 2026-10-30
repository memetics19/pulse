-- Per-monitor TLS certificate expiry thresholds (issue #77). An ssl monitor
-- is degraded at tls_warn_days or fewer days left and down at
-- tls_critical_days or fewer. The defaults are the values the checker used
-- before they were configurable.
ALTER TABLE monitors ADD COLUMN tls_warn_days INTEGER NOT NULL DEFAULT 14;
ALTER TABLE monitors ADD COLUMN tls_critical_days INTEGER NOT NULL DEFAULT 7;

-- Days left on the certificate an ssl check saw. NULL for other monitor
-- types and for ssl checks that got no certificate. The checker also keeps
-- writing the value to status_code for one release.
ALTER TABLE check_results ADD COLUMN cert_days_left INTEGER;

-- Older ssl rows carry the days left in status_code only. Copy them over so
-- the history reads the same before and after the upgrade.
UPDATE check_results SET cert_days_left = status_code
WHERE status_code IS NOT NULL
  AND monitor_id IN (SELECT id FROM monitors WHERE type = 'ssl');
