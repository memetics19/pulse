# Contributing to Pulse

Thanks for contributing. This guide covers the dev setup, the commit policy
(enforced by git hooks), and how to open a pull request.

## Development setup

Pulse is a Go workspace (`go.work`) with three modules — `api`, `agent`, `cli` —
plus a Next.js admin UI in `ui/`.

```sh
# Build the binary (embeds the admin UI) and run it on :8080
make run

# Run the test suites
cd api   && go test ./... -count=1
cd agent && go test ./... -count=1
cd cli   && go test ./... -count=1

# Regenerate DB access code after editing api/internal/db/queries or migrations
make sqlc          # requires sqlc v1.31.1

# UI
cd ui && npm ci && npm run build
```

## Branching

Branch off `main` with a type prefix: `feat/...`, `fix/...`, `docs/...`,
`refactor/...`, `ci/...`. Do not commit directly to `main`.

## Commit policy (enforced)

These rules apply to everyone, including AI assistants:

- **Conventional Commits.** `type(scope): summary` (`feat`, `fix`, `refactor`,
  `docs`, `build`, `ci`, `test`, `chore`). Mark breaking changes with a `!` after
  the type or a `BREAKING CHANGE:` footer. This drives automatic versioning.
- **No AI / co-author attribution.** No `Co-Authored-By`, no "Generated with …"
  lines, no bot trailers.
- **No force-pushes to `main`.**
- **No slop.** Code is `gofmt`-clean, passes `go vet ./...`, and passes the test
  suite before it is committed.

### Enable the hooks (once per clone)

The repo ships hooks under `.githooks/` that enforce the rules above:

```sh
git config core.hooksPath .githooks
```

`commit-msg` rejects AI/co-author trailers; `pre-commit` rejects un-`gofmt`'d Go.
(`--no-verify` bypasses git hooks — don't.)

## Pull requests

1. Push your branch and open a PR against `main`. The
   [PR template](.github/PULL_REQUEST_TEMPLATE.md) loads automatically — fill in
   every section (summary, type, changes, breaking changes, test plan, checklist).
2. CI must be green (vet, gofmt, tests, UI build) before review.
3. Keep PRs focused; one logical change per commit even if the PR bundles several.
4. **Merge-commit** the PR (don't squash) so each Conventional Commit reaches
   `main` — release-please relies on them for the changelog and the version bump.
   If you must squash, the PR title itself must be a Conventional Commit and carry
   any `BREAKING CHANGE:` footer.

## Releases & deployment

Versioning is automated with
[release-please](https://github.com/googleapis/release-please): merging
Conventional Commits to `main` maintains a release PR; merging that PR tags
`vX.Y.Z`, publishes a GitHub release + `CHANGELOG.md`, builds and pushes the
image to GHCR, and deploys to the homelab. See
[docs/deployment.md](docs/deployment.md) for the full flow and required secrets.
