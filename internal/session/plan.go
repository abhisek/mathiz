package session

import (
	"time"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

// PlanCategory represents the reason a skill was included in the plan.
type PlanCategory string

const (
	CategoryFrontier PlanCategory = "frontier"
	CategoryReview   PlanCategory = "review"
	CategoryBooster  PlanCategory = "booster"
)

// PlanSlot is a single slot in the session plan — a skill + tier pair
// that will receive a mini-block of questions.
type PlanSlot struct {
	Skill    skillgraph.Skill
	Tier     skillgraph.Tier
	Category PlanCategory
}

// Plan is the ordered list of skill slots for a session.
type Plan struct {
	Slots    []PlanSlot
	Duration time.Duration // Always 15 minutes
}

// DefaultSessionDuration is the standard session length.
const DefaultSessionDuration = 15 * time.Minute

// QuestionsPerSlot is the number of questions served per mini-block.
const QuestionsPerSlot = 3

// DefaultTotalSlots is the default number of slots in a session plan.
const DefaultTotalSlots = 5
