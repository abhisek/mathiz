package mastery

import "github.com/abhisek/mathiz/internal/store"

// MigrateSnapshot converts old-format snapshot data to the new mastery format.
func MigrateSnapshot(old *store.SnapshotData) *store.MasterySnapshotData {
	if old == nil {
		return &store.MasterySnapshotData{
			Skills: make(map[string]*store.SkillMasteryData),
		}
	}

	masteredSet := make(map[string]bool)
	for _, id := range old.MasteredSet {
		masteredSet[id] = true
	}

	result := &store.MasterySnapshotData{
		Skills: make(map[string]*store.SkillMasteryData),
	}

	// Convert TierProgress entries.
	for id, tp := range old.TierProgress {
		state := string(StateLearning)
		if masteredSet[id] {
			state = string(StateMastered)
		}
		result.Skills[id] = &store.SkillMasteryData{
			SkillID:       tp.SkillID,
			State:         state,
			CurrentTier:   tp.CurrentTier,
			TotalAttempts: tp.TotalAttempts,
			CorrectCount:  tp.CorrectCount,
			SpeedWindow:   DefaultSpeedWindow,
			StreakCap:      DefaultStreakCap,
		}
	}

	// Add mastered skills that might not have TierProgress entries.
	for id := range masteredSet {
		if _, exists := result.Skills[id]; !exists {
			result.Skills[id] = &store.SkillMasteryData{
				SkillID:     id,
				State:       string(StateMastered),
				CurrentTier: "prove",
				SpeedWindow: DefaultSpeedWindow,
				StreakCap:    DefaultStreakCap,
			}
		}
	}

	return result
}
