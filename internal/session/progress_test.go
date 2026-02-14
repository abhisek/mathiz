package session

import (
	"testing"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

func TestTierProgress_Record_Correct(t *testing.T) {
	tp := &TierProgress{SkillID: "add-2digit", CurrentTier: skillgraph.TierLearn}

	tp.Record(true)

	if tp.TotalAttempts != 1 {
		t.Errorf("TotalAttempts = %d, want 1", tp.TotalAttempts)
	}
	if tp.CorrectCount != 1 {
		t.Errorf("CorrectCount = %d, want 1", tp.CorrectCount)
	}
	if tp.Accuracy != 1.0 {
		t.Errorf("Accuracy = %f, want 1.0", tp.Accuracy)
	}
}

func TestTierProgress_Record_Incorrect(t *testing.T) {
	tp := &TierProgress{SkillID: "add-2digit", CurrentTier: skillgraph.TierLearn}

	tp.Record(false)

	if tp.TotalAttempts != 1 {
		t.Errorf("TotalAttempts = %d, want 1", tp.TotalAttempts)
	}
	if tp.CorrectCount != 0 {
		t.Errorf("CorrectCount = %d, want 0", tp.CorrectCount)
	}
	if tp.Accuracy != 0.0 {
		t.Errorf("Accuracy = %f, want 0.0", tp.Accuracy)
	}
}

func TestTierProgress_Record_Mixed(t *testing.T) {
	tp := &TierProgress{SkillID: "add-2digit", CurrentTier: skillgraph.TierLearn}

	tp.Record(true)
	tp.Record(true)
	tp.Record(false)
	tp.Record(true)

	if tp.TotalAttempts != 4 {
		t.Errorf("TotalAttempts = %d, want 4", tp.TotalAttempts)
	}
	if tp.CorrectCount != 3 {
		t.Errorf("CorrectCount = %d, want 3", tp.CorrectCount)
	}
	if tp.Accuracy != 0.75 {
		t.Errorf("Accuracy = %f, want 0.75", tp.Accuracy)
	}
}

func TestTierProgress_IsTierComplete_Met(t *testing.T) {
	tp := &TierProgress{
		SkillID:       "add-2digit",
		CurrentTier:   skillgraph.TierLearn,
		TotalAttempts: 8,
		CorrectCount:  7,
		Accuracy:      7.0 / 8.0, // 87.5%
	}

	cfg := skillgraph.TierConfig{
		ProblemsRequired:  8,
		AccuracyThreshold: 0.75,
	}

	if !tp.IsTierComplete(cfg) {
		t.Error("expected tier to be complete")
	}
}

func TestTierProgress_IsTierComplete_NotEnoughAttempts(t *testing.T) {
	tp := &TierProgress{
		SkillID:       "add-2digit",
		CurrentTier:   skillgraph.TierLearn,
		TotalAttempts: 5,
		CorrectCount:  5,
		Accuracy:      1.0,
	}

	cfg := skillgraph.TierConfig{
		ProblemsRequired:  8,
		AccuracyThreshold: 0.75,
	}

	if tp.IsTierComplete(cfg) {
		t.Error("expected tier NOT to be complete (not enough attempts)")
	}
}

func TestTierProgress_IsTierComplete_LowAccuracy(t *testing.T) {
	tp := &TierProgress{
		SkillID:       "add-2digit",
		CurrentTier:   skillgraph.TierLearn,
		TotalAttempts: 8,
		CorrectCount:  5,
		Accuracy:      5.0 / 8.0, // 62.5%
	}

	cfg := skillgraph.TierConfig{
		ProblemsRequired:  8,
		AccuracyThreshold: 0.75,
	}

	if tp.IsTierComplete(cfg) {
		t.Error("expected tier NOT to be complete (low accuracy)")
	}
}

func TestTierString(t *testing.T) {
	if TierString(skillgraph.TierLearn) != "learn" {
		t.Errorf("TierString(TierLearn) = %q, want %q", TierString(skillgraph.TierLearn), "learn")
	}
	if TierString(skillgraph.TierProve) != "prove" {
		t.Errorf("TierString(TierProve) = %q, want %q", TierString(skillgraph.TierProve), "prove")
	}
}

func TestTierFromString(t *testing.T) {
	if TierFromString("learn") != skillgraph.TierLearn {
		t.Error("TierFromString(learn) != TierLearn")
	}
	if TierFromString("prove") != skillgraph.TierProve {
		t.Error("TierFromString(prove) != TierProve")
	}
	if TierFromString("unknown") != skillgraph.TierLearn {
		t.Error("TierFromString(unknown) should default to TierLearn")
	}
}
