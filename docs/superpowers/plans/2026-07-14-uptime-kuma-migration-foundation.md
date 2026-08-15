# Uptime Kuma Migration Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a safe, atomic, idempotent Uptime Kuma v1 migration workflow with native push monitors, compatibility reporting, CLI distribution, and real integration coverage.

**Architecture:** `pulse-cli` parses source-specific v1 JSON into a normalized resource plan. The Pulse API validates and hashes that plan, then applies it in one SQLite transaction using stable `(source, external_id)` identities. Push heartbeats use hashed bearer tokens and a shared result recorder so HTTP polling, incoming heartbeats, watchdog failures, incidents, and alerts follow one persistence path.

**Tech Stack:** Go 1.25, Cobra, chi, `database/sql`, sqlc, SQLite/modernc, testify, Next.js 15/React 18/TypeScript, GitHub Actions.

**Design:** `docs/superpowers/specs/2026-07-14-uptime-kuma-migration-foundation-design.md`

---

## File structure

New files have one responsibility each:

- `api/internal/db/migrations/9_import_foundation.{up,down}.sql`: source identities, import runs, push-token storage, and the `push` monitor constraint.
- `api/internal/db/queries/imports.sql`: import-run and source-identity queries.
- `api/internal/db/queries/push_tokens.sql`: push-token lifecycle queries.
- `api/internal/monitorvalidation/validation.go`: validation shared by CRUD and imports.
- `api/internal/worker/checkresult/recorder.go`: check persistence plus incident/alert dispatch.
- `api/internal/push/token.go`: token generation, validation, hashing, and prefixing.
- `api/internal/handlers/push.go`: public heartbeat and authenticated rotation endpoints.
- `api/internal/importer/types.go`: normalized API request/response and finding types.
- `api/internal/importer/planner.go`: validation, conflict classification, canonicalization, and plan hashing.
- `api/internal/importer/applier.go`: idempotency and transactional resource mutation.
- `api/internal/handlers/imports.go`: HTTP decoding and status mapping for import planning/apply.
- `cli/internal/uptimekuma/model.go`: complete v1 source fields needed for compatibility decisions.
- `cli/internal/uptimekuma/converter.go`: v1-to-normalized conversion and field findings.
- `cli/internal/uptimekuma/testdata/v1.23.16-backup.json`: sanitized real-export fixture.
- `cli/internal/pulseclient/imports.go`: normalized import DTOs and API calls.
- `cli/importcmd/report.go`: deterministic human/JSON rendering and exit-code mapping.

Generated files under `api/internal/generated/` are changed only by `make sqlc`.

### Task 1: Add import identity, run, and push-token schema

**Files:**
- Create: `api/internal/db/migration_driver.go`
- Create: `api/internal/db/migrations/9_import_foundation.up.sql`
- Create: `api/internal/db/migrations/9_import_foundation.down.sql`
- Create: `api/internal/db/queries/imports.sql`
- Create: `api/internal/db/queries/push_tokens.sql`
- Modify: `api/internal/db/db.go`
- Modify: `api/internal/db/queries/groups.sql`
- Modify: `api/internal/db/queries/monitors.sql`
- Modify: `api/store/store.go`
- Test: `api/internal/db/db_test.go`
- Test: `api/internal/db/db_dsn_test.go`
- Test: `api/internal/db/import_foundation_migration_test.go`
- Generate: `api/internal/generated/*.sql.go`, `api/internal/generated/models.go`

- [ ] **Step 1: Write the migration test first**

Add this test to `api/internal/db/db_test.go`:

```go
func TestImportFoundationSchema(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `INSERT INTO monitors
		(name, url, type, interval_seconds, timeout_seconds, source, external_id)
		VALUES ('Push', '', 'push', 60, 10, 'uptime-kuma', 'monitor:7')`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO monitors
		(name, url, type, interval_seconds, timeout_seconds, source, external_id)
		VALUES ('Duplicate', '', 'push', 60, 10, 'uptime-kuma', 'monitor:7')`)
	require.Error(t, err)

	_, err = db.ExecContext(ctx, `INSERT INTO import_runs
		(source, source_version, input_hash, idempotency_key, conflict_policy, status)
		VALUES ('uptime-kuma', '1.23.16', 'hash', 'request-1', 'fail', 'running')`)
	require.NoError(t, err)

	var foreignKeys int
	require.NoError(t, db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys))
	require.Equal(t, 1, foreignKeys)
}
```

Add imports for `context`, `github.com/memetics19/pulse/api/testutil`, and testify `require` if absent.

- [ ] **Step 2: Run the focused test and confirm the schema is missing**

Run: `cd api && go test ./internal/db -run TestImportFoundationSchema -count=1`

Expected: FAIL because `push` violates the current monitor type check or `import_runs` does not exist.

Add real migration-driver tests starting from embedded migration version 8.
Cover an early duplicate-monitor identity failure, a late restored-group
identity failure, down/re-up, and deleted/highest-ID AUTOINCREMENT cases. Failed
migration 9 attempts must leave the legacy schema and data intact with foreign
keys enabled, and retry must succeed after the collision is corrected.

- [ ] **Step 3: Create migration 9**

Normal migrations retain golang-migrate's SQLite transaction wrapping. A
focused driver wrapper recognizes the marker below, disables foreign keys on a
dedicated connection before beginning its own SQL transaction, rolls back any
failure, and restores foreign keys on every path.

Create `api/internal/db/migrations/9_import_foundation.up.sql` with the complete migration below:

```sql
-- pulse:foreign-keys-off-transaction

CREATE UNIQUE INDEX idx_import_foundation_monitors_preflight
ON monitors(source, external_id) WHERE external_id <> '';

CREATE TABLE import_foundation_sequence_state (
    table_name TEXT PRIMARY KEY,
    seq        INTEGER NOT NULL
);

INSERT INTO import_foundation_sequence_state (table_name, seq)
SELECT name, seq FROM sqlite_sequence WHERE name = 'monitors';

