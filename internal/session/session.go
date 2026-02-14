package session

import (
	"fmt"
	"time"

	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
)

// MaxRecentErrors is the maximum number of recent errors tracked per skill.
const MaxRecentErrors = 5

// HandleAnswer processes a learner's answer, updating session state and tier progress.
// Returns a TierAdvancement if the answer caused a tier transition, nil otherwise.
// Also updates the MasteryService and sets state.MasteryTransition.
func HandleAnswer(state *SessionState, learnerAnswer string) *TierAdvancement {
	q := state.CurrentQuestion
	if q == nil {
		return nil
	}

	correct := problemgen.CheckAnswer(learnerAnswer, q)
	state.LastAnswerCorrect = correct
	state.TotalQuestions++

	if correct {
		state.TotalCorrect++
	}

	// Update per-skill results.
	sr := state.PerSkillResults[q.SkillID]
	if sr != nil {
		sr.Attempted++
		if correct {
			sr.Correct++
		}
	}

	// Track prior questions for dedup.
	state.PriorQuestions[q.SkillID] = append(state.PriorQuestions[q.SkillID], q.Text)

	// Track errors for LLM context.
	if !correct {
		errCtx := BuildErrorContext(q, learnerAnswer)
		errors := state.RecentErrors[q.SkillID]
		errors = append(errors, errCtx)
		if len(errors) > MaxRecentErrors {
			errors = errors[len(errors)-MaxRecentErrors:]
		}
		state.RecentErrors[q.SkillID] = errors
	}

	// Delegate to mastery service if available.
	if state.MasteryService != nil {
		return handleAnswerWithMastery(state, q, correct)
	}

	// Legacy path: update tier progress directly.
	return handleAnswerLegacy(state, q, correct)
}

func handleAnswerWithMastery(state *SessionState, q *problemgen.Question, correct bool) *TierAdvancement {
	responseTimeMs := int(time.Since(state.QuestionStartTime).Milliseconds())

	skill, err := skillgraph.GetSkill(q.SkillID)
	if err != nil {
		return nil
	}

	tierCfg := skill.Tiers[q.Tier]
	transition := state.MasteryService.RecordAnswer(q.SkillID, correct, responseTimeMs, tierCfg)

	// Update spaced rep schedule for review answers.
	if state.SpacedRepSched != nil {
		slot := CurrentSlot(state)
		if slot != nil && slot.Category == CategoryReview {
			state.SpacedRepSched.RecordReview(q.SkillID, correct, time.Now())
		}
	}

	// Sync mastered set from service.
	state.Mastered = state.MasteryService.MasteredSkills()

	// Sync tier progress from mastery service for planner compatibility.
	sm := state.MasteryService.GetMastery(q.SkillID)
	state.TierProgress[q.SkillID] = &TierProgress{
		SkillID:       q.SkillID,
		CurrentTier:   sm.CurrentTier,
		TotalAttempts: sm.TotalAttempts,
		CorrectCount:  sm.CorrectCount,
		Accuracy:      sm.Accuracy(),
	}

	// Store the mastery transition for UI feedback.
	state.MasteryTransition = transition

	// Initialize spaced rep for newly mastered skills.
	if state.SpacedRepSched != nil && transition != nil {
		now := time.Now()
		switch {
		case transition.From == mastery.StateLearning && transition.To == mastery.StateMastered:
			state.SpacedRepSched.InitSkill(q.SkillID, now)
		case transition.From == mastery.StateRusty && transition.To == mastery.StateMastered:
			state.SpacedRepSched.ReInitSkill(q.SkillID, now)
		}
	}

	// Convert to TierAdvancement for backward compatibility.
	if transition != nil {
		return masteryTransitionToTierAdvancement(transition)
	}
	return nil
}

