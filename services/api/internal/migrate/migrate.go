package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/*.sql
var sqlFiles embed.FS

// Run creates the schema_migrations tracking table if needed, then applies
// every embedded *.sql file that has not been applied yet, in lexicographic order.
// It is safe to call on every startup — already-applied migrations are skipped.
//
// On the first run against a pre-existing database (one initialised by
// docker-entrypoint-initdb.d before this runner existed), all known migration
// files are recorded as already applied without re-executing them, so the
// runner does not attempt to recreate tables that already exist.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureTable(ctx, pool); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	files, err := fs.Glob(sqlFiles, "sql/*.sql")
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}
	sort.Strings(files)

	// Baseline: if schema_migrations is empty but the database already has
	// tables (pre-existing install), mark all known migrations as applied
	// without executing them.
	if err := baseline(ctx, pool, files); err != nil {
		return fmt.Errorf("baseline: %w", err)
	}

	applied := 0
	for _, path := range files {
		version := path[len("sql/"):] // strip "sql/" prefix → bare filename
		ok, err := isApplied(ctx, pool, version)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if ok {
			continue
		}

		sql, err := sqlFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		if err := apply(ctx, pool, version, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		slog.Info("migration applied", "version", version)
		applied++
	}

	if applied == 0 {
		slog.Info("migrations up to date", "total", len(files))
	} else {
		slog.Info("migrations complete", "applied", applied, "total", len(files))
	}
	return nil
}

// baseline detects a pre-existing database (schema_migrations empty but core
// tables present) and records all known migrations as applied so they are not
// re-executed against the already-initialised schema.
func baseline(ctx context.Context, pool *pgxpool.Pool, files []string) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // already tracking — nothing to do
	}

	// Check whether the database was pre-initialised (users table exists).
	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='users')`,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil // truly empty database — let the runner apply everything normally
	}

	// Pre-existing database: record all known files as applied without executing them.
	slog.Info("baseline: pre-existing database detected, recording all migrations as applied")
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, path := range files {
		version := path[len("sql/"):]
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations(version) VALUES($1) ON CONFLICT DO NOTHING`, version,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func ensureTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	return err
}

func isApplied(ctx context.Context, pool *pgxpool.Pool, version string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version,
	).Scan(&exists)
	return exists, err
}

func apply(ctx context.Context, pool *pgxpool.Pool, version, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Split on semicolons so multi-statement files work correctly.
	for _, stmt := range splitStatements(sql) {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("statement failed: %w", err)
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations(version) VALUES($1)`, version,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// splitStatements splits SQL on semicolons, dropping empty/whitespace-only chunks.
func splitStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
