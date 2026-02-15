# 10 — AI Lessons, Hints & Compression

## 1. Overview

The AI Lessons module adds three LLM-powered capabilities to Mathiz: **hints** that scaffold learners through difficult questions, **micro-lessons** that remediate after repeated errors, and **context compression** that keeps LLM prompts efficient as history grows. Together, these turn wrong answers into learning moments while keeping token costs bounded.

**Design goals:**

- **Hints as scaffolding**: Hints are already generated with each question (spec 05). This module surfaces them to the learner after a first wrong answer, providing a second chance before moving on.
- **Micro-lessons after repeated errors**: When a learner gets 2+ wrong answers on the same skill within a session, a targeted micro-lesson is generated — explanation, worked example, and a mini-practice question to confirm understanding.
- **No scoring penalty**: Hints and lessons are free learning aids. Using them does not reduce mastery credit. This encourages learners to seek help rather than guess blindly.
- **Session-level compression**: When accumulated error context exceeds a token threshold, the LLM compresses it into a compact summary, reducing prompt size for subsequent question generation calls.
- **Snapshot-level learner profile**: At the end of each session, the LLM generates a holistic learner profile summarizing strengths, weaknesses, and patterns — persisted across sessions and fed into future question generation.
- **Async generation**: Lesson generation and compression run asynchronously to avoid blocking the session flow.

### Consumers

| Module | How it uses AI Lessons |
|--------|----------------------|
| **Session Engine (06)** | Triggers hints after first wrong answer, lessons after 2+ wrong, compression at threshold |
| **Session Screen** | Renders hint overlay, lesson screen, mini-practice UI |
| **Problem Generation (05)** | Receives compressed error context and learner profile for richer prompts |
| **Snapshot (02)** | Persists learner profile in snapshot data |

---

## 2. Hints

### 2.1 Existing Infrastructure

Hints are already generated as part of question generation (spec 05). The `Question.Hint` field contains a short scaffolding hint for Learn tier questions and is empty for Prove tier. **No additional LLM call is needed to produce a hint.**

What's missing is the **UI to show the hint** and the **trigger logic** for when it becomes available.

### 2.2 Hint Availability Rules

A hint becomes available to the learner when **all** of these conditions are true:

1. The current question has a non-empty `Hint` field (Learn tier questions only).
2. The learner has submitted **at least one wrong answer** for the current question.
3. The learner has not already viewed the hint for the current question.

Once available, the session screen shows a key hint: `[h] hint`. Pressing `h` displays the hint in an overlay.

### 2.3 Session State Additions

```go
// Additions to internal/session/state.go

type SessionState struct {
    // ... existing fields ...

    HintShown       bool // Whether the hint was shown for the current question
    HintAvailable   bool // Whether the hint can be shown (wrong answer + hint exists)
    WrongCountBySkill map[string]int // Per-skill wrong answer count in this session
}
```

`HintShown` and `HintAvailable` reset when a new question is served. `WrongCountBySkill` persists for the entire session.

### 2.4 Session Screen Integration

After a wrong answer, if the current question has a non-empty `Hint` and `HintShown` is false:

1. Set `HintAvailable = true`.
2. Update key hints to include `[h] hint`.
3. On `h` keypress:
   - Set `HintShown = true`, `HintAvailable = false`.
   - Display hint text in a styled overlay (similar to the feedback overlay).
   - Any key dismisses the hint overlay.
   - Re-focus the answer input.

```go
// In session screen key handler:

case 'h':
    if m.sess.HintAvailable && !m.sess.HintShown {
        m.sess.HintShown = true
        m.sess.HintAvailable = false
        m.showingHint = true
        return m, nil
    }
```

### 2.5 Hint Overlay Rendering

The hint is displayed in a bordered box above the answer input:

```
┌─ Hint ─────────────────────────────────────────┐
│                                                 │
│  Try adding the ones first: 7 + 5 = 12.        │
│  Write down 2 and carry the 1 to the tens.      │
│                                                 │
└─────────────────────── press any key to close ──┘
```

Styled using the existing theme: `theme.Accent` for the border, `theme.Muted` for the dismiss prompt.

---

## 3. Micro-Lessons

### 3.1 Trigger Condition

A micro-lesson is triggered when the learner accumulates **2 or more wrong answers on the same skill** within the current session. The count tracks total wrong answers per skill, not consecutive ones.

```go
// In HandleAnswer, after recording a wrong answer:

state.WrongCountBySkill[q.SkillID]++
if state.WrongCountBySkill[q.SkillID] >= 2 && state.LessonService != nil {
    state.PendingLesson = true
    state.LessonService.RequestLesson(ctx, buildLessonInput(state, q, learnerAnswer))
}
```

The lesson is requested asynchronously. When the feedback overlay is dismissed, if a lesson is ready, the session screen transitions to the lesson view instead of advancing to the next question.

### 3.2 Lesson Types

