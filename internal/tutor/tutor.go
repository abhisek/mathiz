// Package tutor bundles the per-learner AI tooling — question generation,
// error diagnosis, micro-lessons, and learner-profile compression — behind a
// single construction point. Both the terminal app (internal/app) and the
// treasure-map game (internal/saas/game) build their toolsets here so the
// two surfaces can never drift apart. This package must stay free of UI
// imports (bubbletea etc.) so server-side code can depend on it.
package tutor

import (
	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
)

// Toolset is the per-learner AI tooling. Every field except Generator is
// optional — nil disables that feature.
type Toolset struct {
	Generator  problemgen.Generator
	Diagnosis  *diagnosis.Service  // optional
	Lessons    *lessons.Service    // optional — micro-lessons when a kid struggles
	Compressor *lessons.Compressor // optional
}

// New builds the standard toolset from an LLM provider with default configs.
func New(provider llm.Provider) *Toolset {
	return &Toolset{
		Generator:  problemgen.New(provider, problemgen.DefaultConfig()),
		Diagnosis:  diagnosis.NewService(provider),
		Lessons:    lessons.NewService(provider, lessons.DefaultConfig()),
		Compressor: lessons.NewCompressor(provider, lessons.DefaultCompressorConfig()),
	}
}
