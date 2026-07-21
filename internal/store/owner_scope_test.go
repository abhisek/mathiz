package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abhisek/mathiz/ent"
)

// openIsolationStore returns a store for owner-isolation tests. It uses
// in-memory SQLite by default and PostgreSQL when MATHIZ_TEST_DATABASE_URL
// is set, so the same suite validates both dialects.
func openIsolationStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("MATHIZ_TEST_DATABASE_URL")
	if dsn == "" {
		return openTestStore(t)
	}
	s, err := Open(dsn)
	if err != nil {
		t.Fatalf("open postgres test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ownerNonces gives each test invocation its own nonce (keyed by the
// *testing.T instance), so repeated runs against a persistent database
// (Postgres) never see rows from other tests, prior runs, or -count>1 reruns.
var ownerNonces sync.Map

// testOwner derives a per-test-invocation owner ID, stable within one test.
func testOwner(t *testing.T, name string) string {
	nonce, _ := ownerNonces.LoadOrStore(t, time.Now().UnixNano())
	return fmt.Sprintf("%s/%d/%s", t.Name(), nonce, name)
}

func TestOwnerIsolationEvents(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.EventRepoFor(testOwner(t, "alice"))
	bob := s.EventRepoFor(testOwner(t, "bob"))

	answer := AnswerEventData{
		SessionID: "sess-a", SkillID: "add-1", Tier: "learn",
		Category: "core", QuestionText: "What is 2+3?", CorrectAnswer: "5",
		LearnerAnswer: "5", Correct: true, TimeMs: 1200, AnswerFormat: "integer",
	}
	if err := alice.AppendAnswerEvent(ctx, answer); err != nil {
		t.Fatalf("alice append: %v", err)
	}
	wrong := answer
	wrong.Correct = false
	wrong.SessionID = "sess-b"
	if err := bob.AppendAnswerEvent(ctx, wrong); err != nil {
		t.Fatalf("bob append: %v", err)
	}

	// Each owner sees only their own accuracy.
	acc, err := alice.SkillAccuracy(ctx, "add-1")
	if err != nil {
		t.Fatalf("alice accuracy: %v", err)
	}
	if acc != 1.0 {
		t.Errorf("alice accuracy = %v, want 1.0", acc)
	}
	acc, err = bob.SkillAccuracy(ctx, "add-1")
	if err != nil {
		t.Fatalf("bob accuracy: %v", err)
	}
	if acc != 0.0 {
		t.Errorf("bob accuracy = %v, want 0.0", acc)
	}

	// An owner with no events sees nothing.
	acc, err = s.EventRepoFor(testOwner(t, "carol")).SkillAccuracy(ctx, "add-1")
	if err != nil {
		t.Fatalf("carol accuracy: %v", err)
	}
	if acc != 0.0 {
		t.Errorf("carol accuracy = %v, want 0.0", acc)
	}
}

func TestOwnerIsolationGemsAndSessions(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.EventRepoFor(testOwner(t, "alice"))
	bob := s.EventRepoFor(testOwner(t, "bob"))

	if err := alice.AppendGemEvent(ctx, GemEventData{GemType: "ruby", Rarity: "rare", SessionID: "sess-a", Reason: "streak"}); err != nil {
		t.Fatalf("alice gem: %v", err)
	}
	if err := alice.AppendSessionEvent(ctx, SessionEventData{SessionID: "sess-a", Action: "end", QuestionsServed: 5, CorrectAnswers: 4}); err != nil {
		t.Fatalf("alice session: %v", err)
	}

	byType, total, err := bob.GemCounts(ctx)
	if err != nil {
		t.Fatalf("bob gem counts: %v", err)
	}
	if total != 0 || len(byType) != 0 {
		t.Errorf("bob sees %d gems, want 0", total)
	}

	summaries, err := bob.QuerySessionSummaries(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("bob summaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("bob sees %d sessions, want 0", len(summaries))
	}

	summaries, err = alice.QuerySessionSummaries(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("alice summaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("alice sees %d sessions, want 1", len(summaries))
	}
	if summaries[0].GemCount != 1 {
		t.Errorf("alice session gem count = %d, want 1", summaries[0].GemCount)
	}
}

func TestOwnerIsolationSnapshots(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.SnapshotRepoFor(testOwner(t, "alice"))
	bob := s.SnapshotRepoFor(testOwner(t, "bob"))

	now := time.Now().UTC().Truncate(time.Second)
	if err := alice.Save(ctx, &Snapshot{Sequence: 1, Timestamp: now, Data: SnapshotData{Version: 7}}); err != nil {
		t.Fatalf("alice save: %v", err)
	}

	snap, err := bob.Latest(ctx)
	if err != nil {
		t.Fatalf("bob latest: %v", err)
	}
	if snap != nil {
		t.Fatal("bob sees alice's snapshot")
	}

	snap, err = alice.Latest(ctx)
	if err != nil {
		t.Fatalf("alice latest: %v", err)
	}
	if snap == nil || snap.Data.Version != 7 {
		t.Fatalf("alice snapshot = %+v, want version 7", snap)
	}

	// Prune for bob must not delete alice's snapshots.
	if err := bob.Prune(ctx, 0); err != nil {
		t.Fatalf("bob prune: %v", err)
	}
	snap, err = alice.Latest(ctx)
	if err != nil {
		t.Fatalf("alice latest after bob prune: %v", err)
	}
	if snap == nil {
		t.Fatal("bob's prune deleted alice's snapshot")
	}
}

func TestOwnerIsolationLLMUsage(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.EventRepoFor(testOwner(t, "alice"))
	bob := s.EventRepoFor(testOwner(t, "bob"))

	if err := alice.AppendLLMRequest(ctx, LLMRequestEventData{
		Provider: "anthropic", Model: "haiku", Purpose: "problemgen",
		InputTokens: 100, OutputTokens: 50, LatencyMs: 800, Success: true,
	}); err != nil {
		t.Fatalf("alice llm: %v", err)
	}

	stats, err := bob.LLMUsageByPurpose(ctx)
	if err != nil {
		t.Fatalf("bob usage: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("bob sees %d usage rows, want 0", len(stats))
	}

	stats, err = alice.LLMUsageByPurpose(ctx)
	if err != nil {
		t.Fatalf("alice usage: %v", err)
	}
	if len(stats) != 1 || stats[0].InputTokens != 100 {
		t.Errorf("alice usage = %+v, want 1 row with 100 input tokens", stats)
	}

	events, err := bob.QueryLLMEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("bob llm events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("bob sees %d llm events, want 0", len(events))
	}
}

func TestLocalOwnerIsDefault(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	// The unscoped repos and the explicitly local-scoped repos are the same
	// view. Assert on the delta so runs against a persistent database pass.
	_, before, err := s.EventRepoFor(LocalOwner).GemCounts(ctx)
	if err != nil {
		t.Fatalf("counts before: %v", err)
	}
	if err := s.EventRepo().AppendGemEvent(ctx, GemEventData{GemType: "pearl", Rarity: "common", SessionID: "s", Reason: "r"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	_, after, err := s.EventRepoFor(LocalOwner).GemCounts(ctx)
	if err != nil {
		t.Fatalf("counts after: %v", err)
	}
	if after != before+1 {
		t.Errorf("local owner gem count delta = %d, want 1", after-before)
	}

	// A SaaS owner does not see local data.
	_, total, err := s.EventRepoFor(testOwner(t, "alice")).GemCounts(ctx)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if total != 0 {
		t.Errorf("scoped owner sees %d local gems, want 0", total)
	}
}

func TestOwnerIsolationLessonEvents(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.EventRepoFor(testOwner(t, "alice"))
	bob := s.EventRepoFor(testOwner(t, "bob"))

	lesson := LessonEventData{
		SessionID: "sess-a", SkillID: "add-1", LessonTitle: "Alice's tip",
		Explanation: "carry the one", WorkedExample: "12+9",
		PracticeText: "13+8?", PracticeAnswer: "21",
	}
	if err := alice.AppendLessonEvent(ctx, lesson); err != nil {
		t.Fatalf("alice append: %v", err)
	}
	bobLesson := lesson
	bobLesson.LessonTitle = "Bob's tip"
	if err := bob.AppendLessonEvent(ctx, bobLesson); err != nil {
		t.Fatalf("bob append: %v", err)
	}

	// The notebook query must only surface the owner's own lessons.
	got, err := alice.QueryLessonEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("alice query: %v", err)
	}
	if len(got) != 1 || got[0].LessonTitle != "Alice's tip" {
		t.Errorf("alice sees %d lessons (%v), want only her own", len(got), got)
	}
	got, err = s.EventRepoFor(testOwner(t, "carol")).QueryLessonEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("carol query: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("carol sees %d lessons, want 0", len(got))
	}
}

func TestOwnerIsolationLearnerProfileEvents(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.EventRepoFor(testOwner(t, "alice"))
	bob := s.EventRepoFor(testOwner(t, "bob"))

	v1 := LearnerProfileEventData{
		Summary:     "Alice is strong on addition",
		Strengths:   []string{"addition", "counting"},
		Weaknesses:  []string{"regrouping"},
		Patterns:    []string{"rushes on timed questions"},
		GeneratedAt: "2026-07-21T10:00:00Z",
	}
	if err := alice.AppendLearnerProfileEvent(ctx, v1); err != nil {
		t.Fatalf("alice append v1: %v", err)
	}
	v2 := v1
	v2.Summary = "Alice now regroups reliably"
	v2.Weaknesses = nil
	v2.GeneratedAt = "2026-07-21T11:00:00Z"
	if err := alice.AppendLearnerProfileEvent(ctx, v2); err != nil {
		t.Fatalf("alice append v2: %v", err)
	}
	if err := bob.AppendLearnerProfileEvent(ctx, LearnerProfileEventData{
		Summary: "Bob's profile", Strengths: []string{"subtraction"},
	}); err != nil {
		t.Fatalf("bob append: %v", err)
	}

	// Round-trip: alice sees her two versions, newest first, arrays intact.
	got, err := alice.QueryLearnerProfileEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("alice query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("alice sees %d profile versions, want 2", len(got))
	}
	if got[0].Summary != v2.Summary || got[1].Summary != v1.Summary {
		t.Errorf("order = [%q, %q], want newest first", got[0].Summary, got[1].Summary)
	}
	if got[0].Sequence <= got[1].Sequence {
		t.Errorf("sequences = (%d, %d), want descending", got[0].Sequence, got[1].Sequence)
	}
	oldest := got[1]
	if len(oldest.Strengths) != 2 || oldest.Strengths[0] != "addition" || oldest.Strengths[1] != "counting" {
		t.Errorf("strengths = %v, want [addition counting]", oldest.Strengths)
	}
	if len(oldest.Weaknesses) != 1 || oldest.Weaknesses[0] != "regrouping" {
		t.Errorf("weaknesses = %v, want [regrouping]", oldest.Weaknesses)
	}
	if len(oldest.Patterns) != 1 || oldest.Patterns[0] != "rushes on timed questions" {
		t.Errorf("patterns = %v, want [rushes on timed questions]", oldest.Patterns)
	}
	if oldest.GeneratedAt != "2026-07-21T10:00:00Z" {
		t.Errorf("generated_at = %q", oldest.GeneratedAt)
	}

	// Limit is honored.
	got, err = alice.QueryLearnerProfileEvents(ctx, QueryOpts{Limit: 1})
	if err != nil {
		t.Fatalf("alice limited query: %v", err)
	}
	if len(got) != 1 || got[0].Summary != v2.Summary {
		t.Errorf("limited query = %+v, want just the newest version", got)
	}

	// Isolation: bob sees only his own version; carol sees nothing.
	got, err = bob.QueryLearnerProfileEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("bob query: %v", err)
	}
	if len(got) != 1 || got[0].Summary != "Bob's profile" {
		t.Errorf("bob sees %+v, want only his own version", got)
	}
	got, err = s.EventRepoFor(testOwner(t, "carol")).QueryLearnerProfileEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("carol query: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("carol sees %d profile versions, want 0", len(got))
	}
}

// TestOwnerIsolationActivityQueries covers the activity-timeline read
// methods: mastery transitions, per-session answers, and hint counts must
// never cross owners.
func TestOwnerIsolationActivityQueries(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.EventRepoFor(testOwner(t, "alice"))
	bob := s.EventRepoFor(testOwner(t, "bob"))

	if err := alice.AppendMasteryEvent(ctx, MasteryEventData{
		SkillID: "add-1", FromState: "learning", ToState: "mastered",
		Trigger: "fluency", FluencyScore: 0.9, SessionID: "sess-a",
	}); err != nil {
		t.Fatalf("alice mastery: %v", err)
	}
	if err := bob.AppendMasteryEvent(ctx, MasteryEventData{
		SkillID: "add-1", FromState: "mastered", ToState: "rusty",
		Trigger: "decay", FluencyScore: 0.2, SessionID: "sess-b",
	}); err != nil {
		t.Fatalf("bob mastery: %v", err)
	}
	// Same session ID in both streams: the session-scoped queries must still
	// separate by owner.
	if err := alice.AppendAnswerEvent(ctx, AnswerEventData{
		SessionID: "shared-sess", SkillID: "add-1", Tier: "learn", Category: "core",
		QuestionText: "2+3?", CorrectAnswer: "5", LearnerAnswer: "5",
		Correct: true, TimeMs: 900, AnswerFormat: "integer",
	}); err != nil {
		t.Fatalf("alice answer: %v", err)
	}
	if err := bob.AppendAnswerEvent(ctx, AnswerEventData{
		SessionID: "shared-sess", SkillID: "add-1", Tier: "learn", Category: "core",
		QuestionText: "9-4?", CorrectAnswer: "5", LearnerAnswer: "4",
		Correct: false, TimeMs: 2100, AnswerFormat: "integer",
	}); err != nil {
		t.Fatalf("bob answer: %v", err)
	}
	if err := alice.AppendHintEvent(ctx, HintEventData{
		SessionID: "shared-sess", SkillID: "add-1", QuestionText: "2+3?", HintText: "count up",
	}); err != nil {
		t.Fatalf("alice hint: %v", err)
	}

	// Mastery events: each owner sees only their own transition.
	got, err := alice.QueryMasteryEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("alice mastery query: %v", err)
	}
	if len(got) != 1 || got[0].ToState != "mastered" || got[0].Sequence == 0 {
		t.Errorf("alice mastery = %+v, want her single mastered transition", got)
	}
	got, err = bob.QueryMasteryEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("bob mastery query: %v", err)
	}
	if len(got) != 1 || got[0].ToState != "rusty" {
		t.Errorf("bob mastery = %+v, want his single rusty transition", got)
	}
	got, err = s.EventRepoFor(testOwner(t, "carol")).QueryMasteryEvents(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("carol mastery query: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("carol sees %d mastery events, want 0", len(got))
	}

	// Answers for the shared session ID stay per-owner.
	answers, err := alice.AnswersForSession(ctx, "shared-sess")
	if err != nil {
		t.Fatalf("alice answers: %v", err)
	}
	if len(answers) != 1 || !answers[0].Correct || answers[0].QuestionText != "2+3?" {
		t.Errorf("alice answers = %+v, want only her own", answers)
	}
	answers, err = bob.AnswersForSession(ctx, "shared-sess")
	if err != nil {
		t.Fatalf("bob answers: %v", err)
	}
	if len(answers) != 1 || answers[0].Correct {
		t.Errorf("bob answers = %+v, want only his own", answers)
	}

	// Hints: bob has none, alice one, despite the shared session ID.
	n, err := bob.HintCountForSession(ctx, "shared-sess")
	if err != nil {
		t.Fatalf("bob hints: %v", err)
	}
	if n != 0 {
		t.Errorf("bob hint count = %d, want 0", n)
	}
	n, err = alice.HintCountForSession(ctx, "shared-sess")
	if err != nil {
		t.Fatalf("alice hints: %v", err)
	}
	if n != 1 {
		t.Errorf("alice hint count = %d, want 1", n)
	}
}

// TestSessionSummariesJoinPlan proves QuerySessionSummaries hydrates each
// end event with its start event's plan — and only from the same owner's
// stream, even when session IDs collide across owners.
func TestSessionSummariesJoinPlan(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	alice := s.EventRepoFor(testOwner(t, "alice"))
	bob := s.EventRepoFor(testOwner(t, "bob"))

	if err := alice.AppendSessionEvent(ctx, SessionEventData{
		SessionID: "sess-1", Action: "start",
		PlanSummary: []PlanSlotSummaryData{{SkillID: "add-1", Tier: "learn", Category: "core"}},
		QuestUID:    "q-77", QuestName: "HCF Week",
	}); err != nil {
		t.Fatalf("alice start: %v", err)
	}
	// Bob's start on the same session ID must not bleed into alice's plan.
	if err := bob.AppendSessionEvent(ctx, SessionEventData{
		SessionID: "sess-1", Action: "start",
		PlanSummary: []PlanSlotSummaryData{{SkillID: "mult-9", Tier: "prove", Category: "core"}},
	}); err != nil {
		t.Fatalf("bob start: %v", err)
	}
	if err := alice.AppendSessionEvent(ctx, SessionEventData{
		SessionID: "sess-1", Action: "end", QuestionsServed: 5, CorrectAnswers: 4, DurationSecs: 300,
	}); err != nil {
		t.Fatalf("alice end: %v", err)
	}

	sums, err := alice.QuerySessionSummaries(ctx, QueryOpts{})
	if err != nil {
		t.Fatalf("alice summaries: %v", err)
	}
	if len(sums) != 1 {
		t.Fatalf("alice sees %d summaries, want 1", len(sums))
	}
	got := sums[0]
	if got.Sequence == 0 {
		t.Error("summary Sequence not populated")
	}
	if len(got.Plan) != 1 || got.Plan[0].SkillID != "add-1" ||
		got.Plan[0].Tier != "learn" || got.Plan[0].Category != "core" {
		t.Errorf("summary Plan = %+v, want alice's start plan", got.Plan)
	}
	// Quest attribution rides the start event and joins onto the summary.
	if got.QuestUID != "q-77" || got.QuestName != "HCF Week" {
		t.Errorf("summary quest = (%q, %q), want (q-77, HCF Week)", got.QuestUID, got.QuestName)
	}

	// Cursor pagination: nothing strictly older than the end event's sequence.
	sums, err = alice.QuerySessionSummaries(ctx, QueryOpts{Before: got.Sequence})
	if err != nil {
		t.Fatalf("alice summaries before cursor: %v", err)
	}
	if len(sums) != 0 {
		t.Errorf("summaries before own sequence = %d, want 0", len(sums))
	}
}

// TestOwnerGuardQueryFailsClosed proves the structural safety net: a query
// through the ent client directly (bypassing the repos) on an owner-scoped
// table with a bare context must error, not silently return all tenants'
// rows.
func TestOwnerGuardQueryFailsClosed(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	if _, err := s.Client().AnswerEvent.Query().All(ctx); err == nil || !strings.Contains(err.Error(), "unscoped query") {
		t.Errorf("bare-context AnswerEvent query err = %v, want unscoped-query error", err)
	}
	if _, err := s.Client().Snapshot.Query().Count(ctx); err == nil || !strings.Contains(err.Error(), "unscoped query") {
		t.Errorf("bare-context Snapshot count err = %v, want unscoped-query error", err)
	}
	// Aggregate/GroupBy paths are intercepted too.
	var rows []struct {
		GemType string `json:"gem_type"`
		Count   int    `json:"count"`
	}
	err := s.Client().GemEvent.Query().GroupBy("gem_type").Aggregate(ent.Count()).Scan(ctx, &rows)
	if err == nil || !strings.Contains(err.Error(), "unscoped query") {
		t.Errorf("bare-context GemEvent group-by err = %v, want unscoped-query error", err)
	}

	// Family-scoped control-plane tables are not owner-scoped and must stay
	// reachable without an owner in ctx.
	if _, err := s.Client().Account.Query().Count(ctx); err != nil {
		t.Errorf("bare-context Account count err = %v, want nil", err)
	}
}

// TestOwnerGuardMutationFailsClosed proves mutations on owner-scoped tables
// require an owner in the context and reject conflicting explicit owners.
func TestOwnerGuardMutationFailsClosed(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()

	seqNum, err := s.seq.Next(ctx)
	if err != nil {
		t.Fatalf("next sequence: %v", err)
	}
	_, err = s.Client().GemEvent.Create().
		SetSequence(seqNum).
		SetGemType("ruby").SetRarity("rare").SetSessionID("s").SetReason("r").
		Save(ctx)
	if err == nil || !strings.Contains(err.Error(), "unscoped mutation") {
		t.Errorf("bare-context create err = %v, want unscoped-mutation error", err)
	}

	// Explicit owner conflicting with the context owner is rejected.
	alice := testOwner(t, "alice")
	seqNum, err = s.seq.Next(ctx)
	if err != nil {
		t.Fatalf("next sequence: %v", err)
	}
	_, err = s.Client().GemEvent.Create().
		SetSequence(seqNum).
		SetOwnerID(testOwner(t, "bob")).
		SetGemType("ruby").SetRarity("rare").SetSessionID("s").SetReason("r").
		Save(withOwner(ctx, alice))
	if err == nil || !strings.Contains(err.Error(), "owner conflict") {
		t.Errorf("conflicting-owner create err = %v, want owner-conflict error", err)
	}

	// Nothing was written for either owner.
	_, total, err := s.EventRepoFor(alice).GemCounts(ctx)
	if err != nil {
		t.Fatalf("gem counts: %v", err)
	}
	if total != 0 {
		t.Errorf("alice gem count = %d, want 0 after failed writes", total)
	}
}

// TestOwnerGuardStampsAndScopes proves the guard stamps owner_id on creates
// that don't set it and adds the owner predicate to queries that don't
// filter — the isolation holds even when per-method discipline is forgotten.
func TestOwnerGuardStampsAndScopes(t *testing.T) {
	s := openIsolationStore(t)
	ctx := context.Background()
	alice := testOwner(t, "alice")
	bob := testOwner(t, "bob")

	for _, owner := range []string{alice, bob} {
		seqNum, err := s.seq.Next(ctx)
		if err != nil {
			t.Fatalf("next sequence: %v", err)
		}
		// No SetOwnerID: the mutation hook must stamp it from ctx.
		_, err = s.Client().GemEvent.Create().
			SetSequence(seqNum).
			SetGemType("ruby").SetRarity("rare").SetSessionID("s").SetReason("r").
			Save(withOwner(ctx, owner))
		if err != nil {
			t.Fatalf("create for %s: %v", owner, err)
		}
	}

	// The repo (explicit Where) sees exactly the stamped row.
	_, total, err := s.EventRepoFor(alice).GemCounts(ctx)
	if err != nil {
		t.Fatalf("gem counts: %v", err)
	}
	if total != 1 {
		t.Errorf("alice gem count = %d, want 1", total)
	}

	// A client query with owner ctx but NO explicit Where is still scoped by
	// the interceptor's added predicate.
	n, err := s.Client().GemEvent.Query().Count(withOwner(ctx, alice))
	if err != nil {
		t.Fatalf("client count: %v", err)
	}
	if n != 1 {
		t.Errorf("unfiltered client count for alice = %d, want 1", n)
	}
}