CREATE TABLE monitors_new (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    name                  TEXT NOT NULL,
    url                   TEXT NOT NULL DEFAULT '',
    type                  TEXT NOT NULL CHECK(type IN ('http','https','tcp','ping','dns','ssl','infra','push')),
    interval_seconds      INTEGER NOT NULL DEFAULT 60,
    timeout_seconds       INTEGER NOT NULL DEFAULT 10,
    expected_status       INTEGER,
    keyword_check         TEXT NOT NULL DEFAULT '',
    degraded_threshold_ms INTEGER NOT NULL DEFAULT 500,
    down_threshold_ms     INTEGER NOT NULL DEFAULT 2000,
    is_active             INTEGER NOT NULL DEFAULT 1,
    group_id              INTEGER REFERENCES monitor_groups(id) ON DELETE SET NULL,
    source                TEXT NOT NULL DEFAULT 'internal',
    external_id           TEXT NOT NULL DEFAULT '',
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO monitors_new
SELECT id, name, url, type, interval_seconds, timeout_seconds, expected_status,
       keyword_check, degraded_threshold_ms, down_threshold_ms, is_active,
       group_id, source, external_id, created_at
FROM monitors;

DROP TABLE monitors;
ALTER TABLE monitors_new RENAME TO monitors;

UPDATE sqlite_sequence
SET seq = MAX(seq, (SELECT seq FROM import_foundation_sequence_state
                    WHERE table_name = 'monitors'))
WHERE name = 'monitors'
  AND EXISTS (SELECT 1 FROM import_foundation_sequence_state
              WHERE table_name = 'monitors');

INSERT INTO sqlite_sequence (name, seq)
SELECT 'monitors', seq
FROM import_foundation_sequence_state
WHERE table_name = 'monitors'
  AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'monitors');

DROP TABLE import_foundation_sequence_state;

CREATE TABLE IF NOT EXISTS import_foundation_group_identities (
    group_id    INTEGER PRIMARY KEY,
    source      TEXT NOT NULL,
    external_id TEXT NOT NULL
);

ALTER TABLE monitor_groups ADD COLUMN source TEXT NOT NULL DEFAULT 'internal';
ALTER TABLE monitor_groups ADD COLUMN external_id TEXT NOT NULL DEFAULT '';

UPDATE monitor_groups
SET source = (SELECT source FROM import_foundation_group_identities
              WHERE group_id = monitor_groups.id),
    external_id = (SELECT external_id FROM import_foundation_group_identities
                   WHERE group_id = monitor_groups.id)
WHERE id IN (SELECT group_id FROM import_foundation_group_identities);

CREATE UNIQUE INDEX idx_monitors_source_external
ON monitors(source, external_id) WHERE external_id <> '';

CREATE UNIQUE INDEX idx_groups_source_external
ON monitor_groups(source, external_id) WHERE external_id <> '';

DROP TABLE import_foundation_group_identities;

CREATE TABLE push_monitor_tokens (
    monitor_id  INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    prefix      TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at  DATETIME
);

CREATE TABLE import_runs (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    source             TEXT NOT NULL,
    source_version     TEXT NOT NULL,
    input_hash         TEXT NOT NULL,
    idempotency_key    TEXT NOT NULL UNIQUE,
    conflict_policy    TEXT NOT NULL CHECK(conflict_policy IN ('fail','skip','update')),
    status             TEXT NOT NULL CHECK(status IN ('running','completed','failed')),
    plan_hash          TEXT NOT NULL DEFAULT '',
    summary_json       TEXT NOT NULL DEFAULT '{}',
    error_summary      TEXT NOT NULL DEFAULT '',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at       DATETIME
);
```

Create the down migration to remove imported-only state safely before restoring the original monitor constraint:

```sql
-- pulse:foreign-keys-off-transaction

CREATE TABLE import_foundation_sequence_state (
    table_name TEXT PRIMARY KEY,
    seq        INTEGER NOT NULL
);

INSERT INTO import_foundation_sequence_state (table_name, seq)
SELECT name, seq
FROM sqlite_sequence
WHERE name IN ('monitors', 'monitor_groups');

CREATE TABLE import_foundation_group_identities (
    group_id    INTEGER PRIMARY KEY,
    source      TEXT NOT NULL,
    external_id TEXT NOT NULL
);

INSERT INTO import_foundation_group_identities (group_id, source, external_id)
SELECT id, source, external_id
FROM monitor_groups
WHERE source <> 'internal' OR external_id <> '';

CREATE TABLE monitor_groups_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    display_order INTEGER NOT NULL DEFAULT 0,
    description   TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO monitor_groups_old (id, name, display_order, description, created_at)
SELECT id, name, display_order, description, created_at
FROM monitor_groups;

CREATE TABLE monitors_old (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    name                  TEXT NOT NULL,
    url                   TEXT NOT NULL DEFAULT '',
    type                  TEXT NOT NULL CHECK(type IN ('http','tcp','ping','dns','ssl','infra')),
    interval_seconds      INTEGER NOT NULL DEFAULT 60,
    timeout_seconds       INTEGER NOT NULL DEFAULT 10,
    expected_status       INTEGER,
    keyword_check         TEXT NOT NULL DEFAULT '',
    degraded_threshold_ms INTEGER NOT NULL DEFAULT 500,
    down_threshold_ms     INTEGER NOT NULL DEFAULT 2000,
    is_active             INTEGER NOT NULL DEFAULT 1,
    group_id              INTEGER REFERENCES monitor_groups(id) ON DELETE SET NULL,
    source                TEXT NOT NULL DEFAULT 'internal',
    external_id           TEXT NOT NULL DEFAULT '',
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO monitors_old
SELECT id, name, url, CASE type WHEN 'https' THEN 'http' ELSE type END,
       interval_seconds, timeout_seconds, expected_status, keyword_check,
       degraded_threshold_ms, down_threshold_ms, is_active, group_id, source,
       external_id, created_at
FROM monitors
WHERE type <> 'push';

DELETE FROM check_results
WHERE monitor_id IN (SELECT id FROM monitors WHERE type = 'push');
DELETE FROM ssl_checks
WHERE monitor_id IN (SELECT id FROM monitors WHERE type = 'push');
DELETE FROM notifications
WHERE monitor_id IN (SELECT id FROM monitors WHERE type = 'push');

DROP TABLE push_monitor_tokens;
DROP TABLE import_runs;
DROP INDEX idx_monitors_source_external;
DROP INDEX idx_groups_source_external;

DROP TABLE monitors;
ALTER TABLE monitors_old RENAME TO monitors;
DROP TABLE monitor_groups;
ALTER TABLE monitor_groups_old RENAME TO monitor_groups;

UPDATE sqlite_sequence
SET seq = MAX(seq, (SELECT seq FROM import_foundation_sequence_state
                    WHERE table_name = 'monitors'))
WHERE name = 'monitors'
  AND EXISTS (SELECT 1 FROM import_foundation_sequence_state
              WHERE table_name = 'monitors');

INSERT INTO sqlite_sequence (name, seq)
SELECT 'monitors', seq
FROM import_foundation_sequence_state
WHERE table_name = 'monitors'
  AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'monitors');

