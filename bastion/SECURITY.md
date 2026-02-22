# Security notes (bastion)

## SQL injection audit

All database access uses **parameterized queries** (pgx `$1`, `$2`, …). No user or request input is ever concatenated into SQL.

| File        | Usage |
|------------|--------|
| `api.go`   | `ListAgents`: `SELECT … FROM agents ORDER BY label` (no user input). `CreateAgent`: `INSERT … VALUES ($1, $2, $3)`. `UpdateAgent`: `UPDATE … SET label = $1 WHERE id::text = $2`. `DeleteAgent`: `DELETE … WHERE id::text = $1`. |
| `auth.go`   | `CreateUser`: `INSERT … VALUES ($1, $2)`. `HasAnyUser`: `SELECT count(*) FROM users`. `GetUserByUsername`: `SELECT … WHERE username = $1`. |
| `agentws.go` | `SELECT id::text FROM agents WHERE token = $1`. |
| `db.go`    | `RunMigrations`: runs static embedded SQL (schema only). |

When adding new queries, always use placeholders for any dynamic values.
