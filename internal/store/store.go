package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/abhisek/mathiz/ent"

	// Pure Go SQLite driver (no CGO).
	_ "modernc.org/sqlite"

	// Pure Go PostgreSQL driver (no CGO).
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store holds the ent client and provides access to repositories.
type Store struct {
	db      *sql.DB
	client  *ent.Client
	seq     *sequenceCounter
	dialect string
}

// Open creates a new Store from a DSN. DSNs starting with postgres:// or
// postgresql:// open a PostgreSQL connection (SaaS mode); anything else is
// treated as a SQLite file path (local mode). Auto-migration runs either way.
func Open(dsn string) (*Store, error) {
	if IsPostgresDSN(dsn) {
		return open(dsn, dialect.Postgres, "pgx")
	}
	return open(dsn, dialect.SQLite, "sqlite")
}

// IsPostgresDSN reports whether the DSN targets PostgreSQL.
func IsPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

func open(dsn, entDialect, driver string) (*Store, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if entDialect == dialect.SQLite {
		// Pragmas are per-connection; a pool would leave later connections
		// without busy_timeout and surface spurious SQLITE_BUSY under
		// concurrent writers. One connection serializes access instead.
		db.SetMaxOpenConns(1)
		if err := applyPragmas(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply pragmas: %w", err)
		}
	}

	drv := entsql.OpenDB(entDialect, db)
	client := ent.NewClient(ent.Driver(drv))

	if err := client.Schema.Create(context.Background()); err != nil {
		client.Close()
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}

	seq, err := newSequenceCounter(db)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("init sequence counter: %w", err)
	}

	return &Store{db: db, client: client, seq: seq, dialect: entDialect}, nil
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

// SnapshotRepo returns a SnapshotRepo for the local single-user owner.
func (s *Store) SnapshotRepo() SnapshotRepo {
	return s.SnapshotRepoFor(LocalOwner)
}

// EventRepo returns an EventRepo for the local single-user owner.
func (s *Store) EventRepo() EventRepo {
	return s.EventRepoFor(LocalOwner)
}

// SnapshotRepoFor returns a SnapshotRepo scoped to the given owner. Every
// write stamps the owner and every read filters by it, isolating learners
// that share one database.
func (s *Store) SnapshotRepoFor(owner string) SnapshotRepo {
	return &snapshotRepo{client: s.client, owner: owner}
}

// EventRepoFor returns an EventRepo scoped to the given owner.
func (s *Store) EventRepoFor(owner string) EventRepo {
	return &eventRepo{client: s.client, seq: s.seq, db: s.db, dialect: s.dialect, owner: owner}
}

// LocalOwner is the owner ID used by the local single-user CLI. It predates
// multi-tenancy: rows written before the owner column existed default to "".
const LocalOwner = ""

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
		return p, EnsureDir(p)
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
	return p, EnsureDir(p)
}

// EnsureDir creates the parent directory of path if it doesn't exist.
func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o755)
}
