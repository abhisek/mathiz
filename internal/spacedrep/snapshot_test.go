package spacedrep

import (
	"testing"
	"time"

	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/store"
)

func TestSnapshotRoundTrip(t *testing.T) {
	snap := masterySnap(nil)
	svc := mastery.NewService(snap, nil)
	sched := newTestScheduler(nil, svc, nil)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	sched.InitSkill("skill-a", now)
	sched.InitSkill("skill-b", now)

	// Advance skill-a
	sched.RecordReview("skill-a", true, now.AddDate(0, 0, 1))

	// Export snapshot
	snapData := sched.SnapshotData()
	if len(snapData.Reviews) != 2 {
		t.Fatalf("expected 2 reviews in snapshot, got %d", len(snapData.Reviews))
	}

	// Reconstruct from snapshot
	fullSnap := &store.SnapshotData{
		SpacedRep: snapData,
	}
	sched2 := NewScheduler(fullSnap, svc, nil)

	rsA := sched2.GetReviewState("skill-a")
	if rsA == nil {
		t.Fatal("expected skill-a in restored scheduler")
	}
	if rsA.Stage != 1 {
		t.Errorf("skill-a Stage = %d, want 1", rsA.Stage)
	}
	if rsA.ConsecutiveHits != 1 {
		t.Errorf("skill-a ConsecutiveHits = %d, want 1", rsA.ConsecutiveHits)
	}

	rsB := sched2.GetReviewState("skill-b")
	if rsB == nil {
		t.Fatal("expected skill-b in restored scheduler")
	}
	if rsB.Stage != 0 {
		t.Errorf("skill-b Stage = %d, want 0", rsB.Stage)
	}
}

func TestBootstrapFromMastery_CreateReviewStates(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	masteryData := &store.MasterySnapshotData{
		Skills: map[string]*store.SkillMasteryData{
			"skill-a": {SkillID: "skill-a", State: "mastered", MasteredAt: &masteredStr},
			"skill-b": {SkillID: "skill-b", State: "mastered", MasteredAt: &masteredStr},
			"skill-c": {SkillID: "skill-c", State: "mastered", MasteredAt: &masteredStr},
		},
	}

	data := BootstrapFromMastery(masteryData)
	if len(data.Reviews) != 3 {
		t.Errorf("expected 3 review states, got %d", len(data.Reviews))
	}

	for _, rd := range data.Reviews {
		if rd.Stage != 0 {
			t.Errorf("expected stage 0, got %d", rd.Stage)
		}
		if rd.Graduated {
			t.Error("expected not graduated")
		}
	}
}

func TestBootstrapFromMastery_SkipsNonMastered(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	masteryData := &store.MasterySnapshotData{
		Skills: map[string]*store.SkillMasteryData{
			"skill-a": {SkillID: "skill-a", State: "mastered", MasteredAt: &masteredStr},
			"skill-b": {SkillID: "skill-b", State: "learning"},
			"skill-c": {SkillID: "skill-c", State: "rusty"},
		},
	}

	data := BootstrapFromMastery(masteryData)
	if len(data.Reviews) != 1 {
		t.Errorf("expected 1 review state (only mastered), got %d", len(data.Reviews))
	}
	if _, ok := data.Reviews["skill-a"]; !ok {
		t.Error("expected skill-a in bootstrapped data")
	}
}

func TestBootstrapFromMastery_UsesCorrectDates(t *testing.T) {
	masteredAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	masteredStr := masteredAt.Format(time.RFC3339)
	masteryData := &store.MasterySnapshotData{
		Skills: map[string]*store.SkillMasteryData{
			"skill-a": {SkillID: "skill-a", State: "mastered", MasteredAt: &masteredStr},
		},
	}

	data := BootstrapFromMastery(masteryData)
	rd := data.Reviews["skill-a"]
	if rd == nil {
		t.Fatal("expected skill-a")
	}

	expectedNext := masteredAt.AddDate(0, 0, 1).Format(time.RFC3339)
	if rd.NextReviewDate != expectedNext {
		t.Errorf("NextReviewDate = %s, want %s", rd.NextReviewDate, expectedNext)
	}
	if rd.LastReviewDate != masteredStr {
		t.Errorf("LastReviewDate = %s, want %s", rd.LastReviewDate, masteredStr)
	}
}

func TestBootstrapFromMastery_NilInput(t *testing.T) {
	data := BootstrapFromMastery(nil)
	if len(data.Reviews) != 0 {
		t.Errorf("expected 0 reviews for nil input, got %d", len(data.Reviews))
	}
}

func TestNewScheduler_MigrationPath(t *testing.T) {
	masteredStr := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	snap := &store.SnapshotData{
		Mastery: &store.MasterySnapshotData{
			Skills: map[string]*store.SkillMasteryData{
				"skill-a": {SkillID: "skill-a", State: "mastered", CurrentTier: "learn", MasteredAt: &masteredStr},
			},
		},
		// No SpacedRep data — should trigger bootstrap
	}
	svc := mastery.NewService(snap, nil)
	sched := NewScheduler(snap, svc, nil)

	rs := sched.GetReviewState("skill-a")
	if rs == nil {
		t.Fatal("expected skill-a bootstrapped from mastery")
	}
	if rs.Stage != 0 {
		t.Errorf("Stage = %d, want 0", rs.Stage)
	}
}

func TestNewScheduler_NilSnap(t *testing.T) {
	svc := mastery.NewService(nil, nil)
	sched := NewScheduler(nil, svc, nil)

	states := sched.AllReviewStates()
	if len(states) != 0 {
		t.Errorf("expected 0 states for nil snapshot, got %d", len(states))
	}
}
