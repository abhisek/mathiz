package mastery

import "github.com/abhisek/mathiz/internal/skillgraph"

// RecoveryTierConfig returns the tier config used for rusty skill recovery.
func RecoveryTierConfig() skillgraph.TierConfig {
	return skillgraph.TierConfig{
		Tier:              skillgraph.TierLearn,
		ProblemsRequired:  4,
		AccuracyThreshold: 0.75,
		TimeLimitSecs:     0,
		HintsAllowed:      true,
	}
}