UPDATE sqlite_sequence
SET seq = MAX(seq, (SELECT seq FROM import_foundation_sequence_state
                    WHERE table_name = 'monitor_groups'))
WHERE name = 'monitor_groups'
  AND EXISTS (SELECT 1 FROM import_foundation_sequence_state
              WHERE table_name = 'monitor_groups');

INSERT INTO sqlite_sequence (name, seq)
SELECT 'monitor_groups', seq
FROM import_foundation_sequence_state
WHERE table_name = 'monitor_groups'
  AND NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'monitor_groups');

DROP TABLE import_foundation_sequence_state;
```

SQLite lacks conditional `ADD COLUMN`. The downgrade rebuilds the legacy
five-column `monitor_groups` table and preserves non-default identities in
`import_foundation_group_identities`; migration 9 re-up restores the identities
before recreating the unique index, then removes the sidecar.

The marker keeps foreign-key-off table rebuilds atomic without disabling normal
transaction wrapping for migrations 1-8. The initial monitor identity index is
a collision preflight before destructive DDL, and the sequence state table
prevents rebuilt AUTOINCREMENT tables from reusing deleted or push-only IDs.

- [ ] **Step 4: Add source-identity and lifecycle queries**

Append these named queries to the indicated query files:

```sql
-- api/internal/db/queries/groups.sql
-- name: GetGroupBySourceExternalID :one
SELECT * FROM monitor_groups WHERE source = ? AND external_id = ?;

-- name: CreateImportedGroup :one
INSERT INTO monitor_groups (name, display_order, description, source, external_id)
VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: UpdateImportedGroup :one
UPDATE monitor_groups SET name = ?, display_order = ?, description = ?
WHERE id = ? RETURNING *;

-- api/internal/db/queries/monitors.sql
-- name: GetMonitorBySourceExternalID :one
SELECT * FROM monitors WHERE source = ? AND external_id = ?;

-- api/internal/db/queries/push_tokens.sql
-- name: GetPushTokenByHash :one
SELECT * FROM push_monitor_tokens WHERE token_hash = ?;

-- name: GetPushTokenByMonitor :one
SELECT * FROM push_monitor_tokens WHERE monitor_id = ?;

-- name: UpsertPushToken :one
INSERT INTO push_monitor_tokens (monitor_id, token_hash, prefix)
VALUES (?, ?, ?)
ON CONFLICT(monitor_id) DO UPDATE SET
  token_hash = excluded.token_hash,
  prefix = excluded.prefix,
  rotated_at = CURRENT_TIMESTAMP
RETURNING *;

-- api/internal/db/queries/imports.sql
-- name: CreateImportRun :one
INSERT INTO import_runs
  (source, source_version, input_hash, idempotency_key, conflict_policy, status, plan_hash)
VALUES (?, ?, ?, ?, ?, 'running', ?) RETURNING *;

-- name: GetImportRunByIdempotencyKey :one
SELECT * FROM import_runs WHERE idempotency_key = ?;

-- name: CompleteImportRun :one
UPDATE import_runs SET status = 'completed', summary_json = ?, completed_at = CURRENT_TIMESTAMP
WHERE id = ? RETURNING *;

-- name: FailImportRun :one
UPDATE import_runs SET status = 'failed', error_summary = ?, completed_at = CURRENT_TIMESTAMP
WHERE id = ? RETURNING *;
```

- [ ] **Step 5: Generate sqlc code and expose aliases used by the worker**

Run: `make sqlc`

Expected: sqlc updates generated models/queries without errors.

Add aliases for `PushMonitorToken`, `ImportRun`, and new parameter types to `api/store/store.go`, following the existing explicit alias sections.

- [ ] **Step 6: Run migration and database tests**

Run: `cd api && go test ./internal/db ./internal/generated -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the schema slice**

```bash
git add api/internal/db api/internal/generated api/store/store.go
git commit -m "feat(db): add import identity and push token schema"
```

### Task 2: Extract shared validation and check-result recording

**Files:**
- Create: `api/internal/monitorvalidation/validation.go`
- Create: `api/internal/monitorvalidation/validation_test.go`
- Create: `api/internal/worker/checkresult/recorder.go`
- Create: `api/internal/worker/checkresult/recorder_test.go`
- Modify: `api/internal/handlers/monitors.go`
- Modify: `api/internal/worker/scheduler/scheduler.go`
- Modify: `api/internal/worker/incident/detector.go`
- Modify: `api/internal/worker/incident/detector_test.go`
- Modify: `api/internal/db/queries/check_results.sql`
- Generate: `api/internal/generated/check_results.sql.go`

- [ ] **Step 1: Write type-specific validation tests**

Create table-driven cases asserting that push permits an empty target, HTTP still requires a valid URL, and TCP requires `host:port`:

```go
func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		in   monitorvalidation.Input
		want string
	}{
		{"push without URL", monitorvalidation.Input{Type: "push", IntervalSeconds: 60}, ""},
		{"http without URL", monitorvalidation.Input{Type: "http", IntervalSeconds: 60}, "url is required"},
		{"tcp URL syntax", monitorvalidation.Input{Type: "tcp", URL: "tcp://db:5432", IntervalSeconds: 60}, "tcp target must be host:port"},
		{"tcp host port", monitorvalidation.Input{Type: "tcp", URL: "db:5432", IntervalSeconds: 60}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, monitorvalidation.Validate(tt.in, true))
		})
	}
}
```

- [ ] **Step 2: Verify the validation package does not exist**

Run: `cd api && go test ./internal/monitorvalidation -count=1`

Expected: FAIL because the package is missing.

- [ ] **Step 3: Implement the shared validator and replace handler-local validation**

Create this public input and validator:

```go
type Input struct {
	URL             string
	Type            string
	IntervalSeconds int64
}

func Validate(in Input, allowPrivate bool) string {
	if in.IntervalSeconds < 1 {
		return "interval_seconds must be at least 1"
	}
	valid := map[string]bool{"http": true, "https": true, "tcp": true, "ping": true, "dns": true, "ssl": true, "infra": true, "push": true}
	if !valid[in.Type] {
		return "invalid type"
	}
	if in.Type == "push" {
		return ""
	}
	if in.URL == "" {
		return "url is required"
	}
	if in.Type == "tcp" {
		if strings.Contains(in.URL, "://") {
			return "tcp target must be host:port"
		}
		if _, _, err := net.SplitHostPort(in.URL); err != nil {
			return "tcp target must be host:port"
		}
	}
	if in.Type == "http" || in.Type == "https" {
		if err := netguard.ValidateURL(in.URL, allowPrivate); err != nil {
			return err.Error()
		}
	}
	return ""
}
```

