# 09 — Error Diagnosis

## 1. Overview

The Error Diagnosis module classifies wrong answers into meaningful categories — **careless**, **speed-rush**, or **misconception** — and tags misconceptions against a predefined taxonomy. This transforms undifferentiated wrong answers into actionable signals that improve question generation and slow mastery progression when systematic misunderstandings are detected.

**Design goals:**

- **Hybrid classification**: Fast rule-based classifiers run synchronously after each wrong answer (careless, speed-rush). When rules are inconclusive, an async LLM-based classifier diagnoses the error without blocking the session.
- **Predefined misconception taxonomy**: Each strand has 3–5 predefined common misconceptions. The LLM maps errors to taxonomy entries rather than generating free-form tags, ensuring aggregable, consistent data.
- **Internal-only**: Diagnosis results feed into question generation and mastery progression. The learner sees correct/incorrect + explanation, not the diagnosis itself. No shame mechanics.
- **Misconception slows mastery**: Each misconception-tagged error on a skill adds +1 to the number of correct answers needed to complete the current tier, creating a natural remediation loop.
- **Minimal blast radius**: The module is a pure analysis layer — it reads answer data and writes diagnosis events. It does not modify session flow, mastery state machines, or question generation directly.

### Consumers

| Module | How it uses Error Diagnosis |
|--------|----------------------------|
| **Session Engine (06)** | Calls `Diagnose()` after each wrong answer in `HandleAnswer` |
| **Mastery Service (07)** | Reads misconception penalty count to adjust tier completion thresholds |
| **Problem Generation (05)** | Receives enriched error context (diagnosis + misconception tag) in `GenerateInput.RecentErrors` |
| **Snapshot (02)** | Persists per-skill misconception counts in snapshot data |

---

## 2. Error Categories

Three mutually exclusive categories classify every wrong answer:

| Category | Code | Rule-based? | Description |
|----------|------|-------------|-------------|
| **Careless** | `careless` | Yes | Learner likely knew the answer but slipped. High historical accuracy (>80%) on this skill. |
| **Speed-rush** | `speed-rush` | Yes | Answer submitted too quickly (<2 seconds). Likely a rush or guess. |
| **Misconception** | `misconception` | LLM | Systematic misunderstanding — the error reflects a flawed mental model, not a slip. |

When both careless and speed-rush rules match (high accuracy AND under 2 seconds), **speed-rush takes priority** — speed is the more likely root cause.

If no rule matches and the LLM classifies an error, the result is always `misconception` — the LLM's role is specifically to identify *what* the misconception is, not to re-classify as careless/speed-rush.

If no rule matches and the LLM cannot identify a misconception, the category is `unclassified`. This is a valid outcome — not every wrong answer has a clear diagnosis.

```go
// internal/diagnosis/types.go

package diagnosis

// ErrorCategory classifies a wrong answer.
type ErrorCategory string

const (
    CategoryCareless      ErrorCategory = "careless"
    CategorySpeedRush     ErrorCategory = "speed-rush"
    CategoryMisconception ErrorCategory = "misconception"
    CategoryUnclassified  ErrorCategory = "unclassified"
)
```

---

## 3. Rule-Based Classifiers

### 3.1 Classifier Interface

```go
// internal/diagnosis/classifier.go

package diagnosis

// Classifier is a rule-based error classifier.
// Returns a category and confidence (0.0–1.0), or ("", 0) if the rule doesn't apply.
type Classifier interface {
    Name() string
    Classify(input *ClassifyInput) (ErrorCategory, float64)
}

// ClassifyInput holds the context for classification.
type ClassifyInput struct {
    Question       *problemgen.Question
    LearnerAnswer  string
    ResponseTimeMs int
    SkillAccuracy  float64 // Historical accuracy for this skill (0.0–1.0)
}
```

### 3.2 Speed-Rush Classifier

The simplest classifier: if the learner answered in under 2 seconds, it's a speed-rush regardless of accuracy.

```go
// internal/diagnosis/speed_rush.go

const SpeedRushThresholdMs = 2000

type SpeedRushClassifier struct{}

func (c *SpeedRushClassifier) Name() string { return "speed-rush" }

func (c *SpeedRushClassifier) Classify(input *ClassifyInput) (ErrorCategory, float64) {
    if input.ResponseTimeMs < SpeedRushThresholdMs {
        return CategorySpeedRush, 0.9
    }
    return "", 0
}
```

### 3.3 Careless Classifier

If the learner has >80% historical accuracy on this skill, the wrong answer is likely a slip rather than a gap in understanding.

