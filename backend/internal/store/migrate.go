package store

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationLockID is an arbitrary but stable key for the advisory lock that
// serialises migration runs. Deploys can overlap — a rollback released while
// the previous release is still finishing — and without the lock both would
// try to create the same types.
const migrationLockID int64 = 54172026

const createMigrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    text PRIMARY KEY,
	applied_at timestamptz NOT NULL DEFAULT now()
)`

// Migrate applies every .sql file in fsys the database has not recorded yet,
// in filename order and each in its own transaction, then returns the versions
// it applied. Running it against an up-to-date database is a no-op, which is
// what makes it safe as a release command on every deploy.
//
// baseline records the pending files as applied *without running them*. It is
// for adopting a database whose schema was created before this runner existed
// — the development database built by docker-compose, for instance.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, baseline bool) ([]string, error) {
	versions, err := migrationVersions(fsys)
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	// A session-level lock, so it covers the whole run rather than one
	// statement. It is released explicitly below: returning the connection to
	// the pool would not drop it.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return nil, fmt.Errorf("migrate: acquire lock: %w", err)
	}
	defer func() {
		// WithoutCancel so the lock is still released when the caller's
		// context is what ended the run.
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", migrationLockID)
	}()

	if _, err := conn.Exec(ctx, createMigrationsTable); err != nil {
		return nil, fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return nil, err
	}

	var ran []string
	for _, version := range versions {
		if applied[version] {
			continue
		}
		if err := applyOne(ctx, conn, fsys, version, baseline); err != nil {
			return ran, err
		}
		ran = append(ran, version)
	}
	return ran, nil
}

// PendingMigrations reports the versions Migrate would apply, without
// changing anything. It backs the deploy pipeline's dry run.
func PendingMigrations(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) ([]string, error) {
	versions, err := migrationVersions(fsys)
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	// A dry run must not change the database, so the tracking table is probed
	// rather than created. Its absence is itself the answer: nothing applied.
	var exists bool
	if err := conn.QueryRow(ctx,
		"SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return nil, fmt.Errorf("migrate: probe schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	if exists {
		if applied, err = appliedVersions(ctx, conn); err != nil {
			return nil, err
		}
	}

	var pending []string
	for _, version := range versions {
		if !applied[version] {
			pending = append(pending, version)
		}
	}
	return pending, nil
}

// applyOne runs a single migration and records it, both or neither: the
// insert shares the migration's transaction, so a failure half-way cannot
// leave the database changed but unrecorded.
func applyOne(ctx context.Context, conn *pgxpool.Conn, fsys fs.FS, version string, baseline bool) error {
	statements, err := fs.ReadFile(fsys, version)
	if err != nil {
		return fmt.Errorf("migrate: read %s: %w", version, err)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: begin %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if !baseline {
		// pgx sends a query with no arguments over the simple protocol, which
		// is what allows a whole multi-statement file in one Exec.
		if _, err := tx.Exec(ctx, string(statements)); err != nil {
			return fmt.Errorf("migrate: apply %s: %w", version, err)
		}
	}

	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
		return fmt.Errorf("migrate: record %s: %w", version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit %s: %w", version, err)
	}
	return nil
}

// appliedVersions reads the versions the database has already run.
func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied versions: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("migrate: scan applied version: %w", err)
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

// migrationVersions lists the .sql files in fsys, sorted. Filenames are the
// versions, so the numeric prefix convention (0001_, 0002_, …) is what orders
// the run — keep it zero-padded.
func migrationVersions(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: list migrations: %w", err)
	}

	versions := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			versions = append(versions, e.Name())
		}
	}
	sort.Strings(versions)

	if len(versions) == 0 {
		return nil, fmt.Errorf("migrate: no .sql migrations found")
	}
	return versions, nil
}