func handleAnswerLegacy(state *SessionState, q *problemgen.Question, correct bool) *TierAdvancement {
	tp := state.TierProgress[q.SkillID]
	if tp == nil {
		tp = &TierProgress{
			SkillID:     q.SkillID,
			CurrentTier: skillgraph.TierLearn,
		}
		state.TierProgress[q.SkillID] = tp
	}
	tp.Record(correct)

	skill, err := skillgraph.GetSkill(q.SkillID)
	if err != nil {
		return nil
	}

	tierCfg := skill.Tiers[tp.CurrentTier]
	if tp.IsTierComplete(tierCfg) {
		return advanceTier(state, tp, skill)
	}

	return nil
}

func masteryTransitionToTierAdvancement(t *mastery.StateTransition) *TierAdvancement {
	switch t.Trigger {
	case "tier-complete":
		return &TierAdvancement{
			SkillID:   t.SkillID,
			SkillName: t.SkillName,
			FromTier:  skillgraph.TierLearn,
			ToTier:    skillgraph.TierProve,
			Mastered:  false,
		}
	case "prove-complete":
		return &TierAdvancement{
			SkillID:   t.SkillID,
			SkillName: t.SkillName,
			FromTier:  skillgraph.TierProve,
			ToTier:    skillgraph.TierProve,
			Mastered:  true,
		}
	case "recovery-complete":
		return &TierAdvancement{
			SkillID:   t.SkillID,
			SkillName: t.SkillName,
			FromTier:  skillgraph.TierLearn,
			ToTier:    skillgraph.TierLearn,
			Mastered:  true,
		}
	}
	return nil
}

// advanceTier performs a tier transition and returns the advancement info.
func advanceTier(state *SessionState, tp *TierProgress, skill skillgraph.Skill) *TierAdvancement {
	adv := &TierAdvancement{
		SkillID:   skill.ID,
		SkillName: skill.Name,
		FromTier:  tp.CurrentTier,
	}

	if tp.CurrentTier == skillgraph.TierLearn {
		// Learn → Prove
		adv.ToTier = skillgraph.TierProve
		tp.CurrentTier = skillgraph.TierProve
		tp.TotalAttempts = 0
		tp.CorrectCount = 0
		tp.Accuracy = 0
	} else {
		// Prove → Mastered
		adv.ToTier = skillgraph.TierProve
		adv.Mastered = true
		state.Mastered[skill.ID] = true
	}

	// Update per-skill result.
	if sr := state.PerSkillResults[skill.ID]; sr != nil {
		if adv.Mastered {
			sr.TierAfter = skillgraph.TierProve // highest tier completed
		} else {
			sr.TierAfter = skillgraph.TierProve
		}
	}

	return adv
}

// AdvanceSlot moves to the next slot in the plan, skipping completed slots.
// Returns false if all slots are completed.
func AdvanceSlot(state *SessionState) bool {
	state.QuestionsInSlot = 0
	numSlots := len(state.Plan.Slots)
	if numSlots == 0 {
		return false
	}

	// Try each slot in round-robin.
	for i := 0; i < numSlots; i++ {
		state.CurrentSlotIndex = (state.CurrentSlotIndex + 1) % numSlots
		if !state.CompletedSlots[state.CurrentSlotIndex] {
			return true
		}
	}

	return false // All slots completed.
}

// ShouldAdvanceSlot returns true if the current slot's mini-block is done.
func ShouldAdvanceSlot(state *SessionState) bool {
	return state.QuestionsInSlot >= QuestionsPerSlot
}

// CurrentSlot returns the current plan slot, or nil if invalid.
func CurrentSlot(state *SessionState) *PlanSlot {
	if state.CurrentSlotIndex < 0 || state.CurrentSlotIndex >= len(state.Plan.Slots) {
		return nil
	}
	return &state.Plan.Slots[state.CurrentSlotIndex]
}

// BuildErrorContext constructs an error description string for LLM context.
func BuildErrorContext(question *problemgen.Question, learnerAnswer string) string {
	return fmt.Sprintf(
		"Answered %s for '%s', correct answer was %s",
		learnerAnswer,
		question.Text,
		question.Answer,
	)
}