```go
// internal/diagnosis/careless.go

const CarelessAccuracyThreshold = 0.80

type CarelessClassifier struct{}

func (c *CarelessClassifier) Name() string { return "careless" }

func (c *CarelessClassifier) Classify(input *ClassifyInput) (ErrorCategory, float64) {
    if input.SkillAccuracy > CarelessAccuracyThreshold {
        return CategoryCareless, 0.8
    }
    return "", 0
}
```

### 3.4 Classification Pipeline

Classifiers run in priority order. The first match wins.

```go
// internal/diagnosis/pipeline.go

// DefaultClassifiers returns classifiers in priority order.
func DefaultClassifiers() []Classifier {
    return []Classifier{
        &SpeedRushClassifier{}, // Highest priority
        &CarelessClassifier{},
    }
}

// RunClassifiers executes rule-based classifiers in order.
// Returns the first match, or ("", 0) if no rules apply.
func RunClassifiers(classifiers []Classifier, input *ClassifyInput) (ErrorCategory, float64, string) {
    for _, c := range classifiers {
        cat, conf := c.Classify(input)
        if cat != "" {
            return cat, conf, c.Name()
        }
    }
    return "", 0, ""
}
```

---

## 4. Misconception Taxonomy

### 4.1 Structure

Each misconception has a unique ID, a human-readable label, a description for the LLM prompt, and the strand it belongs to.

```go
// internal/diagnosis/taxonomy.go

package diagnosis

import "github.com/abhisek/mathiz/internal/skillgraph"

// Misconception defines a known misconception pattern.
type Misconception struct {
    ID          string           // Unique identifier, e.g. "npv-place-swap"
    Strand      skillgraph.Strand
    Label       string           // Short display label
    Description string           // Detailed description for LLM matching
    Examples    []string         // Example error patterns
}
```

### 4.2 Seed Taxonomy

The MVP taxonomy defines 3–5 misconceptions per strand (5 strands × ~4 each ≈ 20 total).

#### Number & Place Value

| ID | Label | Description |
|----|-------|-------------|
| `npv-place-swap` | Place value swap | Confuses ones/tens/hundreds positions; e.g., reads 305 as 350 |
| `npv-zero-placeholder` | Zero placeholder ignored | Drops or ignores zero in place value; e.g., 407 becomes 47 |
| `npv-compare-digits` | Digit-count comparison | Compares numbers by digit count alone; thinks 99 > 100 because "9 > 1" |
| `npv-rounding-direction` | Rounding direction | Rounds the wrong way; e.g., rounds 45 down to 40 instead of up to 50 |

#### Addition & Subtraction

| ID | Label | Description |
|----|-------|-------------|
| `add-no-carry` | Forgot to carry/regroup | Adds columns independently without carrying; e.g., 47 + 38 = 715 |
| `add-no-borrow` | Forgot to borrow/regroup | Subtracts smaller from larger in each column regardless of position; e.g., 42 - 17 = 35 |
| `add-sign-confusion` | Sign confusion | Adds when should subtract or vice versa |
| `add-left-to-right` | Left-to-right processing | Processes digits left to right incorrectly, mishandling carries |

#### Multiplication & Division

| ID | Label | Description |
|----|-------|-------------|
| `mul-add-confusion` | Multiplied instead of added (or vice versa) | Confuses multiplication with repeated addition; e.g., 4 × 3 = 7 |
| `mul-partial-product` | Partial product error | Forgets to add partial products in multi-digit multiplication |
| `div-remainder-ignore` | Remainder ignored | Drops the remainder entirely; e.g., 17 ÷ 5 = 3 instead of 3 R2 |
| `div-dividend-divisor-swap` | Dividend/divisor swap | Divides the smaller number by the larger; e.g., 6 ÷ 18 instead of 18 ÷ 6 |

#### Fractions

| ID | Label | Description |
|----|-------|-------------|
| `frac-add-straight` | Straight-across addition | Adds numerators and denominators separately; e.g., 1/2 + 1/3 = 2/5 |
| `frac-larger-denom` | Larger denominator = larger fraction | Thinks 1/8 > 1/4 because 8 > 4 |
| `frac-whole-number-compare` | Whole number comparison | Compares fractions as if they were whole numbers; e.g., 3/4 < 5/8 because 3 < 5 |
| `frac-simplify-error` | Simplification error | Reduces fractions incorrectly; e.g., simplifies 4/6 to 2/4 instead of 2/3 |

#### Measurement

