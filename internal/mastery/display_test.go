package mastery

import (
	"testing"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

func TestResolveDisplayState_NewLocked(t *testing.T) {
	state := ResolveDisplayState(StateNew, false, skillgraph.TierLearn)
	if state != skillgraph.StateLocked {
		t.Errorf("got %d, want StateLocked", state)
	}
}

func TestResolveDisplayState_NewAvailable(t *testing.T) {
	state := ResolveDisplayState(StateNew, true, skillgraph.TierLearn)
	if state != skillgraph.StateAvailable {
		t.Errorf("got %d, want StateAvailable", state)
	}
}

func TestResolveDisplayState_LearningLearnTier(t *testing.T) {
	state := ResolveDisplayState(StateLearning, true, skillgraph.TierLearn)
	if state != skillgraph.StateLearning {
		t.Errorf("got %d, want StateLearning", state)
	}
}

func TestResolveDisplayState_LearningProveTier(t *testing.T) {
	state := ResolveDisplayState(StateLearning, true, skillgraph.TierProve)
	if state != skillgraph.StateProving {
		t.Errorf("got %d, want StateProving", state)
	}
}

func TestResolveDisplayState_Mastered(t *testing.T) {
	state := ResolveDisplayState(StateMastered, true, skillgraph.TierProve)
	if state != skillgraph.StateMastered {
		t.Errorf("got %d, want StateMastered", state)
	}
}

func TestResolveDisplayState_Rusty(t *testing.T) {
	state := ResolveDisplayState(StateRusty, true, skillgraph.TierLearn)
	if state != skillgraph.StateRusty {
		t.Errorf("got %d, want StateRusty", state)
	}
}
