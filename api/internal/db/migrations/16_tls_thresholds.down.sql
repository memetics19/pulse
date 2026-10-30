ALTER TABLE check_results DROP COLUMN cert_days_left;
ALTER TABLE monitors DROP COLUMN tls_critical_days;
ALTER TABLE monitors DROP COLUMN tls_warn_days;
