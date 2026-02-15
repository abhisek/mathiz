package session

import (
	"sync"
	"time"

	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/mastery"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/skillgraph"
	"github.com/abhisek/mathiz/internal/store"
)

// SpacedRepScheduler is the interface for session-level spaced rep operations.
type SpacedRepScheduler interface {
	RecordReview(skillID string, correct bool, now time.Time)
	InitSkill(skillID string, masteredAt time.Time)
	ReInitSkill(skillID string, now time.Time)
}

// SessionPhase represents the current phase of the session.
type SessionPhase int

const (
	PhaseLoading  SessionPhase = iota // Loading state from snapshot
	PhaseActive                       // Serving questions
	PhaseFeedback                     // Showing answer feedback
	PhaseEnding                       // Session time expired or quit confirmed
	PhaseSummary                      // Showing summary screen
)

// SessionState tracks the runtime state of an active session.
type SessionState struct {
	// Plan is the session plan built at start.
	Plan *Plan

	// CurrentSlotIndex is the index into Plan.Slots for the current skill.
	CurrentSlotIndex int

	// QuestionsInSlot is the number of questions served in the current slot.
	QuestionsInSlot int

	// CurrentQuestion is the active question being displayed (nil between questions).
	CurrentQuestion *problemgen.Question

	// TotalQuestions is the count of questions served so far.
	TotalQuestions int

	// TotalCorrect is the count of correct answers so far.
	TotalCorrect int

	// PerSkillResults tracks per-skill stats for the summary screen.
	PerSkillResults map[string]*SkillResult

	// TierProgress tracks cumulative tier progress (loaded from snapshot, updated live).
	TierProgress map[string]*TierProgress

	// Mastered is the set of mastered skill IDs (loaded from snapshot, updated live).
	Mastered map[string]bool

	// StartTime is when the session began.
	StartTime time.Time

	// Elapsed tracks total elapsed time.
	Elapsed time.Duration

	// Phase is the current session phase.
	Phase SessionPhase

	// PriorQuestions tracks questions asked per skill in this session (for dedup).
	PriorQuestions map[string][]string

	// RecentErrors tracks recent errors per skill (for LLM context).
	RecentErrors map[string][]string

	// ShowingFeedback is true when the feedback overlay is displayed.
	ShowingFeedback bool

	// ShowingQuitConfirm is true when the quit confirmation dialog is displayed.
	ShowingQuitConfirm bool

	// LastAnswerCorrect records whether the most recent answer was correct.
	LastAnswerCorrect bool

	// TierAdvanced is set when a tier advancement happens, for feedback display.
	TierAdvanced *TierAdvancement

	// MasteryTransition is set when a mastery state transition happens, for feedback display.
	MasteryTransition *mastery.StateTransition

	// MasteryService manages per-skill mastery state and fluency scoring.
	MasteryService *mastery.Service

	// SessionID is the UUID for this session.
	SessionID string

	// QuestionStartTime tracks when the current question was first displayed.
	QuestionStartTime time.Time

	// TimeExpired indicates the session timer has run out.
	TimeExpired bool

	// CompletedSlots tracks slot indices that have been completed (tier advanced).
	CompletedSlots map[int]bool

	// SpacedRepSched is the spaced repetition scheduler for answer recording (nil if not enabled).
	SpacedRepSched SpacedRepScheduler

	// DiagnosisService classifies wrong answers (nil if diagnosis disabled).
	DiagnosisService *diagnosis.Service

	// LastDiagnosis is the most recent diagnosis result (nil if last answer was correct).
	LastDiagnosis *diagnosis.DiagnosisResult

	// EventRepo is used for querying historical accuracy for diagnosis.
	EventRepo store.EventRepo

	// LessonService generates micro-lessons (nil if lessons disabled).
	LessonService *lessons.Service

	// Compressor handles context compression (nil if compression disabled).
	Compressor *lessons.Compressor

	// HintShown is true if the hint was shown for the current question.
	HintShown bool

	// HintAvailable is true if a hint can be shown (wrong answer + hint exists).
	HintAvailable bool

	// WrongCountBySkill tracks per-skill wrong answer count in this session.
	WrongCountBySkill map[string]int

	// PendingLesson is true when a lesson has been requested but not yet consumed.
	PendingLesson bool

	// ErrorMu protects RecentErrors during async compression callbacks.
	ErrorMu sync.Mutex
}

// SkillResult tracks per-skill performance within a single session.
type SkillResult struct {
	SkillID      string
	SkillName    string
	Category     PlanCategory
	Attempted    int
	Correct      int
	TierBefore   skillgraph.Tier
	TierAfter    skillgraph.Tier
	FluencyScore float64 // Fluency score at end of session (0.0-1.0, -1 if unavailable)
}

// TierAdvancement records a tier transition for display purposes.
type TierAdvancement struct {
	SkillID   string
	SkillName string
	FromTier  skillgraph.Tier
	ToTier    skillgraph.Tier // TierProve or TierLearn+1 sentinel for mastered
	Mastered  bool            // true if this advancement means mastery
}

// NewSessionState creates a new session state with initialized maps.
func NewSessionState(plan *Plan, sessionID string, mastered map[string]bool, tierProgress map[string]*TierProgress) *SessionState {
	if mastered == nil {
		mastered = make(map[string]bool)
	}
	if tierProgress == nil {
		tierProgress = make(map[string]*TierProgress)
	}

	perSkill := make(map[string]*SkillResult)
	for _, slot := range plan.Slots {
		if _, exists := perSkill[slot.Skill.ID]; !exists {
			tp := tierProgress[slot.Skill.ID]
			var tierBefore skillgraph.Tier
			if tp != nil {
				tierBefore = tp.CurrentTier
			} else {
				tierBefore = skillgraph.TierLearn
			}
			perSkill[slot.Skill.ID] = &SkillResult{
				SkillID:    slot.Skill.ID,
				SkillName:  slot.Skill.Name,
				Category:   slot.Category,
				TierBefore: tierBefore,
				TierAfter:  tierBefore,
			}
		}
	}

	return &SessionState{
		Plan:              plan,
		SessionID:         sessionID,
		Mastered:          mastered,
		TierProgress:      tierProgress,
		PerSkillResults:   perSkill,
		PriorQuestions:    make(map[string][]string),
		RecentErrors:      make(map[string][]string),
		StartTime:         time.Now(),
		Phase:             PhaseActive,
		CompletedSlots:    make(map[int]bool),
		WrongCountBySkill: make(map[string]int),
	}
}
