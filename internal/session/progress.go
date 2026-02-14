package session

import "github.com/abhisek/mathiz/internal/skillgraph"

// TierProgress tracks cumulative progress toward completing a tier for a skill.
type TierProgress struct {
	SkillID       string
	CurrentTier   skillgraph.Tier
	TotalAttempts int
	CorrectCount  int
	Accuracy      float64 // CorrectCount / TotalAttempts (computed)
}

// IsTierComplete returns true if the learner has met the tier's completion criteria.
func (tp *TierProgress) IsTierComplete(cfg skillgraph.TierConfig) bool {
	return tp.TotalAttempts >= cfg.ProblemsRequired &&
		tp.Accuracy >= cfg.AccuracyThreshold
}

// Record adds a new answer result to the progress.
func (tp *TierProgress) Record(correct bool) {
	tp.TotalAttempts++
	if correct {
		tp.CorrectCount++
	}
	if tp.TotalAttempts > 0 {
		tp.Accuracy = float64(tp.CorrectCount) / float64(tp.TotalAttempts)
	}
}

// TierString returns the string representation of the current tier.
func TierString(t skillgraph.Tier) string {
	if t == skillgraph.TierProve {
		return "prove"
	}
	return "learn"
}

// TierFromString parses a tier string back to the Tier type.
func TierFromString(s string) skillgraph.Tier {
	if s == "prove" {
		return skillgraph.TierProve
	}
	return skillgraph.TierLearn
}
