# Security design & audit notes

Internal notes on the security-relevant design decisions and audits.
For the disclosure policy, see the root [SECURITY.md](../SECURITY.md).

## Authentication & sessions (bastion)

- Passwords hashed with **bcrypt**; **TOTP is mandatory** for every account.
  A missing user still runs a bcrypt comparison so login timing can't reveal
  whether a username exists.
- **TOTP secrets are encrypted at rest** (AES-256-GCM, `enc:v1:` tagged). The
  key comes from `TOTP_ENC_KEY` (base64 of 32 bytes) or, if unset, is derived
  via HKDF from a stable `JWT_SECRET`; with neither, secrets stay plaintext
  (quick-start) and a warning is logged. Existing plaintext secrets are
  re-encrypted on startup. (Note: a key derived from `JWT_SECRET` is tied to it
  — rotating `JWT_SECRET` then makes stored secrets undecryptable; set a
  dedicated `TOTP_ENC_KEY` to decouple.)
- Sessions are JWTs delivered as **httpOnly cookies only** — never returned
  in response bodies or stored in localStorage (the `blackhaul_authed`
  localStorage value is a UI-only "probably logged in" flag, not a credential).
  `Secure` is set under in-process TLS or when `COOKIE_SECURE=1` (TLS at a
  proxy). HS256; the keyfunc rejects any non-HMAC alg (no alg-confusion).
- JWTs carry `users.token_version`, checked on every request; logout bumps
  the version, revoking **all** of a user's sessions at once.
- An unset/default `JWT_SECRET` cannot sign tokens: the server generates a
  random ephemeral secret at startup (sessions reset on restart) and logs a
  warning. `DEV_MODE=1` opts into a stable dev secret for local work. An
  operator-supplied secret **must be ≥32 bytes** or the server refuses to start.
- **CSRF:** cookie-authenticated state-changing requests must be same-origin
  (`Origin` checked against `Host`); `Bearer`-token API clients are exempt.
  SameSite=Lax is the primary defense; this is belt-and-suspenders.
- Rate limiting (`bastion/ratelimit.go`): sliding window, 10/min per IP on
  `/api/login`, `/api/login/totp`, and `/api/register*`; plus a **per-account**
  limiter (5 failed passwords / 15 min) independent of IP. Behind a proxy
  (`TRUST_PROXY=1`) the **right-most** `X-Forwarded-For` hop is used (the
  left entries are client-spoofable). TOTP locks after 5 failed codes per
  15 minutes per account.

## Daemon tokens

- Tokens are **hashed at rest** (SHA-256, base64url) in the `token_hash`
  column; the presented token is hashed in Go before lookup, so plaintext
  never reaches SQL. Legacy plaintext rows were backfilled and cleared by
  migration `002_auth_hardening.sql`.
- On the daemon side, the token is stored in the **OS keyring**
  (`zalando/go-keyring`), not in the config file.

## Path traversal & daemon input

- The daemon contains every requested path to the hosted root: `safePath`
  applies a lexical guard (`filepath.IsLocal`) **and** resolves symlinks
  (`EvalSymlinks` on the deepest existing ancestor), so a symlink inside the
  hosted dir that points outside it cannot be used to read/write/delete beyond
  the root. Covered by `daemon/safepath_test.go`, `daemon/security_test.go`,
  and `daemon/handlers_test.go`.
- Every bastion-supplied field is validated/bounded as untrusted: `upload_id`
  against a strict charset before it names a temp dir, chunk index/count, read
  size and offset; concurrent uploads and per-request memory are capped, and
  chunked uploads assemble atomically (temp + rename). Files are written `0600`.
- The bastion's static file handler applies the same containment for the
  web console assets; covered by `bastion/static_test.go`.

## Access scoping (daemons)

- Each daemon carries an `owner_id` (migration `003_daemon_ownership.sql`);
  `ListDaemons`, `UpdateDaemon`, `DeleteDaemon`, and the file/meta proxy
  endpoints scope to the authenticated user, returning 404 for a daemon the
  caller doesn't own. Single-user today, but it forecloses a cross-tenant IDOR
  ahead of any multi-user support.

## HTTP & WebSocket hardening

- Every response sends `X-Content-Type-Options: nosniff`, `X-Frame-Options:
  DENY`, and `Referrer-Policy: no-referrer` (plus HSTS under TLS). File
  downloads are served `application/octet-stream` so a stored HTML/SVG file
  can't be MIME-sniffed and rendered in the console's origin.
- The console ships a **Content-Security-Policy** (SvelteKit `kit.csp`, hash
  mode): scripts locked to `'self'` + the hashed theme bootstrap, `object-src`
  / `base-uri` `'none'`, `frame-ancestors 'none'`.
- The daemon WebSocket has a per-frame read limit (no OOM from an oversized
  frame); a missing binary follow-up frame can't stall the read loop; and
  streaming downloads validate each chunk against the requested size.
- CORS headers are emitted only for a configured, matching `Origin`.

## SQL injection audit

All database access uses **parameterized queries** (pgx `$1`, `$2`, …). No
user or request input is ever concatenated into SQL.

| File        | Usage |
|------------|--------|
| `api.go`   | `ListDaemons`: `SELECT … FROM daemons ORDER BY label` (no user input). `CreateDaemon`: `INSERT … VALUES ($1, $2, $3)`. `UpdateDaemon`: `UPDATE … SET label = $1 WHERE id::text = $2`. `DeleteDaemon`: `DELETE … WHERE id::text = $1`. |
| `auth.go`   | `CreateUser`: `INSERT … VALUES ($1, $2)`. `HasAnyUser`: `SELECT count(*) FROM users`. `GetUserByUsername`: `SELECT … WHERE username = $1`. `GetTokenVersion`/`BumpTokenVersion`: `… WHERE id = $1`. |
| `daemonws.go` | `SELECT id::text FROM daemons WHERE token_hash = $1` (presented token is hashed in Go before lookup; plaintext tokens are never stored). |
| `db.go`    | `RunMigrations`: runs static embedded SQL (schema only). `backfillDaemonTokenHashes`: `UPDATE … WHERE id::text = $2`. |

When adding new queries, always use placeholders for any dynamic values.

## Deployment defaults

- Compose binds Postgres to `127.0.0.1` and bakes no JWT secret. The bastion
  service drops all Linux capabilities, sets `no-new-privileges`, and caps
  memory/PIDs; Postgres sets `no-new-privileges` and caps memory.
- The bastion image runs as a non-root user, installs with `npm ci`, and
  has a `HEALTHCHECK` against `/healthz`.
- Release artifacts ship with SHA-256 checksums; the daemon installer verifies
  them by exact filename. The Docker image is published with SBOM and provenance
  attestation. The build context (`.dockerignore`) excludes repo metadata and
  local secrets.
- Change `POSTGRES_PASSWORD` from the dev default before any non-loopback
  deployment; set `JWT_SECRET` (≥32 bytes) and ideally `TOTP_ENC_KEY`.