| ID | Label | Description |
|----|-------|-------------|
| `meas-unit-confusion` | Unit confusion | Mixes up units; e.g., uses centimeters where meters are needed |
| `meas-conversion-direction` | Conversion direction | Multiplies when should divide (or vice versa) during unit conversion |
| `meas-perimeter-area` | Perimeter/area confusion | Computes perimeter when area is asked, or vice versa |

### 4.3 Taxonomy Registry

```go
// internal/diagnosis/taxonomy.go

// registry is the package-level misconception registry, keyed by ID.
var registry map[string]*Misconception

// byStrand indexes misconceptions by strand.
var byStrand map[skillgraph.Strand][]*Misconception

func init() {
    registry = make(map[string]*Misconception)
    byStrand = make(map[skillgraph.Strand][]*Misconception)
    for i := range seedMisconceptions {
        m := &seedMisconceptions[i]
        registry[m.ID] = m
        byStrand[m.Strand] = append(byStrand[m.Strand], m)
    }
}

// GetMisconception returns a misconception by ID, or nil if not found.
func GetMisconception(id string) *Misconception {
    return registry[id]
}

// MisconceptionsByStrand returns all misconceptions for a given strand.
func MisconceptionsByStrand(strand skillgraph.Strand) []*Misconception {
    return byStrand[strand]
}

// AllMisconceptions returns every misconception in the taxonomy.
func AllMisconceptions() []*Misconception {
    result := make([]*Misconception, 0, len(registry))
    for _, m := range registry {
        result = append(result, m)
    }
    return result
}
```

---

## 5. LLM-Assisted Diagnosis

### 5.1 When It Runs

The LLM diagnosis runs **asynchronously** (via a goroutine dispatched from the session engine) whenever:

1. A wrong answer occurs, AND
2. Rule-based classifiers return no match (neither careless nor speed-rush).

It runs on **every** unclassified wrong answer — no batching or pattern-matching threshold.

### 5.2 Diagnosis Request

The LLM receives the question context, the learner's wrong answer, the correct answer, and the list of possible misconceptions for the skill's strand. It must either match one misconception ID or respond with `null`.

```go
// internal/diagnosis/llm_diagnosis.go

package diagnosis

// DiagnosisRequest is sent to the LLM for misconception identification.
type DiagnosisRequest struct {
    SkillID       string   `json:"skill_id"`
    SkillName     string   `json:"skill_name"`
    QuestionText  string   `json:"question_text"`
    CorrectAnswer string   `json:"correct_answer"`
    LearnerAnswer string   `json:"learner_answer"`
    AnswerType    string   `json:"answer_type"`
    Candidates    []string `json:"candidates"` // Misconception IDs for this strand
}

// DiagnosisResponse is the structured LLM output.
type DiagnosisResponse struct {
    MisconceptionID *string `json:"misconception_id"` // null if no match
    Confidence      float64 `json:"confidence"`        // 0.0–1.0
    Reasoning       string  `json:"reasoning"`         // Brief explanation
}
```

### 5.3 Prompt Template

The prompt constrains the LLM to select from the taxonomy only:

```
You are an expert math education diagnostician. A learner answered a math question incorrectly. Your job is to determine if their error matches a known misconception pattern.

## Question
Skill: {{.SkillName}}
Question: {{.QuestionText}}
Correct answer: {{.CorrectAnswer}}
Learner's answer: {{.LearnerAnswer}}
Answer type: {{.AnswerType}}

## Known Misconceptions for This Strand
{{range .Candidates}}
- {{.ID}}: {{.Description}}
{{end}}

## Instructions
- If the learner's error clearly matches one of the listed misconceptions, return its ID.
- If the error does not match any listed misconception, return null for misconception_id.
- Do NOT invent new misconception IDs. Only use IDs from the list above.
- Provide a confidence score (0.0–1.0) reflecting how well the error matches.
- Keep reasoning to one sentence.
```

### 5.4 LLM Diagnoser

