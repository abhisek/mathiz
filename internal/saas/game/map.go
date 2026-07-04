package game

import (
	"context"
	"fmt"
	"time"

	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/spacedrep"
	"github.com/abhisek/mathiz/internal/store"
)

// Map builds the full treasure-map view for a child. It is strictly
// read-only: mastery and spaced-rep services are constructed with nil event
// repos so the decay check can't persist anything from a map render — decay
// transitions are persisted when an expedition starts, like session start
// in the terminal app.
func (m *Manager) Map(ctx context.Context, childUID string) (*MapView, error) {
	snapRepo := m.cfg.Store.SnapshotRepoFor(childUID)
	snap, err := snapRepo.Latest(ctx)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	var snapData *store.SnapshotData
	if snap != nil {
		snapData = &snap.Data
	}

	masterySvc := mastery.NewService(snapData, nil)
	scheduler := spacedrep.NewScheduler(snapData, masterySvc, nil)
	scheduler.RunDecayCheck(ctx, time.Now())

	mastered := masterySvc.MasteredSkills()
	due := make(map[string]bool)
	for _, id := range scheduler.DueSkills(time.Now()) {
		due[id] = true
	}

	view := &MapView{}
	for _, strand := range skillgraph.AllStrands() {
		island := IslandView{
			ID:   string(strand),
			Name: skillgraph.StrandDisplayName(strand),
		}
		for _, skill := range skillgraph.ByStrand(strand) {
			island.Spots = append(island.Spots, spotView(skill, masterySvc, mastered, due))
		}
		view.Islands = append(view.Islands, island)
	}

	byType, total, err := m.cfg.Store.EventRepoFor(childUID).GemCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("gem counts: %w", err)
	}
	view.Gems = GemsView{Total: total, ByType: byType}
	return view, nil
}

func spotView(skill skillgraph.Skill, svc *mastery.Service, mastered, due map[string]bool) SpotView {
	sm := svc.GetMastery(skill.ID)

	spot := SpotView{
		ID:            skill.ID,
		Name:          skill.Name,
		Description:   skill.Description,
		Grade:         skill.GradeLevel,
		Prerequisites: skill.Prerequisites,
		ReviewDue:     due[skill.ID],
	}

	switch sm.State {
	case mastery.StateMastered:
		if due[skill.ID] {
			spot.State = "sinking"
		} else {
			spot.State = "treasure"
		}
		spot.Progress = 1
	case mastery.StateRusty:
		spot.State = "sinking"
		spot.Progress = 1
	case mastery.StateLearning:
		if sm.CurrentTier == skillgraph.TierProve {
			spot.State = "proving"
		} else {
			spot.State = "digging"
		}
		cfg := skill.Tiers[sm.CurrentTier]
		if cfg.ProblemsRequired > 0 {
			spot.Progress = min(1, float64(sm.TotalAttempts)/float64(cfg.ProblemsRequired))
		}
	default: // StateNew
		if skillgraph.IsUnlocked(skill.ID, mastered) {
			spot.State = "ready"
		} else {
			spot.State = "locked"
		}
	}
	return spot
}
