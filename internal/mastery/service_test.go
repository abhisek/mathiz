package mastery

import (
	"context"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// mockEventRepo implements store.EventRepo for testing.
type mockEventRepo struct {
	reviewAccuracy float64
	reviewCount    int
	reviewErr      error
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
func (m *mockEventRepo) AppendMasteryEvent(_ context.Context, _ store.MasteryEventData) error {
	return nil
}
func (m *mockEventRepo) LatestAnswerTime(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (m *mockEventRepo) SkillAccuracy(_ context.Context, _ string) (float64, error) {
	return 0, nil
}
func (m *mockEventRepo) RecentReviewAccuracy(_ context.Context, _ string, _ int) (float64, int, error) {
	return m.reviewAccuracy, m.reviewCount, m.reviewErr
}
func (m *mockEventRepo) AppendDiagnosisEvent(_ context.Context, _ store.DiagnosisEventData) error {
	return nil
}

func testSkillID() string {
	skills := skillgraph.AllSkills()
	if len(skills) == 0 {
		panic("no skills in graph")
	}
	return skills[0].ID
}

func learnTierCfg() skillgraph.TierConfig {
	return skillgraph.DefaultTiers()[0]
}

func proveTierCfg() skillgraph.TierConfig {
	return skillgraph.DefaultTiers()[1]
}

func TestService_NewService_EmptySnapshot(t *testing.T) {
	svc := NewService(nil, nil)
	skillID := testSkillID()
	sm := svc.GetMastery(skillID)
	if sm.State != StateNew {
		t.Errorf("State = %s, want new", sm.State)
	}
}

func TestService_NewService_WithExistingData(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"test-skill": {
					SkillID:     "test-skill",
					State:       "mastered",
					CurrentTier: "prove",
					SpeedWindow: 10,
					StreakCap:    8,
				},
			},
		},
	}
	svc := NewService(snap, nil)
	sm := svc.GetMastery("test-skill")
	if sm.State != StateMastered {
		t.Errorf("State = %s, want mastered", sm.State)
	}
}

func TestService_MasteredSkills(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {State: "mastered", SpeedWindow: 10, StreakCap: 8},
				"s2": {State: "mastered", SpeedWindow: 10, StreakCap: 8},
				"s3": {State: "learning", SpeedWindow: 10, StreakCap: 8},
			},
		},
	}
	svc := NewService(snap, nil)
	mastered := svc.MasteredSkills()
	if len(mastered) != 2 {
		t.Errorf("MasteredSkills count = %d, want 2", len(mastered))
	}
}

func TestStateMachine_NewToLearning(t *testing.T) {
	svc := NewService(nil, nil)
	skillID := testSkillID()
	cfg := learnTierCfg()

	transition := svc.RecordAnswer(skillID, true, 5000, cfg)
	if transition == nil {
		t.Fatal("expected a transition")
	}
	if transition.From != StateNew {
		t.Errorf("From = %s, want new", transition.From)
	}
	if transition.To != StateLearning {
		t.Errorf("To = %s, want learning", transition.To)
	}
	if transition.Trigger != "first-attempt" {
		t.Errorf("Trigger = %s, want first-attempt", transition.Trigger)
	}
}

func TestStateMachine_LearningToMastered(t *testing.T) {
	svc := NewService(nil, nil)
	skillID := testSkillID()
	cfg := learnTierCfg()

	// Complete learn tier: 8 correct answers.
	for i := 0; i < cfg.ProblemsRequired; i++ {
		svc.RecordAnswer(skillID, true, 5000, cfg)
	}

	sm := svc.GetMastery(skillID)
	if sm.CurrentTier != skillgraph.TierProve {
		t.Fatalf("CurrentTier = %d, want Prove", sm.CurrentTier)
	}

	// Complete prove tier.
	proveCfg := proveTierCfg()
	var lastTransition *StateTransition
	for i := 0; i < proveCfg.ProblemsRequired+2; i++ {
		t := svc.RecordAnswer(skillID, true, 10000, proveCfg)
		if t != nil {
			lastTransition = t
		}
	}

	if lastTransition == nil {
		t.Fatal("expected mastery transition")
	}
	if lastTransition.From != StateLearning {
		t.Errorf("From = %s, want learning", lastTransition.From)
	}
	if lastTransition.To != StateMastered {
		t.Errorf("To = %s, want mastered", lastTransition.To)
	}
	if lastTransition.Trigger != "prove-complete" {
		t.Errorf("Trigger = %s, want prove-complete", lastTransition.Trigger)
	}
	if sm.MasteredAt == nil {
		t.Error("expected MasteredAt to be set")
	}
}