```go
// internal/lessons/types.go

package lessons

// Lesson is an LLM-generated micro-lesson for a specific skill and error pattern.
type Lesson struct {
    // SkillID is the skill this lesson targets.
    SkillID string

    // Title is a short title for the lesson (e.g., "Carrying in Addition").
    Title string

    // Explanation is a clear, age-appropriate explanation of the concept.
    // 3-5 sentences aimed at grades 3-5.
    Explanation string

    // WorkedExample is a step-by-step solution to a similar problem.
    // Shows the full process, not just the answer.
    WorkedExample string

    // PracticeQuestion is a simpler question for the learner to try.
    PracticeQuestion PracticeQuestion
}

// PracticeQuestion is a mini-practice embedded in a lesson.
type PracticeQuestion struct {
    // Text is the question prompt.
    Text string

    // Answer is the correct answer.
    Answer string

    // AnswerType is the numeric type of the answer.
    AnswerType string // "integer", "decimal", "fraction"

    // Explanation is a brief explanation shown after the learner answers.
    Explanation string
}
```

### 3.3 Lesson Input

```go
// LessonInput holds all context needed to generate a micro-lesson.
type LessonInput struct {
    // Skill is the target skill.
    Skill skillgraph.Skill

    // Tier is the current difficulty tier.
    Tier skillgraph.Tier

    // RecentErrors are the learner's recent wrong answers on this skill.
    // Includes question text, learner answer, correct answer, and diagnosis.
    RecentErrors []string

    // LastDiagnosis is the most recent diagnosis result (may be nil).
    LastDiagnosis *diagnosis.DiagnosisResult

    // Accuracy is the learner's historical accuracy on this skill.
    Accuracy float64
}
```

### 3.4 Lesson Service

```go
// internal/lessons/service.go

package lessons

import (
    "context"
    "sync"

    "github.com/abhisek/mathiz/internal/llm"
)

// Service generates micro-lessons asynchronously.
type Service struct {
    provider llm.Provider
    cfg      Config

    mu      sync.Mutex
    pending *Lesson     // Most recent generated lesson, waiting to be consumed
    err     error       // Error from last generation attempt
    ready   bool        // Whether a lesson is ready to consume
}

// Config holds lesson generation settings.
type Config struct {
    MaxTokens   int     // Default: 512
    Temperature float64 // Default: 0.5
}

// DefaultConfig returns sensible defaults for lesson generation.
func DefaultConfig() Config {
    return Config{
        MaxTokens:   512,
        Temperature: 0.5,
    }
}

// NewService creates a lesson generation service.
func NewService(provider llm.Provider, cfg Config) *Service {
    return &Service{provider: provider, cfg: cfg}
}

// RequestLesson starts async lesson generation. Only one lesson is in-flight
// at a time — new requests replace pending ones.
func (s *Service) RequestLesson(ctx context.Context, input LessonInput) {
    go func() {
        lesson, err := s.generate(ctx, input)
        s.mu.Lock()
        defer s.mu.Unlock()
        s.pending = lesson
        s.err = err
        s.ready = true
    }()
}

// ConsumeLesson returns the pending lesson if one is ready.
// Returns (nil, false) if no lesson is ready yet.
// After consumption, the pending slot is cleared.
func (s *Service) ConsumeLesson() (*Lesson, bool) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if !s.ready {
        return nil, false
    }
    lesson := s.pending
    s.pending = nil
    s.ready = false
    s.err = nil
    return lesson, lesson != nil
}

func (s *Service) generate(ctx context.Context, input LessonInput) (*Lesson, error) {
    // 1. Build prompt from template + input context
    // 2. Call LLM with lesson schema + purpose "lesson"
    // 3. Parse and validate response
    // 4. Return Lesson
}
```

### 3.5 Prompt Template

```
You are a patient, encouraging math tutor for children in grades 3-5. A student is struggling with a math concept and needs a short, clear lesson.

## Context
Skill: {{.Skill.Name}}
Description: {{.Skill.Description}}
Grade: {{.Skill.GradeLevel}}
Student accuracy on this skill: {{printf "%.0f" (mul .Accuracy 100)}}%

## Recent Errors
{{range .RecentErrors}}
- {{.}}
{{end}}

{{if .LastDiagnosis}}
## Diagnosed Issue
Category: {{.LastDiagnosis.Category}}
{{if .LastDiagnosis.MisconceptionID}}Misconception: {{.LastDiagnosis.MisconceptionID}}{{end}}
{{end}}

## Instructions
Create a micro-lesson that:
1. Explains the concept clearly in 3-5 sentences. Use simple language a child would understand. Address the specific errors shown above.
2. Shows a complete worked example with numbered steps. Pick a problem similar to (but different from) the ones the student got wrong. Show every step.
3. Creates one practice question that is EASIER than the ones the student got wrong. The student should be able to solve it using the explanation and worked example above.
4. The practice question must have a single correct answer. Provide a brief explanation for the practice answer.
5. Use plain ASCII text for all math. No LaTeX, no Unicode symbols. Use / for fractions, * for multiplication.
```

### 3.6 JSON Schema