Use it from monitor create/update handlers and delete the handler-local type map and validator.

- [ ] **Step 4: Make incident detection database-backed**

Add this query and regenerate sqlc:

```sql
-- name: LatestTwoCheckResults :many
SELECT * FROM check_results
WHERE monitor_id = ?
ORDER BY checked_at DESC, id DESC
LIMIT 2;
```

Replace the detector's mutex/map counter with a `LatestTwoCheckResults` check. It creates an incident only when both latest results are `down`, preserving the existing active-incident and maintenance checks. This makes HTTP handlers and the worker consistent across processes and restarts.

- [ ] **Step 5: Write recorder tests before moving scheduler persistence**

Test that `Recorder.Record` persists an up result, applies configured latency thresholds, and creates an incident after two down results:

```go
func TestRecorderCreatesIncidentAfterTwoDownResults(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := store.New(db)
	mon, err := q.CreateMonitor(t.Context(), store.CreateMonitorParams{
		Name: "API", Url: "https://example.com", Type: "http",
		IntervalSeconds: 60, TimeoutSeconds: 10, IsActive: true,
	})
	require.NoError(t, err)
	r := checkresult.New(q, incident.NewDetector(q), nil)
	for range 2 {
		require.NoError(t, r.Record(t.Context(), mon, checkresult.Input{
			Status: "down", CheckedAt: time.Now(), ErrorMessage: "timeout",
		}))
	}
	incidents, err := q.ListActiveIncidents(t.Context())
	require.NoError(t, err)
	require.Len(t, incidents, 1)
}
```

- [ ] **Step 6: Implement `checkresult.Recorder` and delegate scheduler writes**

Define:

```go
type Input struct {
	Status         string
	ResponseTimeMs *int64
	StatusCode     *int64
	ErrorMessage   string
	CheckedAt      time.Time
}

type Recorder struct {
	q        *store.Queries
	detector *incident.Detector
	alerter  *alerter.Dispatcher
}

func New(q *store.Queries, d *incident.Detector, a *alerter.Dispatcher) *Recorder {
	return &Recorder{q: q, detector: d, alerter: a}
}
```

`Record` applies degraded/down latency thresholds when response time exists,
inserts `check_results`, calls the detector, and dispatches the current alert
shape only when a new incident is created. Change scheduler construction to
receive a recorder and replace lines 111-152 of its current `check` method with
one `recorder.Record` call.

- [ ] **Step 7: Run the affected tests**

Run: `cd api && go test ./internal/monitorvalidation ./internal/worker/checkresult ./internal/worker/incident ./internal/worker/scheduler -count=1`

Expected: PASS.

- [ ] **Step 8: Commit the shared behavior slice**

```bash
git add api/internal/monitorvalidation api/internal/worker/checkresult api/internal/worker/incident api/internal/worker/scheduler api/internal/handlers/monitors.go api/internal/db/queries/check_results.sql api/internal/generated
git commit -m "refactor(worker): share monitor validation and result recording"
```

#### Task 2 safety amendment (approved 2026-07-15)

The shared recorder is a transaction boundary, not just a code-extraction
boundary. Before Task 3, Task 2 also includes:

- migration 10, with a partial unique index on unresolved automatic monitor
  incidents identified by `source = 'monitor'` and `external_id = monitor ID`;
- an atomic `CreateAutoIncident` query whose uniqueness conflict means another
  process created the incident and alert first;
- one SQLite transaction covering result insertion, latest-two evaluation,
  active incident and maintenance reads, and automatic incident insertion;
- alert dispatch only after commit, with transport failures retaining the
  dispatcher's non-retryable log-only behavior;
- validation of recorder status and receipt timestamp, and safe handler
  defaults of 500 ms degraded / 2000 ms down when omitted;
- a startup error when a database-backed scheduler lacks a recorder;
- private-network validation for TCP, SSL, and ping targets, plus runtime
  `DialControl` enforcement for TCP/TLS and concrete allowed-IP selection for
  ping; and
- a check-result index on `(monitor_id, checked_at DESC, id DESC)` matching the
  latest-two query.

Tests use two independent SQLite connections/recorders to prove one incident
and one alert, triggers/table removal to prove rollback on DB-side detection
failures, and CRUD/checker coverage for private-target rejection. SQLite uses a
bounded busy timeout so concurrent accepted heartbeats wait for the writer
rather than failing immediately with `SQLITE_BUSY`.

### Task 3: Add secure push creation, heartbeat, and rotation APIs

**Files:**
- Create: `api/internal/push/token.go`
- Create: `api/internal/push/token_test.go`
- Create: `api/internal/handlers/push.go`
- Create: `api/internal/handlers/push_test.go`
- Modify: `api/internal/handlers/monitors.go`
- Modify: `api/internal/handlers/monitors_test.go`
- Modify: `api/internal/server/server.go`
- Modify: `api/internal/middleware/apikey.go`
- Modify: `api/internal/middleware/apikey_test.go`
- Create: `api/internal/middleware/requestlog.go`
- Create: `api/internal/middleware/requestlog_test.go`

- [ ] **Step 1: Write token tests**

```go
func TestTokenLifecycle(t *testing.T) {
	token, err := push.GenerateToken()
	require.NoError(t, err)
	require.Regexp(t, `^[A-Za-z0-9_-]{32}$`, token)
	require.True(t, push.ValidToken(token))
	require.Len(t, push.HashToken(token), 64)
	require.Equal(t, token[:8], push.Prefix(token))
	require.True(t, push.ValidToken("abcdefghij"))
	require.False(t, push.ValidToken("short"))
}
```

- [ ] **Step 2: Run and confirm failure**

Run: `cd api && go test ./internal/push -count=1`

Expected: FAIL because token helpers are missing.

- [ ] **Step 3: Implement token helpers**

Read exactly 24 bytes with `crypto/rand.Read` and encode them with
`base64.RawURLEncoding` to obtain 32 characters without trimming. Validate with
the anchored expression `[A-Za-z0-9_-]{10,128}`, hash with SHA-256 hex, and
return at most eight characters from `Prefix`.

- [ ] **Step 4: Write heartbeat handler integration tests**

Construct a real database, push monitor, hashed token row, DB-backed detector,
and recorder. Mount the handler on a chi router so `chi.URLParam` is exercised,
then assert:

