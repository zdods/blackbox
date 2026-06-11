package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func OpenDB(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// RunMigrations applies pending migration files in filename order, tracking
// applied versions in schema_migrations so each file runs exactly once. Each
// migration executes inside a transaction together with its version record.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("schema_migrations table: %w", err)
	}
	applied := map[string]bool{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	ran := 0
	for _, name := range names {
		if applied[name] {
			continue
		}
		sql, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
		slog.Info("migration applied", "version", name)
		ran++
	}
	if ran == 0 {
		slog.Info("migrations up to date", "applied", len(applied))
	}
	return backfillDaemonTokenHashes(ctx, pool)
}

// backfillDaemonTokenHashes hashes any plaintext daemon tokens left over from
// before 002_auth_hardening and clears the plaintext column. The hash is
// computed in Go so it matches HashDaemonToken exactly.
func backfillDaemonTokenHashes(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx,
		`SELECT id::text, token FROM daemons WHERE token IS NOT NULL AND token_hash IS NULL`)
	if err != nil {
		return err
	}
	type row struct{ id, token string }
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.token); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range pending {
		if _, err := pool.Exec(ctx,
			`UPDATE daemons SET token_hash = $1, token = NULL WHERE id::text = $2`,
			HashDaemonToken(r.token), r.id,
		); err != nil {
			return err
		}
	}
	return nil
}
