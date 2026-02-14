package spacedrep

import (
	"context"
	"sort"
	"time"

	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/store"
)

// Scheduler manages spaced repetition review scheduling.
type Scheduler struct {
	reviews   map[string]*ReviewState
	mastery   *mastery.Service
	eventRepo store.EventRepo
}

// NewScheduler creates a scheduler, loading review state from the snapshot.
// If the snapshot has mastery data but no spaced rep data, it bootstraps
// review states from the mastery snapshot (migration path).
func NewScheduler(snap *store.SnapshotData, masterySvc *mastery.Service, eventRepo store.EventRepo) *Scheduler {
	s := &Scheduler{
		reviews:   make(map[string]*ReviewState),
		mastery:   masterySvc,
		eventRepo: eventRepo,
	}

	if snap == nil {
		return s
	}

	// Load existing spaced rep data if available.
	if snap.SpacedRep != nil {
		s.loadFromSnapshot(snap.SpacedRep)
		return s
	}

	// Bootstrap from mastery data if no spaced rep data exists.
	if snap.Mastery != nil {
		bootstrapped := BootstrapFromMastery(snap.Mastery)
		s.loadFromSnapshot(bootstrapped)
	}

	return s
}

func (s *Scheduler) loadFromSnapshot(data *store.SpacedRepSnapshotData) {
	if data == nil || data.Reviews == nil {
		return
	}
	for skillID, rd := range data.Reviews {
		nextReview, err := time.Parse(time.RFC3339, rd.NextReviewDate)
		if err != nil {
			continue
		}
		lastReview, err := time.Parse(time.RFC3339, rd.LastReviewDate)
		if err != nil {
			continue
		}
		s.reviews[skillID] = &ReviewState{
			SkillID:         rd.SkillID,
			Stage:           rd.Stage,
			NextReviewDate:  nextReview,
			ConsecutiveHits: rd.ConsecutiveHits,
			Graduated:       rd.Graduated,
			LastReviewDate:  lastReview,
		}
	}
}

// RunDecayCheck scans all mastered skills and marks overdue ones as rusty.
// Called at session start. Returns the list of skills that transitioned to rusty.
func (s *Scheduler) RunDecayCheck(ctx context.Context, now time.Time) []*mastery.StateTransition {
	var transitions []*mastery.StateTransition

	for skillID, rs := range s.reviews {
		sm := s.mastery.GetMastery(skillID)
		if sm.State != mastery.StateMastered {
			continue
		}
		if rs.IsRustyThreshold(now) {
			transition := s.mastery.MarkRusty(skillID)
			if transition != nil {
				transitions = append(transitions, transition)
				if s.eventRepo != nil {
					_ = s.eventRepo.AppendMasteryEvent(ctx, store.MasteryEventData{
						SkillID:      skillID,
						FromState:    string(transition.From),
						ToState:      string(transition.To),
						Trigger:      "time-decay",
						FluencyScore: sm.FluencyScore(),
					})
				}
			}
		}
	}
	return transitions
}

// DueSkills returns mastered skills that are due for review, sorted by
// most overdue first. Used by the session planner for review slot selection.
func (s *Scheduler) DueSkills(now time.Time) []string {
	type dueSkill struct {
		id      string
		overdue float64
	}
	var due []dueSkill

	for skillID, rs := range s.reviews {
		sm := s.mastery.GetMastery(skillID)
		if sm.State != mastery.StateMastered {
			continue
		}
		if rs.IsDue(now) {
			due = append(due, dueSkill{id: skillID, overdue: rs.OverdueDays(now)})
		}
	}

	sort.Slice(due, func(i, j int) bool {
		if due[i].overdue != due[j].overdue {
			return due[i].overdue > due[j].overdue
		}
		return due[i].id < due[j].id
	})

	ids := make([]string, len(due))
	for i, d := range due {
		ids[i] = d.id
	}
	return ids
}

// RecordReview updates the review schedule after a review answer.
func (s *Scheduler) RecordReview(skillID string, correct bool, now time.Time) {
	rs := s.reviews[skillID]
	if rs == nil {
		return
	}

	rs.LastReviewDate = now

	if correct {
		rs.ConsecutiveHits++

		if !rs.Graduated {
			rs.Stage++
			if rs.ConsecutiveHits >= GraduationStage {
				rs.Graduated = true
			}
		}

		intervalDays := rs.CurrentIntervalDays()
		rs.NextReviewDate = now.AddDate(0, 0, intervalDays)
	} else {
		rs.ConsecutiveHits = 0
	}
}

// InitSkill initializes review state for a newly mastered skill.
func (s *Scheduler) InitSkill(skillID string, masteredAt time.Time) {
	s.reviews[skillID] = &ReviewState{
		SkillID:         skillID,
		Stage:           0,
		NextReviewDate:  masteredAt.AddDate(0, 0, BaseIntervals[0]),
		ConsecutiveHits: 0,
		Graduated:       false,
		LastReviewDate:  masteredAt,
	}
}

// ReInitSkill re-initializes review state after recovery (Rusty -> Mastered).
func (s *Scheduler) ReInitSkill(skillID string, now time.Time) {
	s.reviews[skillID] = &ReviewState{
		SkillID:         skillID,
		Stage:           0,
		NextReviewDate:  now.AddDate(0, 0, BaseIntervals[0]),
		ConsecutiveHits: 0,
		Graduated:       false,
		LastReviewDate:  now,
	}
}

// GetReviewState returns the review state for a skill, or nil if not tracked.
func (s *Scheduler) GetReviewState(skillID string) *ReviewState {
	return s.reviews[skillID]
}

// AllReviewStates returns all review states (for stats/UI).
func (s *Scheduler) AllReviewStates() map[string]*ReviewState {
	result := make(map[string]*ReviewState, len(s.reviews))
	for id, rs := range s.reviews {
		result[id] = rs
	}
	return result
}

// SnapshotData exports the current review state for persistence.
func (s *Scheduler) SnapshotData() *store.SpacedRepSnapshotData {
	data := &store.SpacedRepSnapshotData{
		Reviews: make(map[string]*store.ReviewStateData),
	}
	for skillID, rs := range s.reviews {
		data.Reviews[skillID] = &store.ReviewStateData{
			SkillID:         rs.SkillID,
			Stage:           rs.Stage,
			NextReviewDate:  rs.NextReviewDate.Format(time.RFC3339),
			ConsecutiveHits: rs.ConsecutiveHits,
			Graduated:       rs.Graduated,
			LastReviewDate:  rs.LastReviewDate.Format(time.RFC3339),
		}
	}
	return data
}
