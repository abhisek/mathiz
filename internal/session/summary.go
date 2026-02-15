package session

import (
	"time"

	"github.com/abhisek/mathiz/internal/gems"
)

// SessionSummary holds the data displayed on the summary screen.
type SessionSummary struct {
	Duration       time.Duration
	TotalQuestions int
	TotalCorrect   int
	Accuracy       float64
	SkillResults   []SkillResult
	GemsEarned     []gems.GemAward
}

// SkillSummaryFluency returns the fluency score for a skill from the mastery service.
// Returns -1 if the mastery service is not available.
func SkillSummaryFluency(state *SessionState, skillID string) float64 {
	if state.MasteryService == nil {
		return -1
	}
	sm := state.MasteryService.GetMastery(skillID)
	return sm.FluencyScore()
}

// BuildSummary creates a SessionSummary from the current session state.
func BuildSummary(state *SessionState) *SessionSummary {
	var results []SkillResult
	for _, slot := range state.Plan.Slots {
		if sr, ok := state.PerSkillResults[slot.Skill.ID]; ok {
			// Avoid duplicates — only add each skill once.
			found := false
			for _, r := range results {
				if r.SkillID == sr.SkillID {
					found = true
					break
				}
			}
			if !found {
				sr.FluencyScore = SkillSummaryFluency(state, sr.SkillID)
				results = append(results, *sr)
			}
		}
	}

	var accuracy float64
	if state.TotalQuestions > 0 {
		accuracy = float64(state.TotalCorrect) / float64(state.TotalQuestions)
	}

	summary := &SessionSummary{
		Duration:       state.Elapsed,
		TotalQuestions: state.TotalQuestions,
		TotalCorrect:  state.TotalCorrect,
		Accuracy:       accuracy,
		SkillResults:   results,
	}

	if state.GemService != nil {
		summary.GemsEarned = state.GemService.SessionGems
	}

	return summary
}
