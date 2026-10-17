package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenClose(t *testing.T) {
	s := openTestStore(t)
	if s.Client() == nil {
		t.Fatal("expected non-nil ent client")
	}
}

func TestPragmasApplied(t *testing.T) {
	s := openTestStore(t)
	db := s.DB()

	tests := []struct {
		pragma string
		want   string
	}{
		// WAL mode falls back to "memory" for in-memory databases,
		// so we skip journal_mode here. It is tested with file-based DBs.
		{"foreign_keys", "1"},
		{"synchronous", "1"}, // NORMAL = 1
	}

	for _, tt := range tests {
		var got string
		err := db.QueryRow("PRAGMA " + tt.pragma).Scan(&got)
		if err != nil {
			t.Errorf("PRAGMA %s: %v", tt.pragma, err)
			continue
		}
		if got != tt.want {
			t.Errorf("PRAGMA %s = %q, want %q", tt.pragma, got, tt.want)
		}
	}
}

func TestSnapshotSaveAndLatest(t *testing.T) {
	s := openTestStore(t)
	repo := s.SnapshotRepo()
	ctx := context.Background()

	// No snapshot yet.
	snap, err := repo.Latest(ctx)
	if err != nil {
		t.Fatalf("latest (empty): %v", err)
	}
	if snap != nil {
		t.Fatal("expected nil snapshot when none exist")
	}

	// Save a snapshot.
	now := time.Now().UTC().Truncate(time.Second)
	err = repo.Save(ctx, &Snapshot{
		Sequence:  42,
		Timestamp: now,
		Data:      SnapshotData{Version: 1},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	// Retrieve it.
	snap, err = repo.Latest(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.Sequence != 42 {
		t.Errorf("sequence = %d, want 42", snap.Sequence)
	}
	if snap.Data.Version != 1 {
		t.Errorf("data.version = %d, want 1", snap.Data.Version)
	}
}

func TestSnapshotLatestReturnsNewest(t *testing.T) {
	s := openTestStore(t)
	repo := s.SnapshotRepo()
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		err := repo.Save(ctx, &Snapshot{
			Sequence:  int64(i + 1),
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Data:      SnapshotData{Version: i + 1},
		})
		if err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	snap, err := repo.Latest(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if snap.Sequence != 3 {
		t.Errorf("sequence = %d, want 3", snap.Sequence)
	}
	if snap.Data.Version != 3 {
		t.Errorf("data.version = %d, want 3", snap.Data.Version)
	}
}

func TestSnapshotPrune(t *testing.T) {
	s := openTestStore(t)
	repo := s.SnapshotRepo()
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 7; i++ {
		err := repo.Save(ctx, &Snapshot{
			Sequence:  int64(i + 1),
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Data:      SnapshotData{Version: 1},
		})
		if err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	// Prune to keep 5.
	if err := repo.Prune(ctx, 5); err != nil {
		t.Fatalf("prune: %v", err)
	}

	// Count remaining snapshots. Direct client access needs the owner in ctx
	// or the owner guard fails the query closed.
	count, err := s.Client().Snapshot.Query().Count(withOwner(ctx, LocalOwner))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 5 {
		t.Errorf("remaining snapshots = %d, want 5", count)
	}

	// Latest should still be sequence 7.
	snap, err := repo.Latest(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if snap.Sequence != 7 {
		t.Errorf("latest sequence = %d, want 7", snap.Sequence)
	}
}

func TestSnapshotPruneWithFewerThanKeep(t *testing.T) {
	s := openTestStore(t)
	repo := s.SnapshotRepo()
	ctx := context.Background()

	// Save only 2 snapshots.
	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 2; i++ {
		err := repo.Save(ctx, &Snapshot{
			Sequence:  int64(i + 1),
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Data:      SnapshotData{Version: 1},
		})
		if err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	// Prune with keep=5 should be a no-op.
	if err := repo.Prune(ctx, 5); err != nil {
		t.Fatalf("prune: %v", err)
	}

	count, err := s.Client().Snapshot.Query().Count(withOwner(ctx, LocalOwner))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("remaining snapshots = %d, want 2", count)
	}
}

func TestSequenceCounter(t *testing.T) {
	s := openTestStore(t)
	db := s.DB()
	ctx := context.Background()

	sc, err := newSequenceCounter(db)
	if err != nil {
		t.Fatalf("new sequence counter: %v", err)
	}

	var seqs []int64
	for i := 0; i < 5; i++ {
		seq, err := sc.Next(ctx)
		if err != nil {
			t.Fatalf("next %d: %v", i, err)
		}
		seqs = append(seqs, seq)
	}

	// Should be monotonically increasing starting from 1.
	for i, seq := range seqs {
		expected := int64(i + 1)
		if seq != expected {
			t.Errorf("seq[%d] = %d, want %d", i, seq, expected)
		}
	}
}

func TestAutoMigrationCreatesTable(t *testing.T) {
	s := openTestStore(t)
	db := s.DB()

	// Check that the snapshots table exists.
	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='snapshots'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if name != "snapshots" {
		t.Errorf("table name = %q, want 'snapshots'", name)
	}
}

func TestSQLiteSingleConnection(t *testing.T) {
	s := openTestStore(t)
	if got := s.DB().Stats().MaxOpenConnections; got != 1 {
		t.Errorf("SQLite MaxOpenConnections = %d, want 1", got)
	}
}

func TestEnvPositiveInt(t *testing.T) {
	const key = "MATHIZ_TEST_POOL_VALUE"
	tests := []struct {
		name    string
		value   string
		want    int
		wantErr bool
	}{
		{"unset uses default", "", 7, false},
		{"valid", "12", 12, false},
		{"zero", "0", 0, true},
		{"negative", "-3", 0, true},
		{"not a number", "lots", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(key, tt.value)
			got, err := envPositiveInt(key, 7)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDSNSSLMode(t *testing.T) {
	tests := map[string]string{
		"postgres://u:p@db:5432/mathiz?sslmode=verify-full": "verify-full",
		"postgres://u:p@db:5432/mathiz?sslmode=disable":     "disable",
		"postgres://u:p@db:5432/mathiz":                     "prefer",
	}
	for dsn, want := range tests {
		if got := dsnSSLMode(dsn); got != want {
			t.Errorf("dsnSSLMode(%q) = %q, want %q", dsn, got, want)
		}
	}
}

// TestPostgresPool runs against a real server only when one is configured.
func TestPostgresPool(t *testing.T) {
	dsn := os.Getenv("MATHIZ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MATHIZ_TEST_DATABASE_URL not set")
	}
	t.Setenv("MATHIZ_DB_MAX_OPEN_CONNS", "8")
	t.Setenv("MATHIZ_DB_MAX_IDLE_CONNS", "50")
	s, err := Open(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if got := s.DB().Stats().MaxOpenConnections; got != 8 {
		t.Errorf("MaxOpenConnections = %d, want 8", got)
	}

	t.Setenv("MATHIZ_DB_MAX_OPEN_CONNS", "nope")
	if _, err := Open(dsn); err == nil {
		t.Error("expected an error for a malformed MATHIZ_DB_MAX_OPEN_CONNS")
	}
}