```go
// internal/diagnosis/llm_diagnosis.go

// Diagnoser performs async LLM-based misconception identification.
type Diagnoser struct {
    provider llm.Provider
    cfg      DiagnoserConfig
}

// DiagnoserConfig holds configuration for the LLM diagnoser.
type DiagnoserConfig struct {
    MaxTokens   int     // Default: 256
    Temperature float64 // Default: 0.3 (low for classification)
}

// DefaultDiagnoserConfig returns sensible defaults.
func DefaultDiagnoserConfig() DiagnoserConfig {
    return DiagnoserConfig{
        MaxTokens:   256,
        Temperature: 0.3,
    }
}

// NewDiagnoser creates an LLM-based diagnoser.
func NewDiagnoser(provider llm.Provider, cfg DiagnoserConfig) *Diagnoser {
    return &Diagnoser{provider: provider, cfg: cfg}
}

// Diagnose sends a wrong answer to the LLM for misconception identification.
// Returns a DiagnosisResult. This is safe to call from a goroutine.
func (d *Diagnoser) Diagnose(ctx context.Context, req *DiagnosisRequest) (*DiagnosisResult, error) {
    // 1. Build prompt from template + candidates
    // 2. Call LLM with structured JSON output schema
    // 3. Validate response: misconception_id must be in candidates or null
    // 4. Return DiagnosisResult
}
```

The LLM call uses `llm.WithPurpose("error-diagnosis")` for logging/tracking.

---

## 6. Diagnosis Result & Service

### 6.1 Result Type

```go
// internal/diagnosis/types.go

// DiagnosisResult is the output of classifying a wrong answer.
type DiagnosisResult struct {
    Category        ErrorCategory  // careless, speed-rush, misconception, unclassified
    MisconceptionID string         // Non-empty only when Category == misconception
    Confidence      float64        // 0.0–1.0
    ClassifierName  string         // Which classifier/LLM produced this result
    Reasoning       string         // LLM reasoning (empty for rule-based)
}
```

### 6.2 Service

The `Service` orchestrates rule-based classifiers and LLM diagnosis.

```go
// internal/diagnosis/service.go

package diagnosis

import (
    "context"
    "github.com/abhisek/mathiz/internal/llm"
    "github.com/abhisek/mathiz/internal/problemgen"
    "github.com/abhisek/mathiz/internal/store"
)

// Service coordinates error diagnosis.
type Service struct {
    classifiers []Classifier
    diagnoser   *Diagnoser     // nil if no LLM provider
    eventRepo   store.EventRepo
    pending     chan diagnosisJob // Buffered channel for async LLM jobs
}

type diagnosisJob struct {
    ctx   context.Context
    req   *DiagnosisRequest
    cb    func(*DiagnosisResult) // Callback with result
}

// NewService creates a diagnosis service.
func NewService(provider llm.Provider, eventRepo store.EventRepo) *Service {
    s := &Service{
        classifiers: DefaultClassifiers(),
        eventRepo:   eventRepo,
        pending:     make(chan diagnosisJob, 32),
    }
    if provider != nil {
        s.diagnoser = NewDiagnoser(provider, DefaultDiagnoserConfig())
        go s.processLoop()
    }
    return s
}

// Diagnose classifies a wrong answer. Rule-based classification is synchronous.
// If rules are inconclusive and an LLM is available, async LLM diagnosis is dispatched.
// Returns the synchronous result immediately; the callback (if provided) fires when
// the LLM result is ready.
func (s *Service) Diagnose(
    ctx context.Context,
    question *problemgen.Question,
    learnerAnswer string,
    responseTimeMs int,
    skillAccuracy float64,
    cb func(*DiagnosisResult),
) *DiagnosisResult {
    input := &ClassifyInput{
        Question:       question,
        LearnerAnswer:  learnerAnswer,
        ResponseTimeMs: responseTimeMs,
        SkillAccuracy:  skillAccuracy,
    }

    // Phase 1: Rule-based (synchronous).
    cat, conf, name := RunClassifiers(s.classifiers, input)
    if cat != "" {
        return &DiagnosisResult{
            Category:       cat,
            Confidence:     conf,
            ClassifierName: name,
        }
    }

    // Phase 2: LLM (async).
    if s.diagnoser != nil {
        s.dispatchLLM(ctx, question, learnerAnswer, cb)
    }

    // Return unclassified immediately; LLM result arrives via callback.
    return &DiagnosisResult{
        Category:       CategoryUnclassified,
        Confidence:     0,
        ClassifierName: "none",
    }
}

func (s *Service) dispatchLLM(
    ctx context.Context,
    q *problemgen.Question,
    learnerAnswer string,
    cb func(*DiagnosisResult),
) {
    skill, err := skillgraph.GetSkill(q.SkillID)
    if err != nil {
        return
    }

    candidates := MisconceptionsByStrand(skill.Strand)
    candidateIDs := make([]string, len(candidates))
    for i, c := range candidates {
        candidateIDs[i] = c.ID
    }

    req := &DiagnosisRequest{
        SkillID:       q.SkillID,
        SkillName:     skill.Name,
        QuestionText:  q.Text,
        CorrectAnswer: q.Answer,
        LearnerAnswer: learnerAnswer,
        AnswerType:    string(q.AnswerType),
        Candidates:    candidateIDs,
    }

    select {
    case s.pending <- diagnosisJob{ctx: ctx, req: req, cb: cb}:
    default:
        // Channel full — drop diagnosis silently. Not critical.
    }
}

func (s *Service) processLoop() {
    for job := range s.pending {
        resp, err := s.diagnoser.Diagnose(job.ctx, job.req)
        if err != nil || resp == nil {
            continue
        }
        if job.cb != nil {
            job.cb(resp)
        }
    }
}

// Close shuts down the async processing loop.
func (s *Service) Close() {
    close(s.pending)
}
```

