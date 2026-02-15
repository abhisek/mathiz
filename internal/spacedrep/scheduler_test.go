package spacedrep

import (
	"context"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/store"
)

// mockEventRepo is a minimal mock for tests.
type mockEventRepo struct {
	masteryEvents []store.MasteryEventData
}

func (m *mockEventRepo) AppendLLMRequest(_ context.Context, _ store.LLMRequestEventData) error {
	return nil
}
func (m *mockEventRepo) AppendSessionEvent(_ context.Context, _ store.SessionEventData) error {
	return nil
}
func (m *mockEventRepo) AppendAnswerEvent(_ context.Context, _ store.AnswerEventData) error {
	return nil
}
func (m *mockEventRepo) AppendMasteryEvent(_ context.Context, data store.MasteryEventData) error {
	m.masteryEvents = append(m.masteryEvents, data)
	return nil
}
func (m *mockEventRepo) LatestAnswerTime(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (m *mockEventRepo) SkillAccuracy(_ context.Context, _ string) (float64, error) {
	return 0, nil
}
func (m *mockEventRepo) RecentReviewAccuracy(_ context.Context, _ string, _ int) (float64, int, error) {
	return 0, 0, nil
}
func (m *mockEventRepo) AppendDiagnosisEvent(_ context.Context, _ store.DiagnosisEventData) error {
	return nil
}
func (m *mockEventRepo) AppendHintEvent(_ context.Context, _ store.HintEventData) error {
	return nil
}
func (m *mockEventRepo) AppendLessonEvent(_ context.Context, _ store.LessonEventData) error {
	return nil
}

func (m *mockEventRepo) AppendGemEvent(_ context.Context, _ store.GemEventData) error {
	return nil
}

func (m *mockEventRepo) QueryGemEvents(_ context.Context, _ store.QueryOpts) ([]store.GemEventRecord, error) {
	return nil, nil
}

func (m *mockEventRepo) GemCounts(_ context.Context) (map[string]int, int, error) {
	return nil, 0, nil
}

func (m *mockEventRepo) QuerySessionSummaries(_ context.Context, _ store.QueryOpts) ([]store.SessionSummaryRecord, error) {
	return nil, nil
}

func newTestScheduler(reviews map[string]*ReviewState, masterySvc *mastery.Service, eventRepo store.EventRepo) *Scheduler {
	if reviews == nil {
		reviews = make(map[string]*ReviewState)
	}
	return &Scheduler{
		reviews:   reviews,
		mastery:   masterySvc,
		eventRepo: eventRepo,
	}
}

func masterySnap(skills map[string]*store.SkillMasteryData) *store.SnapshotData {
	return &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{Skills: skills},
	}
}

func TestInitSkill_SetsStageZero(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	rs := sched.GetReviewState("skill-a")
	if rs == nil {
		t.Fatal("expected review state")
	}
	if rs.Stage != 0 {
		t.Errorf("Stage = %d, want 0", rs.Stage)
	}
	if rs.ConsecutiveHits != 0 {
		t.Errorf("ConsecutiveHits = %d, want 0", rs.ConsecutiveHits)
	}
	if rs.Graduated {
		t.Error("expected not graduated")
	}
	expectedNext := masteredAt.AddDate(0, 0, 1)
	if !rs.NextReviewDate.Equal(expectedNext) {
		t.Errorf("NextReviewDate = %v, want %v", rs.NextReviewDate, expectedNext)
	}
}

func TestRecordReview_Correct_AdvancesStage(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	now := masteredAt.AddDate(0, 0, 1) // Day 1
	sched.RecordReview("skill-a", true, now)

	rs := sched.GetReviewState("skill-a")
	if rs.Stage != 1 {
		t.Errorf("Stage = %d, want 1", rs.Stage)
	}
	if rs.ConsecutiveHits != 1 {
		t.Errorf("ConsecutiveHits = %d, want 1", rs.ConsecutiveHits)
	}
	expectedNext := now.AddDate(0, 0, 3) // Stage 1 interval = 3 days
	if !rs.NextReviewDate.Equal(expectedNext) {
		t.Errorf("NextReviewDate = %v, want %v", rs.NextReviewDate, expectedNext)
	}
}

func TestRecordReview_Correct_MultipleTimes_Graduation(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	now := masteredAt
	for i := 0; i < 6; i++ {
		now = now.AddDate(0, 0, BaseIntervals[min(i, MaxStage)])
		sched.RecordReview("skill-a", true, now)
	}

	rs := sched.GetReviewState("skill-a")
	if !rs.Graduated {
		t.Error("expected graduated after 6 correct reviews")
	}
	if rs.ConsecutiveHits != 6 {
		t.Errorf("ConsecutiveHits = %d, want 6", rs.ConsecutiveHits)
	}
}

