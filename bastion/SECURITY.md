# Security notes (bastion)

## SQL injection audit

All database access uses **parameterized queries** (pgx `$1`, `$2`, …). No user or request input is ever concatenated into SQL.

| File        | Usage |
|------------|--------|
| `api.go`   | `ListDaemons`: `SELECT … FROM daemons ORDER BY label` (no user input). `CreateDaemon`: `INSERT … VALUES ($1, $2, $3)`. `UpdateDaemon`: `UPDATE … SET label = $1 WHERE id::text = $2`. `DeleteDaemon`: `DELETE … WHERE id::text = $1`. |
| `auth.go`   | `CreateUser`: `INSERT … VALUES ($1, $2)`. `HasAnyUser`: `SELECT count(*) FROM users`. `GetUserByUsername`: `SELECT … WHERE username = $1`. |
| `daemonws.go` | `SELECT id::text FROM daemons WHERE token = $1`. |
| `db.go`    | `RunMigrations`: runs static embedded SQL (schema only). |

When adding new queries, always use placeholders for any dynamic values.
