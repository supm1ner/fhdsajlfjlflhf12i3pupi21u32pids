// Package database provides cotton-id's PostgreSQL access: a pooled pgx
// connection established with retry/backoff, a hand-written embedded-SQL
// migrator, and a health check.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgx connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Connect opens a pgxpool to dsn, retrying with capped exponential backoff until
// the database is reachable or ctx is cancelled. This satisfies the "backend
// waits for its dependencies" requirement: on a cold compose start Postgres may
// not be ready yet.
func Connect(ctx context.Context, dsn string, log *slog.Logger) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	// Conservative pool sizing for a single-node deployment.
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute

	const maxAttempts = 30
	backoff := 250 * time.Millisecond
	const maxBackoff = 5 * time.Second

	var pool *pgxpool.Pool
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			// NewWithConfig is lazy; Ping forces an actual connection.
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				if log != nil {
					log.Info("database connected", slog.Int("attempt", attempt))
				}
				return &DB{Pool: pool}, nil
			}
			pool.Close()
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("database connect cancelled: %w", ctx.Err())
		}
		if log != nil {
			log.Warn("database not ready, retrying",
				slog.Int("attempt", attempt),
				slog.Duration("backoff", backoff),
				slog.String("error", err.Error()),
			)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("database connect cancelled: %w", ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
	return nil, fmt.Errorf("database unreachable after %d attempts: %w", maxAttempts, err)
}

// Health runs a trivial query to confirm the database is reachable and
// responsive. It is used by /healthz.
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var one int
	if err := db.Pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("database health query failed: %w", err)
	}
	return nil
}

// Close releases the pool.
func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