```go
var LessonSchema = &llm.Schema{
    Name:        "micro-lesson",
    Description: "A micro-lesson with explanation, worked example, and practice question",
    Definition: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "title": map[string]any{
                "type":        "string",
                "description": "Short title for the lesson (3-8 words)",
            },
            "explanation": map[string]any{
                "type":        "string",
                "description": "Clear, age-appropriate explanation of the concept (3-5 sentences)",
            },
            "worked_example": map[string]any{
                "type":        "string",
                "description": "Step-by-step solution to a similar problem, with numbered steps",
            },
            "practice_question": map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "text": map[string]any{
                        "type":        "string",
                        "description": "A simpler practice question for the student to try",
                    },
                    "answer": map[string]any{
                        "type":        "string",
                        "description": "The correct answer",
                    },
                    "answer_type": map[string]any{
                        "type": "string",
                        "enum": []any{"integer", "decimal", "fraction"},
                    },
                    "explanation": map[string]any{
                        "type":        "string",
                        "description": "Brief explanation of the practice answer",
                    },
                },
                "required":             []any{"text", "answer", "answer_type", "explanation"},
                "additionalProperties": false,
            },
        },
        "required":             []any{"title", "explanation", "worked_example", "practice_question"},
        "additionalProperties": false,
    },
}
```

### 3.7 Lesson Screen

The lesson is displayed as a full-screen view within the session, styled with the existing theme. The learner reads through the lesson and then answers the practice question.

**Layout:**

```
╭─ Lesson: Carrying in Addition ──────────────────────╮
│                                                      │
│  When you add numbers and the sum in a column is     │
│  10 or more, you need to "carry" to the next         │
│  column. Think of it like trading 10 ones for 1 ten. │
│                                                      │
│  ── Worked Example ──                                │
│                                                      │
│  Let's solve 48 + 35:                                │
│  1. Add the ones: 8 + 5 = 13                         │
│  2. Write 3, carry the 1 to tens column              │
│  3. Add the tens: 4 + 3 + 1 = 8                      │
│  4. Answer: 83                                        │
│                                                      │
│  ── Your Turn ──                                     │
│                                                      │
│  What is 27 + 15?                                    │
│                                                      │
│  > _                                                 │
│                                                      │
╰──────────────────────────── [enter] submit  [q] skip ╯
```

**Flow:**

1. Lesson content (title, explanation, worked example) is displayed.
2. Practice question is shown at the bottom with a text input.
3. Learner submits an answer:
   - **Correct**: Show brief "Correct!" feedback with the explanation. Any key returns to session.
   - **Wrong**: Show "Not quite" with the correct answer and explanation. Any key returns to session.
4. Pressing `q` skips the practice question and returns to the session.
5. After the lesson (whether practice was answered or skipped), the session resumes with the next question.

**Practice answer checking** uses the same `problemgen.CheckAnswer` logic — the `PracticeQuestion` is converted to a temporary `problemgen.Question` for checking.

### 3.8 Session Screen State

```go
// Additions to session screen model

type SessionScreen struct {
    // ... existing fields ...

    showingHint   bool
    showingLesson bool
    currentLesson *lessons.Lesson
    practiceInput textinput.Model
    practiceState practiceState // idle, answering, showingResult
    practiceCorrect bool
}

type practiceState int

const (
    practiceIdle practiceState = iota
    practiceAnswering
    practiceShowingResult
)
```

---

## 4. Context Compression — Session Level

### 4.1 Purpose

During a session, `SessionState.RecentErrors` accumulates per-skill error descriptions. Each error is ~50-100 characters. After many wrong answers on a skill, this context becomes large, wasting tokens in question generation prompts.

Session-level compression replaces verbose individual error strings with a compact LLM-generated summary when the accumulated text exceeds a threshold.

### 4.2 Compression Trigger

Compression triggers when the total character count of a skill's `RecentErrors` exceeds **800 characters** (roughly ~200 tokens). This is checked after each wrong answer is recorded.

```go
// In HandleAnswer, after appending to RecentErrors:

totalLen := 0
for _, e := range state.RecentErrors[q.SkillID] {
    totalLen += len(e)
}
if totalLen > SessionCompressionThreshold && state.Compressor != nil {
    state.Compressor.CompressErrors(ctx, q.SkillID, state.RecentErrors[q.SkillID])
}
```

```go
const SessionCompressionThreshold = 800 // characters
```

### 4.3 Compression Service

```go
// internal/lessons/compress.go

// Compressor handles context compression for session and snapshot levels.
type Compressor struct {
    provider llm.Provider
    cfg      CompressorConfig
}

// CompressorConfig holds compression settings.
type CompressorConfig struct {
    SessionMaxTokens  int     // Default: 256
    ProfileMaxTokens  int     // Default: 512
    Temperature       float64 // Default: 0.3
}

// DefaultCompressorConfig returns sensible defaults.
func DefaultCompressorConfig() CompressorConfig {
    return CompressorConfig{
        SessionMaxTokens:  256,
        ProfileMaxTokens:  512,
        Temperature:       0.3,
    }
}

// NewCompressor creates a context compressor.
func NewCompressor(provider llm.Provider, cfg CompressorConfig) *Compressor {
    return &Compressor{provider: provider, cfg: cfg}
}

// CompressErrors compresses a skill's error history into a summary.
// Runs asynchronously. The callback receives the compressed summary.
func (c *Compressor) CompressErrors(
    ctx context.Context,
    skillID string,
    errors []string,
    cb func(skillID string, summary string),
) {
    go func() {
        summary, err := c.compressSession(ctx, skillID, errors)
        if err != nil || cb == nil {
            return
        }
        cb(skillID, summary)
    }()
}
```

