package mastery

import (
	"testing"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

func TestRecoveryTierConfig(t *testing.T) {
	cfg := RecoveryTierConfig()

	if cfg.Tier != skillgraph.TierLearn {
		t.Errorf("Tier = %d, want TierLearn", cfg.Tier)
	}
	if cfg.ProblemsRequired != 4 {
		t.Errorf("ProblemsRequired = %d, want 4", cfg.ProblemsRequired)
	}
	if !almostEqual(cfg.AccuracyThreshold, 0.75) {
		t.Errorf("AccuracyThreshold = %f, want 0.75", cfg.AccuracyThreshold)
	}
	if cfg.TimeLimitSecs != 0 {
		t.Errorf("TimeLimitSecs = %d, want 0", cfg.TimeLimitSecs)
	}
	if !cfg.HintsAllowed {
		t.Error("expected HintsAllowed to be true")
	}
}
