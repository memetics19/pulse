# Deployment

Pulse ships as a single container image published to the GitHub Container
Registry (GHCR) at `ghcr.io/memetics19/pulse`. Releases and deployment are
automated: merge work to `main`, merge the release PR, and the new version is
built, published, and rolled out to the homelab.

## Release & deploy flow

```
commit (Conventional Commits) ─▶ main
        │
        ├─ ci.yml            run tests on every push / PR
        │
        ├─ release-please.yml maintains a release PR from the commit history
        │        │
        │        └─ merge release PR ─▶ tag vX.Y.Z + GitHub release + CHANGELOG
        │
        └─ release.yml (on tag vX.Y.Z)
                 ├─ test          re-run the suite
                 ├─ release       build + push ghcr.io/memetics19/pulse:{version,latest}
                 └─ deploy        join Netbird → SSH to the host → pull + restart
```

Tests gate everything: `release.yml` re-runs them before building, and `deploy`
only runs after `release` succeeds, so a broken build never reaches the homelab.

## Versioning

Versions are computed by [release-please](https://github.com/googleapis/release-please)
from Conventional Commit messages: `fix:` → patch, `feat:` → minor, `!` /
`BREAKING CHANGE:` → major (minor while pre-1.0). The seed version lives in
`.release-please-manifest.json`.

## Homelab host

The status page runs on a private host (`10.2.0.115`) reachable over a
[Netbird](https://netbird.io) VPN (the **managed cloud**, not self-hosted — so
no management URL is needed). The deploy job joins the same Netbird network from
the GitHub runner with an ephemeral setup key, then connects over SSH.

### One-time host setup

```sh
# Docker + compose plugin must be installed.
git clone https://github.com/memetics19/pulse.git ~/pulse
cd ~/pulse
# If the GHCR package is private, authenticate once (else make the package public):
#   echo "$GHCR_READ_TOKEN" | docker login ghcr.io -u memetics19 --password-stdin
docker compose -f deploy/docker-compose.homelab.yml up -d
```

`deploy/docker-compose.homelab.yml` pulls the `pulse` image from GHCR (no local
build) and serves it, plus the docs site, behind Caddy (`deploy/Caddyfile`).

### Required GitHub secrets

| Secret | Purpose |
|---|---|
| `PULSE_TOKEN` | GHCR push (already configured) |
| `NETBIRD_SETUP_KEY` | Ephemeral key so the runner joins the managed Netbird network |
| `SSH_PRIVATE_KEY` | Deploy key authorized on the host |
| `SSH_HOST_FINGERPRINT` | Host key, to pin the SSH connection |
| `DEPLOY_HOST` | `10.2.0.115` |
| `DEPLOY_USER` | `ubuntu` |

### What the deploy job runs on the host

```sh
cd ~/pulse && git fetch --tags && git pull --ff-only
docker compose -f deploy/docker-compose.homelab.yml pull
docker compose -f deploy/docker-compose.homelab.yml up -d
docker image prune -f
```

## Docs deployment

The documentation site (`docs.shreeda.xyz`) deploys to the **same host over the
same Netbird network**, on its own schedule, via `.github/workflows/deploy-docs.yml`:

- **On pull requests** touching `docs/**` or `mkdocs.yml`, it runs `mkdocs build`
  to validate the docs (CI gate).
- **On push to `main`** touching those paths, it joins Netbird, SSHes to the host,
  `git pull`s, and re-renders the docs:
  ```sh
  docker compose -f deploy/docker-compose.homelab.yml run --rm docs-build
  ```
  The `docs-build` one-shot writes the static site into the `docs_site` volume,
  which the already-running Caddy serves live — no Caddy restart needed.

Docs changes therefore ship on merge to `main` without waiting for an app
release. (App releases also re-render the docs as part of `compose up -d`.)

## Rollback

Pin the previous version and bring the stack back up on the host:

```sh
cd ~/pulse
docker compose -f deploy/docker-compose.homelab.yml pull pulse  # or edit the image tag
docker run ... ghcr.io/memetics19/pulse:<previous-version>       # or set the tag and `up -d`
```

The simplest rollback is to re-point the `pulse` image tag to the previous
`vX.Y.Z` and run `docker compose ... up -d`.

## Manual / local production stack

For a self-built stack (no GHCR), `deploy/docker-compose.prod.yml` builds the
image locally instead of pulling it.
