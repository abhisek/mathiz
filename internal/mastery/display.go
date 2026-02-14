package mastery

import "github.com/abhisek/mathiz/internal/skillgraph"

// ResolveDisplayState maps a mastery state + graph position + tier progress
// into the display state used by the UI.
func ResolveDisplayState(
	state MasteryState,
	prerequisitesMet bool,
	currentTier skillgraph.Tier,
) skillgraph.SkillState {
	switch state {
	case StateNew:
		if prerequisitesMet {
			return skillgraph.StateAvailable
		}
		return skillgraph.StateLocked
	case StateLearning:
		if currentTier == skillgraph.TierProve {
			return skillgraph.StateProving
		}
		return skillgraph.StateLearning
	case StateMastered:
		return skillgraph.StateMastered
	case StateRusty:
		return skillgraph.StateRusty
	default:
		return skillgraph.StateLocked
	}
}
