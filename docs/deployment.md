# Deployment

Pulse ships as a container image on the GitHub Container Registry,
`ghcr.io/memetics19/pulse`, and as binaries and packages on each GitHub Release. The agent
has its own image, `ghcr.io/memetics19/pulse-agent`. Both images are built for linux/amd64
and linux/arm64, so the same tag runs on an x86 server and on an arm64 board such as a
Raspberry Pi. Releases are automated: merge work to `main`, merge the release PR, and the
new version is built and published.

## Release flow

```
commit (Conventional Commits) -> main
   |
   +- ci.yml, security.yml   tests, coverage gates, lint and Trivy on every push and PR
   |
   +- release-please.yml     keeps a release PR up to date from the commit history
         |
         +- merge the release PR -> tag vX.Y.Z, GitHub Release, CHANGELOG.md
               |
               +- release.yml (called for that tag)
                     +- test       the whole suite again
                     +- images     ghcr.io/memetics19/pulse and pulse-agent, X.Y.Z, X.Y and latest
                     |             amd64 and arm64, with SBOM and provenance, signed with cosign
                     +- binaries   pulse_<os>_<arch>, .deb and .rpm packages, SBOMs, SHA256SUMS
                                   and its signature on the Release, each with build provenance
```

Every published file and image can be checked against this repository. See
[Verify a release](security.md#verify-a-release).

Every workflow uses the repository's own `GITHUB_TOKEN`. There is no personal access token
to renew. Release-please needs "Allow GitHub Actions to create and approve pull requests"
under Settings, Actions, General. A `vX.Y.Z` tag pushed by hand runs `release.yml` too.

## Versioning

[release-please](https://github.com/googleapis/release-please) computes the version from
Conventional Commit messages. `fix:` is a patch, `feat:` a minor version, and `!` or a
`BREAKING CHANGE:` footer a major one. The current version is in
`.release-please-manifest.json`.

## Running it

Pulse needs nothing but its image and a volume for the SQLite file:

```sh
docker run -d --name pulse -p 8080:8080 -v pulse_data:/data ghcr.io/memetics19/pulse:latest
```

Docker pulls the image for the host's architecture by itself.

The agent image runs `pulse-agent` with the token in the environment:

```sh
docker run -d --name pulse-agent -e PULSE_AGENT_TOKEN=<token> \
  ghcr.io/memetics19/pulse-agent:latest --server https://status.example.com
```

Inside a container the agent reports what the container can see. For the metrics of the
host itself, install the binary or the package on the host instead.

`deploy/docker-compose.homelab.yml` runs the published image behind Caddy together with
the docs site, and `deploy/docker-compose.prod.yml` builds the image locally instead. Put
a reverse proxy that terminates TLS in front of either. See
[Configuration](configuration.md) for `PULSE_SECURE_COOKIES`, `PULSE_TRUSTED_PROXIES` and
`PULSE_ALLOW_PRIVATE_MONITORS`. Without the last one, monitors that point at private
addresses (10.x, 192.168.x) are refused, because that guard stops monitor URLs from probing
internal networks.

The server reads only `PULSE_*` settings, apart from `RESEND_API_KEY` and
`SLACK_WEBHOOK_URL`. Pulse 3.0 renamed `API_PORT` to `PULSE_PORT`, `SQLITE_PATH` to
`PULSE_DB` and `DATABASE_URL` to `PULSE_DATABASE_URL`. The first two old names still work
until 4.0 and log a warning at startup. `DATABASE_URL` is not read, because hosting
platforms set it on their own. See [Renamed in 3.0](configuration.md#renamed-in-30).

To run on another port inside the container, set `PULSE_PORT` and publish that port. The
image's health check probes the same port:

```sh
docker run -d --name pulse -p 9090:9090 -e PULSE_PORT=9090 -v pulse_data:/data ghcr.io/memetics19/pulse:latest
```

## Backup and restore

`pulse backup` copies the database while Pulse runs. You no longer need to stop the
container and copy the volume.

```sh
docker exec pulse pulse backup
```

It writes a snapshot with SQLite's `VACUUM INTO`. The snapshot is one consistent file with
no `-wal` or `-shm`. Pulse keeps serving pages and checking monitors while it runs. Do not
copy a live `pulse.db` with `cp`. Recent changes can still sit in `pulse.db-wal`, and a
copy of the main file alone can be torn.

Pulse checks the snapshot with `PRAGMA integrity_check`, then prints its path and size:

```text
backup written: /data/pulse-backup-20261030T101500Z.db (212992 bytes, schema version 14)
pulse.json copied to /data/pulse-backup-20261030T101500Z.db.json
```

Without `--out`, the snapshot goes next to the database. Use `--out <file>` to choose the
file. Pulse never replaces a file that exists. It also copies `pulse.json` beside the
snapshot as `<file>.json`. Restore does not need that copy. It records the setup state in
case you rebuild an install by hand.

To keep a snapshot off the host:

```sh
docker exec pulse pulse backup --out /data/before-upgrade.db
docker cp pulse:/data/before-upgrade.db .
```

With the `.deb` or `.rpm` package, run it as the service user and point it at the data
directory:

```sh
sudo -u pulse env PULSE_DATA_DIR=/var/lib/pulse pulse backup
```

### Restore

`pulse restore <file>` puts a snapshot back. Stop Pulse first. Restore refuses while the
server runs.

```sh
docker stop pulse
docker run --rm --user root -v pulse_data:/data -v "$PWD:/backup:ro" \
  ghcr.io/memetics19/pulse:<version> restore /backup/before-upgrade.db
docker start pulse
```

`--user root` lets restore read a file you copied in. The restored database gets the owner
and mode of the one it replaces, so the server can still open it. On an empty volume it
gets the owner of the data directory.

With the `.deb` or `.rpm` package:

```sh
sudo systemctl stop pulse
sudo -u pulse env PULSE_DATA_DIR=/var/lib/pulse pulse restore /var/lib/pulse/before-upgrade.db
sudo systemctl start pulse
```

Restore does this:

1. It checks that the server is stopped. A running server holds a lock on `pulse.lock` in
   the data directory, and restore needs that lock. The kernel drops the lock when the
   server exits, even after a crash. Restore also asks SQLite for sole access to the
   database. That catches a server from an older release, which takes no lock.
2. It copies the snapshot next to the database and runs `PRAGMA integrity_check` on the
   copy. Your snapshot file is never changed.
3. It refuses a snapshot from a newer release. Its schema version must not be higher than
   the newest migration this binary knows. An older snapshot is fine. Pulse migrates it
   when it starts.
4. It folds the old database's write-ahead log back into the file. The old database stays
   next to the new one as `pulse.db.pre-restore-<time>`.
5. It renames the checked copy over `pulse.db` in one step.

Restore writes into the database this install uses, so it keeps the current
`pulse.json`. On an empty volume there is no `pulse.json`. Pulse then adopts the restored
file when it starts. See [When pulse.json is missing](configuration.md#when-pulsejson-is-missing).

Why a lock and not a check of `/healthz`? The port can differ from the default, and the
server can run in another network namespace, such as another container on the same
volume. The lock lives on the volume itself, so any process that sees the database also
sees the lock.

### Automatic backups

Set `PULSE_BACKUP_DIR` and the server takes a snapshot there once a day. It keeps the
newest seven. Set `PULSE_BACKUP_KEEP` to keep a different number. The snapshots are named
`pulse-auto-<time>.db`, each with its copy of `pulse.json`. Retention removes only files
with that name, so a backup you take by hand in the same directory stays.

```yaml
environment:
  PULSE_BACKUP_DIR: /data/backups
  PULSE_BACKUP_KEEP: "7"
```

The first snapshot comes about a minute after the server starts, unless the directory
already holds one from the last 24 hours. A snapshot that fails is logged, and the next
try comes an hour later. A directory on the data volume does not survive the loss of that
volume. Copy the snapshots somewhere else as well.

The `.deb` and `.rpm` service can only write under `/var/lib/pulse`. Use a directory there,
such as `/var/lib/pulse/backups`, or add yours to `ReadWritePaths` in a drop-in for the
unit.

## status.shreeda.xyz and docs.shreeda.xyz

The public instance runs in a homelab managed by
[anvil](https://github.com/memetics19/anvil). Nothing on GitHub can reach that host, and
the repository holds no SSH key, password or VPN key for it. The host pulls instead:

- Every 10 minutes it looks for a newer `vX.Y.Z` tag. It pulls that image, copies the data
  volume, switches over, and checks `/healthz`. If the new version does not answer within
  90 seconds, it goes back to the previous image and the copied data.
- On the same run it rebuilds the docs site from the head of `main` with mkdocs.

So a merged release reaches status.shreeda.xyz within about 10 minutes, and a docs change
on `main` reaches docs.shreeda.xyz the same way, without waiting for a release.

## Rollback

On a host you run yourself, start the previous version's image on the same volume:

```sh
docker stop pulse && docker rm pulse
docker run -d --name pulse -p 8080:8080 -v pulse_data:/data ghcr.io/memetics19/pulse:<previous-version>
```

A release can change the database schema, and an older release cannot open a newer schema.
Take a snapshot with `pulse backup` before you upgrade. To go back, start the previous
version with that snapshot restored. See [Restore](#restore).
