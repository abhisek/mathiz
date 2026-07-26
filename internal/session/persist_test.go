package session

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/skillgraph"
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
	refreshProfile(ctx, store.LocalOwner, snapRepo, eventRepo, comp, lessons.ProfileInput{}, nil)
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
	refreshProfile(ctx, store.LocalOwner, snapRepo, eventRepo, comp, lessons.ProfileInput{}, prev)
	events, err = eventRepo.QueryLearnerProfileEvents(ctx, store.QueryOpts{})
	if err != nil {
		t.Fatalf("query after identical refresh: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events after identical refresh = %d, want still 1", len(events))
	}

	// Changed summary: a second version, newest first.
	refreshProfile(ctx, store.LocalOwner, snapRepo, eventRepo, comp, lessons.ProfileInput{}, prev)
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
	refreshProfile(ctx, store.LocalOwner, snapRepo, nil, comp, lessons.ProfileInput{}, nil)
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
	// The refresh needs evidence to summarise: a session that answered
	// nothing is skipped (see TestNoProfileRefreshWithoutQuestions).
	state.TotalQuestions = 3

	comp := compressorWith(llm.MockResponse{Content: []byte(profileJSONv1)})
	if err := SaveSnapshotWithProfile(ctx, store.LocalOwner, snapRepo, comp, state, store.SnapshotData{Version: 4}); err != nil {
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

// TestRefreshProfileLogsFailures pins the production-visibility contract added
// for the "placeholder profile" report: a profile refresh that fails must
// leave a structured, owner-attributable error behind. It used to return
// silently, which is why the original report could not be confirmed from
// production data.
func TestRefreshProfileLogsFailures(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-abc"
	snapRepo := st.SnapshotRepoFor(owner)
	eventRepo := st.EventRepoFor(owner)

	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: store.SnapshotData{Version: 4}}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// An exhausted mock provider fails the generation call.
	refreshProfile(ctx, owner, snapRepo, eventRepo, compressorWith(), lessons.ProfileInput{}, nil)

	out := logs.String()
	if !strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("failed profile refresh logged no ERROR record; got %q", out)
	}
	if !strings.Contains(out, `"owner_id":"`+owner+`"`) {
		t.Errorf("profile failure log is not owner-attributable; got %q", out)
	}

	// The previous profile must survive a failed refresh — no version event.
	events, err := eventRepo.QueryLearnerProfileEvents(ctx, store.QueryOpts{})
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("failed refresh appended %d profile version(s), want 0", len(events))
	}
}

// TestProfileInputFidelity pins what the profiler is actually told. Each
// assertion here corresponds to something the prompt previously asserted
// falsely or unintelligibly.
func TestProfileInputFidelity(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-fidelity"
	snapRepo := st.SnapshotRepoFor(owner)

	// Two real skills from the graph: one practised, one only planned.
	all := skillgraph.AllSkills()
	if len(all) < 2 {
		t.Fatal("need at least two skills in the graph")
	}
	practised, untouched := all[0], all[1]

	state := &SessionState{
		TotalQuestions: 3,
		PerSkillResults: map[string]*SkillResult{
			practised.ID: {SkillID: practised.ID, SkillName: practised.Name, Attempted: 3, Correct: 2},
			untouched.ID: {SkillID: untouched.ID, SkillName: untouched.Name},
			// A synthetic untagged-quest skill must never reach the profiler.
			"quest:abc-123": {SkillID: "quest:abc-123", SkillName: "Captain's quest", Attempted: 4, Correct: 1},
		},
		RecentErrors: map[string][]string{
			practised.ID:    {"Answered 5 for '2+2', correct answer was 4"},
			"quest:abc-123": {"Answered 7 for 'HCF of 12 and 18', correct answer was 6"},
		},
		EventRepo: st.EventRepoFor(owner),
	}

	snapData := store.SnapshotData{Version: 4, Mastery: &store.MasterySnapshotData{
		Skills: map[string]*store.SkillMasteryData{
			practised.ID: {
				SkillID: practised.ID, State: "learning", TotalAttempts: 10, CorrectCount: 8,
				SpeedScores: []float64{0.9, 0.8}, SpeedWindow: 10, Streak: 4, StreakCap: 8,
			},
			"quest:abc-123": {SkillID: "quest:abc-123", State: "learning", TotalAttempts: 4, CorrectCount: 1},
		},
	}}

	// A compressor with no canned response fails generation; we only care
	// about the input that was built, which the mock records.
	provider := llm.NewMockProvider()
	comp := lessons.NewCompressor(provider, lessons.DefaultCompressorConfig())
	if err := SaveSnapshotWithProfile(ctx, owner, snapRepo, comp, state, snapData); err != nil {
		t.Fatalf("save: %v", err)
	}

	// The refresh is async; wait for the provider to be called.
	var prompt string
	for i := 0; i < 100; i++ {
		if calls := provider.Requests(); len(calls) > 0 {
			prompt = calls[0].Messages[0].Content
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if prompt == "" {
		t.Fatal("profile generation was never attempted")
	}

	if strings.Contains(prompt, "fluency=0.00") {
		t.Error("prompt still claims zero fluency; it should carry the recomputed score")
	}
	if !strings.Contains(prompt, practised.Name) {
		t.Errorf("prompt does not name the practised skill %q", practised.Name)
	}
	if strings.Contains(prompt, untouched.ID) {
		t.Errorf("prompt reports skill %q the child never attempted", untouched.ID)
	}
	if strings.Contains(prompt, "quest:abc-123") || strings.Contains(prompt, "Captain's quest") {
		t.Error("synthetic untagged-quest skill leaked into the profile prompt")
	}
}

// TestNoProfileRefreshWithoutQuestions: a session that asked nothing has no
// evidence to summarise, and asking anyway is how filler profiles get written.
func TestNoProfileRefreshWithoutQuestions(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-empty"
	snapRepo := st.SnapshotRepoFor(owner)

	provider := llm.NewMockProvider(llm.MockResponse{Content: []byte(profileJSONv1)})
	comp := lessons.NewCompressor(provider, lessons.DefaultCompressorConfig())
	state := &SessionState{
		TotalQuestions:  0,
		PerSkillResults: map[string]*SkillResult{},
		RecentErrors:    map[string][]string{},
		EventRepo:       st.EventRepoFor(owner),
	}
	if err := SaveSnapshotWithProfile(ctx, owner, snapRepo, comp, state, store.SnapshotData{Version: 4}); err != nil {
		t.Fatalf("save: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if n := len(provider.Requests()); n != 0 {
		t.Errorf("question-less session made %d profile call(s), want 0", n)
	}
}
