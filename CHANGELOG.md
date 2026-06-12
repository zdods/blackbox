# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-06-11

### Changed

- **Project renamed: blackbox → blackhaul.** Everything user-facing follows:
  the daemon binary is `blackhaul-daemon`, the server image is
  `ghcr.io/zdods/blackhaul-bastion`, env vars are `BLACKHAUL_*`, the config
  dir is `~/.blackhaul-daemon`, the OS keyring service, systemd/launchd
  service names, the Homebrew cask, and the default Postgres database name
  (`blackhaul`). Existing installs must migrate by hand (pre-1.0, no shim):
  rename the config dir, re-enter the token, reinstall the service unit, and
  `ALTER DATABASE blackbox RENAME TO blackhaul`. Old GitHub URLs redirect.

## [0.3.0] - 2026-06-11

### Added

- Theme system for the console: light, dark, and system themes plus the
  previous Nord palette as a legacy option; the choice persists in the
  browser and follows the OS setting by default.
- Static landing page under `site/` for the project front door.

### Changed

- Console redesign: modern app shell with a sticky header (brand, theme
  picker, log out), card-based hosts and file views, and refreshed
  login/register pages. The terminal-window chrome and `$` prompt
  prefixes are gone; the `[▪‿▪]` face now doubles as a status indicator
  (loading, error, offline).
- README no longer embeds console screenshots.

## [0.2.0] - 2026-06-10

### Added

- Structured logs: one line per request with status, duration, and a
  request ID (also returned as `X-Request-ID`); `LOG_FORMAT=json` for
  JSON lines, `LOG_LEVEL=debug` to include static-asset requests.
- Mobile-friendly console: fluid login/register forms and a file
  browser that fits small screens (modified column hidden, upload row
  wraps, full-width preview modal under 640px).
- The daemon explains failed connections: dial errors include the HTTP
  status and a hint for common reverse-proxy mistakes (ws:// against an
  HTTPS redirect, proxy not forwarding upgrade headers). New
  [deployment guide](docs/deployment.md) with nginx/OpenResty, Caddy,
  and Traefik configs.

### Changed

- Database migrations are tracked in `schema_migrations` and each
  applies exactly once (previously every migration re-ran on boot).

### Fixed

- A request racing the daemon's WebSocket auth handshake could corrupt
  frames (unsynchronized `auth_ok` write); the daemon now registers in
  the hub only after the handshake completes.

### Changed

- `docker compose up` now pulls the published GHCR image by default (pin
  with `BASTION_IMAGE_TAG`); building from source moved to the
  `docker-compose.dev.yml` overlay (`make dev`).
- Console fonts (JetBrains Mono) are self-hosted — no Google Fonts CDN
  request, works offline.
- Hosts list responds faster with several connected daemons: disk stats
  are fetched concurrently instead of serially.

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

[Unreleased]: https://github.com/zdods/blackhaul/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/zdods/blackhaul/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/zdods/blackhaul/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/zdods/blackhaul/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/zdods/blackhaul/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/zdods/blackhaul/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/zdods/blackhaul/releases/tag/v0.0.1
