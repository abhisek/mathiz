package mastery

import (
	"testing"

	"github.com/abhisek/mathiz/internal/store"
)

func TestMigrateSnapshot_EmptySnapshot(t *testing.T) {
	result := MigrateSnapshot(nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Skills) != 0 {
		t.Errorf("Skills count = %d, want 0", len(result.Skills))
	}
}

func TestMigrateSnapshot_OldFormat(t *testing.T) {
	old := &store.SnapshotData{
		TierProgress: map[string]*store.TierProgressData{
			"s1": {SkillID: "s1", CurrentTier: "learn", TotalAttempts: 5, CorrectCount: 4},
			"s2": {SkillID: "s2", CurrentTier: "prove", TotalAttempts: 3, CorrectCount: 3},
		},
		MasteredSet: []string{"s3"},
	}

	result := MigrateSnapshot(old)

	if len(result.Skills) != 3 {
		t.Errorf("Skills count = %d, want 3", len(result.Skills))
	}

	// s1: learning (has tier progress, not mastered)
	if result.Skills["s1"].State != "learning" {
		t.Errorf("s1 State = %s, want learning", result.Skills["s1"].State)
	}
	if result.Skills["s1"].TotalAttempts != 5 {
		t.Errorf("s1 TotalAttempts = %d, want 5", result.Skills["s1"].TotalAttempts)
	}

	// s2: learning (has tier progress, not mastered)
	if result.Skills["s2"].State != "learning" {
		t.Errorf("s2 State = %s, want learning", result.Skills["s2"].State)
	}

	// s3: mastered (in mastered set, no tier progress)
	if result.Skills["s3"].State != "mastered" {
		t.Errorf("s3 State = %s, want mastered", result.Skills["s3"].State)
	}
}

func TestMigrateSnapshot_MasteredWithTierProgress(t *testing.T) {
	old := &store.SnapshotData{
		TierProgress: map[string]*store.TierProgressData{
			"s1": {SkillID: "s1", CurrentTier: "prove", TotalAttempts: 6, CorrectCount: 6},
		},
		MasteredSet: []string{"s1"},
	}

	result := MigrateSnapshot(old)
	if result.Skills["s1"].State != "mastered" {
		t.Errorf("State = %s, want mastered", result.Skills["s1"].State)
	}
}

func TestMigrateSnapshot_DefaultFluency(t *testing.T) {
	old := &store.SnapshotData{
		TierProgress: map[string]*store.TierProgressData{
			"s1": {SkillID: "s1", CurrentTier: "learn"},
		},
	}

	result := MigrateSnapshot(old)
	sd := result.Skills["s1"]

	if sd.SpeedWindow != DefaultSpeedWindow {
		t.Errorf("SpeedWindow = %d, want %d", sd.SpeedWindow, DefaultSpeedWindow)
	}
	if sd.StreakCap != DefaultStreakCap {
		t.Errorf("StreakCap = %d, want %d", sd.StreakCap, DefaultStreakCap)
	}
	if sd.Streak != 0 {
		t.Errorf("Streak = %d, want 0", sd.Streak)
	}
}
