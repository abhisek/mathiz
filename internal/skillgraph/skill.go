package skillgraph

// Strand represents a math content strand.
type Strand string

const (
	StrandNumberPlace Strand = "number-and-place-value"
	StrandAddSub      Strand = "addition-and-subtraction"
	StrandMultDiv     Strand = "multiplication-and-division"
	StrandFractions   Strand = "fractions"
	StrandMeasurement Strand = "measurement"
)

// AllStrands returns all strands in display order.
func AllStrands() []Strand {
	return []Strand{
		StrandNumberPlace,
		StrandAddSub,
		StrandMultDiv,
		StrandFractions,
		StrandMeasurement,
	}
}

// StrandDisplayName returns a human-readable name for a strand.
func StrandDisplayName(s Strand) string {
	switch s {
	case StrandNumberPlace:
		return "Number & Place Value"
	case StrandAddSub:
		return "Addition & Subtraction"
	case StrandMultDiv:
		return "Multiplication & Division"
	case StrandFractions:
		return "Fractions"
	case StrandMeasurement:
		return "Measurement"
	default:
		return string(s)
	}
}

// Tier represents a difficulty tier.
type Tier int

const (
	TierLearn Tier = iota // Practice with hints available, untimed
	TierProve             // Timed assessment without hints, demonstrates mastery
)

// TierConfig holds the configuration for a skill tier.
type TierConfig struct {
	Tier              Tier
	ProblemsRequired  int
	AccuracyThreshold float64
	TimeLimitSecs     int
	HintsAllowed      bool
}

// DefaultTiers returns the default Learn and Prove tier configurations.
func DefaultTiers() [2]TierConfig {
	return [2]TierConfig{
		{Tier: TierLearn, ProblemsRequired: 8, AccuracyThreshold: 0.75, TimeLimitSecs: 0, HintsAllowed: true},
		{Tier: TierProve, ProblemsRequired: 6, AccuracyThreshold: 0.85, TimeLimitSecs: 30, HintsAllowed: false},
	}
}

// Skill represents a single math skill node in the graph.
type Skill struct {
	ID            string
	Name          string
	Description   string
	Strand        Strand
	GradeLevel    int
	CommonCoreID  string
	EstimatedMins int
	Keywords      []string
	Prerequisites []string
	Tiers         [2]TierConfig
}

// SkillState represents a skill's state relative to the learner.
type SkillState int

const (
	StateLocked    SkillState = iota // One or more prerequisites not yet mastered
	StateAvailable                   // All prerequisites mastered; skill not yet started
	StateLearning                    // Learn tier in progress
	StateProving                     // Learn tier passed; Prove tier in progress
	StateMastered                    // Prove tier passed
	StateRusty                       // Previously mastered but flagged by spaced repetition
)

// Icon returns the display icon for a skill state.
func (s SkillState) Icon() string {
	switch s {
	case StateLocked:
		return "🔒"
	case StateAvailable:
		return "🔓"
	case StateLearning:
		return "📖"
	case StateProving:
		return "📝"
	case StateMastered:
		return "✅"
	case StateRusty:
		return "🔄"
	default:
		return "?"
	}
}

// Label returns the display label for a skill state.
func (s SkillState) Label() string {
	switch s {
	case StateLocked:
		return "Locked"
	case StateAvailable:
		return "Available"
	case StateLearning:
		return "Learning"
	case StateProving:
		return "Proving"
	case StateMastered:
		return "Mastered"
	case StateRusty:
		return "Rusty"
	default:
		return "Unknown"
	}
}
