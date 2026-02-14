package session

import "time"

// SessionSummary holds the data displayed on the summary screen.
type SessionSummary struct {
	Duration       time.Duration
	TotalQuestions int
	TotalCorrect   int
	Accuracy       float64
	SkillResults   []SkillResult
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
				results = append(results, *sr)
			}
		}
	}

	var accuracy float64
	if state.TotalQuestions > 0 {
		accuracy = float64(state.TotalCorrect) / float64(state.TotalQuestions)
	}

	return &SessionSummary{
		Duration:       state.Elapsed,
		TotalQuestions: state.TotalQuestions,
		TotalCorrect:  state.TotalCorrect,
		Accuracy:       accuracy,
		SkillResults:   results,
	}
}
