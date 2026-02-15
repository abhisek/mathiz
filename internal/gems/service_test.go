package gems

import (
	"context"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/store"
)

// mockEventRepo implements store.EventRepo for gems tests.
type mockEventRepo struct {
	gemEvents []store.GemEventData
	counts    map[string]int
	total     int
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
func (m *mockEventRepo) LatestAnswerTime(_ context.Context, _ string) (time.Time, error) {
	return time.Time{}, nil
}
func (m *mockEventRepo) SkillAccuracy(_ context.Context, _ string) (float64, error) {
	return 0, nil
}
func (m *mockEventRepo) AppendMasteryEvent(_ context.Context, _ store.MasteryEventData) error {
	return nil
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
func (m *mockEventRepo) AppendGemEvent(_ context.Context, data store.GemEventData) error {
	m.gemEvents = append(m.gemEvents, data)
	return nil
}
func (m *mockEventRepo) QueryGemEvents(_ context.Context, _ store.QueryOpts) ([]store.GemEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) GemCounts(_ context.Context) (map[string]int, int, error) {
	return m.counts, m.total, nil
}
func (m *mockEventRepo) QuerySessionSummaries(_ context.Context, _ store.QueryOpts) ([]store.SessionSummaryRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) QueryLLMEvents(_ context.Context, _ store.QueryOpts) ([]store.LLMRequestEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) GetLLMEvent(_ context.Context, _ int) (*store.LLMRequestEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) LLMUsageByPurpose(_ context.Context) ([]store.LLMUsageStats, error) {
	return nil, nil
}

func newTestService() (*Service, *mockEventRepo) {
	repo := &mockEventRepo{
		counts: map[string]int{"mastery": 3, "streak": 2},
		total:  5,
	}
	svc := NewService(repo)
	return svc, repo
}

func TestAwardMastery(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	award := svc.AwardMastery(ctx, "add-3", "Addition (Grade 3)", "sess-1")

	if award.Type != GemMastery {
		t.Errorf("Type = %q, want %q", award.Type, GemMastery)
	}
	if award.SkillID != "add-3" {
		t.Errorf("SkillID = %q, want %q", award.SkillID, "add-3")
	}
	if award.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", award.SessionID, "sess-1")
	}
	if len(repo.gemEvents) != 1 {
		t.Fatalf("persisted %d events, want 1", len(repo.gemEvents))
	}
	if repo.gemEvents[0].GemType != "mastery" {
		t.Errorf("persisted type = %q, want %q", repo.gemEvents[0].GemType, "mastery")
	}
	if repo.gemEvents[0].SkillID == nil || *repo.gemEvents[0].SkillID != "add-3" {
		t.Error("persisted event missing skill_id")
	}
	if len(svc.SessionGems) != 1 {
		t.Errorf("SessionGems = %d, want 1", len(svc.SessionGems))
	}
}

func TestAwardRecovery(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	award := svc.AwardRecovery(ctx, "sub-3", "Subtraction (Grade 3)", "sess-2")

	if award.Type != GemRecovery {
		t.Errorf("Type = %q, want %q", award.Type, GemRecovery)
	}
	if len(repo.gemEvents) != 1 {
		t.Fatalf("persisted %d events, want 1", len(repo.gemEvents))
	}
	if repo.gemEvents[0].Rarity != string(award.Rarity) {
		t.Errorf("persisted rarity = %q, want %q", repo.gemEvents[0].Rarity, award.Rarity)
	}
}

func TestAwardRetention(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	award := svc.AwardRetention(ctx, "mul-3", "Multiplication (Grade 3)", "sess-3")

	if award.Type != GemRetention {
		t.Errorf("Type = %q, want %q", award.Type, GemRetention)
	}
	if len(repo.gemEvents) != 1 {
		t.Fatalf("persisted %d events, want 1", len(repo.gemEvents))
	}
}

func TestAwardStreak(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	award := svc.AwardStreak(ctx, 10, "sess-4")

	if award.Type != GemStreak {
		t.Errorf("Type = %q, want %q", award.Type, GemStreak)
	}
	if award.Rarity != RarityRare {
		t.Errorf("Rarity = %q, want %q", award.Rarity, RarityRare)
	}
	if award.SkillID != "" {
		t.Errorf("SkillID = %q, want empty for streak gem", award.SkillID)
	}
	if len(repo.gemEvents) != 1 {
		t.Fatalf("persisted %d events, want 1", len(repo.gemEvents))
	}
	if repo.gemEvents[0].SkillID != nil {
		t.Error("persisted streak gem should have nil skill_id")
	}
}

func TestAwardSession(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	award := svc.AwardSession(ctx, 0.85, "sess-5")

	if award.Type != GemSession {
		t.Errorf("Type = %q, want %q", award.Type, GemSession)
	}
	if award.Rarity != RarityEpic {
		t.Errorf("Rarity = %q, want %q (85%% accuracy)", award.Rarity, RarityEpic)
	}
	if len(repo.gemEvents) != 1 {
		t.Fatalf("persisted %d events, want 1", len(repo.gemEvents))
	}
}

func TestResetSession(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AwardStreak(ctx, 5, "sess-1")
	svc.AwardStreak(ctx, 10, "sess-1")
	if len(svc.SessionGems) != 2 {
		t.Fatalf("SessionGems = %d, want 2", len(svc.SessionGems))
	}

	svc.ResetSession()
	if svc.SessionGems != nil {
		t.Errorf("SessionGems after reset = %v, want nil", svc.SessionGems)
	}
}

func TestSnapshotData(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	snap := svc.SnapshotData(ctx)
	if snap.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5", snap.TotalCount)
	}
	if snap.CountByType["mastery"] != 3 {
		t.Errorf("CountByType[mastery] = %d, want 3", snap.CountByType["mastery"])
	}
	if snap.CountByType["streak"] != 2 {
		t.Errorf("CountByType[streak] = %d, want 2", snap.CountByType["streak"])
	}
}

func TestPersist_NilEventRepo(t *testing.T) {
	svc := &Service{
		depthMap: ComputeDepthMap(),
	}
	ctx := context.Background()

	// Should not panic with nil eventRepo.
	award := svc.AwardStreak(ctx, 5, "sess-1")
	if award == nil {
		t.Error("expected non-nil award even with nil eventRepo")
	}
	if len(svc.SessionGems) != 1 {
		t.Errorf("SessionGems = %d, want 1", len(svc.SessionGems))
	}
}

func TestMultipleAwards_SessionAccumulation(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AwardMastery(ctx, "add-3", "Addition", "sess-1")
	svc.AwardStreak(ctx, 5, "sess-1")
	svc.AwardSession(ctx, 0.95, "sess-1")

	if len(svc.SessionGems) != 3 {
		t.Errorf("SessionGems = %d, want 3", len(svc.SessionGems))
	}

	// Verify types.
	types := map[GemType]bool{}
	for _, g := range svc.SessionGems {
		types[g.Type] = true
	}
	for _, expected := range []GemType{GemMastery, GemStreak, GemSession} {
		if !types[expected] {
			t.Errorf("missing gem type %q in session gems", expected)
		}
	}
}
