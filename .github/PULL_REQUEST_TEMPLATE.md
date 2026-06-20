<!--
Conventional Commits drive releases. Keep commit messages — and, if you
squash-merge, the PR title — in `type(scope): summary` form.
No AI / co-author attribution (the commit-msg hook enforces this).
-->

## Summary

<!-- What does this PR do, and why? 1–3 sentences. -->

## Type of change

- [ ] `feat` — new feature
- [ ] `fix` — bug fix
- [ ] `refactor` — no behaviour change
- [ ] `docs`
- [ ] `ci` / `build` — pipeline or tooling
- [ ] `test`
- [ ] `chore`
- [ ] **Breaking change** (`!` / `BREAKING CHANGE:` footer)

## Changes

<!-- Bullet the notable changes, grouped by area: api / worker / ui / cli / ci / docs. -->

-

## Breaking changes

<!-- Describe the break and how consumers migrate. Delete this section if none. -->

## Test plan

- [ ] `cd api && go test ./... -count=1`
- [ ] `cd agent && go test ./... -count=1`
- [ ] `cd cli && go test ./... -count=1`
- [ ] `cd ui && npx tsc --noEmit` (if the UI changed)
- [ ] Manual check: <!-- what you clicked / ran -->

## Checklist

- [ ] Commits follow Conventional Commits
- [ ] No `Co-Authored-By` / AI attribution (enforced by the `commit-msg` hook)
- [ ] `gofmt`-clean and `go vet ./...` passes
- [ ] Docs updated (`docs/`, `README.md`) where relevant
- [ ] No secrets or `.env` files committed

## Deployment notes

<!-- New env vars, secrets, migrations, or host steps? Otherwise write "none". -->