### 4.4 Session Compression Prompt

```
You are summarizing a math student's error patterns on a specific skill. Create a concise summary that captures the key patterns without losing important details.

## Errors
{{range .Errors}}
- {{.}}
{{end}}

## Instructions
Summarize these errors in 2-3 sentences. Focus on:
- What types of mistakes the student is making (e.g., forgetting to carry, confusing operations)
- Any patterns you see across multiple errors
- What the student seems to understand vs. what they're struggling with

Keep the summary concise and factual. Do not include encouragement or advice — this summary is used internally for generating better practice questions.
```

### 4.5 Session Compression Schema

```go
var SessionCompressionSchema = &llm.Schema{
    Name:        "error-summary",
    Description: "Compressed summary of a student's error patterns on a skill",
    Definition: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "summary": map[string]any{
                "type":        "string",
                "description": "2-3 sentence summary of error patterns",
            },
        },
        "required":             []any{"summary"},
        "additionalProperties": false,
    },
}
```

### 4.6 Applying Compression

When the compression callback fires, the session state replaces the verbose errors with the compressed summary:

```go
// Callback in HandleAnswer:
func(skillID string, summary string) {
    state.mu.Lock()
    defer state.mu.Unlock()
    // Replace all errors with a single compressed summary.
    state.RecentErrors[skillID] = []string{
        "[compressed] " + summary,
    }
}
```

The `[compressed]` prefix signals to the question generation prompt that this is a summary, not a single error. The problem generation system prompt handles both formats naturally since the errors are consumed as free text by the LLM.

---

## 5. Context Compression — Learner Profile

### 5.1 Purpose

The learner profile is a persistent, cross-session summary of the learner's strengths, weaknesses, and learning patterns. It is generated at the end of each session and stored in the snapshot. Future sessions can include the profile in question generation prompts for better personalization.

### 5.2 Profile Type

```go
// internal/lessons/types.go

// LearnerProfile is a holistic summary of the learner's patterns.
type LearnerProfile struct {
    // Summary is a 3-5 sentence profile of the learner.
    Summary string

    // Strengths lists skills or strands where the learner performs well.
    Strengths []string

    // Weaknesses lists skills or strands where the learner struggles.
    Weaknesses []string

    // Patterns describes common error patterns observed across sessions.
    Patterns []string

    // GeneratedAt is when this profile was last generated.
    GeneratedAt time.Time
}
```

### 5.3 Profile Generation

Profile generation is triggered at the **end of each session** (when the session completes or the learner exits). It is async and non-blocking — if it fails, the session still ends normally.

```go
// GenerateProfile creates a learner profile from session and mastery data.
func (c *Compressor) GenerateProfile(
    ctx context.Context,
    input ProfileInput,
) (*LearnerProfile, error) {
    // 1. Build prompt from input
    // 2. Call LLM with profile schema + purpose "profile"
    // 3. Parse and return profile
}

// ProfileInput holds all context for profile generation.
type ProfileInput struct {
    // PerSkillResults from the session that just ended.
    PerSkillResults map[string]*session.SkillResult

    // MasterySnapshots from the mastery service.
    MasteryData map[string]*store.SkillMasteryData

    // ErrorHistory is a map of skill ID → compressed error summaries.
    ErrorHistory map[string][]string

    // PreviousProfile is the existing profile (may be nil for first session).
    PreviousProfile *LearnerProfile

    // SessionCount is how many sessions the learner has completed.
    SessionCount int
}
```

### 5.4 Profile Prompt

```
You are creating a learner profile for a math tutoring system. This profile helps personalize future practice sessions for a student in grades 3-5.

## Session Results
{{range $skillID, $result := .PerSkillResults}}
- {{$skillID}}: {{$result.Attempted}} attempted, {{$result.Correct}} correct ({{printf "%.0f" (accuracy $result)}}%)
{{end}}

## Mastery State
{{range $skillID, $data := .MasteryData}}
- {{$skillID}}: state={{$data.State}}, fluency={{printf "%.2f" $data.FluencyScore}}
{{end}}

## Error History
{{range $skillID, $errors := .ErrorHistory}}
### {{$skillID}}
{{range $errors}}
- {{.}}
{{end}}
{{end}}

{{if .PreviousProfile}}
## Previous Profile
{{.PreviousProfile.Summary}}
Strengths: {{join .PreviousProfile.Strengths ", "}}
Weaknesses: {{join .PreviousProfile.Weaknesses ", "}}
{{end}}

## Instructions
Create a concise learner profile:
1. Write a 3-5 sentence summary of the student's current abilities, focusing on what they know well and where they need work.
2. List 2-4 specific strengths (e.g., "solid multiplication facts", "good with simple fractions").
3. List 2-4 specific weaknesses (e.g., "struggles with carrying in addition", "confuses fraction denominators").
4. List 1-3 error patterns observed (e.g., "frequently rushes and makes careless mistakes", "consistently forgets to borrow in subtraction").

If a previous profile exists, update it with new evidence rather than starting fresh. Keep all entries concise (5-10 words each for strengths/weaknesses/patterns).
```

### 5.5 Profile Schema

