// Package sqlite is the driven adapter for keyword storage.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB wraps the pooled connection. A local tool needs one writer, so the pool is
// intentionally capped at a single connection.
type DB struct {
	*sql.DB
}

// Open opens (and creates) the database file. Use ":memory:" for a throwaway run.
func Open(databasePath string) (*DB, error) {
	conn, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", databasePath, err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sqlite: enable foreign keys: %w", err)
	}
	db := &DB{DB: conn}
	if err := db.Migrate(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return db, nil
}

// Migrate applies embedded migrations in lexical order, once each.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("sqlite: create migration table: %w", err)
	}

	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		version := path.Base(file)
		var applied int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version).Scan(&applied); err != nil {
			return fmt.Errorf("sqlite: check migration %s: %w", version, err)
		}
		if applied > 0 {
			continue
		}
		body, err := migrationFS.ReadFile(file)
		if err != nil {
			return err
		}
		for _, statement := range strings.Split(string(body), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := d.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("sqlite: apply %s: %w", version, err)
			}
		}
		if _, err := d.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("sqlite: record migration %s: %w", version, err)
		}
	}
	return nil
}
