package session

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
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

// seedAnswers records answers from earlier sessions: the refresh needs
// profileMinAttempts lifetime answers before it writes a profile.
func seedAnswers(t *testing.T, repo store.EventRepo, skillID string, attempts, correct int) {
	t.Helper()
	for i := range attempts {
		if err := repo.AppendAnswerEvent(context.Background(), store.AnswerEventData{
			SessionID: "earlier", SkillID: skillID, Tier: "learn", Category: "core",
			QuestionText: "What is 2+2?", CorrectAnswer: "4", LearnerAnswer: "4",
			Correct: i < correct, TimeMs: 1000, AnswerFormat: "integer",
		}); err != nil {
			t.Fatalf("seed answer: %v", err)
		}
	}
}

// signalProvider is a canned-response llm.Provider that announces every call.
// The profile refresh runs in a goroutine the caller gets no handle on, so
// tests wait on this signal instead of polling a mock: it returns the instant
// the goroutine lands, which means the timeout can be generous without
// slowing a healthy run or flaking on a loaded one.
type signalProvider struct {
	mu       sync.Mutex
	requests []llm.Request
	called   chan struct{}
	content  []byte
}

func newSignalProvider(content []byte) *signalProvider {
	return &signalProvider{called: make(chan struct{}, 4), content: content}
}

func (p *signalProvider) Generate(_ context.Context, req llm.Request) (*llm.Response, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	select {
	case p.called <- struct{}{}:
	default: // never block the goroutine under test
	}
	if p.content == nil {
		return nil, &llm.ErrProviderUnavailable{}
	}
	return &llm.Response{Content: p.content, Model: "signal"}, nil
}

func (p *signalProvider) ModelID() string { return "signal" }

// firstPrompt waits for the refresh goroutine's call and returns its prompt.
func (p *signalProvider) firstPrompt(t *testing.T) string {
	t.Helper()
	select {
	case <-p.called:
	case <-time.After(10 * time.Second):
		t.Fatal("profile generation was never attempted")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[0].Messages[0].Content
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
	seedAnswers(t, eventRepo, skillgraph.AllSkills()[0].ID, profileMinAttempts, 10)

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

	seedAnswers(t, state.EventRepo, practised.ID, 40, 30)
	seedAnswers(t, state.EventRepo, "quest:abc-123", 20, 5)

	snapData := store.SnapshotData{Version: 4, Mastery: &store.MasterySnapshotData{
		Skills: map[string]*store.SkillMasteryData{
			practised.ID: {
				SkillID: practised.ID, State: "learning", TotalAttempts: 10, CorrectCount: 8,
				SpeedScores: []float64{0.9, 0.8}, SpeedWindow: 10, Streak: 4, StreakCap: 8,
			},
			"quest:abc-123": {SkillID: "quest:abc-123", State: "learning", TotalAttempts: 4, CorrectCount: 1},
		},
	}}

	// Generation itself fails (no canned content); we only care about the
	// input that was built, which the provider records.
	provider := newSignalProvider(nil)
	comp := lessons.NewCompressor(provider, lessons.DefaultCompressorConfig())
	if err := SaveSnapshotWithProfile(ctx, owner, snapRepo, comp, state, snapData); err != nil {
		t.Fatalf("save: %v", err)
	}

	prompt := provider.firstPrompt(t)

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
	// Accuracy is lifetime, from the answer history, not the snapshot's
	// current-tier counters (8/10).
	if !strings.Contains(prompt, "30/40 correct") {
		t.Error("prompt does not carry the lifetime answer totals")
	}
}

// TestNoProfileRefreshWithoutQuestions: a session that asked nothing has no
// evidence to summarise, and asking anyway is how filler profiles get written.
func TestNoProfileRefreshWithoutQuestions(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-empty"
	snapRepo := st.SnapshotRepoFor(owner)

	provider := newSignalProvider([]byte(profileJSONv1))
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

	// Proving a call never happens needs a bounded wait, but this way the
	// bound only decides how long we look — a regression trips the first
	// case immediately rather than slipping past a fixed sleep.
	select {
	case <-provider.called:
		t.Error("question-less session generated a profile")
	case <-time.After(250 * time.Millisecond):
	}
}

// TestProfileRefreshDoesNotRevertProgress is the regression test for the
// lost-update hazard: the refresh goroutine outlives the play slot, so if it
// wrote a whole modified copy of the snapshot it read, a session that finished
// in between would have its mastery, spaced-rep and gems silently rolled back.
// The refresh must touch the profile and nothing else.
func TestProfileRefreshDoesNotRevertProgress(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-progress"
	snapRepo := st.SnapshotRepoFor(owner)

	skill := skillgraph.AllSkills()[0]
	masteryAt := func(attempts int) *store.MasterySnapshotData {
		return &store.MasterySnapshotData{Skills: map[string]*store.SkillMasteryData{
			skill.ID: {SkillID: skill.ID, State: "learning", TotalAttempts: attempts, CorrectCount: attempts},
		}}
	}

	// Session A saves, then a later session B saves more progress.
	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: store.SnapshotData{Version: 4, Mastery: masteryAt(5)}}); err != nil {
		t.Fatalf("save A: %v", err)
	}
	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now().Add(time.Second), Data: store.SnapshotData{Version: 4, Mastery: masteryAt(20)}}); err != nil {
		t.Fatalf("save B: %v", err)
	}

	// Session A's profile refresh lands late.
	if err := snapRepo.UpdateLearnerProfile(ctx, &store.LearnerProfileData{
		Summary: "late arrival", Strengths: []string{"addition"},
	}); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	latest, err := snapRepo.Latest(ctx)
	if err != nil || latest == nil {
		t.Fatalf("latest: %v", err)
	}
	if got := latest.Data.Mastery.Skills[skill.ID].TotalAttempts; got != 20 {
		t.Errorf("mastery attempts = %d, want 20 — the late profile write reverted progress", got)
	}
	if latest.Data.LearnerProfile == nil || latest.Data.LearnerProfile.Summary != "late arrival" {
		t.Error("profile was not stored on the latest snapshot")
	}
}

