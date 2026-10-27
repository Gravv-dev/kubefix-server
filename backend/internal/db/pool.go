package db

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Pool wraps *pgxpool.Pool.
type Pool struct {
	*pgxpool.Pool
}

// NewPool creates a connection pool from the DATABASE_URL env var.
func NewPool(ctx context.Context) (*Pool, error) {
	dbURL := getEnv("DATABASE_URL", "postgres://kubefix:kubefix@localhost:5432/kubefix?sslmode=disable")
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Pool{pool}, nil
}

// Migrate applies each embedded SQL migration once, in lexical order.
func Migrate(ctx context.Context, pool *Pool) error {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		content, err := fs.ReadFile(migrations, "migrations/"+f)
		if err != nil {
			return err
		}
		applied, err := applyMigration(ctx, pool, f, string(content))
		if err != nil {
			return err
		}
		if applied {
			log.Printf("applied migration %s", f)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, pool *Pool, name, content string) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1145783650)`); err != nil {
		return false, err
	}

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, tx.Commit(ctx)
	}

	// Databases initialized before schema_migrations was introduced already have
	// the initial schema, so record that baseline instead of recreating tables.
	if name == "001_initial_schema.sql" {
		if err := tx.QueryRow(ctx, `SELECT to_regclass('public.users') IS NOT NULL`).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
				return false, err
			}
			return false, tx.Commit(ctx)
		}
	}

	if _, err := tx.Exec(ctx, content); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