func TestStateMachine_TierComplete(t *testing.T) {
	svc := NewService(nil, nil)
	skillID := testSkillID()
	cfg := learnTierCfg()

	var tierCompleteTransition *StateTransition
	for i := 0; i < cfg.ProblemsRequired; i++ {
		tr := svc.RecordAnswer(skillID, true, 5000, cfg)
		if tr != nil && tr.Trigger == "tier-complete" {
			tierCompleteTransition = tr
		}
	}

	if tierCompleteTransition == nil {
		t.Fatal("expected tier-complete transition")
	}
	if tierCompleteTransition.From != StateLearning {
		t.Errorf("From = %s, want learning", tierCompleteTransition.From)
	}
	if tierCompleteTransition.To != StateLearning {
		t.Errorf("To = %s, want learning", tierCompleteTransition.To)
	}
}

func TestStateMachine_MasteredToRusty_TimeTrigger(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {State: "mastered", CurrentTier: "prove", SpeedWindow: 10, StreakCap: 8},
			},
		},
	}
	svc := NewService(snap, nil)

	transition := svc.MarkRusty("s1")
	if transition == nil {
		t.Fatal("expected transition")
	}
	if transition.From != StateMastered {
		t.Errorf("From = %s, want mastered", transition.From)
	}
	if transition.To != StateRusty {
		t.Errorf("To = %s, want rusty", transition.To)
	}
	if transition.Trigger != "time-decay" {
		t.Errorf("Trigger = %s, want time-decay", transition.Trigger)
	}
	sm := svc.GetMastery("s1")
	if sm.RustyAt == nil {
		t.Error("expected RustyAt to be set")
	}
}

func TestStateMachine_MasteredToRusty_ReviewPerformance(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {State: "mastered", CurrentTier: "prove", SpeedWindow: 10, StreakCap: 8},
			},
		},
	}
	repo := &mockEventRepo{
		reviewAccuracy: 0.25, // 1/4 correct
		reviewCount:    4,
	}
	svc := NewService(snap, repo)

	transition := svc.CheckReviewPerformance(context.Background(), "s1")
	if transition == nil {
		t.Fatal("expected transition")
	}
	if transition.From != StateMastered {
		t.Errorf("From = %s, want mastered", transition.From)
	}
	if transition.To != StateRusty {
		t.Errorf("To = %s, want rusty", transition.To)
	}
	if transition.Trigger != "review-performance" {
		t.Errorf("Trigger = %s, want review-performance", transition.Trigger)
	}
}

func TestStateMachine_NotRusty_GoodReview(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {State: "mastered", CurrentTier: "prove", SpeedWindow: 10, StreakCap: 8},
			},
		},
	}
	repo := &mockEventRepo{
		reviewAccuracy: 0.75, // 3/4 correct
		reviewCount:    4,
	}
	svc := NewService(snap, repo)

	transition := svc.CheckReviewPerformance(context.Background(), "s1")
	if transition != nil {
		t.Error("expected no transition for good review")
	}
}

func TestStateMachine_NotRusty_TooFewReviews(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {State: "mastered", CurrentTier: "prove", SpeedWindow: 10, StreakCap: 8},
			},
		},
	}
	repo := &mockEventRepo{
		reviewAccuracy: 0.0,
		reviewCount:    2, // Not enough
	}
	svc := NewService(snap, repo)

	transition := svc.CheckReviewPerformance(context.Background(), "s1")
	if transition != nil {
		t.Error("expected no transition with < 4 reviews")
	}
}

func TestStateMachine_RustyToMastered_Recovery(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {State: "rusty", CurrentTier: "learn", SpeedWindow: 10, StreakCap: 8},
			},
		},
	}
	svc := NewService(snap, nil)

	recoveryCfg := RecoveryTierConfig()

	// Answer 4 recovery questions: 3 correct, 1 wrong.
	svc.RecordAnswer("s1", true, 5000, recoveryCfg)
	svc.RecordAnswer("s1", true, 5000, recoveryCfg)
	svc.RecordAnswer("s1", false, 5000, recoveryCfg)
	transition := svc.RecordAnswer("s1", true, 5000, recoveryCfg)

	if transition == nil {
		t.Fatal("expected recovery transition")
	}
	if transition.From != StateRusty {
		t.Errorf("From = %s, want rusty", transition.From)
	}
	if transition.To != StateMastered {
		t.Errorf("To = %s, want mastered", transition.To)
	}
	if transition.Trigger != "recovery-complete" {
		t.Errorf("Trigger = %s, want recovery-complete", transition.Trigger)
	}
	sm := svc.GetMastery("s1")
	if sm.RustyAt != nil {
		t.Error("expected RustyAt to be cleared")
	}
}