func TestRecordReview_Correct_Graduated_StaysGraduated(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	now := masteredAt
	for i := 0; i < 6; i++ {
		now = now.AddDate(0, 0, BaseIntervals[min(i, MaxStage)])
		sched.RecordReview("skill-a", true, now)
	}

	// One more correct review as graduated
	now = now.AddDate(0, 0, 90)
	sched.RecordReview("skill-a", true, now)

	rs := sched.GetReviewState("skill-a")
	if !rs.Graduated {
		t.Error("expected still graduated")
	}
	expectedNext := now.AddDate(0, 0, 90)
	if !rs.NextReviewDate.Equal(expectedNext) {
		t.Errorf("NextReviewDate = %v, want %v", rs.NextReviewDate, expectedNext)
	}
}

func TestRecordReview_Incorrect_ResetsConsecutiveHits(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	now := masteredAt.AddDate(0, 0, 1)
	sched.RecordReview("skill-a", true, now) // ConsecutiveHits = 1
	now = now.AddDate(0, 0, 3)
	sched.RecordReview("skill-a", true, now) // ConsecutiveHits = 2
	now = now.AddDate(0, 0, 7)
	sched.RecordReview("skill-a", true, now) // ConsecutiveHits = 3

	now = now.AddDate(0, 0, 14)
	sched.RecordReview("skill-a", false, now) // Incorrect

	rs := sched.GetReviewState("skill-a")
	if rs.ConsecutiveHits != 0 {
		t.Errorf("ConsecutiveHits = %d, want 0 after incorrect", rs.ConsecutiveHits)
	}
}

func TestRecordReview_Incorrect_DoesNotChangeStage(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	// Advance to stage 3
	now := masteredAt
	for i := 0; i < 3; i++ {
		now = now.AddDate(0, 0, BaseIntervals[i])
		sched.RecordReview("skill-a", true, now)
	}

	stageBefore := sched.GetReviewState("skill-a").Stage
	now = now.AddDate(0, 0, 14)
	sched.RecordReview("skill-a", false, now)

	rs := sched.GetReviewState("skill-a")
	if rs.Stage != stageBefore {
		t.Errorf("Stage changed from %d to %d on incorrect answer", stageBefore, rs.Stage)
	}
}

func TestReInitSkill_ResetsToStageZero(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	// Advance to stage 4 and graduate
	now := masteredAt
	for i := 0; i < 6; i++ {
		now = now.AddDate(0, 0, BaseIntervals[min(i, MaxStage)])
		sched.RecordReview("skill-a", true, now)
	}

	// Re-init (recovery)
	recoveredAt := now.AddDate(0, 0, 30)
	sched.ReInitSkill("skill-a", recoveredAt)

	rs := sched.GetReviewState("skill-a")
	if rs.Stage != 0 {
		t.Errorf("Stage = %d, want 0 after reinit", rs.Stage)
	}
	if rs.Graduated {
		t.Error("expected not graduated after reinit")
	}
	if rs.ConsecutiveHits != 0 {
		t.Errorf("ConsecutiveHits = %d, want 0", rs.ConsecutiveHits)
	}
	expectedNext := recoveredAt.AddDate(0, 0, 1)
	if !rs.NextReviewDate.Equal(expectedNext) {
		t.Errorf("NextReviewDate = %v, want %v", rs.NextReviewDate, expectedNext)
	}
}

func TestRunDecayCheck_MarksOverdueRusty(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	snap := masterySnap(map[string]*store.SkillMasteryData{
		"skill-a": {SkillID: "skill-a", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
	})
	svc := mastery.NewService(snap, nil)
	eventRepo := &mockEventRepo{}

	// Create review state that is past rusty threshold
	reviewDate := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC) // stage 0, interval 1 day
	reviews := map[string]*ReviewState{
		"skill-a": {
			SkillID:        "skill-a",
			Stage:          0,
			NextReviewDate: reviewDate,
			LastReviewDate: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
	}
	sched := newTestScheduler(reviews, svc, eventRepo)

	// Now is 2 days after review date (grace = 0.5 days)
	now := reviewDate.Add(2 * 24 * time.Hour)
	transitions := sched.RunDecayCheck(context.Background(), now)

	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].To != mastery.StateRusty {
		t.Errorf("transition To = %q, want %q", transitions[0].To, mastery.StateRusty)
	}
	if len(eventRepo.masteryEvents) != 1 {
		t.Errorf("expected 1 mastery event, got %d", len(eventRepo.masteryEvents))
	}
}

