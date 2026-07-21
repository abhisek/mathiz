package session

import (
	"context"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/store"
)

const profileJSONv1 = `{"summary":"Solid on addition","strengths":["addition"],"weaknesses":["regrouping"],"patterns":["rushes"]}`
const profileJSONv2 = `{"summary":"Regrouping has clicked","strengths":["addition"],"weaknesses":[],"patterns":["rushes"]}`

func newPersistTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func compressorWith(responses ...llm.MockResponse) *lessons.Compressor {
	return lessons.NewCompressor(llm.NewMockProvider(responses...), lessons.DefaultCompressorConfig())
}

// TestRefreshProfileVersionsOnlyChanges drives the profile-refresh body
// synchronously: the first generation appends a version event, an identical
// regeneration appends nothing, and a changed summary appends a second
// version.
func TestRefreshProfileVersionsOnlyChanges(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	snapRepo := st.SnapshotRepo()
	eventRepo := st.EventRepo()

	// A snapshot must exist for the refresh to fold the profile into.
	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: store.SnapshotData{Version: 4}}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	comp := compressorWith(
		llm.MockResponse{Content: []byte(profileJSONv1)}, // first generation
		llm.MockResponse{Content: []byte(profileJSONv1)}, // identical regeneration
		llm.MockResponse{Content: []byte(profileJSONv2)}, // changed summary
	)

	// First generation: no previous profile, one event.
	refreshProfile(ctx, snapRepo, eventRepo, comp, lessons.ProfileInput{}, nil)
	events, err := eventRepo.QueryLearnerProfileEvents(ctx, store.QueryOpts{})
	if err != nil {
		t.Fatalf("query after first refresh: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events after first refresh = %d, want 1", len(events))
	}
	if events[0].Summary != "Solid on addition" ||
		len(events[0].Strengths) != 1 || events[0].Strengths[0] != "addition" ||
		len(events[0].Weaknesses) != 1 || events[0].Weaknesses[0] != "regrouping" ||
		len(events[0].Patterns) != 1 || events[0].Patterns[0] != "rushes" {
		t.Errorf("first version = %+v, want the generated profile", events[0])
	}
	if events[0].GeneratedAt == "" {
		t.Error("first version has empty GeneratedAt")
	}
	// The snapshot carries the same profile for fast reads.
	latest, err := snapRepo.Latest(ctx)
	if err != nil || latest == nil || latest.Data.LearnerProfile == nil {
		t.Fatalf("latest snapshot after refresh = %+v, err %v", latest, err)
	}
	prev := latest.Data.LearnerProfile
	if prev.Summary != "Solid on addition" {
		t.Errorf("snapshot profile summary = %q", prev.Summary)
	}

	// Identical regeneration (same summary + lists, fresh GeneratedAt): the
	// snapshot re-saves but no new version event is appended.
	refreshProfile(ctx, snapRepo, eventRepo, comp, lessons.ProfileInput{}, prev)
	events, err = eventRepo.QueryLearnerProfileEvents(ctx, store.QueryOpts{})
	if err != nil {
		t.Fatalf("query after identical refresh: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events after identical refresh = %d, want still 1", len(events))
	}

	// Changed summary: a second version, newest first.
	refreshProfile(ctx, snapRepo, eventRepo, comp, lessons.ProfileInput{}, prev)
	events, err = eventRepo.QueryLearnerProfileEvents(ctx, store.QueryOpts{})
	if err != nil {
		t.Fatalf("query after changed refresh: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events after changed refresh = %d, want 2", len(events))
	}
	if events[0].Summary != "Regrouping has clicked" {
		t.Errorf("newest version summary = %q", events[0].Summary)
	}
	if len(events[0].Weaknesses) != 0 {
		t.Errorf("newest version weaknesses = %v, want empty", events[0].Weaknesses)
	}

	// A nil event repo (profile versioning unwired) must not panic and must
	// still save the snapshot.
	comp = compressorWith(llm.MockResponse{Content: []byte(profileJSONv1)})
	refreshProfile(ctx, snapRepo, nil, comp, lessons.ProfileInput{}, nil)
	latest, err = snapRepo.Latest(ctx)
	if err != nil || latest == nil || latest.Data.LearnerProfile == nil || latest.Data.LearnerProfile.Summary != "Solid on addition" {
		t.Errorf("snapshot after nil-repo refresh = %+v, err %v", latest, err)
	}
}

// TestSaveSnapshotWithProfileAppendsVersionEvent covers the full wiring: the
// shared end-of-session path picks the event repo off the session state and
// the async refresh lands a version event.
func TestSaveSnapshotWithProfileAppendsVersionEvent(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	snapRepo := st.SnapshotRepo()
	eventRepo := st.EventRepo()

	state := NewSessionState(&Plan{}, "sess-1", nil, nil)
	state.EventRepo = eventRepo

	comp := compressorWith(llm.MockResponse{Content: []byte(profileJSONv1)})
	if err := SaveSnapshotWithProfile(ctx, snapRepo, comp, state, store.SnapshotData{Version: 4}); err != nil {
		t.Fatalf("save: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		events, err := eventRepo.QueryLearnerProfileEvents(ctx, store.QueryOpts{})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(events) == 1 {
			if events[0].Summary != "Solid on addition" {
				t.Errorf("version summary = %q", events[0].Summary)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no learner profile event after async refresh (have %d)", len(events))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestProfileChanged(t *testing.T) {
	base := &store.LearnerProfileData{
		Summary:    "s",
		Strengths:  []string{"a"},
		Weaknesses: []string{"w"},
		Patterns:   []string{"p"},
	}
	same := &store.LearnerProfileData{
		Summary:    "s",
		Strengths:  []string{"a"},
		Weaknesses: []string{"w"},
		Patterns:   []string{"p"},
		// A different GeneratedAt alone is not a change.
		GeneratedAt: "2026-07-21T12:00:00Z",
	}
	if profileChanged(base, same) {
		t.Error("identical content reported as changed")
	}
	if !profileChanged(nil, base) {
		t.Error("first profile not reported as changed")
	}
	cases := map[string]*store.LearnerProfileData{
		"summary":    {Summary: "x", Strengths: []string{"a"}, Weaknesses: []string{"w"}, Patterns: []string{"p"}},
		"strengths":  {Summary: "s", Strengths: []string{"a", "b"}, Weaknesses: []string{"w"}, Patterns: []string{"p"}},
		"weaknesses": {Summary: "s", Strengths: []string{"a"}, Weaknesses: nil, Patterns: []string{"p"}},
		"patterns":   {Summary: "s", Strengths: []string{"a"}, Weaknesses: []string{"w"}, Patterns: []string{"q"}},
	}
	for name, next := range cases {
		if !profileChanged(base, next) {
			t.Errorf("%s change not detected", name)
		}
	}
}