```go
router := chi.NewRouter()
router.Get("/api/push/{token}", h.Heartbeat)
req := httptest.NewRequest(http.MethodGet, "/api/push/abcdefghij?status=up&msg=OK&ping=12", nil)
rr := httptest.NewRecorder()
router.ServeHTTP(rr, req)
require.Equal(t, http.StatusOK, rr.Code)
latest, err := q.LatestCheckResult(t.Context(), monitor.ID)
require.NoError(t, err)
require.Equal(t, "up", latest.Status)
require.Equal(t, int64(12), *latest.ResponseTimeMs)
```

Add cases for POST, `degraded`/other invalid status, ping above
`100000000000`, empty ping, message over 1024 bytes, default `OK` message,
unknown token, inactive monitor, non-push monitor, and rotation invalidating the
old hash. Create a minimal push monitor, replay its exact returned `push_url`,
and assert it records `up` with no response time.

Add a request-logging test that captures the logger output for
`/api/push/abcdefghij?msg=secret` and asserts neither `abcdefghij` nor `secret`
appears.

- [ ] **Step 5: Implement push handler methods**

Create `NewPush(q, recorder)` with:

```go
func (h *Push) Heartbeat(w http.ResponseWriter, r *http.Request)
func (h *Push) Rotate(w http.ResponseWriter, r *http.Request)
```

Heartbeat validates before lookup, hashes the path token, loads token and monitor,
requires `monitor.Type == "push" && monitor.IsActive`, accepts only `up` or
`down` (default `up`), defaults omitted/empty `msg` to `OK`, and treats
omitted/empty `ping` as no response time before parsing a non-empty bounded
value. It records through `checkresult.Recorder`. Every credential lookup
failure returns the same `404 {"error":"push monitor not found"}`. Rotation
verifies monitor type, generates and upserts a token, and returns
`{token,push_url}` once.

- [ ] **Step 6: Generate a token on normal push-monitor creation**

Extend `handlers.Monitors` with `db func() *sql.DB`, update `NewMonitors` callers
to pass `a.DB` in the server and a test DB closure in handler tests, and create a
push monitor plus its token inside one `sql.Tx`. Change the create response to an
additive envelope:

```go
type monitorCreateResponse struct {
	generated.Monitor
	PushToken string `json:"push_token,omitempty"`
	PushURL   string `json:"push_url,omitempty"`
}
```

For `type == "push"`, begin a transaction, create the monitor through
`generated.New(tx)`, generate/upsert the credential, commit, and return the
one-time values. Roll back on every error. For every other type, keep the JSON
monitor fields unchanged and omit push fields. Decode `is_active` through a
pointer so omission uses the schema-compatible active default while explicit
false remains inactive. Reject updates that change a monitor type to or from
`push`; delete/recreate is required to cross the credential boundary.

Build `push_url` from the request's scheme/host, honoring a single
`X-Forwarded-Proto` value of `http` or `https`, and append
`?status=up&msg=OK&ping=`. The token helper receives no request object and never
logs the resulting URL.

- [ ] **Step 7: Wire routes and scopes**

Register GET and POST `/api/push/{token}` in the public section. Register
`POST /api/monitors/{id}/push-token/rotate` inside authenticated monitor routes.
Map `/api/imports` to `imports:write` in middleware and add an exact-scope test.
Inside `server.New`, construct a DB-backed detector, dispatcher, and recorder
from the existing live `generated.Queries` handle and config values, then give
that recorder to the push handler. The detector is database-backed from Task 2,
so this HTTP-side instance remains consistent with the worker-side instance.

Replace `chimiddleware.Logger` with `middleware.RequestLogger`. The replacement
wraps status/latency like the current logger but converts every `/api/push/{token}`
path to `/api/push/[REDACTED]` and omits its query string before logging. Other
routes retain method, path, status, response bytes, and duration.

- [ ] **Step 8: Run handler and middleware tests**

Run: `cd api && go test ./internal/push ./internal/handlers ./internal/middleware ./internal/server -count=1`

Expected: PASS.

- [ ] **Step 9: Commit push HTTP support**

```bash
git add api/internal/push api/internal/handlers api/internal/server api/internal/middleware
git commit -m "feat(push): add secure heartbeat and rotation APIs"
```

### Task 4: Add push missed-heartbeat watchdogs

**Files:**
- Modify: `api/internal/worker/scheduler/scheduler.go`
- Modify: `api/internal/worker/scheduler/reconcile_test.go`
- Create: `api/internal/worker/scheduler/push_test.go`
- Modify: `api/internal/worker/worker.go`

- [ ] **Step 1: Write deterministic watchdog tests with an injected clock**

Add an unexported scheduler clock interface:

```go
type clock interface {
	Now() time.Time
	NewTimer(time.Duration) timer
}

type timer interface {
	C() <-chan time.Time
	Stop() bool
}
```

Use a fake clock in `push_test.go` to verify no immediate down result, a down
result at `created_at + interval + 5s`, and a received up result extending the
next deadline.

- [ ] **Step 2: Run the test and confirm normal scheduler behavior fails**

Run: `cd api && go test ./internal/worker/scheduler -run Push -count=1`

Expected: FAIL because push is treated like a polling checker or no watchdog exists.

- [ ] **Step 3: Implement `runPushMonitor`**

The loop loads `LatestCheckResult`; `sql.ErrNoRows` uses `mon.CreatedAt` as the
base. It waits until `base + interval + 5*time.Second`, reloads before declaring
failure, and records:

```go
checkresult.Input{
	Status: "down",
	CheckedAt: s.clock.Now(),
	ErrorMessage: "no heartbeat received before deadline",
}
```

`RunMonitor` delegates push monitors to this loop and never requests a checker.
Reconciliation already restarts the loop when the type or interval fingerprint
changes.

- [ ] **Step 4: Pass the shared recorder from worker construction**

Create one recorder in `worker.Run`, pass it into `scheduler.New`, and preserve
all existing checker registrations except no checker is registered for `push`.

- [ ] **Step 5: Run worker and scheduler suites**

Run: `cd api && go test ./internal/worker/... -count=1`

Expected: PASS.

- [ ] **Step 6: Commit watchdog support**

```bash
git add api/internal/worker
git commit -m "feat(push): detect missed heartbeat deadlines"
```

### Task 5: Add push monitor administration UI

**Files:**
- Modify: `ui/src/lib/types.ts`
- Modify: `ui/src/lib/api.ts`
- Modify: `ui/src/app/admin/monitors/page.tsx`
- Modify: `ui/src/app/admin/api-keys/page.tsx`

- [ ] **Step 1: Add typed API responses**

Add `push` to `Monitor['type']` and define:

