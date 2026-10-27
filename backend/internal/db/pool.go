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

// Migrate runs all embedded SQL migrations in lexical order.
func Migrate(ctx context.Context, pool *Pool) error {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
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
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return err
		}
		log.Printf("applied migration %s", f)
	}
	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
