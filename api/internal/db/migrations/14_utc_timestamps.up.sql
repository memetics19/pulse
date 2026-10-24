-- Rewrite timestamps the Go driver stored with a non-UTC offset (issue #48).
--
-- modernc.org/sqlite writes a time.Time as time.Time.String(), for example
-- "2026-10-23 10:00:00 +0530 IST". A value taken straight from time.Now()
-- also carries a " m=+0.5" monotonic suffix. Queries compare these as text,
-- so rows with different offsets do not sort in time order. The app now
-- writes UTC only. This rewrites older rows into the exact text the driver
-- writes for a UTC time, so old and new rows compare the same way:
--
--   "2026-10-23 10:00:00.5 +0530 IST m=+0.5"  ->  "2026-10-23 04:30:00.5 +0000 UTC"
--
-- In each statement v is the stored text and f is the length of its
-- fractional seconds, dot included, or 0. The offset starts after the space
-- that follows them. datetime() applies the offset and returns UTC, and the
-- fraction is kept as written.
--
-- Rows already in UTC, rows written by CURRENT_TIMESTAMP (UTC with no offset)
-- and text in any other shape do not match the WHERE clause, so running this
-- again changes nothing. coalesce keeps the original text if datetime()
-- cannot parse it. Only the columns the app writes from Go are listed. Every
-- other timestamp column is filled by CURRENT_TIMESTAMP, which is already UTC.

UPDATE maintenance_windows SET starts_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT starts_at AS v,
               CASE WHEN substr(starts_at, 20, 1) = '.' THEN instr(substr(starts_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE starts_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND starts_at NOT GLOB '* +0000 UTC';

UPDATE maintenance_windows SET ends_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT ends_at AS v,
               CASE WHEN substr(ends_at, 20, 1) = '.' THEN instr(substr(ends_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE ends_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND ends_at NOT GLOB '* +0000 UTC';

UPDATE check_results SET checked_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT checked_at AS v,
               CASE WHEN substr(checked_at, 20, 1) = '.' THEN instr(substr(checked_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE checked_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND checked_at NOT GLOB '* +0000 UTC';

UPDATE agent_diagnostics SET collected_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT collected_at AS v,
               CASE WHEN substr(collected_at, 20, 1) = '.' THEN instr(substr(collected_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE collected_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND collected_at NOT GLOB '* +0000 UTC';

UPDATE infra_metrics_raw SET collected_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT collected_at AS v,
               CASE WHEN substr(collected_at, 20, 1) = '.' THEN instr(substr(collected_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE collected_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND collected_at NOT GLOB '* +0000 UTC';

-- (agent_id, bucket_at) is unique. If one bucket was stored twice under two
-- offsets, OR REPLACE keeps one row. The rollup rebuilds recent buckets from
-- raw rows on its next pass.
UPDATE OR REPLACE infra_metrics_1m SET bucket_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT bucket_at AS v,
               CASE WHEN substr(bucket_at, 20, 1) = '.' THEN instr(substr(bucket_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE bucket_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND bucket_at NOT GLOB '* +0000 UTC';

UPDATE incidents SET started_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT started_at AS v,
               CASE WHEN substr(started_at, 20, 1) = '.' THEN instr(substr(started_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE started_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND started_at NOT GLOB '* +0000 UTC';

UPDATE incidents SET resolved_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT resolved_at AS v,
               CASE WHEN substr(resolved_at, 20, 1) = '.' THEN instr(substr(resolved_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE resolved_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND resolved_at NOT GLOB '* +0000 UTC';

UPDATE sessions SET expires_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT expires_at AS v,
               CASE WHEN substr(expires_at, 20, 1) = '.' THEN instr(substr(expires_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE expires_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND expires_at NOT GLOB '* +0000 UTC';

UPDATE api_keys SET last_used_at = (
  SELECT coalesce(datetime(substr(v, 1, 19) || substr(v, 21 + f, 3) || ':' || substr(v, 24 + f, 2))
                  || substr(v, 20, f) || ' +0000 UTC', v)
  FROM (SELECT last_used_at AS v,
               CASE WHEN substr(last_used_at, 20, 1) = '.' THEN instr(substr(last_used_at, 20), ' ') - 1 ELSE 0 END AS f)
)
WHERE last_used_at GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9] [0-9][0-9]:[0-9][0-9]:[0-9][0-9]* [+-][0-9][0-9][0-9][0-9] *'
  AND last_used_at NOT GLOB '* +0000 UTC';
