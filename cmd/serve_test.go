package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/store"
)

func TestCheckAccounts(t *testing.T) {
	ctx := context.Background()
	open := func(t *testing.T) (*store.Store, *family.Service) {
		t.Helper()
		st, err := store.Open("file::memory:?cache=shared")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { st.Close() })
		return st, family.New(st.Client())
	}

	t.Run("fresh database", func(t *testing.T) {
		st, svc := open(t)
		if err := checkAccounts(ctx, st, svc, "supabase"); err != nil {
			t.Fatalf("fresh: %v", err)
		}
	})

	t.Run("accounts of the configured provider", func(t *testing.T) {
		st, svc := open(t)
		if _, err := svc.EnsureAccount(ctx, family.Identity{Provider: "supabase", Subject: "a", Email: "a@example.com"}); err != nil {
			t.Fatal(err)
		}
		if err := checkAccounts(ctx, st, svc, "supabase"); err != nil {
			t.Fatalf("same provider: %v", err)
		}
	})

	t.Run("provider pin", func(t *testing.T) {
		st, svc := open(t)
		for _, sub := range []string{"a", "b"} {
			if _, err := svc.EnsureAccount(ctx, family.Identity{Provider: "supabase", Subject: sub, Email: sub + "@example.com"}); err != nil {
				t.Fatal(err)
			}
		}
		err := checkAccounts(ctx, st, svc, "test")
		if err == nil || !strings.Contains(err.Error(), `2 accounts belong to provider "supabase"`) || !strings.Contains(err.Error(), "mathiz auth remap --from supabase --to test") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("legacy column", func(t *testing.T) {
		st, svc := open(t)
		if _, err := st.DB().Exec("ALTER TABLE accounts ADD COLUMN supabase_user_id text"); err != nil {
			t.Fatal(err)
		}
		err := checkAccounts(ctx, st, svc, "supabase")
		if err == nil || !strings.Contains(err.Error(), "DROP COLUMN supabase_user_id") {
			t.Fatalf("err = %v", err)
		}
	})
}
