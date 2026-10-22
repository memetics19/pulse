# Contributing to Pulse

Thanks for contributing. This guide covers the dev setup, the commit policy
(enforced by git hooks), and how to open a pull request.

## Development setup

Pulse is a Go workspace (`go.work`) with three modules, `api`, `agent` and `cli`,
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

The `commit-msg` hook rejects AI and co-author trailers, and `pre-commit` rejects Go
that is not `gofmt`'d. Don't skip them with `--no-verify`.

## Pull requests

1. Push your branch and open a PR against `main`. The
   [PR template](.github/PULL_REQUEST_TEMPLATE.md) loads automatically. Fill in
   every section (summary, type, changes, breaking changes, test plan, checklist).
2. CI must be green (vet, gofmt, tests, UI build) before review.
3. Keep PRs focused, with one logical change per commit.
4. PRs are **squash-merged**, so the PR title becomes the commit on `main` that
   release-please reads for the changelog and the version. The title must be a
   Conventional Commit (CI checks it). Mark a breaking change with `!` in the
   title, for example `feat!: ...`, and describe it in a `BREAKING CHANGE:` footer
   in the squash message.

## Releases & deployment

Versioning is automated with
[release-please](https://github.com/googleapis/release-please): merging
Conventional Commits to `main` keeps a release PR up to date. Merging that PR tags
`vX.Y.Z`, publishes a GitHub release + `CHANGELOG.md`, and builds and pushes the
image to GHCR. The homelab picks the new image up by itself. See
[docs/deployment.md](docs/deployment.md) for the full flow.
