# Deployment

Pulse ships as a container image on the GitHub Container Registry,
`ghcr.io/memetics19/pulse`, and as binaries on each GitHub Release. Releases are automated:
merge work to `main`, merge the release PR, and the new version is built and published.

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
                     +- release    image ghcr.io/memetics19/pulse:X.Y.Z, X.Y and latest
                     +- binaries   pulse_<os>_<arch> and SHA256SUMS on the Release
```

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

`deploy/docker-compose.homelab.yml` runs the published image behind Caddy together with
the docs site, and `deploy/docker-compose.prod.yml` builds the image locally instead. Put
a reverse proxy that terminates TLS in front of either. See
[Configuration](configuration.md) for `PULSE_SECURE_COOKIES`, `PULSE_TRUSTED_PROXIES` and
`PULSE_ALLOW_PRIVATE_MONITORS`. Without the last one, monitors that point at private
addresses (10.x, 192.168.x) are refused, because that guard stops monitor URLs from probing
internal networks.

The server reads only `PULSE_*` settings, apart from `RESEND_API_KEY` and
`SLACK_WEBHOOK_URL`. Pulse 3.0 renamed `API_PORT` to `PULSE_PORT`, `SQLITE_PATH` to
`PULSE_DB` and `DATABASE_URL` to `PULSE_DATABASE_URL`. The old names still work until 4.0
and log a warning at startup. See [Renamed in 3.0](configuration.md#renamed-in-30).

To run on another port inside the container, set `PULSE_PORT` and publish that port:

```sh
docker run -d --name pulse -p 9090:9090 -e PULSE_PORT=9090 -v pulse_data:/data ghcr.io/memetics19/pulse:latest
```

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

A release can change the database schema. Keep a copy of the volume from before the upgrade
if you might need to go back.
