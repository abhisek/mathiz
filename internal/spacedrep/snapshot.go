package spacedrep

import (
	"time"

	"github.com/abhisek/mathiz/internal/store"
)

// BootstrapFromMastery creates initial review states for mastered skills
// that don't have existing spaced rep data. Used during migration.
func BootstrapFromMastery(masterySnap *store.MasterySnapshotData) *store.SpacedRepSnapshotData {
	data := &store.SpacedRepSnapshotData{
		Reviews: make(map[string]*store.ReviewStateData),
	}
	if masterySnap == nil || masterySnap.Skills == nil {
		return data
	}
	for skillID, skill := range masterySnap.Skills {
		if skill.State != "mastered" || skill.MasteredAt == nil {
			continue
		}
		masteredAt, err := time.Parse(time.RFC3339, *skill.MasteredAt)
		if err != nil {
			continue
		}
		nextReview := masteredAt.AddDate(0, 0, BaseIntervals[0])
		data.Reviews[skillID] = &store.ReviewStateData{
			SkillID:         skillID,
			Stage:           0,
			NextReviewDate:  nextReview.Format(time.RFC3339),
			ConsecutiveHits: 0,
			Graduated:       false,
			LastReviewDate:  masteredAt.Format(time.RFC3339),
		}
	}
	return data
}