```go
var ProfileSchema = &llm.Schema{
    Name:        "learner-profile",
    Description: "Holistic learner profile summarizing strengths, weaknesses, and patterns",
    Definition: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "summary": map[string]any{
                "type":        "string",
                "description": "3-5 sentence overview of the learner's abilities",
            },
            "strengths": map[string]any{
                "type":  "array",
                "items": map[string]any{"type": "string"},
                "description": "2-4 specific strengths (5-10 words each)",
            },
            "weaknesses": map[string]any{
                "type":  "array",
                "items": map[string]any{"type": "string"},
                "description": "2-4 specific weaknesses (5-10 words each)",
            },
            "patterns": map[string]any{
                "type":  "array",
                "items": map[string]any{"type": "string"},
                "description": "1-3 observed error patterns (5-10 words each)",
            },
        },
        "required":             []any{"summary", "strengths", "weaknesses", "patterns"},
        "additionalProperties": false,
    },
}
```

### 5.6 Snapshot Persistence

The learner profile is stored in the existing `SnapshotData`:

```go
// Addition to internal/store/repo.go

type SnapshotData struct {
    // ... existing fields ...

    // LearnerProfile is the AI-generated learner summary.
    // Nil if no profile has been generated yet.
    LearnerProfile *LearnerProfileData `json:"learner_profile,omitempty"`
}

// LearnerProfileData is the serializable form of LearnerProfile.
type LearnerProfileData struct {
    Summary     string   `json:"summary"`
    Strengths   []string `json:"strengths"`
    Weaknesses  []string `json:"weaknesses"`
    Patterns    []string `json:"patterns"`
    GeneratedAt string   `json:"generated_at"` // RFC3339
}
```

### 5.7 Feeding Profile to Question Generation

The learner profile is optionally included in the question generation prompt. `GenerateInput` gains an optional field:

```go
// Addition to internal/problemgen/types.go

type GenerateInput struct {
    // ... existing fields ...

    // LearnerProfile is an optional AI-generated summary of the learner.
    // Included in the prompt when available for better personalization.
    LearnerProfile string
}
```

The problem generation user message template gains a new section:

```
{{if .LearnerProfile}}
Learner profile:
{{.LearnerProfile}}
{{end}}
```

This is appended after the "Recent errors" section. When no profile exists, the section is omitted entirely.

---

## 6. Persistence — Lesson & Hint Events

### 6.1 Hint Event

Hint usage is tracked for analytics (not for scoring). A hint event is appended when the learner views a hint.

```go
// Addition to internal/store/repo.go

// HintEventData records that a hint was shown to the learner.
type HintEventData struct {
    SessionID    string
    SkillID      string
    QuestionText string
    HintText     string
}

// Addition to EventRepo interface:
//
//     AppendHintEvent(ctx context.Context, data HintEventData) error
```

### 6.2 Lesson Event

```go
// LessonEventData records that a micro-lesson was generated and shown.
type LessonEventData struct {
    SessionID          string
    SkillID            string
    LessonTitle        string
    PracticeAttempted  bool   // Whether the learner tried the practice question
    PracticeCorrect    bool   // Whether the practice answer was correct
    PracticeSkipped    bool   // Whether the learner pressed q to skip
}

// Addition to EventRepo interface:
//
//     AppendLessonEvent(ctx context.Context, data LessonEventData) error
```

### 6.3 Ent Schemas

```go
// ent/schema/hint_event.go

package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
)

type HintEvent struct {
    ent.Schema
}

func (HintEvent) Mixin() []ent.Mixin {
    return []ent.Mixin{EventMixin{}}
}

func (HintEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("session_id"),
        field.String("skill_id"),
        field.String("question_text"),
        field.String("hint_text"),
    }
}
```

```go
// ent/schema/lesson_event.go

package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
)

type LessonEvent struct {
    ent.Schema
}

func (LessonEvent) Mixin() []ent.Mixin {
    return []ent.Mixin{EventMixin{}}
}

func (LessonEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("session_id"),
        field.String("skill_id"),
        field.String("lesson_title"),
        field.Bool("practice_attempted"),
        field.Bool("practice_correct"),
        field.Bool("practice_skipped"),
    }
}
```

---

## 7. Dependency Injection

### 7.1 app.Options

```go
// Additions to internal/app/options.go

type Options struct {
    // ... existing fields ...

    LessonService *lessons.Service
    Compressor    *lessons.Compressor
}
```

### 7.2 Wiring in cmd/play.go

```go
// In cmd/play.go, after creating the LLM provider:

var lessonService *lessons.Service
var compressor *lessons.Compressor
if provider != nil {
    lessonService = lessons.NewService(provider, lessons.DefaultConfig())
    compressor = lessons.NewCompressor(provider, lessons.DefaultCompressorConfig())
}

opts := app.Options{
    // ... existing fields ...
    LessonService: lessonService,
    Compressor:    compressor,
}
```

### 7.3 Session State Wiring

```go
// Additions to internal/session/state.go

type SessionState struct {
    // ... existing fields ...

    LessonService *lessons.Service
    Compressor    *lessons.Compressor
}
```

### 7.4 Home Screen

The home screen constructor gains the new services:

