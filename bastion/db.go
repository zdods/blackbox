package main

import (
	"context"
	"embed"
	"fmt"
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

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		sql, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
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
