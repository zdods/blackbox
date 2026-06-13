# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- Daemon and file endpoints are now scoped to the authenticated user (each
  daemon carries an `owner_id`), so a request can only reach daemons the caller
  registered — single-user today, but it forecloses a cross-tenant access bug if
  multi-user is ever added. CORS headers are sent only for a configured, matching
  origin instead of unconditionally.
- TOTP (2FA) secrets are now encrypted at rest with AES-256-GCM, using
  `TOTP_ENC_KEY` or a key derived from a stable `JWT_SECRET` (existing plaintext
  secrets are migrated on startup). An operator-supplied `JWT_SECRET` must be at
  least 32 bytes. Login adds a per-account attempt limiter and equalizes timing
  so a username can't be enumerated, the rate limiter reads the correct
  (right-most) `X-Forwarded-For` hop behind a proxy, state-changing requests get
  a same-origin (CSRF) check, and `COOKIE_SECURE=1` forces a Secure session
  cookie when TLS terminates at a proxy.

## [0.5.1] - 2026-06-13

### Security

- Hardened the server's HTTP and WebSocket surface: every response now sends
  `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, and
  `Referrer-Policy: no-referrer` (plus HSTS under TLS); file downloads are
  served as `application/octet-stream` so a stored HTML/SVG file can't be
  sniffed and rendered in-origin; the console ships a Content-Security-Policy.
  Inbound daemon WebSocket frames are size-capped (no OOM from an oversized
  frame), a missing binary follow-up frame can no longer stall the connection,
  and streaming downloads validate each chunk against the requested size.
- Daemon now contains symlink escapes: a symlink inside a hosted directory that
  points outside it can no longer be used to read, write, or delete files beyond
  the hosted root. The daemon also validates and bounds every server-supplied
  field (upload id, chunk index/count, read size and offset), caps concurrent
  uploads, assembles chunked uploads atomically, and writes files `0600`.

### Fixed

- Mobile responsiveness across the console. The header no longer wraps the
  "log out" button onto two lines on narrow phones (≤320px); long filenames in
  the file browser now break at hyphens instead of mid-word; file sizes stay on
  one line; and the file preview modal stops truncating short filenames on
  small screens. Per-row and host-card actions also get larger touch targets on
  mobile. Layout is verified at 320/375/768/1280px via the responsive e2e suite.

## [0.5.0] - 2026-06-13

### Changed

- File transfers now use bounded memory end to end. Downloads stream straight
  to disk in the browser (large files no longer buffer in a tab-memory Blob),
  and the server saves them under the file's real name via a
  `Content-Disposition` header. Uploads larger than 6 MB must use the chunked
  protocol (the console already does this automatically); the single-request
  upload path is capped so the server can never buffer a large file in RAM.
  Concurrent transfers per server are capped to bound worst-case memory.
- Compose project name pinned to `blackhaul`, so containers and the data
  volume are named `blackhaul-*` regardless of the checkout directory.
  Existing compose deployments get a fresh volume on next `up` — to keep
  data, either set `COMPOSE_PROJECT_NAME` to your old project name or copy
  the old volume's contents before removing it.

## [0.4.1] - 2026-06-11

### Added

- Console favicon and home-screen icons: the [▪‿▪] face as an SVG
  favicon with PNG fallbacks, apple-touch-icon, and a web manifest so
  the console can be added to a phone home screen as a standalone app.

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

[Unreleased]: https://github.com/zdods/blackhaul/compare/v0.5.1...HEAD
[0.5.1]: https://github.com/zdods/blackhaul/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/zdods/blackhaul/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/zdods/blackhaul/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/zdods/blackhaul/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/zdods/blackhaul/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/zdods/blackhaul/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/zdods/blackhaul/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/zdods/blackhaul/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/zdods/blackhaul/releases/tag/v0.0.1