func TestRunDecayCheck_SkipsWithinGrace(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	snap := masterySnap(map[string]*store.SkillMasteryData{
		"skill-a": {SkillID: "skill-a", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
	})
	svc := mastery.NewService(snap, nil)

	// Stage 2 (7-day interval), 2 days overdue -> grace is 3.5 days -> not rusty
	reviewDate := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	reviews := map[string]*ReviewState{
		"skill-a": {
			SkillID:        "skill-a",
			Stage:          2,
			NextReviewDate: reviewDate,
			LastReviewDate: time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC),
		},
	}
	sched := newTestScheduler(reviews, svc, nil)

	now := reviewDate.Add(2 * 24 * time.Hour)
	transitions := sched.RunDecayCheck(context.Background(), now)

	if len(transitions) != 0 {
		t.Errorf("expected 0 transitions within grace, got %d", len(transitions))
	}
}

func TestRunDecayCheck_SkipsAlreadyRusty(t *testing.T) {
	rustyStr := time.Date(2025, 1, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	snap := masterySnap(map[string]*store.SkillMasteryData{
		"skill-a": {SkillID: "skill-a", State: "rusty", CurrentTier: "learn", RustyAt: &rustyStr},
	})
	svc := mastery.NewService(snap, nil)

	reviewDate := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	reviews := map[string]*ReviewState{
		"skill-a": {
			SkillID:        "skill-a",
			Stage:          0,
			NextReviewDate: reviewDate,
		},
	}
	sched := newTestScheduler(reviews, svc, nil)

	now := reviewDate.Add(10 * 24 * time.Hour)
	transitions := sched.RunDecayCheck(context.Background(), now)

	if len(transitions) != 0 {
		t.Errorf("expected 0 transitions for already rusty, got %d", len(transitions))
	}
}

func TestRunDecayCheck_SkipsLearning(t *testing.T) {
	snap := masterySnap(map[string]*store.SkillMasteryData{
		"skill-a": {SkillID: "skill-a", State: "learning", CurrentTier: "learn"},
	})
	svc := mastery.NewService(snap, nil)

	reviewDate := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	reviews := map[string]*ReviewState{
		"skill-a": {
			SkillID:        "skill-a",
			Stage:          0,
			NextReviewDate: reviewDate,
		},
	}
	sched := newTestScheduler(reviews, svc, nil)

	now := reviewDate.Add(10 * 24 * time.Hour)
	transitions := sched.RunDecayCheck(context.Background(), now)

	if len(transitions) != 0 {
		t.Errorf("expected 0 transitions for learning skill, got %d", len(transitions))
	}
}

func TestDueSkills_SortedMostOverdueFirst(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	snap := masterySnap(map[string]*store.SkillMasteryData{
		"skill-a": {SkillID: "skill-a", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
		"skill-b": {SkillID: "skill-b", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
		"skill-c": {SkillID: "skill-c", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
	})
	svc := mastery.NewService(snap, nil)

	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	reviews := map[string]*ReviewState{
		"skill-a": {SkillID: "skill-a", Stage: 1, NextReviewDate: now.Add(-2 * 24 * time.Hour)},  // 2 days overdue
		"skill-b": {SkillID: "skill-b", Stage: 1, NextReviewDate: now.Add(-5 * 24 * time.Hour)},  // 5 days overdue
		"skill-c": {SkillID: "skill-c", Stage: 1, NextReviewDate: now.Add(-10 * 24 * time.Hour)}, // 10 days overdue
	}
	sched := newTestScheduler(reviews, svc, nil)

	due := sched.DueSkills(now)
	if len(due) != 3 {
		t.Fatalf("expected 3 due skills, got %d", len(due))
	}
	if due[0] != "skill-c" || due[1] != "skill-b" || due[2] != "skill-a" {
		t.Errorf("unexpected order: %v", due)
	}
}

func TestDueSkills_ExcludesNotDue(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	snap := masterySnap(map[string]*store.SkillMasteryData{
		"skill-a": {SkillID: "skill-a", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
		"skill-b": {SkillID: "skill-b", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
	})
	svc := mastery.NewService(snap, nil)

	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	reviews := map[string]*ReviewState{
		"skill-a": {SkillID: "skill-a", Stage: 1, NextReviewDate: now.Add(-1 * 24 * time.Hour)}, // due
		"skill-b": {SkillID: "skill-b", Stage: 1, NextReviewDate: now.Add(5 * 24 * time.Hour)},  // not due
	}
	sched := newTestScheduler(reviews, svc, nil)

	due := sched.DueSkills(now)
	if len(due) != 1 {
		t.Fatalf("expected 1 due skill, got %d", len(due))
	}
	if due[0] != "skill-a" {
		t.Errorf("expected skill-a, got %s", due[0])
	}
}

func TestDueSkills_ExcludesRusty(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	rustyStr := time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	snap := masterySnap(map[string]*store.SkillMasteryData{
		"skill-a": {SkillID: "skill-a", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
		"skill-b": {SkillID: "skill-b", State: "rusty", CurrentTier: "learn", RustyAt: &rustyStr},
	})
	svc := mastery.NewService(snap, nil)

	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	reviews := map[string]*ReviewState{
		"skill-a": {SkillID: "skill-a", Stage: 1, NextReviewDate: now.Add(-1 * 24 * time.Hour)},
		"skill-b": {SkillID: "skill-b", Stage: 1, NextReviewDate: now.Add(-5 * 24 * time.Hour)},
	}
	sched := newTestScheduler(reviews, svc, nil)

	due := sched.DueSkills(now)
	if len(due) != 1 {
		t.Fatalf("expected 1 due skill (rusty excluded), got %d", len(due))
	}
	if due[0] != "skill-a" {
		t.Errorf("expected skill-a, got %s", due[0])
	}
}

func TestGraduation_After6Reviews(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	now := masteredAt
	for i := 0; i < 6; i++ {
		now = now.AddDate(0, 0, BaseIntervals[min(i, MaxStage)])
		sched.RecordReview("skill-a", true, now)
	}

	rs := sched.GetReviewState("skill-a")
	if !rs.Graduated {
		t.Error("expected graduated after 6 reviews")
	}
	if rs.ConsecutiveHits != 6 {
		t.Errorf("ConsecutiveHits = %d, want 6", rs.ConsecutiveHits)
	}
}

func TestGraduation_ResetOnWrongAnswer(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	now := masteredAt
	// 4 correct
	for i := 0; i < 4; i++ {
		now = now.AddDate(0, 0, BaseIntervals[min(i, MaxStage)])
		sched.RecordReview("skill-a", true, now)
	}
	// 1 wrong
	now = now.AddDate(0, 0, 30)
	sched.RecordReview("skill-a", false, now)

	rs := sched.GetReviewState("skill-a")
	if rs.ConsecutiveHits != 0 {
		t.Errorf("ConsecutiveHits = %d, want 0 after wrong answer", rs.ConsecutiveHits)
	}
	if rs.Graduated {
		t.Error("should not be graduated")
	}

	// 6 more correct (should graduate)
	for i := 0; i < 6; i++ {
		now = now.AddDate(0, 0, BaseIntervals[min(i, MaxStage)])
		sched.RecordReview("skill-a", true, now)
	}

	rs = sched.GetReviewState("skill-a")
	if !rs.Graduated {
		t.Error("expected graduated after 6 consecutive correct")
	}
}

func TestGraduation_IntervalIs90Days(t *testing.T) {
	rs := &ReviewState{Stage: 6, Graduated: true}
	got := rs.CurrentIntervalDays()
	if got != 90 {
		t.Errorf("CurrentIntervalDays() = %d, want 90 for graduated", got)
	}
}

func TestGraduation_LostOnRecovery(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	masteredAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", masteredAt)

	now := masteredAt
	for i := 0; i < 6; i++ {
		now = now.AddDate(0, 0, BaseIntervals[min(i, MaxStage)])
		sched.RecordReview("skill-a", true, now)
	}

	if !sched.GetReviewState("skill-a").Graduated {
		t.Fatal("should be graduated before reinit")
	}

	now = now.AddDate(0, 0, 200)
	sched.ReInitSkill("skill-a", now)

	rs := sched.GetReviewState("skill-a")
	if rs.Graduated {
		t.Error("should not be graduated after reinit")
	}
	if rs.Stage != 0 {
		t.Errorf("Stage = %d, want 0", rs.Stage)
	}
}

func TestRecordReview_NilReviewState(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	// Should not panic
	sched.RecordReview("nonexistent", true, time.Now())
}

func TestAllReviewStates(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", now)
	sched.InitSkill("skill-b", now)

	states := sched.AllReviewStates()
	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
