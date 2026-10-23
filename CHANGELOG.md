# Changelog

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