---

## 7. Mastery Integration — Misconception Penalty

### 7.1 Mechanism

When a wrong answer is diagnosed as a **misconception**, the mastery service adds **+1 to the number of correct answers required** to complete the current tier for that skill. This means:

- Default Learn tier: 8 problems, 75% accuracy → 6 correct needed.
- After 1 misconception error: 7 correct needed.
- After 2 misconception errors: 8 correct needed.
- And so on.

This creates a natural remediation loop — the learner must demonstrate more correct answers on the same skill to overcome the misconception.

### 7.2 Tracking

The misconception penalty count is tracked per-skill in `SkillMastery`:

```go
// Addition to internal/mastery/mastery.go

type SkillMastery struct {
    // ... existing fields ...

    // MisconceptionPenalty is the number of extra correct answers required
    // to complete the current tier, incremented by misconception diagnoses.
    // Reset to 0 on tier advancement.
    MisconceptionPenalty int
}
```

### 7.3 Adjusted Tier Completion Check

The existing `IsTierComplete` method is modified to account for the penalty:

```go
// Modified in internal/mastery/mastery.go

func (sm *SkillMastery) IsTierComplete(cfg skillgraph.TierConfig) bool {
    if sm.TotalAttempts < cfg.ProblemsRequired {
        return false
    }
    requiredCorrect := int(float64(cfg.ProblemsRequired) * cfg.AccuracyThreshold)
    requiredCorrect += sm.MisconceptionPenalty
    return sm.CorrectCount >= requiredCorrect
}
```

### 7.4 Penalty Reset

The penalty resets to 0 when a tier is advanced (Learn → Prove, Prove → Mastered, recovery complete). This ensures the penalty doesn't carry over to the next tier.

### 7.5 Snapshot Persistence

Add `MisconceptionPenalty` to `SkillMasteryData` in the store package:

```go
// Addition to internal/store/repo.go

type SkillMasteryData struct {
    // ... existing fields ...

    MisconceptionPenalty int `json:"misconception_penalty,omitempty"`
}
```

---

## 8. Session Engine Integration

### 8.1 HandleAnswer Hook

After `HandleAnswer` determines the answer is wrong, it calls the diagnosis service:

```go
// Modified in internal/session/session.go

// In HandleAnswer, after the error tracking block (line ~44):
if !correct && state.DiagnosisService != nil {
    responseTimeMs := int(time.Since(state.QuestionStartTime).Milliseconds())
    skillAccuracy, _ := state.EventRepo.SkillAccuracy(context.Background(), q.SkillID)

    result := state.DiagnosisService.Diagnose(
        context.Background(),
        q,
        learnerAnswer,
        responseTimeMs,
        skillAccuracy,
        func(asyncResult *diagnosis.DiagnosisResult) {
            // LLM result arrived — update diagnosis event and mastery penalty.
            if asyncResult.Category == diagnosis.CategoryMisconception {
                state.ApplyMisconceptionPenalty(q.SkillID)
                state.UpdateErrorContext(q.SkillID, asyncResult)
            }
            // Persist async diagnosis event.
            state.AppendDiagnosisEvent(q, learnerAnswer, asyncResult)
        },
    )

    // Apply synchronous result immediately.
    state.LastDiagnosis = result
}
```

### 8.2 Enriched Error Context

Replace the plain-text error context with diagnosis-enriched context for the LLM:

```go
// Modified BuildErrorContext in internal/session/session.go

func BuildErrorContext(question *problemgen.Question, learnerAnswer string, diag *diagnosis.DiagnosisResult) string {
    base := fmt.Sprintf(
        "Answered %s for '%s', correct answer was %s",
        learnerAnswer,
        question.Text,
        question.Answer,
    )
    if diag == nil || diag.Category == diagnosis.CategoryUnclassified {
        return base
    }
    enriched := fmt.Sprintf("%s [%s", base, diag.Category)
    if diag.MisconceptionID != "" {
        m := diagnosis.GetMisconception(diag.MisconceptionID)
        if m != nil {
            enriched += ": " + m.Label
        }
    }
    enriched += "]"
    return enriched
}
```

