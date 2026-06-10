# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-06-10

### Fixed

- Homebrew cask publishing from the release pipeline (the v0.1.0 cask had
  to be published manually due to a template bug in the tap token).

## [0.1.0] - 2026-06-10

### Added

- `--version` flag on both binaries; version embedded at build time
  (`blackbox/pkg/version` via ldflags) in the daemon, the bastion, and the
  Docker image.
- Prebuilt daemon binaries (GoReleaser) for Linux, macOS, and Windows ×
  amd64/arm64, attached to GitHub Releases with archives and SHA-256
  checksums.
- One-line daemon installer: `curl -fsSL https://raw.githubusercontent.com/zdods/blackbox/main/install.sh | sh`
  — detects OS/arch, verifies checksums, installs to `/usr/local/bin`.
- Homebrew tap for the daemon: `brew install zdods/tap/blackbox-daemon`.
- Security policy ([SECURITY.md](SECURITY.md)) with private vulnerability
  reporting, plus contributor docs and issue/PR templates.

### Security

- Patched web console dependency advisories; bumped base images
  (alpine 3.24, golang 1.26) and Go dependencies.

## [0.0.1] - 2026-06-09

First tagged release: self-hosted single-user cloud storage with a Go
bastion (auth, daemon hub, file-proxy API), a cross-platform daemon
(outbound-only WebSocket, OS-keyring token storage), and a SvelteKit
console.

### Added

- Bastion Docker image published to GHCR on semver tags
  (`ghcr.io/zdods/blackbox-bastion`): multi-arch (amd64/arm64), SBOM +
  provenance attestation.
- `GET /healthz` liveness endpoint (DB ping), wired into the Docker
  `HEALTHCHECK`.

### Security

- Rate limiting on login/TOTP/register (sliding window, 10/min per IP);
  TOTP lockout after 5 failed codes per 15 min per account.
- Default JWT secret can no longer sign tokens: unset secret → random
  ephemeral secret at startup (`DEV_MODE=1` keeps a stable dev secret).
- Daemon tokens hashed at rest (SHA-256); plaintext backfilled and cleared
  by migration.
- Session revocation via `users.token_version`; logout revokes all
  sessions; JWT no longer returned in login response bodies (cookie only).
- Compose binds Postgres to `127.0.0.1`; bastion container runs as
  non-root.

[Unreleased]: https://github.com/zdods/blackbox/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/zdods/blackbox/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/zdods/blackbox/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/zdods/blackbox/releases/tag/v0.0.1