```ts
export type MonitorCreateResponse = Monitor & {
  push_token?: string
  push_url?: string
}

export type PushTokenResponse = {
  token: string
  push_url: string
}
```

Change `adminCreateMonitor` to return `MonitorCreateResponse` and add:

```ts
export async function adminRotatePushToken(id: number): Promise<PushTokenResponse> {
  const res = await fetch(`${BASE}/api/monitors/${id}/push-token/rotate`, {
    method: 'POST', credentials: 'include', headers: ADMIN_HEADERS,
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
```

Add `imports:write` to the API-key page's `ALL_SCOPES` list so the documented
CLI key can be created without manually crafting a request.

- [ ] **Step 2: Implement the push form and one-time reveal**

In the monitor page:

- add `push` to `TYPES`;
- hide URL, timeout, latency, and keyword controls for push;
- relabel interval as `Expected heartbeat every (s)`;
- do not offer type conversion to or from `push` while editing; require
  delete/recreate for that transition;
- capture `push_url` from create/rotate responses in one-time reveal state;
- show `Rotate token` only while editing a push monitor; and
- require a confirmation click before rotation.

Use the existing API-key one-time reveal banner pattern, including copy and
dismiss actions, so no new design system is introduced.

- [ ] **Step 3: Build the static UI**

Run: `cd ui && NEXT_PUBLIC_API_URL="" npm run build`

Expected: Next.js static export succeeds with no TypeScript errors.

- [ ] **Step 4: Commit UI support**

```bash
git add ui/src/lib/types.ts ui/src/lib/api.ts ui/src/app/admin/monitors/page.tsx
git commit -m "feat(ui): manage push heartbeat monitors"
```

### Task 6: Replace the fabricated parser with a versioned v1 converter

**Files:**
- Create: `cli/internal/uptimekuma/model.go`
- Create: `cli/internal/uptimekuma/converter.go`
- Create: `cli/internal/uptimekuma/converter_test.go`
- Create: `cli/internal/uptimekuma/testdata/v1.23.16-backup.json`
- Modify: `cli/internal/uptimekuma/parser.go`
- Modify: `cli/internal/uptimekuma/parser_test.go`
- Create: `cli/internal/pulseclient/imports.go`

- [ ] **Step 1: Add a sanitized real v1 fixture**

Start from an actual Uptime Kuma 1.23.16 export and retain these representative
entries with fake hosts/secrets: top-level group, nested group, HTTP, keyword,
TCP with separate hostname/port, ping, DNS, push with ten-character token,
JSON-query, and an authenticated HTTP monitor. Remove notification provider
objects completely while retaining monitor notification IDs for behavior-change
classification.

- [ ] **Step 2: Write parser and conversion golden assertions**

```go
func TestConvertV1Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/v1.23.16-backup.json")
	require.NoError(t, err)
	backup, err := uptimekuma.ParseV1(data)
	require.NoError(t, err)
	require.Equal(t, "1.23.16", backup.Version)

	plan := uptimekuma.Convert(backup)
	require.Equal(t, "db.example.test:5432", findMonitor(plan, "TCP").URL)
	require.Equal(t, "push", findMonitor(plan, "Heartbeat").Type)
	require.Empty(t, findMonitor(plan, "Heartbeat").URL)
	require.Equal(t, "Core / Internal", findGroup(plan, "group:2").Name)
	require.Contains(t, findingCodes(plan), "nested_group_flattened")
	require.Contains(t, findingCodes(plan), "http_auth_unsupported")
}
```

Add rejection tests for missing version, malformed version, and `2.0.0` with an
error mentioning the removed JSON backup/restore path.

- [ ] **Step 3: Define source and normalized DTOs**

The v1 model includes ID, name, description, parent, URL, method, hostname, port,
type, interval, timeout, active, max retries, retry interval, keyword/inversion,
accepted statuses, headers/body/auth presence, TLS flags, DNS settings, tags,
notification IDs, and push token. Sensitive strings are parsed only to determine
presence and are never placed in finding messages.

Define client-side normalized types in `cli/internal/pulseclient/imports.go` with
the exact JSON names from the design: `ImportRequest`, `Resources`, `GroupInput`,
`MonitorInput`, `Finding`, `ResourceReport`, `ImportResponse`, and `Counts`.
`GroupInput` and `MonitorInput` each carry a `findings` array so the source
adapter's field-level compatibility decisions survive JSON transport. The
server adds its own validation/conflict findings and never removes adapter
findings.

- [ ] **Step 4: Implement parser and converter**

`ParseV1` uses `json.Decoder`, validates major version `1`, and returns typed
source data. `Convert` sorts by source ID, builds group paths with cycle
detection, uses `net.JoinHostPort`, maps default `200-299` to nil expected status,
maps one explicit integer to a pointer, and emits stable finding codes. It never
maps unsupported monitors into runnable Pulse monitors.

- [ ] **Step 5: Remove fabricated expectations**

Delete `push -> http`, `tcp://...`, and standalone Uptime Kuma SSL examples from
the old tests. Keep malformed JSON and empty-list coverage using versioned input.

- [ ] **Step 6: Run CLI conversion tests**

Run: `cd cli && go test ./internal/uptimekuma ./internal/pulseclient -count=1`

Expected: PASS.

- [ ] **Step 7: Commit source conversion**

```bash
git add cli/internal/uptimekuma cli/internal/pulseclient/imports.go
git commit -m "feat(cli): parse and classify Uptime Kuma v1 exports"
```

### Task 7: Implement authoritative import planning and plan hashes

**Files:**
- Create: `api/internal/importer/types.go`
- Create: `api/internal/importer/planner.go`
- Create: `api/internal/importer/planner_test.go`
- Create: `api/internal/importer/hash.go`
- Create: `api/internal/importer/hash_test.go`

- [ ] **Step 1: Write planner status tests**

Create tests with real SQLite rows covering compatible create, behavior-change
blocking, unsupported blocking, invalid target, same-source conflict, unrelated
same-name resource, and `fail`/`skip`/`update` policies. Assert a same-name
internal monitor is not taken over.

```go
plan, err := importer.NewPlanner(q, true).Plan(t.Context(), req)
require.NoError(t, err)
require.Equal(t, importer.StateBlocked, plan.State)
require.Equal(t, "behavior_change", plan.Resources.Monitors[0].Status)
require.NotEmpty(t, plan.PlanHash)
```

- [ ] **Step 2: Define the server contract**

Mirror the CLI JSON DTOs exactly. Use constants:

