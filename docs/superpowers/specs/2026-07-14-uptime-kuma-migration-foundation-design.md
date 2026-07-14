# Uptime Kuma Migration Foundation Design

**Status:** Approved on 2026-07-14

## Context

Pulse currently has a small `pulse-cli import uptime-kuma` command that reads an
Uptime Kuma JSON backup and creates one group followed by monitors through
individual REST calls. That path is not safe for real migrations: Uptime Kuma
v2 removed JSON backup/restore, real v1 exports use protocol-specific fields
that the parser ignores, push monitors are incorrectly converted to HTTP,
imports can leave partial state, reruns create duplicates, and the CLI is not
tested or shipped by the normal release workflow.

This design establishes a reliable migration foundation for Uptime Kuma v1
JSON exports. It adds a native push/heartbeat monitor because converting push
to polling is not semantically valid. It deliberately does not add gRPC.

## Goals

- Explicitly support Uptime Kuma v1 JSON configuration exports.
- Reject Uptime Kuma v2 and unknown input formats with actionable guidance.
- Produce a dry-run report before any write.
- Detect invalid, unsupported, behavior-changing, and conflicting resources.
- Apply a validated import atomically.
- Make reruns and network retries idempotent.
- Preserve stable source identities for imported groups and monitors.
- Add native, Uptime Kuma-compatible push heartbeat ingestion.
- Protect push credentials at rest and during reporting.
- Test the importer against sanitized real-export fixtures and the real Pulse
  router/database stack.
- Run CLI tests in CI and publish an installable `pulse-cli` binary.

## Non-goals

- Uptime Kuma v2 data-directory or database migration.
- gRPC as a monitor type or management transport.
- Importing historical heartbeat/check data.
- Importing notification provider secrets or status-page definitions.
- Adding every Uptime Kuma monitor type.
- A complete general-purpose audit log.
- Full Pulse export/apply, OpenAPI, recurring maintenance, or general bulk CRUD.
  Those remain later workstreams that can reuse the normalized import contract.

## Chosen approach

The CLI owns source-specific parsing and conversion. The Pulse API owns
authoritative validation, conflict detection, and transactional persistence.
The CLI sends normalized Pulse resources rather than raw Uptime Kuma backups,
so the server never needs to understand every upstream backup version and the
same API can later support CSV, YAML, or other migration adapters.

Two alternatives were rejected:

- Client-side create-and-rollback cannot guarantee cleanup after a crash or
  network loss.
- Direct database migration bypasses validation and scoped authorization and is
  brittle across Pulse versions.

## Architecture

The migration flow is:

1. Read and size-limit the local backup file.
2. Detect and validate the Uptime Kuma v1 export version.
3. Parse the export with a versioned v1 schema.
4. Convert every source resource into a normalized import plan.
5. Classify compatibility locally and redact sensitive values from output.
6. Send the normalized plan to Pulse for authoritative validation.
7. Display the server report and plan hash.
8. Stop for `--dry-run`, or submit the same plan and hash for apply.
9. Revalidate against current database state and apply all resource mutations in
   one SQLite transaction.
10. Return an import result manifest and persist an import-run record.

The primary internal units are:

- **Uptime Kuma v1 parser:** decodes the complete fields required for supported
  conversion and rejects incompatible versions.
- **Converter:** maps v1 resources into source-neutral group and monitor inputs,
  recording field-level compatibility findings.
- **Reporter:** renders deterministic human and JSON output without secrets.
- **Import planner:** performs Pulse validation and conflict detection and
  produces a stable plan hash.
- **Import applier:** verifies the plan hash and applies the plan atomically.
- **Check recorder:** centralizes check-result persistence and incident/alert
  evaluation so scheduled checks and push heartbeats use identical behavior.
- **Push heartbeat service:** authenticates heartbeat tokens, records incoming
  results, rotates credentials, and detects missed heartbeats.

## Source support and compatibility rules

### Supported source format

