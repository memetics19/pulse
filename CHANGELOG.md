# Changelog

## [3.0.0](https://github.com/memetics19/pulse/compare/v2.1.0...v3.0.0) (2026-10-23)


### ⚠ BREAKING CHANGES

* API_PORT is renamed to PULSE_PORT, SQLITE_PATH to PULSE_DB and DATABASE_URL to PULSE_DATABASE_URL. The old names are still read when the new name is unset, with a startup warning, and stop working in 4.0.
* **agent:** pulse-agent no longer accepts --token. Set PULSE_AGENT_TOKEN or pass --token-file. When both are set, --token-file now takes precedence over PULSE_AGENT_TOKEN (the environment variable used to win).

### Features

* **agent:** the token comes from PULSE_AGENT_TOKEN or --token-file only ([#40](https://github.com/memetics19/pulse/issues/40)) ([aa2ef9b](https://github.com/memetics19/pulse/commit/aa2ef9bf7cc241b59fcf2ed36c5b04d0267b6005))
* Prometheus metrics at /metrics, off by default ([#43](https://github.com/memetics19/pulse/issues/43)) ([1b9a952](https://github.com/memetics19/pulse/commit/1b9a95263db5ed5080f53d8deacde2489b0d335e))
* PULSE_* names for every server setting ([#42](https://github.com/memetics19/pulse/issues/42)) ([86a3c8d](https://github.com/memetics19/pulse/commit/86a3c8d4f7414323bce1dfbf836af1ad1717bd27))


### Bug Fixes

* clear the code scanning alerts, and govulncheck in CI ([#38](https://github.com/memetics19/pulse/issues/38)) ([22a1854](https://github.com/memetics19/pulse/commit/22a185455ce11fedc1002618d2a57af15bd63767))
* incremental rollup, and prune infra metrics and expired sessions ([#58](https://github.com/memetics19/pulse/issues/58)) ([61c9aba](https://github.com/memetics19/pulse/commit/61c9aba611bbca7fd9dc03299ce445043b3dbbcd)), closes [#52](https://github.com/memetics19/pulse/issues/52)
* PULSE_DB is the database file everywhere, and the healthcheck follows PULSE_PORT ([#57](https://github.com/memetics19/pulse/issues/57)) ([53d0ef5](https://github.com/memetics19/pulse/commit/53d0ef5f5c43c3c7528e926cd5fa44490aa7dd63)), closes [#53](https://github.com/memetics19/pulse/issues/53)
* **security:** 2FA can only be turned off with a code, and setup never drops it ([#56](https://github.com/memetics19/pulse/issues/56)) ([12b0ec0](https://github.com/memetics19/pulse/commit/12b0ec09b05a78cb64a5778c526eda3e11432cfe)), closes [#49](https://github.com/memetics19/pulse/issues/49)
* **security:** a 2FA code works once, and password or 2FA changes sign out other sessions ([#60](https://github.com/memetics19/pulse/issues/60)) ([e17de72](https://github.com/memetics19/pulse/commit/e17de728f2f4a91da3ab930a22b532c471447a74)), closes [#49](https://github.com/memetics19/pulse/issues/49)
* **security:** notification webhooks cannot reach internal addresses ([#54](https://github.com/memetics19/pulse/issues/54)) ([dd6b020](https://github.com/memetics19/pulse/commit/dd6b0207f3b02738b78e5624928bb8a49bf3053f)), closes [#50](https://github.com/memetics19/pulse/issues/50)
* **security:** only one setup can ever create the admin account ([#55](https://github.com/memetics19/pulse/issues/55)) ([c5b8161](https://github.com/memetics19/pulse/commit/c5b8161116880097e6a655affe0c7a6cbfd483a6)), closes [#45](https://github.com/memetics19/pulse/issues/45)

## [2.1.0](https://github.com/memetics19/pulse/compare/v2.0.0...v2.1.0) (2026-10-23)


### Features

* .deb and .rpm packages for pulse, pulse-agent and pulse-cli ([#36](https://github.com/memetics19/pulse/issues/36)) ([fb91448](https://github.com/memetics19/pulse/commit/fb91448b6ec520fd36f00cf5768f14a177c30c88))

## [2.0.0](https://github.com/memetics19/pulse/compare/v1.1.0...v2.0.0) (2026-10-22)


### ⚠ BREAKING CHANGES

* GET /api/status is scoped to the status page of the request's Host and returns a sanitized DTO instead of raw monitor models.

### Features

* **agent:** collect and store host diagnostic bundles ([#20](https://github.com/memetics19/pulse/issues/20)) ([185b2b8](https://github.com/memetics19/pulse/commit/185b2b84b4e1c05bcd42b29087a4e5f341edb242))
* import monitors and groups from an Uptime Kuma backup with the CLI ([8bfb0d7](https://github.com/memetics19/pulse/commit/8bfb0d7bfdb30b57c69954c14fddb8fd23fd7fce))


### Bug Fixes

* check every error golangci-lint flags, and make lint blocking ([#34](https://github.com/memetics19/pulse/issues/34)) ([39f6a8f](https://github.com/memetics19/pulse/commit/39f6a8f588fb1dcd7d6761e355ed746edeb0ee93))
* patch all known vulnerabilities and gate CI on Trivy ([#29](https://github.com/memetics19/pulse/issues/29)) ([bea114a](https://github.com/memetics19/pulse/commit/bea114ae9eb1e68ca6190d7849dc0cd6c7c8548f))
* security and reliability hardening, agent token and proxy config, coverage gates ([e782450](https://github.com/memetics19/pulse/commit/e782450ab01702bd7729c71b81d44c5ba8f959f6))

## [1.1.0](https://github.com/memetics19/pulse/compare/v1.0.0...v1.1.0) (2026-07-04)


### Features

* **deploy:** distribute pulse as a cross-platform binary with an OS/arch-aware installer ([#7](https://github.com/memetics19/pulse/issues/7)) ([a344878](https://github.com/memetics19/pulse/commit/a344878997f682165660e346ef3ed01c9a30df75))

## 1.0.0 (2026-06-20)


### Bug Fixes

* **checker:** floor response time to 1ms; fix timezone-sensitive pruner test ([8f547ef](https://github.com/memetics19/pulse/commit/8f547ef642e3e4fedab26e6787d40ce0973bf315))
