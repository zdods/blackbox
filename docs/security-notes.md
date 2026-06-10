# Security design & audit notes

Internal notes on the security-relevant design decisions and audits.
For the disclosure policy, see the root [SECURITY.md](../SECURITY.md).

## Authentication & sessions (bastion)

- Passwords hashed with **bcrypt**; **TOTP is mandatory** for every account.
- Sessions are JWTs delivered as **httpOnly cookies only** — never returned
  in response bodies or stored in localStorage.
- JWTs carry `users.token_version`, checked on every request; logout bumps
  the version, revoking **all** of a user's sessions at once.
- An unset/default `JWT_SECRET` cannot sign tokens: the server generates a
  random ephemeral secret at startup (sessions reset on restart) and logs a
  warning. `DEV_MODE=1` opts into a stable dev secret for local work.
- Rate limiting (`bastion/ratelimit.go`): sliding window, 10/min per IP on
  `/api/login`, `/api/login/totp`, and `/api/register*` (`TRUST_PROXY=1`
  keys on `X-Forwarded-For` behind a proxy); TOTP locks after 5 failed
  codes per 15 minutes per account.

## Daemon tokens

- Tokens are **hashed at rest** (SHA-256, base64url) in the `token_hash`
  column; the presented token is hashed in Go before lookup, so plaintext
  never reaches SQL. Legacy plaintext rows were backfilled and cleared by
  migration `002_auth_hardening.sql`.
- On the daemon side, the token is stored in the **OS keyring**
  (`zalando/go-keyring`), not in the config file.

## Path traversal

- The daemon resolves every requested path against the hosted root and
  rejects escapes; covered by `daemon/safepath_test.go` and
  `daemon/handlers_test.go`.
- The bastion's static file handler applies the same containment for the
  web console assets; covered by `bastion/static_test.go`.

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

- Compose binds Postgres to `127.0.0.1` and bakes no JWT secret.
- The bastion image runs as a non-root user, installs with `npm ci`, and
  has a `HEALTHCHECK` against `/healthz`.
- Release artifacts ship with SHA-256 checksums; the Docker image is
  published with SBOM and provenance attestation.