Example output: `"Answered 715 for '47 + 38', correct answer was 85 [misconception: Forgot to carry/regroup]"`

### 8.3 SessionState Additions

```go
// Additions to internal/session/state.go

type SessionState struct {
    // ... existing fields ...

    DiagnosisService *diagnosis.Service  // nil if diagnosis disabled
    LastDiagnosis    *diagnosis.DiagnosisResult // Most recent diagnosis
}

// ApplyMisconceptionPenalty increments the penalty for a skill.
func (s *SessionState) ApplyMisconceptionPenalty(skillID string) {
    if s.MasteryService != nil {
        sm := s.MasteryService.GetMastery(skillID)
        sm.MisconceptionPenalty++
    }
}
```

---

## 9. Persistence — Diagnosis Event

### 9.1 Ent Schema

```go
// ent/schema/diagnosis_event.go

package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
)

// DiagnosisEvent holds a diagnosis result for a single wrong answer.
type DiagnosisEvent struct {
    ent.Schema
}

func (DiagnosisEvent) Mixin() []ent.Mixin {
    return []ent.Mixin{
        EventMixin{},
    }
}

func (DiagnosisEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("session_id"),
        field.String("skill_id"),
        field.String("question_text"),
        field.String("correct_answer"),
        field.String("learner_answer"),
        field.String("category"),          // careless, speed-rush, misconception, unclassified
        field.String("misconception_id").
            Optional().
            Nillable(),                    // Only set for misconception category
        field.Float("confidence"),
        field.String("classifier_name"),   // Which classifier produced this
        field.String("reasoning").
            Optional().
            Default(""),                   // LLM reasoning, empty for rule-based
    }
}
```

### 9.2 EventRepo Extension

```go
// Addition to internal/store/repo.go

// DiagnosisEventData captures a diagnosis result for persistence.
type DiagnosisEventData struct {
    SessionID       string
    SkillID         string
    QuestionText    string
    CorrectAnswer   string
    LearnerAnswer   string
    Category        string
    MisconceptionID *string
    Confidence      float64
    ClassifierName  string
    Reasoning       string
}

// Addition to EventRepo interface:
//
//     AppendDiagnosisEvent(ctx context.Context, data DiagnosisEventData) error
//
//     SkillMisconceptions(ctx context.Context, skillID string) ([]DiagnosisEventData, error)
```

---

## 10. Dependency Injection

### 10.1 app.Options

Add the diagnosis service to the options struct:

```go
// Addition to internal/app/options.go

type Options struct {
    // ... existing fields ...

    DiagnosisService *diagnosis.Service
}
```

### 10.2 Wiring in cmd/play.go

```go
// In cmd/play.go, after creating the LLM provider and event repo:

var diagService *diagnosis.Service
if provider != nil {
    diagService = diagnosis.NewService(provider, eventRepo)
    defer diagService.Close()
}

opts := app.Options{
    // ... existing fields ...
    DiagnosisService: diagService,
}
```

---

## 11. Package Structure

```
internal/diagnosis/
├── types.go            # ErrorCategory, DiagnosisResult, ClassifyInput
├── classifier.go       # Classifier interface, RunClassifiers, DefaultClassifiers
├── speed_rush.go       # SpeedRushClassifier
├── careless.go         # CarelessClassifier
├── taxonomy.go         # Misconception type, registry, seed data
├── taxonomy_seed.go    # seedMisconceptions slice (all 19 entries)
├── llm_diagnosis.go    # Diagnoser, DiagnosisRequest/Response, prompt template
├── service.go          # Service (orchestrator), async processing loop
├── service_test.go     # Service tests
├── classifier_test.go  # Rule-based classifier tests
├── taxonomy_test.go    # Taxonomy registry tests
└── llm_diagnosis_test.go # LLM diagnoser tests (with mock provider)
```

---

## 12. Testing Strategy

### 12.1 Unit Tests — Classifiers