```go
const (
	StatusCompatible     = "compatible"
	StatusBehaviorChange = "behavior_change"
	StatusUnsupported    = "unsupported"
	StatusInvalid        = "invalid"
	StatusConflict       = "conflict"
	StateReady           = "ready"
	StateBlocked         = "blocked"
)
```

`Finding` contains `Code`, `ResourceKind`, `ExternalID`, `Field`, `Severity`, and
`Message`. Reject finding messages over 512 UTF-8 bytes and replace every exact
push-token value present elsewhere in the request with `[REDACTED]` before
hashing, returning, logging, or persisting the finding. The first-party converter
uses fixed messages that contain no arbitrary source values.

- [ ] **Step 3: Implement validation and conflict classification**

Validate source/version/policy/idempotency key, duplicate external IDs within the
request, group references, monitor values through `monitorvalidation.Validate`,
and push token format. Query existing groups/monitors only by `(source,
external_id)`. Apply the requested conflict policy to produce create/update/skip
actions while preserving all findings supplied by the CLI.

- [ ] **Step 4: Implement canonical plan hashing**

Copy and sort groups and monitors by external ID, sort findings by
`resource_kind/external_id/code/field`, serialize only normalized desired state,
actions, and fingerprints of matched database rows, then compute SHA-256 hex.
Exclude `dry_run`, `expected_plan_hash`, and `idempotency_key`. Two semantically
identical requests in different slice order must produce the same hash.

- [ ] **Step 5: Run planner tests**

Run: `cd api && go test ./internal/importer -run 'Plan|Hash' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit planner support**

```bash
git add api/internal/importer
git commit -m "feat(import): add compatibility planning and stable hashes"
```

### Task 8: Apply plans transactionally with request and resource idempotency

**Files:**
- Create: `api/internal/importer/applier.go`
- Create: `api/internal/importer/applier_test.go`
- Create: `api/internal/handlers/imports.go`
- Create: `api/internal/handlers/imports_test.go`
- Modify: `api/internal/server/server.go`

- [ ] **Step 1: Write rollback and idempotency tests first**

In the rollback test, install this SQLite trigger before applying a two-monitor
plan:

```sql
CREATE TRIGGER fail_second_import
BEFORE INSERT ON monitors
WHEN NEW.external_id = 'monitor:2'
BEGIN
  SELECT RAISE(ABORT, 'forced import failure');
END;
```

Assert apply fails, no imported groups/monitors/tokens exist, and the import run
is `failed`. Add a second test applying the same request twice with the same
idempotency key and assert the stored completed response is returned without new
rows.

- [ ] **Step 2: Implement `Applier.Apply`**

Algorithm:

1. Look up `idempotency_key`; return completed summary, 409 for running, or the
   stored failed result.
2. Compute `input_hash` as SHA-256 of the canonical normalized resources and
   source/version, then plan against the current database and compare
   `expected_plan_hash`.
3. Reject blocked plans before creating a run.
4. Create a running import-run row.
5. Begin `sql.Tx`, build `generated.New(tx)`, and re-plan inside the transaction.
6. Create/update/skip groups while recording external-ID-to-database-ID mapping.
7. Create/update/skip monitors with resolved group IDs.
8. Upsert hashed imported push tokens for created/updated push monitors.
9. Commit the resource transaction.
10. Store the redacted completed summary; on any pre-commit error roll back and
    mark the run failed.

Never persist plaintext push tokens in the summary.

- [ ] **Step 3: Implement the HTTP handler**

`handlers.Imports.Post` decodes with `DisallowUnknownFields`, enforces the 1 MiB
body cap already applied by middleware, and calls planner for dry-run or applier
for apply. Status mapping is:

- 200 for ready dry-run or replayed completion;
- 201 for a newly completed apply;
- 400 for malformed/invalid requests;
- 409 for blocked compatibility, conflict, stale hash, or running idempotency key;
- 500 for redacted internal failures.

- [ ] **Step 4: Register the authenticated import route**

Construct the handler with `a.DB` access and `cfg.AllowPrivateMonitors`, then add
`r.Post("/api/imports", imports.Post)` inside `RequireSessionOrAPIKey` routes.

- [ ] **Step 5: Run transactional and handler tests**

Run: `cd api && go test ./internal/importer ./internal/handlers ./internal/server -count=1`

Expected: PASS, including the forced-trigger rollback test.

- [ ] **Step 6: Commit atomic apply support**

```bash
git add api/internal/importer api/internal/handlers/imports.go api/internal/handlers/imports_test.go api/internal/server/server.go
git commit -m "feat(import): apply migration plans atomically"
```

### Task 9: Turn `pulse-cli import` into a plan/apply workflow

**Files:**
- Modify: `cli/internal/pulseclient/client.go`
- Modify: `cli/internal/pulseclient/client_test.go`
- Modify: `cli/internal/pulseclient/imports.go`
- Modify: `cli/importcmd/import.go`
- Modify: `cli/importcmd/import_test.go`
- Create: `cli/importcmd/report.go`
- Create: `cli/importcmd/report_test.go`
- Modify: `cli/cmd/pulse-cli/main.go`

- [ ] **Step 1: Write HTTP client tests for plan and apply**

Use `httptest.Server` to assert both calls send the normalized payload, bearer
token, context, idempotency key, and apply plan hash. Return a 409 body and assert
the client includes a maximum 64 KiB redacted message rather than only the status.

- [ ] **Step 2: Implement context-aware client calls**

Add:

```go
func (c *Client) Import(ctx context.Context, req ImportRequest) (ImportResponse, error)
```

Normalize the base URL with `strings.TrimRight`, use
`http.NewRequestWithContext`, decode JSON on success, and read at most 64 KiB on
failure. Do not include request bodies or authorization values in errors.

- [ ] **Step 3: Write command behavior tests**

Cover explicit-token override, environment-token fallback, missing token,
dry-run making one request,
apply making plan then apply requests, conflict modes, behavior-change acceptance,
JSON output, invalid output mode, v2 rejection without network calls, and plan
size over 1 MiB.

- [ ] **Step 4: Implement flags and orchestration**

Use an options struct:

```go
type Options struct {
	File                  string
	Server                string
	Token                 string
	DryRun                bool
	ConflictPolicy        string
	AcceptBehaviorChanges bool
	Output                string
}
```

Resolve a non-empty hidden `--token` first for backward compatibility, otherwise
use `PULSE_TOKEN`; examples and help recommend the environment variable. Validate
`conflict` in `fail|skip|update` and output in `human|json`. Parse, convert, encode
to measure size, generate one cryptographically random idempotency key, plan,
render, stop for dry-run/blockers, then apply using the returned hash.
Pass `cmd.Context()` into both client calls so Ctrl-C and parent cancellation
terminate planning or apply requests.

- [ ] **Step 5: Implement stable reporting and exit errors**

Define `ExitError{Code int; Err error}` and map malformed input `2`, blocked
compatibility `3`, conflict/stale plan `4`, and API/auth/transport `5`. Human
output groups findings by status and JSON output serializes only the response.
Update `main` to `errors.As` the returned error and exit with its code; success is
zero.

- [ ] **Step 6: Run all CLI tests**

Run: `cd cli && go test ./... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the CLI workflow**