// TestProfileRefreshDoesNotAddSnapshots: appending a second row per session
// halved how much history Prune(keep) actually retained.
func TestProfileRefreshDoesNotAddSnapshots(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-rows"
	snapRepo := st.SnapshotRepoFor(owner)

	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: store.SnapshotData{Version: 4}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := snapRepo.UpdateLearnerProfile(ctx, &store.LearnerProfileData{
			Summary: "v", Strengths: []string{"addition"},
		}); err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
	}

	// Pruning to 1 must leave the single row intact, profile included.
	if err := snapRepo.Prune(ctx, 1); err != nil {
		t.Fatalf("prune: %v", err)
	}
	latest, err := snapRepo.Latest(ctx)
	if err != nil || latest == nil {
		t.Fatalf("latest: %v", err)
	}
	if latest.Data.LearnerProfile == nil {
		t.Error("profile lost; refreshes are still appending rows")
	}
}

// TestProfileSurvivesSnapshotReadFailure: when the snapshot read fails, the
// profile must be recovered from the versioned event stream rather than
// silently dropped from the snapshot being written.
func TestProfileSurvivesSnapshotReadFailure(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-recover"
	eventRepo := st.EventRepoFor(owner)

	if err := eventRepo.AppendLearnerProfileEvent(ctx, store.LearnerProfileEventData{
		Summary:    "from the event stream",
		Strengths:  []string{"addition"},
		Weaknesses: []string{"regrouping"},
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	got := recoverProfile(ctx, owner, eventRepo)
	if got == nil {
		t.Fatal("no profile recovered from the event stream")
	}
	if got.Summary != "from the event stream" {
		t.Errorf("summary = %q, want the newest versioned profile", got.Summary)
	}
}

func TestProfileRefreshDue(t *testing.T) {
	enough := profileMinAttempts
	cases := []struct {
		name  string
		facts refreshFacts
		want  bool
	}{
		{"too little evidence", refreshFacts{LifetimeAttempts: enough - 1}, false},
		{"too little evidence even with mastery change", refreshFacts{LifetimeAttempts: 5, MasteryChanged: true}, false},
		{"first profile", refreshFacts{LifetimeAttempts: enough}, true},
		{"mastery changed", refreshFacts{LifetimeAttempts: enough, HasProfile: true, MasteryChanged: true}, true},
		{"enough sessions since profile", refreshFacts{LifetimeAttempts: enough, HasProfile: true, SessionsSinceProfile: profileEverySessions}, true},
		{"not due", refreshFacts{LifetimeAttempts: 500, HasProfile: true, SessionsSinceProfile: profileEverySessions - 1}, false},
	}
	for _, tc := range cases {
		if got, reason := profileRefreshDue(tc.facts); got != tc.want {
			t.Errorf("%s: due = %v (%s), want %v", tc.name, got, reason, tc.want)
		}
	}
}

func TestMasteryStandingChanged(t *testing.T) {
	id := skillgraph.AllSkills()[0].ID
	snap := func(state, tier string) *store.MasterySnapshotData {
		return &store.MasterySnapshotData{Skills: map[string]*store.SkillMasteryData{
			id: {SkillID: id, State: state, CurrentTier: tier},
		}}
	}
	otherID := skillgraph.AllSkills()[1].ID
	other := &store.MasterySnapshotData{Skills: map[string]*store.SkillMasteryData{
		otherID: {SkillID: otherID, State: "learning", CurrentTier: "learn"},
	}}
	cases := []struct {
		name       string
		prev, next *store.MasterySnapshotData
		want       bool
	}{
		{"same standing", snap("learning", "learn"), snap("learning", "learn"), false},
		// learn to prove changes the tier and keeps the state.
		{"tier advanced", snap("learning", "learn"), snap("learning", "prove"), true},
		{"mastered", snap("learning", "prove"), snap("mastered", "prove"), true},
		{"decayed", snap("mastered", "prove"), snap("rusty", "prove"), true},
		{"skill started", other, snap("learning", "learn"), true},
		{"skill untouched", other, snap("new", "learn"), false},
		{"no previous mastery data", nil, snap("learning", "learn"), false},
		{"no mastery data", snap("learning", "learn"), nil, false},
		{"off-curriculum skill", other, &store.MasterySnapshotData{Skills: map[string]*store.SkillMasteryData{
			"quest:abc": {SkillID: "quest:abc", State: "learning"},
		}}, false},
	}
	for _, tc := range cases {
		if got := masteryStandingChanged(tc.prev, tc.next); got != tc.want {
			t.Errorf("%s: changed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSessionsSinceKeepsSubSecondOrder: the session that triggered a
// profile ends a moment before the profile is made. Rounded down to the
// second, the profile time fell before that end and counted the session as
// newer than the profile.
func TestSessionsSinceKeepsSubSecondOrder(t *testing.T) {
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ended := base.Add(300 * time.Millisecond)
	profile := &store.LearnerProfileData{GeneratedAt: base.Add(500 * time.Millisecond).Format(time.RFC3339Nano)}
	sessions := []lessons.SessionSummary{{EndedAt: ended}, {EndedAt: base.Add(-time.Hour)}}
	if n := sessionsSince(sessions, profile); n != 0 {
		t.Errorf("sessions since profile = %d, want 0", n)
	}
	later := append([]lessons.SessionSummary{{EndedAt: base.Add(time.Minute)}}, sessions...)
	if n := sessionsSince(later, profile); n != 1 {
		t.Errorf("sessions since profile = %d, want 1", n)
	}
	// An old second-precision value still parses.
	old := &store.LearnerProfileData{GeneratedAt: base.Format(time.RFC3339)}
	if n := sessionsSince(later, old); n != 2 {
		t.Errorf("sessions since second-precision profile = %d, want 2", n)
	}
	if n := sessionsSince(later, &store.LearnerProfileData{GeneratedAt: "garbage"}); n != len(later) {
		t.Errorf("sessions since unreadable profile time = %d, want %d", n, len(later))
	}
}

// captureLogs sends slog output to a buffer for the rest of the test.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	var logs syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &logs
}

// waitForLog waits until the refresh goroutine logs want. A fixed sleep
// would let a skip test pass on a slow runner before the goroutine ran.
func waitForLog(t *testing.T, logs *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("no log %q; got %q", want, logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// expectSkipped waits for the refresh to log its decision, then fails if
// it called the model.
func expectSkipped(t *testing.T, logs *syncBuffer, provider *signalProvider, reason, why string) {
	t.Helper()
	waitForLog(t, logs, `"reason":"`+reason+`"`)
	select {
	case <-provider.called:
		t.Error(why)
	default:
	}
}

// endSession records a completed session the way both drivers do: the end
// event lands before the snapshot save.
func endSession(t *testing.T, repo store.EventRepo, id, skillID string, served, correct int) {
	t.Helper()
	plan := []store.PlanSlotSummaryData{{SkillID: skillID, Tier: "learn", Category: "core"}}
	for _, action := range []string{"start", "end"} {
		if err := repo.AppendSessionEvent(context.Background(), store.SessionEventData{
			SessionID: id, Action: action, QuestionsServed: served, CorrectAnswers: correct, PlanSummary: plan,
		}); err != nil {
			t.Fatalf("session %s %s: %v", id, action, err)
		}
	}
}

// TestFirstExpeditionWritesNoProfile: five answers are too little evidence
// for a whole-child profile.
func TestFirstExpeditionWritesNoProfile(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-new"
	skill := skillgraph.AllSkills()[0]
	repo := st.EventRepoFor(owner)
	seedAnswers(t, repo, skill.ID, 5, 4)
	endSession(t, repo, "sess-1", skill.ID, 5, 4)

	logs := captureLogs(t)
	provider := newSignalProvider([]byte(profileJSONv1))
	state := &SessionState{
		TotalQuestions:  5,
		PerSkillResults: map[string]*SkillResult{skill.ID: {SkillID: skill.ID, SkillName: skill.Name, Attempted: 5, Correct: 4}},
		RecentErrors:    map[string][]string{},
		EventRepo:       repo,
	}
	snapData := store.SnapshotData{Version: 4, Mastery: &store.MasterySnapshotData{Skills: map[string]*store.SkillMasteryData{
		skill.ID: {SkillID: skill.ID, State: "learning", CurrentTier: "learn", TotalAttempts: 5, CorrectCount: 4},
	}}}
	if err := SaveSnapshotWithProfile(ctx, owner, st.SnapshotRepoFor(owner), lessons.NewCompressor(provider, lessons.DefaultCompressorConfig()), state, snapData); err != nil {
		t.Fatalf("save: %v", err)
	}
	expectSkipped(t, logs, provider, "too little evidence", "a first five-question expedition generated a profile")
}

// strongHistory sets up the FOU-241 case: a child with 40/45 lifetime on a
// mastered skill, a profile made after their last session, and a new
// expedition on that skill that goes badly.
func strongHistory(t *testing.T, st *store.Store, owner string) (skillgraph.Skill, *SessionState, store.SnapshotData) {
	t.Helper()
	ctx := context.Background()
	skill := skillgraph.AllSkills()[0]
	repo := st.EventRepoFor(owner)
	snapRepo := st.SnapshotRepoFor(owner)

	seedAnswers(t, repo, skill.ID, 40, 39)
	for i := range 8 {
		endSession(t, repo, "earlier-"+string(rune('a'+i)), skill.ID, 5, 5)
	}
	mastered := func(attempts, correct int) *store.MasterySnapshotData {
		return &store.MasterySnapshotData{Skills: map[string]*store.SkillMasteryData{
			skill.ID: {SkillID: skill.ID, State: "mastered", CurrentTier: "prove", TotalAttempts: attempts, CorrectCount: correct},
		}}
	}
	if err := snapRepo.Save(ctx, &store.Snapshot{Timestamp: time.Now(), Data: store.SnapshotData{
		Version: 4, Mastery: mastered(20, 20),
		LearnerProfile: &store.LearnerProfileData{
			Summary: "Strong on this skill", Strengths: []string{skill.Name}, Weaknesses: []string{"none yet"},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
	}}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	// This expedition: 1 of 5 correct, no change of state or tier.
	seedAnswers(t, repo, skill.ID, 5, 1)
	endSession(t, repo, "bad-day", skill.ID, 5, 1)
	state := &SessionState{
		TotalQuestions:  5,
		PerSkillResults: map[string]*SkillResult{skill.ID: {SkillID: skill.ID, SkillName: skill.Name, Attempted: 5, Correct: 1}},
		RecentErrors:    map[string][]string{skill.ID: {"Answered 5 for '2+2', correct answer was 4"}},
		EventRepo:       repo,
	}
	return skill, state, store.SnapshotData{Version: 4, Mastery: mastered(25, 21)}
}

// TestBadExpeditionDoesNotRewriteStrongProfile is the FOU-241 regression:
// one bad expedition with no change of standing leaves the profile alone.
func TestBadExpeditionDoesNotRewriteStrongProfile(t *testing.T) {
	st := newPersistTestStore(t)
	owner := "child-strong"
	_, state, snapData := strongHistory(t, st, owner)

	logs := captureLogs(t)
	provider := newSignalProvider([]byte(profileJSONv1))
	if err := SaveSnapshotWithProfile(context.Background(), owner, st.SnapshotRepoFor(owner), lessons.NewCompressor(provider, lessons.DefaultCompressorConfig()), state, snapData); err != nil {
		t.Fatalf("save: %v", err)
	}
	expectSkipped(t, logs, provider, "not due", "one bad expedition rewrote the profile of a child with strong history")
}

// TestGroundedPromptLeadsWithHistory: when the refresh is due, the prompt
// carries the lifetime record ahead of this session, and names diagnosed
// misconceptions from earlier sessions.
func TestGroundedPromptLeadsWithHistory(t *testing.T) {
	st := newPersistTestStore(t)
	ctx := context.Background()
	owner := "child-grounded"
	skill, state, snapData := strongHistory(t, st, owner)
	misconception := "npv-place-swap"
	for range 2 {
		if err := state.EventRepo.AppendDiagnosisEvent(ctx, store.DiagnosisEventData{
			SessionID: "earlier-a", SkillID: skill.ID, QuestionText: "What is 2+2?", CorrectAnswer: "4",
			LearnerAnswer: "5", Category: "misconception", MisconceptionID: &misconception,
			Confidence: 0.9, ClassifierName: "rules",
		}); err != nil {
			t.Fatalf("seed diagnosis: %v", err)
		}
	}
	// Mastery decayed this session, so the refresh is due.
	snapData.Mastery.Skills[skill.ID].State = "rusty"

	provider := newSignalProvider(nil)
	if err := SaveSnapshotWithProfile(ctx, owner, st.SnapshotRepoFor(owner), lessons.NewCompressor(provider, lessons.DefaultCompressorConfig()), state, snapData); err != nil {
		t.Fatalf("save: %v", err)
	}
	prompt := provider.firstPrompt(t)

	history, session := strings.Index(prompt, "Learner History"), strings.Index(prompt, "This Session")
	if history < 0 || session < 0 || history > session {
		t.Fatalf("prompt does not put the learner history before this session:\n%s", prompt)
	}
	for _, want := range []string{
		"Sessions completed: 9",
		"40/45 correct",                             // lifetime, not this session's 1/5
		"misconception: Place value swap (2 times)", // diagnosis history, read back
		"Recent Sessions",
		"Previous Profile (from ",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
}

// failingHistory is an event repo whose history reads fail.
type failingHistory struct{ store.EventRepo }

func (failingHistory) AnswerTallies(context.Context) ([]store.AnswerTally, error) {
	return nil, errors.New("database gone")
}

// TestProfileSkippedWhenHistoryFails: without history the refresh does not
// fall back to a profile from this session alone.
func TestProfileSkippedWhenHistoryFails(t *testing.T) {
	st := newPersistTestStore(t)
	owner := "child-nohistory"
	_, state, snapData := strongHistory(t, st, owner)
	state.EventRepo = failingHistory{state.EventRepo}
	snapData.Mastery.Skills[skillgraph.AllSkills()[0].ID].State = "rusty"

	logs := captureLogs(t)
	provider := newSignalProvider([]byte(profileJSONv1))
	if err := SaveSnapshotWithProfile(context.Background(), owner, st.SnapshotRepoFor(owner), lessons.NewCompressor(provider, lessons.DefaultCompressorConfig()), state, snapData); err != nil {
		t.Fatalf("save: %v", err)
	}
	// The skip log is the last thing the refresh goroutine does.
	waitForLog(t, logs, "learner profile skipped")
	if out := logs.String(); !strings.Contains(out, `"owner_id":"`+owner+`"`) {
		t.Errorf("skip log is not owner-attributable; got %q", out)
	}
	select {
	case <-provider.called:
		t.Error("a refresh without history generated a profile")
	default:
	}
}

// syncBuffer is a bytes.Buffer safe for the refresh goroutine to log into
// while the test reads it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