```go
func TestSpeedRushClassifier(t *testing.T) {
    c := &SpeedRushClassifier{}

    // Under threshold → speed-rush
    cat, conf := c.Classify(&ClassifyInput{ResponseTimeMs: 1500})
    assert(cat == CategorySpeedRush)
    assert(conf == 0.9)

    // Over threshold → no match
    cat, _ = c.Classify(&ClassifyInput{ResponseTimeMs: 3000})
    assert(cat == "")
}

func TestCarelessClassifier(t *testing.T) {
    c := &CarelessClassifier{}

    // High accuracy → careless
    cat, _ := c.Classify(&ClassifyInput{SkillAccuracy: 0.85})
    assert(cat == CategoryCareless)

    // Low accuracy → no match
    cat, _ = c.Classify(&ClassifyInput{SkillAccuracy: 0.60})
    assert(cat == "")
}

func TestClassifierPriority(t *testing.T) {
    // Both speed-rush AND careless match → speed-rush wins
    input := &ClassifyInput{
        ResponseTimeMs: 1000,
        SkillAccuracy:  0.90,
    }
    cat, _, _ := RunClassifiers(DefaultClassifiers(), input)
    assert(cat == CategorySpeedRush)
}
```

### 12.2 Unit Tests — Taxonomy

```go
func TestTaxonomyRegistry(t *testing.T) {
    // All misconceptions are registered.
    all := AllMisconceptions()
    assert(len(all) == 19)

    // Lookup by ID.
    m := GetMisconception("add-no-carry")
    assert(m != nil)
    assert(m.Strand == skillgraph.StrandAddSub)

    // Lookup by strand.
    addSub := MisconceptionsByStrand(skillgraph.StrandAddSub)
    assert(len(addSub) == 4)
}
```

### 12.3 Unit Tests — Service

```go
func TestServiceRuleBasedPath(t *testing.T) {
    svc := NewService(nil, nil) // No LLM provider

    result := svc.Diagnose(ctx, question, "wrong", 1500, 0.50, nil)
    assert(result.Category == CategorySpeedRush) // Under 2s
}

func TestServiceLLMFallback(t *testing.T) {
    mock := llm.NewMockProvider(...)
    svc := NewService(mock, nil)
    defer svc.Close()

    var asyncResult *DiagnosisResult
    done := make(chan struct{})
    cb := func(r *DiagnosisResult) {
        asyncResult = r
        close(done)
    }

    // Slow answer, low accuracy → rules don't match → LLM dispatched.
    result := svc.Diagnose(ctx, question, "715", 5000, 0.40, cb)
    assert(result.Category == CategoryUnclassified) // Sync result

    <-done
    assert(asyncResult.Category == CategoryMisconception)
    assert(asyncResult.MisconceptionID == "add-no-carry")
}
```

### 12.4 Unit Tests — Mastery Penalty

```go
func TestMisconceptionPenaltySlowsTierCompletion(t *testing.T) {
    sm := &SkillMastery{
        TotalAttempts:        8,
        CorrectCount:         6, // 75% of 8
        MisconceptionPenalty: 1,
    }
    cfg := skillgraph.TierConfig{ProblemsRequired: 8, AccuracyThreshold: 0.75}

    // Without penalty: 6 correct needed → complete.
    // With 1 penalty: 7 correct needed → NOT complete.
    assert(!sm.IsTierComplete(cfg))

    sm.CorrectCount = 7
    assert(sm.IsTierComplete(cfg))
}
```

### 12.5 Integration Test

```go
func TestDiagnosisInSessionFlow(t *testing.T) {
    // 1. Set up session with diagnosis service (mock LLM).
    // 2. Submit wrong answer with slow time + low accuracy.
    // 3. Verify rule-based returns unclassified.
    // 4. Wait for async LLM callback.
    // 5. Verify misconception tagged.
    // 6. Verify MisconceptionPenalty incremented.
    // 7. Verify enriched error context passed to next GenerateInput.
    // 8. Verify DiagnosisEvent persisted.
}
```

---

## 13. Example Flows

### 13.1 Careless Error

```
Learner has 90% accuracy on "add-3digit".
Question: "What is 345 + 278?"
Learner answers: "613" (transposed digits)
Response time: 8 seconds

→ SpeedRushClassifier: 8000ms ≥ 2000ms → no match
→ CarelessClassifier: 90% > 80% → MATCH
→ Result: {Category: "careless", Confidence: 0.8}
→ No mastery penalty applied.
→ Error context: "Answered 613 for '345 + 278', correct answer was 623 [careless]"
```

### 13.2 Speed-Rush Error

