package session

import (
	"context"
	"fmt"
	"time"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/spacedrep"
)

// MaxRecentErrors is the maximum number of recent errors tracked per skill.
const MaxRecentErrors = 5

// compressionThreshold is the character count triggering session-level compression.
const compressionThreshold = lessons.SessionCompressionThreshold

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

	// Track errors for LLM context and run diagnosis.
	state.LastDiagnosis = nil
	if !correct {
		// Track per-skill wrong count.
		state.WrongCountBySkill[q.SkillID]++

		var diag *diagnosis.DiagnosisResult
		if state.DiagnosisService != nil {
			responseTimeMs := int(time.Since(state.QuestionStartTime).Milliseconds())
			var skillAccuracy float64
			if state.EventRepo != nil {
				skillAccuracy, _ = state.EventRepo.SkillAccuracy(context.Background(), q.SkillID)
			}
			diag = state.DiagnosisService.Diagnose(
				context.Background(),
				q,
				learnerAnswer,
				responseTimeMs,
				skillAccuracy,
				func(asyncResult *diagnosis.DiagnosisResult) {
					if asyncResult.Category == diagnosis.CategoryMisconception && state.MasteryService != nil {
						sm := state.MasteryService.GetMastery(q.SkillID)
						sm.MisconceptionPenalty++
					}
				},
			)
			state.LastDiagnosis = diag

			// Apply synchronous misconception penalty.
			if diag.Category == diagnosis.CategoryMisconception && state.MasteryService != nil {
				sm := state.MasteryService.GetMastery(q.SkillID)
				sm.MisconceptionPenalty++
			}
		}

		errCtx := BuildErrorContext(q, learnerAnswer, diag)
		state.ErrorMu.Lock()
		errors := state.RecentErrors[q.SkillID]
		errors = append(errors, errCtx)
		if len(errors) > MaxRecentErrors {
			errors = errors[len(errors)-MaxRecentErrors:]
		}
		state.RecentErrors[q.SkillID] = errors
		state.ErrorMu.Unlock()

		// Mark hint available (if question has a hint and not already shown).
		if q.Hint != "" && !state.HintShown {
			state.HintAvailable = true
		}

		// Trigger micro-lesson if 2+ wrong on this skill.
		if state.WrongCountBySkill[q.SkillID] >= 2 && state.LessonService != nil {
			state.PendingLesson = true
			skill, skillErr := skillgraph.GetSkill(q.SkillID)
			if skillErr == nil {
				var accuracy float64
				if state.EventRepo != nil {
					accuracy, _ = state.EventRepo.SkillAccuracy(context.Background(), q.SkillID)
				}
				state.LessonService.RequestLesson(context.Background(), lessons.LessonInput{
					Skill:         skill,
					Tier:          q.Tier,
					RecentErrors:  state.RecentErrors[q.SkillID],
					LastDiagnosis: state.LastDiagnosis,
					Accuracy:      accuracy,
				})
			}
		}

		// Check compression threshold.
		totalLen := 0
		for _, e := range state.RecentErrors[q.SkillID] {
			totalLen += len(e)
		}
		if totalLen > compressionThreshold && state.Compressor != nil {
			errorsCopy := make([]string, len(state.RecentErrors[q.SkillID]))
			copy(errorsCopy, state.RecentErrors[q.SkillID])
			state.Compressor.CompressErrors(
				context.Background(),
				q.SkillID,
				errorsCopy,
				func(skillID string, summary string) {
					state.ErrorMu.Lock()
					defer state.ErrorMu.Unlock()
					state.RecentErrors[skillID] = []string{"[compressed] " + summary}
				},
			)
		}
	}

	// Update streak tracking.
	state.PendingGemAward = nil
	if correct {
		state.ConsecutiveCorrect++
		// Check streak gem thresholds.
		if state.GemService != nil && state.ConsecutiveCorrect >= state.NextStreakThreshold {
			award := state.GemService.AwardStreak(context.Background(), state.ConsecutiveCorrect, state.SessionID)
			state.PendingGemAward = award
			state.NextStreakThreshold = gems.NextStreakThreshold(state.ConsecutiveCorrect)
		}
	} else {
		state.ConsecutiveCorrect = 0
		state.NextStreakThreshold = gems.BaseStreakThreshold
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
			prevHits := 0
			if rs := state.SpacedRepSched.GetReviewState(q.SkillID); rs != nil {
				prevHits = rs.ConsecutiveHits
			}
			state.SpacedRepSched.RecordReview(q.SkillID, correct, time.Now())

			// Check for graduation (retention gem).
			if correct && state.GemService != nil {
				if rs := state.SpacedRepSched.GetReviewState(q.SkillID); rs != nil {
					if rs.Graduated && prevHits < spacedrep.GraduationStage {
						award := state.GemService.AwardRetention(context.Background(), q.SkillID, skill.Name, state.SessionID)
						state.PendingGemAward = award
					}
				}
			}
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

	// Award mastery/recovery gems on state transitions.
	if transition != nil && state.GemService != nil {
		ctx := context.Background()
		switch {
		case transition.From == mastery.StateLearning && transition.To == mastery.StateMastered:
			award := state.GemService.AwardMastery(ctx, transition.SkillID, transition.SkillName, state.SessionID)
			state.PendingGemAward = award // mastery takes display priority over streak
		case transition.From == mastery.StateRusty && transition.To == mastery.StateMastered:
			award := state.GemService.AwardRecovery(ctx, transition.SkillID, transition.SkillName, state.SessionID)
			state.PendingGemAward = award
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
// When a diagnosis is available, it enriches the context with the category
// and misconception label.
func BuildErrorContext(question *problemgen.Question, learnerAnswer string, diag *diagnosis.DiagnosisResult) string {
	base := fmt.Sprintf(
		"Answered %s for '%s', correct answer was %s",
		learnerAnswer,
		question.Text,
		question.Answer,
	)
	if diag == nil || diag.Category == diagnosis.CategoryUnclassified {
		return base
	}
	enriched := fmt.Sprintf("%s [%s", base, diag.Category)
	if diag.MisconceptionID != "" {
		m := diagnosis.GetMisconception(diag.MisconceptionID)
		if m != nil {
			enriched += ": " + m.Label
		}
	}
	enriched += "]"
	return enriched
}
