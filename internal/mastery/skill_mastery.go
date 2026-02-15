package mastery

import (
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

// SkillMastery holds all mastery-related data for a single skill.
type SkillMastery struct {
	SkillID       string
	State         MasteryState
	CurrentTier   skillgraph.Tier
	TotalAttempts int
	CorrectCount  int
	Fluency       FluencyMetrics
	MasteredAt    *time.Time // When mastery was first achieved
	RustyAt       *time.Time // When skill was last marked rusty

	// MisconceptionPenalty is the number of extra correct answers required
	// to complete the current tier, incremented by misconception diagnoses.
	// Reset to 0 on tier advancement.
	MisconceptionPenalty int
}

// Accuracy returns the current accuracy ratio.
func (sm *SkillMastery) Accuracy() float64 {
	if sm.TotalAttempts == 0 {
		return 0.0
	}
	return float64(sm.CorrectCount) / float64(sm.TotalAttempts)
}

// FluencyScore returns the computed fluency score (0.0-1.0).
func (sm *SkillMastery) FluencyScore() float64 {
	return FluencyScore(&sm.Fluency, sm.Accuracy())
}

// IsTierComplete checks if the current tier's criteria are met.
// MisconceptionPenalty adds extra correct answers required beyond the
// base accuracy threshold.
func (sm *SkillMastery) IsTierComplete(cfg skillgraph.TierConfig) bool {
	if sm.TotalAttempts < cfg.ProblemsRequired {
		return false
	}
	requiredCorrect := int(float64(cfg.ProblemsRequired)*cfg.AccuracyThreshold + 0.5)
	requiredCorrect += sm.MisconceptionPenalty
	return sm.CorrectCount >= requiredCorrect
}