```go
// Updated signature for internal/screens/home/home.go

func New(
    generator problemgen.Generator,
    eventRepo store.EventRepo,
    snapRepo store.SnapshotRepo,
    diagService *diagnosis.Service,
    lessonService *lessons.Service,
    compressor *lessons.Compressor,
) *HomeScreen
```

---

## 8. Session Lifecycle Integration

### 8.1 HandleAnswer — Full Flow

After integrating hints, lessons, and compression, the wrong-answer path in `HandleAnswer` becomes:

```go
// In HandleAnswer, after determining answer is wrong:

// 1. Track per-skill wrong count.
state.WrongCountBySkill[q.SkillID]++

// 2. Build error context (with diagnosis from spec 09).
errCtx := BuildErrorContext(q, learnerAnswer, state.LastDiagnosis)
state.RecentErrors[q.SkillID] = append(state.RecentErrors[q.SkillID], errCtx)

// 3. Mark hint available (if question has a hint and not already shown).
if q.Hint != "" && !state.HintShown {
    state.HintAvailable = true
}

// 4. Trigger micro-lesson if 2+ wrong on this skill.
if state.WrongCountBySkill[q.SkillID] >= 2 && state.LessonService != nil {
    state.PendingLesson = true
    state.LessonService.RequestLesson(ctx, buildLessonInput(state, q, learnerAnswer))
}

// 5. Check compression threshold.
totalLen := 0
for _, e := range state.RecentErrors[q.SkillID] {
    totalLen += len(e)
}
if totalLen > SessionCompressionThreshold && state.Compressor != nil {
    state.Compressor.CompressErrors(ctx, q.SkillID, state.RecentErrors[q.SkillID],
        func(skillID string, summary string) {
            state.mu.Lock()
            defer state.mu.Unlock()
            state.RecentErrors[skillID] = []string{"[compressed] " + summary}
        },
    )
}
```

### 8.2 Feedback Done → Lesson Transition

When feedback is dismissed (any key after wrong answer feedback):

```go
func (m *SessionScreen) handleFeedbackDone() (tea.Model, tea.Cmd) {
    m.showingFeedback = false

    // Check if a lesson is ready.
    if m.sess.PendingLesson {
        if lesson, ok := m.sess.LessonService.ConsumeLesson(); ok {
            m.currentLesson = lesson
            m.showingLesson = true
            m.practiceState = practiceAnswering
            m.practiceInput.Reset()
            cmd := m.practiceInput.Focus()
            m.sess.PendingLesson = false
            return m, cmd
        }
        // Lesson not ready yet — don't block, proceed to next question.
        m.sess.PendingLesson = false
    }

    return m.advanceToNextQuestion()
}
```

### 8.3 Session End → Profile Generation

When the session completes (all slots done or learner quits):

```go
// In session completion handler:

if compressor != nil && snapRepo != nil {
    go func() {
        profile, err := compressor.GenerateProfile(ctx, buildProfileInput(state, snapRepo))
        if err != nil {
            return
        }
        // Save to snapshot.
        snap, _ := snapRepo.Latest(ctx)
        snap.LearnerProfile = &store.LearnerProfileData{
            Summary:     profile.Summary,
            Strengths:   profile.Strengths,
            Weaknesses:  profile.Weaknesses,
            Patterns:    profile.Patterns,
            GeneratedAt: time.Now().Format(time.RFC3339),
        }
        snapRepo.Save(ctx, snap)
    }()
}
```

---

## 9. LLM Purpose Labels

Each LLM call uses a distinct purpose label for logging and cost tracking:

| Call | Purpose Label |
|------|--------------|
| Question generation | `"question-gen"` (existing) |
| Error diagnosis | `"error-diagnosis"` (existing) |
| Hint display | No LLM call (hint from question) |
| Micro-lesson generation | `"lesson"` |
| Session error compression | `"session-compress"` |
| Learner profile generation | `"profile"` |

---

## 10. Package Structure

```
internal/
  lessons/
    types.go            # Lesson, PracticeQuestion, LearnerProfile, LessonInput, ProfileInput
    config.go           # Config, CompressorConfig, DefaultConfig(), DefaultCompressorConfig()
    service.go          # Service (async lesson generation)
    compress.go         # Compressor (session compression + profile generation)
    schema.go           # LessonSchema, SessionCompressionSchema, ProfileSchema
    prompt.go           # Prompt templates for lessons, compression, profiles
    service_test.go     # Service tests (mock provider)
    compress_test.go    # Compression + profile tests (mock provider)
    prompt_test.go      # Prompt template tests
ent/schema/
    hint_event.go       # HintEvent schema
    lesson_event.go     # LessonEvent schema
```

---

## 11. Testing Strategy

### 11.1 Lesson Service Tests

```go
func TestLessonService_GeneratesLesson(t *testing.T) {
    // Setup: MockProvider returns valid lesson JSON
    // Act: RequestLesson, poll ConsumeLesson
    // Assert: Lesson has title, explanation, worked example, practice question
}

func TestLessonService_ConsumeClearsLesson(t *testing.T) {
    // Act: Generate lesson, consume it, consume again
    // Assert: Second consume returns (nil, false)
}

func TestLessonService_NewRequestReplacesOld(t *testing.T) {
    // Act: Request two lessons in sequence
    // Assert: ConsumeLesson returns the second one
}

func TestLessonService_LLMError(t *testing.T) {
    // Setup: MockProvider returns error
    // Assert: ConsumeLesson returns (nil, false)
}
```