The parser accepts JSON with a v1 version and `monitorList`. The supported
version family is `1.x`; tests use fixtures from maintained late-v1 exports.
Missing versions, malformed versions, `2.x`, and unknown major versions are
invalid. For v2, the error explains that JSON backup/restore was removed and
that a v2 migration adapter is not yet available.

The input file may contain sensitive notification data. The CLI reads it
locally but sends only normalized group and monitor data needed by Pulse. It
never prints or logs raw backup JSON.

### Resource statuses

Every group and monitor receives exactly one highest-severity status:

- `compatible`: Pulse can preserve the relevant behavior.
- `behavior_change`: the resource can be imported, but identified behavior or
  organization will differ.
- `unsupported`: Pulse cannot safely run the resource.
- `invalid`: source data is malformed or lacks required values.
- `conflict`: an existing Pulse resource with the stable source identity or a
  conflicting target identity requires policy resolution.

Findings contain a stable code, resource identity, field, and redacted message.
Apply is blocked by `invalid` or `unsupported`. Conflicts are handled by the
selected conflict policy. `behavior_change` is blocked unless the request sets
`accept_behavior_changes` and the CLI uses
`--accept-behavior-changes`. The older `allow_lossy` name is not used.

### Monitor conversion

- Uptime Kuma `http` maps to Pulse `http` when it is a GET request with behavior
  Pulse can express.
- Uptime Kuma `keyword` maps to Pulse `http` plus `keyword_check` when matching
  is non-inverted and the remaining HTTP behavior is supported.
- TCP targets are built from `hostname` and `port` with IPv6-safe host/port
  joining. The source `url` field is not used as the TCP target.
- Ping and DNS targets come from `hostname`, not `url`.
- Push maps to native Pulse `push` and retains the source push token through the
  secure token-import path.
- Top-level Uptime Kuma groups map to Pulse groups. Nested group paths are
  flattened to `Parent / Child` and reported as a behavior change because Pulse
  groups are currently flat.
- Active state, interval, timeout, name, supported expected status, keyword, and
  group membership are preserved when representable.
- The default Uptime Kuma accepted-status rule `200-299` maps to Pulse's default
  2xx behavior. A single explicit status such as `200` maps to
  `expected_status`. Multiple values, mixed ranges, or any other range are
  unsupported because Pulse cannot express them exactly.
- Custom methods, request bodies, headers, authentication, custom TLS behavior,
  inverted keywords, JSON-query checks, and incompatible accepted-status rules
  make the monitor unsupported rather than creating a monitor that would check
  the wrong thing.
- Retry settings, notification associations, tags, and other non-execution
  metadata that Pulse cannot yet represent are reported as behavior changes.
- Other monitor types are unsupported and remain visible in the report.

No monitor is silently skipped or converted to a different protocol.

## Import API

The normalized import API is protected by a dedicated `imports:write` API-key
scope or an authenticated admin session.

The endpoint is `POST /api/imports`. Planning and applying use the same request
shape so validation cannot drift:

```json
{
  "source": "uptime-kuma",
  "source_version": "1.23.16",
  "dry_run": true,
  "conflict_policy": "fail",
  "accept_behavior_changes": false,
  "idempotency_key": "client-generated-random-value",
  "expected_plan_hash": "",
  "resources": {
    "groups": [],
    "monitors": []
  }
}
```

The CLI always performs a planning request first. The planning response includes
the complete redacted report and `plan_hash`. Apply sends the same normalized
resources with `dry_run: false` and `expected_plan_hash` set. The server
recomputes validation and conflicts inside the apply transaction and rejects a
stale hash with HTTP 409.

The normalized request body remains subject to Pulse's existing 1 MiB API body
limit in the first release. The CLI checks the encoded plan size before sending
and reports a clear size-limit error.

### Conflict policies

- `fail` is the default and blocks apply when any conflict exists.
- `skip` leaves the existing resource unchanged and records it as skipped.
- `update` updates mutable fields on the resource with the same stable source
  identity. It does not take over an unrelated internal resource based only on
  a matching display name.