func TestStateMachine_RustyRecoveryFails(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {State: "rusty", CurrentTier: "learn", SpeedWindow: 10, StreakCap: 8},
			},
		},
	}
	svc := NewService(snap, nil)

	recoveryCfg := RecoveryTierConfig()

	// Answer 4 recovery questions: 1 correct, 3 wrong.
	svc.RecordAnswer("s1", true, 5000, recoveryCfg)
	svc.RecordAnswer("s1", false, 5000, recoveryCfg)
	svc.RecordAnswer("s1", false, 5000, recoveryCfg)
	transition := svc.RecordAnswer("s1", false, 5000, recoveryCfg)

	if transition != nil {
		t.Error("expected no transition (recovery failed)")
	}
	sm := svc.GetMastery("s1")
	if sm.State != StateRusty {
		t.Errorf("State = %s, want rusty", sm.State)
	}
}

func TestStateMachine_MarkRusty_NotMastered(t *testing.T) {
	svc := NewService(nil, nil)
	skillID := testSkillID()

	// Make skill Learning.
	svc.RecordAnswer(skillID, true, 5000, learnTierCfg())

	transition := svc.MarkRusty(skillID)
	if transition != nil {
		t.Error("expected nil transition for non-mastered skill")
	}
}

func TestService_RecordAnswer_UpdatesFluency(t *testing.T) {
	svc := NewService(nil, nil)
	skillID := testSkillID()
	cfg := learnTierCfg()

	svc.RecordAnswer(skillID, true, 5000, cfg)
	sm := svc.GetMastery(skillID)

	if sm.Fluency.Streak != 1 {
		t.Errorf("Streak = %d, want 1", sm.Fluency.Streak)
	}
	if len(sm.Fluency.SpeedScores) != 1 {
		t.Errorf("SpeedScores length = %d, want 1", len(sm.Fluency.SpeedScores))
	}

	// Wrong answer resets streak.
	svc.RecordAnswer(skillID, false, 5000, cfg)
	if sm.Fluency.Streak != 0 {
		t.Errorf("Streak after wrong = %d, want 0", sm.Fluency.Streak)
	}
}

func TestService_SnapshotRoundTrip(t *testing.T) {
	svc := NewService(nil, nil)
	skillID := testSkillID()
	cfg := learnTierCfg()

	// Build some state.
	for i := 0; i < 5; i++ {
		svc.RecordAnswer(skillID, true, 5000, cfg)
	}

	// Snapshot.
	snapData := svc.SnapshotData()

	// Recreate service from snapshot.
	svc2 := NewService(&store.SnapshotData{Mastery: snapData}, nil)
	sm := svc2.GetMastery(skillID)

	if sm.State != StateLearning {
		t.Errorf("State = %s, want learning", sm.State)
	}
	if sm.TotalAttempts != 5 {
		t.Errorf("TotalAttempts = %d, want 5", sm.TotalAttempts)
	}
	if sm.CorrectCount != 5 {
		t.Errorf("CorrectCount = %d, want 5", sm.CorrectCount)
	}
	if sm.Fluency.Streak != 5 {
		t.Errorf("Streak = %d, want 5", sm.Fluency.Streak)
	}
	if len(sm.Fluency.SpeedScores) != 5 {
		t.Errorf("SpeedScores = %d, want 5", len(sm.Fluency.SpeedScores))
	}
}

func TestStateMachine_RecoveryCumulative(t *testing.T) {
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"s1": {
					State:         "rusty",
					CurrentTier:   "learn",
					TotalAttempts: 2,
					CorrectCount:  2,
					SpeedWindow:   10,
					StreakCap:     8,
				},
			},
		},
	}
	svc := NewService(snap, nil)

	recoveryCfg := RecoveryTierConfig()

	// 2 more answers (total 4: 3 correct, 1 wrong = 75%).
	svc.RecordAnswer("s1", false, 5000, recoveryCfg)
	transition := svc.RecordAnswer("s1", true, 5000, recoveryCfg)

	if transition == nil {
		t.Fatal("expected recovery transition")
	}
	if transition.Trigger != "recovery-complete" {
		t.Errorf("Trigger = %s, want recovery-complete", transition.Trigger)
	}
}