### 11.2 Compression Tests

```go
func TestCompressor_SessionCompression(t *testing.T) {
    // Setup: MockProvider returns compressed summary
    // Act: CompressErrors with 10 error strings
    // Assert: Callback receives "[compressed] " + summary
}

func TestCompressor_ThresholdNotMet(t *testing.T) {
    // Errors total < 800 chars → no compression triggered
}

func TestCompressor_ProfileGeneration(t *testing.T) {
    // Setup: MockProvider returns valid profile JSON
    // Act: GenerateProfile with session results and mastery data
    // Assert: Profile has summary, strengths, weaknesses, patterns
}

func TestCompressor_ProfileWithPrevious(t *testing.T) {
    // Setup: Provide a PreviousProfile in ProfileInput
    // Assert: Prompt includes previous profile for updating
}

func TestCompressor_ProfileFirstSession(t *testing.T) {
    // Setup: No PreviousProfile
    // Assert: Prompt omits previous profile section
}
```

### 11.3 Hint Flow Tests

```go
func TestHint_AvailableAfterWrongAnswer(t *testing.T) {
    // Submit wrong answer on question with hint
    // Assert: HintAvailable = true
}

func TestHint_NotAvailableOnCorrectAnswer(t *testing.T) {
    // Submit correct answer
    // Assert: HintAvailable = false
}

func TestHint_NotAvailableWhenNoHint(t *testing.T) {
    // Submit wrong answer on Prove tier question (no hint)
    // Assert: HintAvailable = false
}

func TestHint_ShownOnlyOnce(t *testing.T) {
    // Show hint, submit another wrong answer
    // Assert: HintAvailable = false (already shown)
}
```

### 11.4 Lesson Trigger Tests

```go
func TestLesson_TriggersAfterTwoWrong(t *testing.T) {
    // Submit 2 wrong answers on same skill
    // Assert: PendingLesson = true
}

func TestLesson_NoTriggerOnFirstWrong(t *testing.T) {
    // Submit 1 wrong answer
    // Assert: PendingLesson = false
}

func TestLesson_DifferentSkillsDontCrossCount(t *testing.T) {
    // 1 wrong on skill A, 1 wrong on skill B
    // Assert: No lesson triggered
}
```

### 11.5 Integration Tests

```go
func TestFullFlow_WrongAnswer_Hint_Lesson(t *testing.T) {
    // 1. Generate question (Learn tier, has hint)
    // 2. Submit wrong answer → hint becomes available
    // 3. View hint → overlay shown
    // 4. Submit wrong answer again → lesson triggered
    // 5. Lesson appears after feedback
    // 6. Answer practice question
    // 7. Session resumes with next question
}

func TestFullFlow_SessionCompression(t *testing.T) {
    // 1. Submit many wrong answers on one skill (>800 chars of errors)
    // 2. Verify compression callback fires
    // 3. Verify RecentErrors replaced with compressed summary
    // 4. Next question generation uses compressed context
}

func TestFullFlow_ProfileGeneration(t *testing.T) {
    // 1. Complete a session
    // 2. Verify profile generated and saved to snapshot
    // 3. Start new session
    // 4. Verify profile loaded and included in question generation
}
```

---

## 12. Example Flows

### 12.1 Hint Flow

```
Learner is on Learn tier for "add-3digit-regroup".
Question: "What is 567 + 285?"
Hint (pre-generated): "Try adding the ones first: 7 + 5 = 12. Write down 2 and carry 1."

→ Learner answers "842" (wrong, correct is 852)
→ Feedback: "Not quite. The answer is 852."
→ HintAvailable = true, key hints now show [h] hint
→ Learner presses 'h'
→ Hint overlay shows: "Try adding the ones first: 7 + 5 = 12. Write down 2 and carry 1."
→ HintEvent persisted
→ Learner presses any key → overlay dismissed
→ Learner answers "852" (correct)
→ Next question
```

### 12.2 Lesson Flow

```
Learner has 1 wrong on "add-3digit-regroup".
New question: "What is 348 + 479?"

→ Learner answers "717" (wrong, correct is 827)
→ WrongCountBySkill["add-3digit-regroup"] = 2 → lesson triggered
→ Feedback shown: "Not quite. The answer is 827."
→ Lesson generation starts (async)
→ Learner dismisses feedback
→ Lesson is ready → lesson screen shown:

  ╭─ Lesson: Carrying in Addition ──────────────────╮
  │                                                  │
  │  When you add numbers column by column, if the   │
  │  sum is 10 or more, you need to carry. ...       │
  │                                                  │
  │  ── Worked Example ──                            │
  │  Let's solve 48 + 35: ...                        │
  │                                                  │
  │  ── Your Turn ──                                 │
  │  What is 27 + 15?                                │
  │  > 42                                            │
  ╰──────────────────────────────────────────────────╯

→ Learner answers "42" (correct!)
→ "Correct!" feedback shown
→ LessonEvent persisted (practice_attempted=true, practice_correct=true)
→ Any key → session resumes with next question
```

