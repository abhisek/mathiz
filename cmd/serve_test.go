package cmd

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/store"
)

func TestCheckProviderPin(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := family.New(st.Client())

	if err := checkProviderPin(ctx, svc, "supabase"); err != nil {
		t.Fatalf("fresh: %v", err)
	}
	for _, sub := range []string{"a", "b"} {
		if _, err := svc.EnsureAccount(ctx, family.Identity{Provider: "supabase", Subject: sub, Email: sub + "@example.com"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkProviderPin(ctx, svc, "supabase"); err != nil {
		t.Fatalf("same provider: %v", err)
	}
	err = checkProviderPin(ctx, svc, "test")
	if err == nil || !strings.Contains(err.Error(), `2 accounts belong to provider "supabase"`) ||
		!strings.Contains(err.Error(), "mathiz auth remap --from supabase --to test --map") {
		t.Fatalf("err = %v", err)
	}
}

// TestLegacyAccountsRefusedBeforeMigrate opens a database shaped like the
// pre-pluggable-auth release, with a row, and expects the named fix rather
// than the auto-migrate error that adding NOT NULL columns would raise.
func TestLegacyAccountsRefusedBeforeMigrate(t *testing.T) {
	dsn := "file:legacy-accounts?mode=memory&cache=shared"
	keep, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { keep.Close() })
	for _, stmt := range []string{
		"CREATE TABLE `accounts` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `uid` text NOT NULL, `supabase_user_id` text NOT NULL, `email` text NOT NULL DEFAULT (''), `display_name` text NOT NULL DEFAULT (''), `created_at` datetime NOT NULL)",
		"INSERT INTO accounts (uid, supabase_user_id, created_at) VALUES ('u1', 'sb-1', '2026-01-01 00:00:00')",
	} {
		if _, err := keep.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	_, err = store.Open(dsn, store.BeforeMigrate(checkLegacyAccounts))
	if err == nil || !strings.Contains(err.Error(), "predates pluggable auth") {
		t.Fatalf("err = %v, want the legacy-accounts refusal", err)
	}
	var cols int
	if err := keep.QueryRow("SELECT COUNT(*) FROM pragma_table_info('accounts')").Scan(&cols); err != nil || cols != 6 {
		t.Errorf("the refused open changed the table: %d columns, %v", cols, err)
	}
}

func TestReadRemapCSV(t *testing.T) {
	pairs, err := readRemapCSV(strings.NewReader("old_subject,new_subject\n a , b \nc,d\n"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(pairs) != 2 || pairs[0] != (family.RemapPair{OldSubject: "a", NewSubject: "b"}) || pairs[1].NewSubject != "d" {
		t.Errorf("pairs = %+v", pairs)
	}
	for name, in := range map[string]string{
		"empty":       "",
		"header only": "old_subject,new_subject\n",
		"three cols":  "a,b,c\n",
	} {
		if _, err := readRemapCSV(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
