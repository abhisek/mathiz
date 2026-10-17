package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
func Open(dsn string, opts ...Option) (*Store, error) {
	if IsPostgresDSN(dsn) {
		return open(dsn, dialect.Postgres, "pgx", opts)
	}
	return open(dsn, dialect.SQLite, "sqlite", opts)
}

// Option configures Open.
type Option func(*openOptions)

type openOptions struct {
	beforeMigrate []func(context.Context, *Store) error
}

// BeforeMigrate runs check on the raw database before auto-migrate. A
// check that must see the schema as an older release left it (a column
// this release removed) has to run here: auto-migrate may fail on that
// schema first, with an error that doesn't name the fix. Only DB(),
// ColumnExists and the dialect are usable at that point.
func BeforeMigrate(check func(context.Context, *Store) error) Option {
	return func(o *openOptions) { o.beforeMigrate = append(o.beforeMigrate, check) }
}

// IsPostgresDSN reports whether the DSN targets PostgreSQL.
func IsPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

func open(dsn, entDialect, driver string, opts []Option) (*Store, error) {
	var o openOptions
	for _, opt := range opts {
		opt(&o)
	}

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
	} else {
		if err := configurePostgres(db, dsn); err != nil {
			db.Close()
			return nil, err
		}
	}

	for _, check := range o.beforeMigrate {
		if err := check(context.Background(), &Store{db: db, dialect: entDialect}); err != nil {
			db.Close()
			return nil, err
		}
	}

	drv := entsql.OpenDB(entDialect, db)
	client := ent.NewClient(ent.Driver(drv))
	registerOwnerGuard(client)

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

// ColumnExists reports whether table has column. Auto-migrate adds columns
// but never drops them, so callers use this to detect a column a release
// removed from the schema.
func (s *Store) ColumnExists(ctx context.Context, table, column string) (bool, error) {
	var q string
	if s.dialect == dialect.Postgres {
		// to_regclass resolves the table through search_path, as queries
		// do, and is NULL (count 0) when the table doesn't exist.
		q = `SELECT COUNT(*) FROM pg_attribute
			WHERE attrelid = to_regclass($1) AND attname = $2 AND NOT attisdropped`
	} else {
		q = `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`
	}
	var n int
	if err := s.db.QueryRowContext(ctx, q, table, column).Scan(&n); err != nil {
		return false, fmt.Errorf("inspect %s.%s: %w", table, column, err)
	}
	return n > 0, nil
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

// MinPostgresMajor is the oldest PostgreSQL major the store supports: the
// oldest one CI and the dev stack have run the owner-isolation suite on.
const MinPostgresMajor = 16

// Pool defaults for PostgreSQL. Without a cap, a traffic burst opens one
// connection per concurrent query and exhausts the server's
// max_connections (100 by default), which fails every other client too.
const (
	defaultMaxOpenConns    = 20
	defaultMaxIdleConns    = 10
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
)

// configurePostgres applies the pool limits and refuses servers older than
// MinPostgresMajor, so an unsupported server fails at boot instead of on
// the first query that needs a newer feature.
func configurePostgres(db *sql.DB, dsn string) error {
	maxOpen, err := envPositiveInt("MATHIZ_DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
	if err != nil {
		return err
	}
	maxIdle, err := envPositiveInt("MATHIZ_DB_MAX_IDLE_CONNS", defaultMaxIdleConns)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(min(maxIdle, maxOpen))
	db.SetConnMaxLifetime(defaultConnMaxLifetime)
	db.SetConnMaxIdleTime(defaultConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var versionNum int
	if err := db.QueryRowContext(ctx, "SELECT current_setting('server_version_num')::int").Scan(&versionNum); err != nil {
		return fmt.Errorf("read postgres version: %w", err)
	}
	major := versionNum / 10000
	if major < MinPostgresMajor {
		return fmt.Errorf("postgres %d is not supported: mathiz needs PostgreSQL %d or newer", major, MinPostgresMajor)
	}
	slog.Info("store: postgres", "server_version_num", versionNum, "sslmode", dsnSSLMode(dsn),
		"max_open_conns", maxOpen, "max_idle_conns", min(maxIdle, maxOpen))
	return nil
}

// dsnSSLMode reports the effective sslmode: the DSN's, else PGSSLMODE
// (pgx honours it), else libpq's default "prefer", which silently falls
// back to plaintext.
func dsnSSLMode(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "unknown"
	}
	if m := u.Query().Get("sslmode"); m != "" {
		return m
	}
	if m := os.Getenv("PGSSLMODE"); m != "" {
		return m
	}
	return "prefer"
}

// envPositiveInt reads an optional positive integer. A malformed value is
// an error rather than a silent default, so a typo can't hide a pool limit.
func envPositiveInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, v)
	}
	return n, nil
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
		if IsPostgresDSN(p) {
			return p, nil // a DSN is not a filesystem path
		}
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
