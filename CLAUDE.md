# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
CGO_ENABLED=0 go build ./...          # Build (CGO must be disabled - no gcc in container)
make mathiz                            # Production binary to bin/mathiz
make generate                          # Run ent code generation (required before building after schema changes)
make fmt                               # Format code
golangci-lint run                      # Lint
```

## Testing

```bash
go test ./...                                              # All tests
go test -run TestServiceName ./internal/mastery            # Single test
go test -v ./internal/session/...                          # Verbose, one package
```

## Code Generation

Ent ORM generates code from schemas in `ent/schema/`. After modifying or adding schemas:
```bash
CGO_ENABLED=0 go generate ./ent
```

## Architecture

**Mathiz** is an AI-native terminal math tutor (grades 3-5) built with Bubble Tea v2, Ent ORM (SQLite), and multi-provider LLM integration.

### Data Flow
CLI (`cmd/`) → `app.AppModel` (Bubble Tea) → `router` (screen stack) → `screen.Screen` implementations → domain packages

### Key Packages
- **`cmd/run.go`** — Dependency wiring: opens SQLite store, creates LLM provider, builds `app.Options`
- **`internal/app/`** — Root Bubble Tea model, receives `app.Options` for DI
- **`internal/router/`** — Stack-based screen navigation (push/pop)
- **`internal/screen/`** — `Screen` interface all screens implement; `KeyHintProvider` optional interface for footer hints
- **`internal/skillgraph/`** — 52-skill DAG, package-level singleton initialized in `init()`, panics on validation failure
- **`internal/llm/`** — Provider interface with adapters (Anthropic, OpenAI, Gemini), retry/logging decorators, factory pattern
- **`internal/problemgen/`** — LLM-based question generation with validator chain (structural, answer format, math check)
- **`internal/session/`** — Session engine: Planner, Plan, TierProgress, SessionState
- **`internal/mastery/`** — State machine (new → learning → mastered → rusty), fluency scoring
- **`internal/diagnosis/`** — Hybrid error classification: rule-based sync + LLM async
- **`internal/lessons/`** — Hints, micro-lessons, context compression, learner profiles
- **`internal/store/`** — Event sourcing + snapshots; `EventRepo` and `SnapshotRepo` interfaces

### Persistence Pattern
Event sourcing via Ent schemas sharing `EventMixin` (sequence number + timestamp). State reconstructed from snapshots + events.

## Charm Libraries v2 API

These libraries use v2 APIs that differ significantly from v1:
- `charm.land/bubbletea/v2`: `View()` returns `tea.View` (not string). Use `tea.NewView()` and `v.SetContent()`. AltScreen is a View field, not a program option.
- `charm.land/bubbles/v2/textinput`: No `CursorStyle`/`TextStyle`/`PlaceholderStyle` fields. Use `Focus()` method.
- `charm.land/lipgloss/v2`: Pinned to specific beta commit.

## Responsive Screen Layout

Screens receive `(width, height int)` in `View()`. Use **measure-then-render** instead of hardcoded breakpoints:

1. Pre-render each element, measure its height with `lipgloss.Height()`.
2. Greedily include elements from highest to lowest priority based on available space.
3. When an element doesn't fit, try a cheaper variant (e.g. borderless menu instead of bordered).
4. Only upgrade to decorative elements (mascots, ASCII art) when there is surplus space.
5. Use `width < N` for horizontal/text concerns (compact labels), **not** for deciding what to show vertically.

See `internal/screens/home/home.go` `View()` for the reference implementation.

## Environment Variables

- `MATHIZ_LLM_PROVIDER` — `anthropic`, `openai`, or `gemini`
- `MATHIZ_ANTHROPIC_API_KEY`, `MATHIZ_OPENAI_API_KEY`, `MATHIZ_GEMINI_API_KEY`

## Testing Patterns

- Table-driven tests with mock implementations of interfaces
- `tea.KeyPressMsg{Code: rune}` for simulating key events (bubbletea v2)
- Skill graph has only 1 root skill — use `AllSkills()` not `RootSkills()[:N]` in tests
