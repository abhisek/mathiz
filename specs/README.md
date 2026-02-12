# Mathiz

An AI-native terminal (TUI) application that helps a single learner build math mastery through a skill dependency graph, adaptive practice, spaced repetition, and LLM-generated questions and micro-lessons.

## Overview

Mathiz is designed to support math learners across K-12, with an extensible skill graph that can grow to cover any grade level. The MVP starts with Grade 3-5 fundamentals, but the architecture is built to accommodate additional grades and topics over time. It combines graph-guided progression with gamification (gems/levels) to make practice feel like a game while building durable automaticity.

## Tech Stack

- **Language:** Go
- **TUI Framework:** Charmbracelet
- **Storage:** SQLite (local, append-only event log + snapshots)
- **LLM Provider:** Required -- powers question generation, micro-lessons, hints, diagnosis, and context compression (strict JSON, validated)

## Core Modules

- **Skill Graph** - Extensible graph of skill nodes with prerequisites; MVP starts with ~30-60 nodes covering place value, addition/subtraction, multiplication, and division, designed to scale across K-12
- **Problem Generation** - LLM-generated questions based on the learner's current context, skill state, and session plan -- no fixed question bank
- **Session Planner** - Graph-guided session planning: 60% frontier practice, 30% spaced review, 10% confidence boosters
- **Mastery & Scoring** - Fluency scoring (accuracy + speed + consistency), mastery state machine (new, learning, mastered, rusty)
- **Error Diagnosis** - Rule-based detection of careless errors, speed-rush mistakes, and misconceptions
- **Spaced Repetition** - Per-skill review scheduling based on mastery strength and recency
- **Rewards (Gems)** - Mastery, retention, and recovery gems tied to real learning milestones
- **AI Module** - Core system component: generates all questions, micro-lessons, hints, and context compression snapshots; outputs are constrained to strict JSON and programmatically validated
- **Persistence** - SQLite with append-only event log and periodic compressed snapshots

## Key Screens

Home, Session (question-by-question practice), Session Summary, Skill Map, Gem Vault, History, Settings

## Component Specs

Each component below gets its own spec document. Components are listed in dependency order -- earlier components are prerequisites for later ones.

| # | Component | Spec | Description |
|---|-----------|------|-------------|
| 1 | **Project Skeleton & TUI Framework** | `01-skeleton.md` | Go module, Charmbracelet app shell, screen routing, shared UI primitives (input, list, progress), Home screen |
| 2 | **Persistence Layer** | `02-persistence.md` | SQLite setup, migrations, schema (all tables), repository interfaces, append-only event log |
| 3 | **Skill Graph** | `03-skill-graph.md` | Skill data model, seed graph JSON (~30-60 nodes), graph traversal APIs (prerequisites, frontier, blocked, available), Skill Map screen |
| 4 | **LLM Integration** | `04-llm.md` | Provider abstraction (multi-provider), prompt templates, strict JSON schema enforcement, validation pipeline, token limits, error handling |
| 5 | **Problem Generation** | `05-problem-gen.md` | LLM-powered question generation from skill + learner context, programmatic answer validation, difficulty tiers, deduplication within session |
| 6 | **Session Engine** | `06-session.md` | Session planner (target skill, frontier/review/booster mix), session lifecycle (start/serve/record/complete), Session screen (question UI, input, timer), Session Summary screen |
| 7 | **Mastery & Scoring** | `07-mastery.md` | Per-skill metrics (accuracy, speed, consistency, assist rate), fluency score (0-1), mastery state machine (new -> learning -> mastered -> rusty), configurable mastery criteria |
| 8 | **Spaced Repetition** | `08-spaced-rep.md` | Per-skill review scheduling, next-review-date computation, decay detection, rusty labeling, integration with session planner |
| 9 | **Error Diagnosis** | `09-diagnosis.md` | Rule-based error classifiers (careless, speed-rush, misconception), misconception tagging, AI-assisted diagnosis fallback, intervention recommendations |
| 10 | **AI Lessons, Hints & Compression** | `10-ai-lessons.md` | Hint generation, micro-lesson generation (explanation + worked example + mini practice), context compression snapshots, all with strict JSON + validation |
| 11 | **Rewards (Gems)** | `11-rewards.md` | Gem types (mastery/retention/recovery), rarity levels, award triggers, Gem Vault screen, History screen |

### Dependency graph

```
1 Skeleton
├── 2 Persistence
│   ├── 3 Skill Graph
│   │   ├── 5 Problem Generation (needs 4)
│   │   ├── 6 Session Engine (needs 5)
│   │   │   ├── 7 Mastery & Scoring
│   │   │   │   ├── 8 Spaced Repetition
│   │   │   │   ├── 9 Error Diagnosis
│   │   │   │   └── 11 Rewards
│   │   │   └── 10 AI Lessons & Compression (needs 4)
│   │   └── 8 Spaced Repetition
│   └── 4 LLM Integration
```

## Design Principles

- Clean, minimal, learning-first UI
- No shame mechanics -- errors are treated as signals
- Progress feels like a game, but rewards reflect real mastery
- AI-native: the LLM is a core dependency, not an optional add-on; all questions and lessons are generated contextually, not pulled from a fixed bank
- AI outputs are constrained to strict JSON schemas and programmatically validated
