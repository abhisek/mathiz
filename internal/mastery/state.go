package mastery

// MasteryState represents a skill's position in the mastery lifecycle.
type MasteryState string

const (
	StateNew      MasteryState = "new"
	StateLearning MasteryState = "learning"
	StateMastered MasteryState = "mastered"
	StateRusty    MasteryState = "rusty"
)

// StateTransition records a mastery state change for display and event logging.
type StateTransition struct {
	SkillID   string
	SkillName string
	From      MasteryState
	To        MasteryState
	Trigger   string // "first-attempt", "tier-complete", "prove-complete", "time-decay", "review-performance", "recovery-complete"
}