```bash
git add cli
git commit -m "feat(cli): add dry-run and idempotent import apply"
```

### Task 10: Prove real end-to-end import behavior

**Files:**
- Create: `api/internal/server/import_integration_test.go`
- Modify: `cli/importcmd/import_test.go`
- Modify: `api/internal/handlers/push_test.go`

- [ ] **Step 1: Add a real-router import integration test**

Build `app.App` with `testutil.NewTestDB`, create an API key holding
`imports:write`, call `server.New`, submit a normalized dry-run, then apply with
the returned hash. Assert groups, TCP target, push monitor, hashed token, import
run, and repeated `skip`/`update` behavior through real routes.

- [ ] **Step 2: Add a CLI-to-real-router test**

Serve the real Pulse router with `httptest.NewServer`, execute the Cobra command
against the sanitized v1 fixture, and assert the command succeeds without the
old permissive `/api/monitors` mock. Verify the database contains the expected
source/external identities and no unsupported monitors.

- [ ] **Step 3: Add migration-continuity push verification**

Read the fake imported push token from the fixture, call the Pulse-compatible
`/api/push/{token}` endpoint, and assert an up check result exists. This proves an
existing v1 heartbeat client can switch only its base hostname.

- [ ] **Step 4: Run complete Go verification**

Run:

```bash
cd api && go vet ./... && go test ./... -count=1
cd ../cli && go vet ./... && go test ./... -count=1
cd ../agent && go vet ./... && go test ./... -count=1
```

Expected: all commands PASS.

- [ ] **Step 5: Commit end-to-end coverage**

```bash
git add api/internal/server/import_integration_test.go api/internal/handlers/push_test.go cli/importcmd/import_test.go
git commit -m "test(import): verify migration through the real API"
```

### Task 11: Ship the CLI and correct the documentation

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `Makefile`
- Modify: `deploy/install.sh`
- Modify: `docs/getting-started.md`
- Modify: `README.md`

- [ ] **Step 1: Expand local and CI verification to every Go module**

Make `make test` execute `go test ./... -count=1` separately in `api`, `cli`, and
`agent`. Make lint/vet and gofmt checks cover the same module directories. Mirror
those commands in CI so importer tests gate pull requests and releases.

- [ ] **Step 2: Publish both binaries**

Extend the release target loop to build:

```bash
(cd api && GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w" -o "../dist-bin/pulse_${os}_${arch}" ./cmd/pulse)
(cd cli && GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w" -o "../dist-bin/pulse-cli_${os}_${arch}" ./cmd/pulse-cli)
```

Include both patterns in SHA256SUMS and GitHub release assets.

- [ ] **Step 3: Install both binaries**

Update `deploy/install.sh` to download and verify `pulse` and `pulse-cli`, install
both under `/usr/local/bin`, and keep the systemd service pointed only at
`/usr/local/bin/pulse`. If either current-release asset is missing, fail with the
exact missing URL rather than silently installing a partial toolset.

- [ ] **Step 4: Correct migration documentation**

Document:

- the explicit Uptime Kuma v1-only support boundary;
- the v2 removal of JSON backup/restore;
- `PULSE_TOKEN`, `--dry-run`, `--conflict`, `--accept-behavior-changes`, and
  `--output json`;
- compatibility statuses and exit codes;
- atomicity, rerun behavior, and the 1 MiB normalized-plan limit;
- push endpoint continuity and token rotation; and
- unsupported fields/types without claiming groups or monitors are silently
  imported.

- [ ] **Step 5: Run final release-shaped verification**

Run:

```bash
make test
make ui
for module in api cli agent; do (cd "$module" && go vet ./...); done
git diff --check
```

Expected: all tests/builds pass and `git diff --check` prints nothing.

- [ ] **Step 6: Build local release binaries as a smoke test**

Run:

```bash
(cd api && CGO_ENABLED=0 go build -trimpath -o ../bin/pulse ./cmd/pulse)
(cd cli && CGO_ENABLED=0 go build -trimpath -o ../bin/pulse-cli ./cmd/pulse-cli)
./bin/pulse-cli import uptime-kuma --help
```

Expected: both binaries build and help shows dry-run, conflict,
accept-behavior-changes, and output flags.

- [ ] **Step 7: Commit release and documentation changes**

```bash
git add .github/workflows Makefile deploy/install.sh docs/getting-started.md README.md
git commit -m "build: test and distribute the migration CLI"
```

### Task 12: Final review against the approved design

**Files:**
- Review: `docs/superpowers/specs/2026-07-14-uptime-kuma-migration-foundation-design.md`
- Review: all files changed by Tasks 1-11

- [ ] **Step 1: Run the complete verification set once more**

Run:

```bash
make test
make ui
cd api && go test ./... -race -count=1
cd ../cli && go test ./... -race -count=1
cd ../agent && go test ./... -race -count=1
```

Expected: PASS. If the SQLite driver makes a race-enabled test unsupported,
record the exact failing package and fix the test or implementation; do not omit
the non-race suite.

- [ ] **Step 2: Verify security properties with database queries**

After the integration test fixture runs, assert:

```sql
SELECT COUNT(*) FROM push_monitor_tokens WHERE length(token_hash) = 64;
SELECT COUNT(*) FROM push_monitor_tokens WHERE token_hash = 'abcdefghij';
SELECT COUNT(*) FROM import_runs WHERE summary_json LIKE '%abcdefghij%';
```

Expected: hashed-token count equals imported push monitor count; plaintext and
summary-secret counts are zero.

- [ ] **Step 3: Verify design coverage manually**

Check off every goal and non-goal in the design. Confirm v2 is rejected, gRPC is
absent, unsupported resources block apply, behavior changes require explicit
acceptance, dry-run writes nothing, apply rolls back atomically, and the CLI is
present in release assets.

- [ ] **Step 4: Review commit boundaries and working tree**

Run: `git status --short && git log --oneline --decorate -12`

Expected: no uncommitted implementation files and one focused commit for each
task slice.
