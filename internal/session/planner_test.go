package session

import (
	"context"
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// mockEventRepo implements store.EventRepo for testing.
type mockEventRepo struct {
	answerTimes map[string]time.Time
	accuracies  map[string]float64
}

func newMockEventRepo() *mockEventRepo {
	return &mockEventRepo{
		answerTimes: make(map[string]time.Time),
		accuracies:  make(map[string]float64),
	}
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
func (m *mockEventRepo) LatestAnswerTime(_ context.Context, skillID string) (time.Time, error) {
	return m.answerTimes[skillID], nil
}
func (m *mockEventRepo) SkillAccuracy(_ context.Context, skillID string) (float64, error) {
	return m.accuracies[skillID], nil
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
func (m *mockEventRepo) QueryLLMEvents(_ context.Context, _ store.QueryOpts) ([]store.LLMRequestEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) GetLLMEvent(_ context.Context, _ int) (*store.LLMRequestEventRecord, error) {
	return nil, nil
}
func (m *mockEventRepo) LLMUsageByPurpose(_ context.Context) ([]store.LLMUsageStats, error) {
	return nil, nil
}
func (m *mockEventRepo) LLMUsageByModel(_ context.Context) ([]store.LLMModelUsage, error) {
	return nil, nil
}

func TestBuildPlan_AllFrontier(t *testing.T) {
	repo := newMockEventRepo()
	planner := NewPlanner(context.Background(), repo)

	// No mastered skills → all slots are frontier.
	plan, err := planner.BuildPlan(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Duration != DefaultSessionDuration {
		t.Errorf("Duration = %v, want %v", plan.Duration, DefaultSessionDuration)
	}

	if len(plan.Slots) != DefaultTotalSlots {
		t.Errorf("Slots count = %d, want %d", len(plan.Slots), DefaultTotalSlots)
	}

	for _, slot := range plan.Slots {
		if slot.Category != CategoryFrontier {
			t.Errorf("expected all frontier, got %s", slot.Category)
		}
	}
}

func TestBuildPlan_MixedPlan(t *testing.T) {
	repo := newMockEventRepo()
	planner := NewPlanner(context.Background(), repo)

	// Master the root skill and its immediate dependents to get 3 mastered.
	roots := skillgraph.RootSkills()
	mastered := make(map[string]bool)
	mastered[roots[0].ID] = true
	repo.answerTimes[roots[0].ID] = time.Now().Add(-3 * time.Hour)
	repo.accuracies[roots[0].ID] = 0.9

	// Also master some skills that depend on the root.
	deps := skillgraph.Dependents(roots[0].ID)
	for i := 0; i < 2 && i < len(deps); i++ {
		mastered[deps[i].ID] = true
		repo.answerTimes[deps[i].ID] = time.Now().Add(-time.Duration(i+1) * time.Hour)
		repo.accuracies[deps[i].ID] = 0.85
	}

	plan, err := planner.BuildPlan(mastered, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var frontier, review, booster int
	for _, slot := range plan.Slots {
		switch slot.Category {
		case CategoryFrontier:
			frontier++
		case CategoryReview:
			review++
		case CategoryBooster:
			booster++
		}
	}

	// Expect 3 frontier, 1 review, 1 booster.
	if frontier != 3 {
		t.Errorf("frontier count = %d, want 3", frontier)
	}
	if review != 1 {
		t.Errorf("review count = %d, want 1", review)
	}
	if booster != 1 {
		t.Errorf("booster count = %d, want 1", booster)
	}
}

func TestBuildPlan_FrontierPriority(t *testing.T) {
	repo := newMockEventRepo()
	planner := NewPlanner(context.Background(), repo)

	plan, err := planner.BuildPlan(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Slots) == 0 {
		t.Fatal("expected non-empty plan")
	}

	// First slot should be a low-grade skill.
	first := plan.Slots[0]
	if first.Skill.GradeLevel > 3 {
		t.Errorf("first slot grade = %d, expected grade 3 (lowest first)", first.Skill.GradeLevel)
	}
}

func TestBuildPlan_ReviewSelection(t *testing.T) {
	repo := newMockEventRepo()
	planner := NewPlanner(context.Background(), repo)

	roots := skillgraph.RootSkills()
	mastered := make(map[string]bool)

	// Master root + dependents with different last-practiced times.
	now := time.Now()
	mastered[roots[0].ID] = true
	repo.answerTimes[roots[0].ID] = now.Add(-3 * 24 * time.Hour) // oldest
	repo.accuracies[roots[0].ID] = 0.85

	deps := skillgraph.Dependents(roots[0].ID)
	for i := 0; i < 2 && i < len(deps); i++ {
		mastered[deps[i].ID] = true
		repo.answerTimes[deps[i].ID] = now.Add(-time.Duration(i+1) * time.Hour) // more recent
		repo.accuracies[deps[i].ID] = 0.85
	}

	plan, err := planner.BuildPlan(mastered, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the review slot — should be root (least recently practiced).
	for _, slot := range plan.Slots {
		if slot.Category == CategoryReview {
			if slot.Skill.ID != roots[0].ID {
				t.Errorf("review skill = %s, want %s (least recently practiced)", slot.Skill.ID, roots[0].ID)
			}
			break
		}
	}
}

func TestBuildPlan_BoosterSelection(t *testing.T) {
	repo := newMockEventRepo()
	planner := NewPlanner(context.Background(), repo)

	roots := skillgraph.RootSkills()
	mastered := make(map[string]bool)

	// Master root + dependents with different accuracies.
	mastered[roots[0].ID] = true
	repo.answerTimes[roots[0].ID] = time.Now()
	repo.accuracies[roots[0].ID] = 0.7

	deps := skillgraph.Dependents(roots[0].ID)
	if len(deps) >= 2 {
		mastered[deps[0].ID] = true
		repo.answerTimes[deps[0].ID] = time.Now()
		repo.accuracies[deps[0].ID] = 0.95 // highest

		mastered[deps[1].ID] = true
		repo.answerTimes[deps[1].ID] = time.Now()
		repo.accuracies[deps[1].ID] = 0.8
	}

	plan, err := planner.BuildPlan(mastered, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the booster slot — should be highest accuracy.
	found := false
	for _, slot := range plan.Slots {
		if slot.Category == CategoryBooster {
			found = true
			// Booster should always be Learn tier.
			if slot.Tier != skillgraph.TierLearn {
				t.Errorf("booster tier = %d, want TierLearn", slot.Tier)
			}
			if len(deps) >= 2 && slot.Skill.ID != deps[0].ID {
				t.Errorf("booster skill = %s, want %s (highest accuracy)", slot.Skill.ID, deps[0].ID)
			}
			break
		}
	}
	if !found && len(mastered) >= 3 {
		t.Error("expected a booster slot")
	}
}

func TestBuildPlan_Duration(t *testing.T) {
	repo := newMockEventRepo()
	planner := NewPlanner(context.Background(), repo)

	plan, err := planner.BuildPlan(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Duration != 15*time.Minute {
		t.Errorf("Duration = %v, want 15m", plan.Duration)
	}
}
