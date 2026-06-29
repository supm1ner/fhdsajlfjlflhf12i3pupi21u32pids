package database

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// migration is a single forward migration parsed from an embedded *.up.sql file.
type migration struct {
	version int
	name    string
	sql     string
}

// Migrate applies every pending *.up.sql migration from src in ascending version
// order, tracking applied versions in a schema_migrations table. It is
// idempotent: already-applied versions are skipped, so re-running against an
// up-to-date database is a no-op (the platform-foundation requirement). Each
// migration runs inside a transaction together with its bookkeeping insert, so a
// failure rolls back cleanly and leaves the version unrecorded.
//
// File naming convention: NNNN_name.up.sql where NNNN is a zero-padded integer
// version (e.g. 0001_init.up.sql).
func Migrate(ctx context.Context, pool poolExecer, src fs.FS, log *slog.Logger) error {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return err
	}

	migs, err := loadMigrations(src)
	if err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		if err := applyOne(ctx, pool, m); err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", m.version, m.name, err)
		}
		if log != nil {
			log.Info("migration applied",
				slog.Int("version", m.version),
				slog.String("name", m.name),
			)
		}
	}
	return nil
}

// poolExecer is the subset of *pgxpool.Pool used by the migrator. Declaring it
// as an interface keeps Migrate testable and decoupled.
type poolExecer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

func ensureMigrationsTable(ctx context.Context, pool poolExecer) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, pool poolExecer) (map[int]bool, error) {
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func applyOne(ctx context.Context, pool poolExecer, m migration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Roll back on any early return; the commit below makes this a no-op.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
		m.version, m.name,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// loadMigrations parses all *.up.sql files from src into version-sorted slice.
func loadMigrations(src fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var migs []migration
	seen := make(map[int]bool)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		version, label, err := parseMigrationName(name)
		if err != nil {
			return nil, err
		}
		if seen[version] {
			return nil, fmt.Errorf("duplicate migration version %d", version)
		}
		seen[version] = true

		body, err := fs.ReadFile(src, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		migs = append(migs, migration{version: version, name: label, sql: string(body)})
	}

	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

// parseMigrationName splits "0001_init.up.sql" into (1, "init").
func parseMigrationName(filename string) (int, string, error) {
	base := strings.TrimSuffix(filename, ".up.sql")
	idx := strings.IndexByte(base, '_')
	if idx <= 0 {
		return 0, "", fmt.Errorf("malformed migration filename %q (want NNNN_name.up.sql)", filename)
	}
	version, err := strconv.Atoi(base[:idx])
	if err != nil {
		return 0, "", fmt.Errorf("migration %q: invalid version: %w", filename, err)
	}
	return version, base[idx+1:], nil
}

// poolAdapter adapts *pgxpool.Pool to poolExecer.
type poolAdapter struct{ db *DB }

func (a poolAdapter) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return a.db.Pool.Exec(ctx, sql, args...)
}
func (a poolAdapter) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return a.db.Pool.Query(ctx, sql, args...)
}
func (a poolAdapter) Begin(ctx context.Context) (pgx.Tx, error) {
	return a.db.Pool.Begin(ctx)
}

// RunMigrations is the convenience entry point used by main.go: it adapts the
// live pool and applies all embedded migrations.
func (db *DB) RunMigrations(ctx context.Context, src fs.FS, log *slog.Logger) error {
	return Migrate(ctx, poolAdapter{db: db}, src, log)
}
