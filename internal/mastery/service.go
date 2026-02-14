package mastery

import (
	"context"
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// Service provides mastery state management for all skills.
type Service struct {
	skills    map[string]*SkillMastery
	eventRepo store.EventRepo
}

// NewService creates a mastery service, loading state from the snapshot.
func NewService(snap *store.SnapshotData, eventRepo store.EventRepo) *Service {
	s := &Service{
		skills:    make(map[string]*SkillMastery),
		eventRepo: eventRepo,
	}

	if snap == nil {
		return s
	}

	// Prefer new mastery format if available.
	if snap.Mastery != nil {
		s.loadFromMasterySnapshot(snap.Mastery)
		return s
	}

	// Migrate from old format.
	if len(snap.TierProgress) > 0 || len(snap.MasteredSet) > 0 {
		migrated := MigrateSnapshot(snap)
		s.loadFromMasterySnapshot(migrated)
	}

	return s
}

func (s *Service) loadFromMasterySnapshot(data *store.MasterySnapshotData) {
	if data == nil || data.Skills == nil {
		return
	}
	for id, sd := range data.Skills {
		sm := &SkillMastery{
			SkillID:       id,
			State:         MasteryState(sd.State),
			CurrentTier:   tierFromString(sd.CurrentTier),
			TotalAttempts: sd.TotalAttempts,
			CorrectCount:  sd.CorrectCount,
			Fluency: FluencyMetrics{
				SpeedScores: sd.SpeedScores,
				SpeedWindow: sd.SpeedWindow,
				Streak:      sd.Streak,
				StreakCap:    sd.StreakCap,
			},
		}
		if sd.MasteredAt != nil {
			t, err := time.Parse(time.RFC3339, *sd.MasteredAt)
			if err == nil {
				sm.MasteredAt = &t
			}
		}
		if sd.RustyAt != nil {
			t, err := time.Parse(time.RFC3339, *sd.RustyAt)
			if err == nil {
				sm.RustyAt = &t
			}
		}
		// Ensure defaults.
		if sm.Fluency.SpeedWindow == 0 {
			sm.Fluency.SpeedWindow = DefaultSpeedWindow
		}
		if sm.Fluency.StreakCap == 0 {
			sm.Fluency.StreakCap = DefaultStreakCap
		}
		s.skills[id] = sm
	}
}

// GetMastery returns the mastery record for a skill.
// Returns a default (StateNew) record if the skill hasn't been encountered.
func (s *Service) GetMastery(skillID string) *SkillMastery {
	if sm, ok := s.skills[skillID]; ok {
		return sm
	}
	sm := &SkillMastery{
		SkillID:     skillID,
		State:       StateNew,
		CurrentTier: skillgraph.TierLearn,
		Fluency:     DefaultFluencyMetrics(),
	}
	s.skills[skillID] = sm
	return sm
}

// MasteredSkills returns the set of mastered skill IDs.
func (s *Service) MasteredSkills() map[string]bool {
	result := make(map[string]bool)
	for id, sm := range s.skills {
		if sm.State == StateMastered {
			result[id] = true
		}
	}
	return result
}

// RecordAnswer updates mastery state after a learner answers a question.
// Returns a StateTransition if the answer caused a state change, nil otherwise.
func (s *Service) RecordAnswer(skillID string, correct bool, responseTimeMs int, tierCfg skillgraph.TierConfig) *StateTransition {
	sm := s.GetMastery(skillID)
	skillName := resolveSkillName(skillID)

	var transition *StateTransition

	// If state is New, transition to Learning.
	if sm.State == StateNew {
		transition = &StateTransition{
			SkillID:   skillID,
			SkillName: skillName,
			From:      StateNew,
			To:        StateLearning,
			Trigger:   "first-attempt",
		}
		sm.State = StateLearning
	}

	// Update attempt counters.
	sm.TotalAttempts++
	if correct {
		sm.CorrectCount++
	}

	// Update fluency metrics.
	speedScore := SpeedScore(responseTimeMs, tierCfg)
	RecordSpeed(&sm.Fluency, speedScore)

	if correct {
		sm.Fluency.Streak++
	} else {
		sm.Fluency.Streak = 0
	}

	// Check tier completion.
	if sm.IsTierComplete(tierCfg) {
		if t := s.advanceTier(sm, skillName); t != nil {
			transition = t // More significant transition overrides first-attempt.
		}
	}

	return transition
}

func (s *Service) advanceTier(sm *SkillMastery, skillName string) *StateTransition {
	switch {
	case sm.State == StateLearning && sm.CurrentTier == skillgraph.TierLearn:
		// Learn → Prove: advance tier, reset counters.
		sm.CurrentTier = skillgraph.TierProve
		sm.TotalAttempts = 0
		sm.CorrectCount = 0
		return &StateTransition{
			SkillID:   sm.SkillID,
			SkillName: skillName,
			From:      StateLearning,
			To:        StateLearning,
			Trigger:   "tier-complete",
		}

	case sm.State == StateLearning && sm.CurrentTier == skillgraph.TierProve:
		// Prove → Mastered.
		now := time.Now()
		sm.State = StateMastered
		sm.MasteredAt = &now
		return &StateTransition{
			SkillID:   sm.SkillID,
			SkillName: skillName,
			From:      StateLearning,
			To:        StateMastered,
			Trigger:   "prove-complete",
		}

	case sm.State == StateRusty:
		// Recovery complete → Mastered.
		sm.State = StateMastered
		sm.RustyAt = nil
		return &StateTransition{
			SkillID:   sm.SkillID,
			SkillName: skillName,
			From:      StateRusty,
			To:        StateMastered,
			Trigger:   "recovery-complete",
		}
	}
	return nil
}

// MarkRusty transitions a mastered skill to rusty state.
// Returns a StateTransition, or nil if the skill is not currently mastered.
func (s *Service) MarkRusty(skillID string) *StateTransition {
	sm := s.GetMastery(skillID)
	if sm.State != StateMastered {
		return nil
	}

	now := time.Now()
	sm.State = StateRusty
	sm.RustyAt = &now

	// Reset counters for recovery tracking.
	sm.TotalAttempts = 0
	sm.CorrectCount = 0
	sm.CurrentTier = skillgraph.TierLearn

	return &StateTransition{
		SkillID:   sm.SkillID,
		SkillName: resolveSkillName(skillID),
		From:      StateMastered,
		To:        StateRusty,
		Trigger:   "time-decay",
	}
}

// CheckReviewPerformance checks if a mastered skill should go rusty
// based on recent review performance.
func (s *Service) CheckReviewPerformance(ctx context.Context, skillID string) *StateTransition {
	sm := s.GetMastery(skillID)
	if sm.State != StateMastered {
		return nil
	}

	if s.eventRepo == nil {
		return nil
	}

	accuracy, count, err := s.eventRepo.RecentReviewAccuracy(ctx, skillID, 4)
	if err != nil || count < 4 {
		return nil
	}

	if accuracy < 0.50 {
		t := s.MarkRusty(skillID)
		if t != nil {
			t.Trigger = "review-performance"
		}
		return t
	}
	return nil
}

// SnapshotData exports the current mastery state for persistence.
func (s *Service) SnapshotData() *store.MasterySnapshotData {
	data := &store.MasterySnapshotData{
		Skills: make(map[string]*store.SkillMasteryData),
	}

	for id, sm := range s.skills {
		sd := &store.SkillMasteryData{
			SkillID:       id,
			State:         string(sm.State),
			CurrentTier:   tierToString(sm.CurrentTier),
			TotalAttempts: sm.TotalAttempts,
			CorrectCount:  sm.CorrectCount,
			SpeedScores:   sm.Fluency.SpeedScores,
			SpeedWindow:   sm.Fluency.SpeedWindow,
			Streak:        sm.Fluency.Streak,
			StreakCap:      sm.Fluency.StreakCap,
		}
		if sm.MasteredAt != nil {
			s := sm.MasteredAt.Format(time.RFC3339)
			sd.MasteredAt = &s
		}
		if sm.RustyAt != nil {
			s := sm.RustyAt.Format(time.RFC3339)
			sd.RustyAt = &s
		}
		data.Skills[id] = sd
	}

	return data
}

// AllSkillMasteries returns all skill mastery records (for stats/UI).
func (s *Service) AllSkillMasteries() map[string]*SkillMastery {
	result := make(map[string]*SkillMastery, len(s.skills))
	for id, sm := range s.skills {
		result[id] = sm
	}
	return result
}

func resolveSkillName(skillID string) string {
	skill, err := skillgraph.GetSkill(skillID)
	if err != nil {
		return skillID
	}
	return skill.Name
}

func tierFromString(s string) skillgraph.Tier {
	if s == "prove" {
		return skillgraph.TierProve
	}
	return skillgraph.TierLearn
}

func tierToString(t skillgraph.Tier) string {
	if t == skillgraph.TierProve {
		return "prove"
	}
	return "learn"
}