### 12.3 Session Compression Flow

```
Skill "add-3digit-regroup" has 10 errors accumulated (total ~1200 chars):
  1. "Answered 842 for '567 + 285', correct was 852 [misconception: Forgot to carry/regroup]"
  2. "Answered 717 for '348 + 479', correct was 827 [misconception: Forgot to carry/regroup]"
  ... (8 more)

→ Threshold exceeded (1200 > 800)
→ Compression triggered
→ LLM produces: "Student consistently forgets to carry when adding columns that sum to 10+.
   Errors show correct column-by-column addition but missing the carry step,
   particularly in tens-to-hundreds. Understands basic addition facts."
→ RecentErrors["add-3digit-regroup"] replaced with:
   ["[compressed] Student consistently forgets to carry when adding columns..."]
→ Next question generation uses compact context (~50 tokens instead of ~300)
```

### 12.4 Learner Profile Flow

```
Session ends. Results:
  - add-3digit: 8/10 correct
  - mul-facts: 5/5 correct
  - frac-add: 2/6 correct

→ Profile generation triggered
→ LLM produces:
  Summary: "This student has strong multiplication facts and decent addition skills,
    but struggles significantly with fraction addition. They frequently add
    numerators and denominators straight across. Speed is generally appropriate."
  Strengths: ["solid multiplication facts", "good addition accuracy"]
  Weaknesses: ["fraction addition - straight-across error", "carrying in complex addition"]
  Patterns: ["adds fraction numerators and denominators separately"]

→ Profile saved to snapshot
→ Next session: profile included in question generation prompts
```

---

## 13. Dependencies

| Dependency | Direction | What |
|-----------|-----------|------|
| **LLM Integration (04)** | Uses | `llm.Provider` for lesson generation, compression, profile generation |
| **Problem Generation (05)** | Uses | `problemgen.Question`, `problemgen.CheckAnswer` for practice question checking |
| **Session Engine (06)** | Integrates | Hooks into `HandleAnswer` for hint/lesson triggers, session end for profile |
| **Mastery (07)** | Reads | `SkillMastery` data for profile generation |
| **Error Diagnosis (09)** | Reads | `DiagnosisResult` for lesson context |
| **Skill Graph (03)** | Uses | `skillgraph.Skill` for lesson context |
| **Persistence (02)** | Uses | `EventRepo` for hint/lesson events, `SnapshotRepo` for profile persistence |

---

## 14. Verification Checklist

### Hints
- [ ] Hint becomes available after first wrong answer on a question with a non-empty hint
- [ ] Hint is NOT available on correct answers
- [ ] Hint is NOT available on Prove tier questions (empty hint)
- [ ] Pressing `h` displays the hint overlay
- [ ] Hint can only be viewed once per question
- [ ] Key hints update to show `[h] hint` when available
- [ ] `HintEvent` persisted when hint is viewed
- [ ] `HintShown` and `HintAvailable` reset on new question

### Micro-Lessons
- [ ] Lesson triggers after 2+ wrong answers on the same skill in a session
- [ ] Lesson does NOT trigger on first wrong answer
- [ ] Wrong answers on different skills don't cross-count
- [ ] Lesson generation is async (doesn't block feedback)
- [ ] Lesson appears after feedback is dismissed (if ready)
- [ ] If lesson isn't ready when feedback is dismissed, session proceeds normally
- [ ] Lesson displays title, explanation, worked example, and practice question
- [ ] Practice question accepts answer input
- [ ] Practice answer checked using `problemgen.CheckAnswer` normalization
- [ ] Correct practice answer shows "Correct!" feedback
- [ ] Wrong practice answer shows correct answer + explanation
- [ ] Pressing `q` skips practice question
- [ ] `LessonEvent` persisted with practice_attempted, practice_correct, practice_skipped
- [ ] Session resumes after lesson

### Session Compression
- [ ] Compression triggers when per-skill error context exceeds 800 characters
- [ ] Compression is async (doesn't block answer flow)
- [ ] Compressed summary replaces verbose error strings in `RecentErrors`
- [ ] Compressed entries prefixed with `[compressed]`
- [ ] Compressed context used in subsequent question generation

### Learner Profile
- [ ] Profile generated at end of session
- [ ] Profile generation is async (doesn't block session exit)
- [ ] Profile includes summary, strengths, weaknesses, patterns
- [ ] Profile saved to `SnapshotData.LearnerProfile`
- [ ] Previous profile included in generation prompt for updates
- [ ] Profile fed into `GenerateInput.LearnerProfile` for question generation
- [ ] Missing profile (first session) handled gracefully

### General
- [ ] `LessonService` and `Compressor` wired through `app.Options`
- [ ] Home screen constructor accepts new services
- [ ] All three purpose labels registered: `"lesson"`, `"session-compress"`, `"profile"`
- [ ] Nil LLM provider → hints still work (no LLM needed), lessons and compression disabled
- [ ] Ent schemas for `HintEvent` and `LessonEvent` have all required fields
- [ ] All unit tests pass
- [ ] Integration tests validate full hint → lesson → compression → profile flow
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
- [ ] `go test ./internal/lessons/...` passes
