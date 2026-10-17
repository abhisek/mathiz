package family

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func seedAccounts(t *testing.T, svc *Service, provider string, subjects ...string) map[string]string {
	t.Helper()
	uids := make(map[string]string)
	for _, sub := range subjects {
		a, err := svc.EnsureAccount(context.Background(), Identity{Provider: provider, Subject: sub, Email: sub + "@example.com"})
		if err != nil {
			t.Fatalf("seed %s/%s: %v", provider, sub, err)
		}
		uids[sub] = a.UID
	}
	return uids
}

func TestRemapProvider(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	uids := seedAccounts(t, svc, "supabase", "old-1", "old-2")
	pairs := []RemapPair{{"old-1", "new-1"}, {"old-2", "new-2"}}

	dry, err := svc.RemapProvider(ctx, "supabase", "nextauth", pairs, true)
	if err != nil || dry.Remapped != 2 {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	if foreign, _ := svc.CountForeignAccounts(ctx, "nextauth"); foreign["supabase"] != 2 {
		t.Fatalf("dry run wrote: %v", foreign)
	}

	res, err := svc.RemapProvider(ctx, "supabase", "nextauth", pairs, false)
	if err != nil || res.Remapped != 2 || res.AlreadyDone != 0 {
		t.Fatalf("remap = %+v, %v", res, err)
	}
	// The account (and so its family) is the same row under its new identity.
	moved, err := svc.EnsureAccount(ctx, Identity{Provider: "nextauth", Subject: "new-1"})
	if err != nil || moved.UID != uids["old-1"] {
		t.Fatalf("new identity resolved to %v, %v; want %s", moved, err, uids["old-1"])
	}

	again, err := svc.RemapProvider(ctx, "supabase", "nextauth", pairs, false)
	if err != nil || again.Remapped != 0 || again.AlreadyDone != 2 {
		t.Fatalf("rerun = %+v, %v; want a no-op", again, err)
	}
}

func TestRemapProviderRefusals(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		seed    func(svc *Service)
		pairs   []RemapPair
		from    string
		to      string
		wantErr string
	}{
		{
			name:    "unmapped account",
			seed:    func(svc *Service) { seedAccounts(t, svc, "supabase", "a", "b") },
			pairs:   []RemapPair{{"a", "x"}},
			wantErr: "1 supabase accounts are missing",
		},
		{
			name:    "unknown old subject",
			seed:    func(svc *Service) { seedAccounts(t, svc, "supabase", "a") },
			pairs:   []RemapPair{{"a", "x"}, {"ghost", "y"}},
			wantErr: `row 2: no supabase account has subject "ghost"`,
		},
		{
			name: "new subject taken",
			seed: func(svc *Service) {
				seedAccounts(t, svc, "supabase", "a")
				seedAccounts(t, svc, "nextauth", "x")
			},
			pairs:   []RemapPair{{"a", "x"}},
			wantErr: `row 1: nextauth subject "x" already belongs to another account`,
		},
		{name: "duplicate old", pairs: []RemapPair{{"a", "x"}, {"a", "y"}}, wantErr: "same old subject"},
		{name: "duplicate new", pairs: []RemapPair{{"a", "x"}, {"b", "x"}}, wantErr: "same new subject"},
		{name: "empty subject", pairs: []RemapPair{{"a", ""}}, wantErr: "empty subject"},
		{name: "same provider", from: "nextauth", to: "nextauth", wantErr: "two different providers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			if tc.seed != nil {
				tc.seed(svc)
			}
			from, to := tc.from, tc.to
			if from == "" {
				from, to = "supabase", "nextauth"
			}
			before, _ := svc.CountForeignAccounts(ctx, "nobody")
			_, err := svc.RemapProvider(ctx, from, to, tc.pairs, false)
			if !errors.Is(err, ErrRemap) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want ErrRemap containing %q", err, tc.wantErr)
			}
			after, _ := svc.CountForeignAccounts(ctx, "nobody")
			if len(before) != len(after) || before["supabase"] != after["supabase"] || before["nextauth"] != after["nextauth"] {
				t.Errorf("a refused remap wrote: before %v, after %v", before, after)
			}
		})
	}
}
