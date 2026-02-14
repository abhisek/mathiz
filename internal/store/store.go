package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/abhisek/mathiz/ent"

	// Pure Go SQLite driver (no CGO).
	_ "modernc.org/sqlite"
)

// Store holds the ent client and provides access to repositories.
type Store struct {
	db     *sql.DB
	client *ent.Client
}

// Open creates a new Store connected to the SQLite database at dsn.
// It applies recommended pragmas and runs auto-migration.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := applyPragmas(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply pragmas: %w", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))

	if err := client.Schema.Create(context.Background()); err != nil {
		client.Close()
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}

	return &Store{db: db, client: client}, nil
}

// Client returns the underlying ent client.
func (s *Store) Client() *ent.Client {
	return s.client
}

// DB returns the underlying *sql.DB for raw queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.client.Close()
}

// SnapshotRepo returns a SnapshotRepo backed by this store.
func (s *Store) SnapshotRepo() SnapshotRepo {
	return &snapshotRepo{client: s.client}
}

// applyPragmas configures SQLite for optimal single-user performance.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// DefaultDBPath resolves the database file path in priority order:
// 1. MATHIZ_DB environment variable
// 2. $XDG_DATA_HOME/mathiz/mathiz.db
// 3. ~/.local/share/mathiz/mathiz.db
func DefaultDBPath() (string, error) {
	if p := os.Getenv("MATHIZ_DB"); p != "" {
		return p, ensureDir(p)
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	p := filepath.Join(dataHome, "mathiz", "mathiz.db")
	return p, ensureDir(p)
}

// ensureDir creates the parent directory of path if it doesn't exist.
func ensureDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o755)
}