Groups and monitors receive `source` and `external_id`. Partial unique indexes
on `(source, external_id)` where `external_id <> ''` enforce identity without
affecting existing internal resources. Uptime Kuma identities use source IDs,
for example `group:12` and `monitor:42`.

SQLite has no old-version-safe conditional `ADD COLUMN`. Migration 9 downgrade
therefore rebuilds the legacy group table and preserves group identities in a
migration sidecar; migration 9 re-up restores those identities and removes the
sidecar.

Migrations normally use golang-migrate's SQLite transaction wrapping. The two
migration 9 files carry an explicit marker handled by a focused driver wrapper:
it disables foreign keys on one connection, runs the marked SQL in its own
transaction, rolls back on failure, and restores foreign keys on every path.
Migration 9 preflights duplicate monitor identities before rebuilding tables
and preserves each rebuilt AUTOINCREMENT table's `sqlite_sequence` high-water.
Real v8-to-v9, early-collision, late-failure, rollback/re-up, and sequence-reuse
tests exercise this production driver boundary.

`idempotency_key` protects a submitted apply from duplicate network delivery.
Repeated delivery returns the stored result. A later intentional rerun with a
new key is still idempotent at the resource level through source identity and
the selected conflict policy.

### Transaction and import-run records

An `import_runs` row is created before resource mutation with source, version,
input hash, idempotency key, policy, state, and timestamps. Group and monitor
mutations occur in one transaction. On success the transaction commits and the
run is marked completed with counts. On failure the transaction rolls back and
the run is marked failed with a redacted error summary. A failed run therefore
leaves traceability without leaving partial imported resources.

The response manifest includes run ID, plan hash, created/updated/skipped counts,
resource findings, and completion state. It never contains raw authentication
values or push tokens.

## Native push monitor behavior

### Storage and lifecycle

`push` becomes a valid monitor type. Push monitors are allowed to have an empty
`url`; other monitor types use type-specific target validation.

Credentials live in a separate `push_monitor_tokens` table with one active
credential per monitor, a unique SHA-256 token hash, a non-sensitive prefix for
identification, and creation/rotation timestamps. Plaintext tokens are never
persisted. Imported Uptime Kuma push tokens are hashed during the transaction.

New or rotated tokens are returned once. Rotation replaces the stored hash and
immediately invalidates the previous token. Token management requires the
normal monitor write scope or an admin session. Creating a push monitor through
`POST /api/monitors` returns the new token and complete push URL once. Imported
tokens are accepted and hashed but are not echoed back. The authenticated
`POST /api/monitors/{id}/push-token/rotate` endpoint returns a replacement token
and URL once.

### Compatible heartbeat endpoint

Pulse accepts the established Uptime Kuma heartbeat form:

```text
GET  /api/push/{token}?status=up&msg=OK&ping=123
POST /api/push/{token}?status=up&msg=OK&ping=123
```

The endpoint is public because the high-entropy token is the credential. It:

- hashes the supplied token and looks up an active push monitor;
- accepts only `up` or `down` status;
- defaults the message to `OK` and limits its UTF-8 encoding to 1,024 bytes;
- accepts an optional finite ping value from 0 through 100,000,000,000
  milliseconds, matching the upstream compatibility bound;
- records receipt time on the server;
- writes a standard check result and invokes the shared incident/alert flow; and
- returns the same not-found response for missing, invalid, inactive, and
  non-push monitors.

Logs and errors never include the token or full request URL.

Imported tokens must match `[A-Za-z0-9_-]{10,128}` so legacy ten-character v1
tokens remain valid. Pulse-generated tokens contain 32 URL-safe random
characters with at least 192 bits of entropy.

### Missed heartbeat detection

Push monitors do not run a network checker. The scheduler runs a watchdog whose
first deadline is monitor creation plus `interval_seconds` and whose subsequent
deadline is the last heartbeat receipt plus `interval_seconds` plus a five-second
network-jitter allowance. Once a deadline is missed, it records a down result
through the shared check recorder. A later valid heartbeat records recovery.