```
Learner has 90% accuracy on "mul-facts".
Question: "What is 7 × 8?"
Learner answers: "54"
Response time: 900ms

→ SpeedRushClassifier: 900ms < 2000ms → MATCH (priority)
→ CarelessClassifier: not reached (speed-rush already matched)
→ Result: {Category: "speed-rush", Confidence: 0.9}
→ No mastery penalty applied.
→ Error context: "Answered 54 for '7 × 8', correct answer was 56 [speed-rush]"
```

### 13.3 Misconception Error

```
Learner has 40% accuracy on "add-3digit-regroup".
Question: "What is 47 + 38?"
Learner answers: "715"
Response time: 12 seconds

→ SpeedRushClassifier: 12000ms ≥ 2000ms → no match
→ CarelessClassifier: 40% ≤ 80% → no match
→ Sync result: {Category: "unclassified"}
→ LLM diagnosis dispatched (async)...
→ LLM responds: {misconception_id: "add-no-carry", confidence: 0.92, reasoning: "Added 7+8=15 and 4+3=7 without carrying the 1"}
→ Async result: {Category: "misconception", MisconceptionID: "add-no-carry"}
→ MisconceptionPenalty for "add-3digit-regroup" incremented to 1
→ Enriched error context: "Answered 715 for '47 + 38', correct answer was 85 [misconception: Forgot to carry/regroup]"
```

### 13.4 Unclassified Error

```
Learner has 55% accuracy on "frac-compare".
Question: "Which is larger: 3/4 or 5/8?"
Learner answers: "5/8"
Response time: 15 seconds

→ SpeedRushClassifier: no match
→ CarelessClassifier: no match
→ LLM diagnosis dispatched...
→ LLM responds: {misconception_id: null, confidence: 0.3, reasoning: "Error could be multiple causes"}
→ Final result: {Category: "unclassified"}
→ No mastery penalty applied.
```

---

## 14. Dependencies

| Dependency | Direction | What |
|-----------|-----------|------|
| **LLM Integration (04)** | Uses | `llm.Provider` for async misconception diagnosis |
| **Problem Generation (05)** | Uses | `problemgen.Question` for answer context |
| **Session Engine (06)** | Integrates | Hooks into `HandleAnswer` for diagnosis dispatch |
| **Mastery (07)** | Integrates | Adds misconception penalty to tier completion |
| **Skill Graph (03)** | Uses | `skillgraph.Skill`, `skillgraph.Strand` for taxonomy lookup |
| **Persistence (02)** | Uses | `EventRepo` for diagnosis event storage |

---

## 15. Open Questions

1. **Penalty cap**: Should the misconception penalty have a maximum (e.g., +3) to prevent infinite tier extension? *Recommendation: cap at +3 for MVP.*
2. **Cross-session misconceptions**: Should misconception counts persist across sessions (via snapshot) or reset per session? *Recommendation: persist in snapshot; the penalty reflects cumulative misconception evidence.*
3. **Taxonomy expansion**: Process for adding new misconceptions post-MVP — manual additions to seed data, or an admin flow? *Deferred to post-MVP.*

---

## 16. Verification Checklist

- [ ] `SpeedRushClassifier` returns `speed-rush` for answers under 2 seconds
- [ ] `CarelessClassifier` returns `careless` for skills with >80% accuracy
- [ ] Speed-rush takes priority when both rules match
- [ ] Taxonomy registers all 19 misconceptions across 5 strands
- [ ] `GetMisconception` returns correct entries by ID
- [ ] `MisconceptionsByStrand` returns 3–4 entries per strand
- [ ] LLM diagnoser sends structured request with strand-specific candidates
- [ ] LLM response validated: misconception_id must be in candidates or null
- [ ] Async LLM diagnosis does not block session flow
- [ ] `Diagnose` returns synchronous rule-based result immediately
- [ ] Async callback fires with LLM result
- [ ] `MisconceptionPenalty` increments on misconception diagnosis
- [ ] `IsTierComplete` accounts for misconception penalty
- [ ] Penalty resets to 0 on tier advancement
- [ ] Enriched error context includes diagnosis category and misconception label
- [ ] `DiagnosisEvent` ent schema has all required fields
- [ ] `AppendDiagnosisEvent` persists to database
- [ ] `DiagnosisService` wired through `app.Options`
- [ ] `HandleAnswer` calls diagnosis service on wrong answers
- [ ] `BuildErrorContext` produces enriched strings for problem generation
- [ ] Service gracefully handles nil LLM provider (rule-based only)
- [ ] Dropped async jobs (full channel) don't cause errors
- [ ] `Close()` shuts down processing loop cleanly
- [ ] All unit tests pass
- [ ] Integration test validates full session flow with diagnosis
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