Scheduler reconciliation starts, updates, and stops push watchdogs when monitor
configuration or active state changes, just as it manages polling monitors.

## CLI experience

The primary commands are:

```text
PULSE_TOKEN=... pulse-cli import uptime-kuma \
  --file backup.json \
  --server https://status.example.com \
  --dry-run

PULSE_TOKEN=... pulse-cli import uptime-kuma \
  --file backup.json \
  --server https://status.example.com \
  --conflict update \
  --accept-behavior-changes
```

`PULSE_TOKEN` is preferred so secrets do not appear in process listings. The
legacy `--token` option may remain temporarily for compatibility but is hidden
from examples and emits no token value.

Human output presents totals followed by actionable findings grouped by status.
`--output json` emits a stable machine-readable report. Exit codes distinguish
success (`0`), malformed input (`2`), blocked compatibility (`3`), conflict or
stale plan (`4`), and authentication/API/transport failure (`5`). Dry-run never
creates a group, monitor, token, or import-run record.

The admin monitor form includes `Push` in the type selector, replaces the URL
field with an "Expected heartbeat every" interval control, and displays the
new endpoint once after creation. Existing push monitors show only the stored
token prefix and a rotation action because Pulse cannot recover hashed tokens.
The rotation confirmation warns that the previous endpoint stops working
immediately.

## Error handling

- Parser errors identify the JSON location or missing required field without
  echoing source values that may contain secrets.
- API validation errors use stable finding codes rather than relying only on
  prose.
- API clients include a size-limited, redacted server error message in failures
  instead of reporting only the HTTP status.
- Context and cancellation flow from Cobra through HTTP requests and server
  transactions.
- An interrupted apply rolls back resource mutations. A retried request with the
  same idempotency key returns the recorded result or reports that the earlier
  attempt did not complete.
- A stale plan or changed conflict state returns HTTP 409 and instructs the CLI
  to rerun planning.

## Testing strategy

Testing follows TDD and includes:

- Sanitized fixtures produced from real Uptime Kuma late-v1 exports.
- Parser golden tests for version detection, HTTP, keyword, TCP, ping, DNS,
  groups, nested groups, push, unsupported types, and secret redaction.
- Conversion tests for IPv4, IPv6, missing host/port, behavior changes, and
  unsupported execution semantics.
- Import planner tests for all statuses, stable plan hashes, and all conflict
  policies.
- Real SQLite/router integration tests for planning and apply; permissive mock
  handlers are not sufficient.
- A forced failure after earlier resource inserts proving that the resource
  transaction rolls back completely.
- Repeated-delivery and intentional-rerun tests proving request and resource
  idempotency.
- Push endpoint tests for GET and POST, up/down, invalid input, inactive
  monitors, unknown tokens, redacted logging, token rotation, missed deadlines,
  recovery, and incident creation.
- CLI tests for human/JSON reports, exit codes, environment-token handling,
  cancellation, size limits, and server error propagation.
- CI jobs that vet, format, and test all Go modules (`api`, `cli`, and `agent`).
- Release checks that build and publish both `pulse` and `pulse-cli` and verify
  documented installation commands.

## Documentation and release behavior

Documentation will say "Uptime Kuma v1 JSON import" everywhere and include a
prominent v2 limitation. It will explain dry-run, conflict modes, behavior-change
acceptance, push URL migration, token safety, rerun behavior, and unsupported
fields.

The release workflow publishes `pulse-cli_<os>_<arch>` alongside the server
binary and includes both in checksums. The installer either installs both
binaries or the documentation provides an equally tested dedicated CLI install
path. CI and release workflows must test the CLI before claiming importer
support.

## Later workstreams

The normalized planner/applier and source identity model are intended to support
later, separately designed work for:

- Uptime Kuma v2 migration from supported data backups;
- complete Pulse configuration export and idempotent apply;
- versioned OpenAPI documentation and generated clients;
- general bulk operations;
- a full audit-event subsystem; and
- recurring maintenance schedules.
