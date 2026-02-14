# 05 — Problem Generation

## 1. Overview

The Problem Generation module is the engine that produces all math questions in Mathiz. It uses the LLM Integration layer (spec 04) to generate contextually appropriate questions for a given skill, tier, and learner state. There is **no fixed question bank** — every question is generated on demand by the LLM, validated programmatically, and served to the session engine.

**Design goals:**

- **On-demand generation**: One question at a time, generated just before it's shown. Simple, stateless, no pre-caching.
- **Skill-aware**: Every question is grounded in a specific skill's metadata (name, description, keywords, grade, tier config).
- **Context-enriched**: Recent errors and session history are included in the prompt so the LLM can vary questions and target weak spots.
- **Validated**: Every generated question undergoes programmatic validation — JSON schema conformance (via spec 04), answer correctness verification, and structural checks.
- **Deduplicated**: Previously asked questions are passed in the prompt to prevent repetition within a session.
- **Dual format**: Questions can be **numeric input** (learner types the answer) or **multiple choice** (learner picks from 4 options). The LLM chooses the format based on the skill and question.
- **Plain text math**: All questions use ASCII plain text (`345 + 278 = ?`, `3/4 + 1/2 = ?`). No LaTeX, no Unicode math symbols.

### Consumers

| Module | How it uses Problem Generation |
|--------|-------------------------------|
| **Session Engine (06)** | Calls `Generate()` to get the next question during a session |
| **Diagnostic Placement (03)** | Calls `Generate()` to produce probe questions during initial placement |

---

## 2. Core Types

### Question

```go
// internal/problemgen/types.go

package problemgen

// Question represents a generated math question ready for display.
type Question struct {
    // Text is the question prompt displayed to the learner.
    // Plain ASCII text, e.g. "What is 345 + 278?" or "Which fraction is larger: 3/4 or 2/3?"
    Text string

    // Format indicates how the learner answers this question.
    Format AnswerFormat

    // Answer is the canonical correct answer as a string.
    // For numeric: "623", "0.75", "3/4"
    // For multiple choice: the text of the correct option (e.g. "3/4")
    Answer string

    // AnswerType describes the numeric type of the answer for validation.
    AnswerType AnswerType

    // Choices is populated only when Format is FormatMultipleChoice.
    // Contains exactly 4 options, one of which matches Answer.
    Choices []string

    // Hint is an optional short hint the learner can request (Learn tier only).
    // Empty string if no hint was generated.
    Hint string

    // Difficulty is the LLM's self-assessed difficulty (1-5).
    // Used for analytics, not for gating.
    Difficulty int

    // Explanation is a brief worked solution shown after the learner answers.
    // Always present.
    Explanation string

    // SkillID is the skill this question was generated for.
    SkillID string

    // Tier is the tier this question was generated for.
    Tier skillgraph.Tier
}
```

### Answer Types

```go
// AnswerType describes the numeric representation of the correct answer.
type AnswerType string

const (
    AnswerTypeInteger  AnswerType = "integer"   // e.g. "623", "-15"
    AnswerTypeDecimal  AnswerType = "decimal"    // e.g. "3.75", "0.5"
    AnswerTypeFraction AnswerType = "fraction"   // e.g. "3/4", "7/2"
)
```

### Answer Format

```go
// AnswerFormat describes how the learner provides their answer.
type AnswerFormat string

const (
    // FormatNumeric means the learner types a numeric answer.
    FormatNumeric AnswerFormat = "numeric"

    // FormatMultipleChoice means the learner picks from 4 choices.
    FormatMultipleChoice AnswerFormat = "multiple_choice"
)
```

### Generation Context

```go
// GenerateInput holds all context needed to generate a question.
type GenerateInput struct {
    // Skill is the target skill for the question.
    Skill skillgraph.Skill

    // Tier is the difficulty tier (Learn or Prove).
    Tier skillgraph.Tier

    // PriorQuestions contains the Text of questions already asked in this
    // session for this skill. Used for deduplication in the prompt.
    PriorQuestions []string

    // RecentErrors contains descriptions of the learner's recent mistakes
    // on this skill (e.g. "answered 623 for 345 + 289, correct was 634").
    // Up to 5 most recent errors. Empty slice if no history.
    RecentErrors []string
}
```

---

## 3. Validator Interface

The problem generation module uses a **pluggable validator pattern**. All validation — including the built-in structural checks and math verification — is implemented behind a common interface. Callers and other components can register their own validators.

### Interface

```go
// internal/problemgen/validator.go

package problemgen

// Validator checks a generated question for correctness.
// Implementations should be stateless and safe for concurrent use.
type Validator interface {
    // Name returns a short identifier for this validator (for error messages
    // and logging), e.g. "structural", "math-check", "answer-format".
    Name() string

    // Validate checks the question and returns nil if it passes.
    // Returns a ValidationError if the question fails the check.
    // The validator receives the full GenerateInput for context (e.g., to
    // know which skill/tier the question was generated for).
    Validate(q *Question, input GenerateInput) *ValidationError
}

// ValidationError describes why a question failed validation.
type ValidationError struct {
    Validator string // Name of the validator that failed
    Message   string // Human-readable description of the failure
    Retryable bool   // Whether regeneration is likely to fix this
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validator %q: %s", e.Validator, e.Message)
}
```

### Built-in Validators

The module ships with three built-in validators that cover the core validation pipeline. They all implement the `Validator` interface.

```go
// StructuralValidator checks that required fields are present, within
// length limits, and have valid enum values.
type StructuralValidator struct{}

// AnswerFormatValidator checks that the answer string matches the declared
// answer_type (integer, decimal, fraction) and that multiple choice
// constraints are satisfied.
type AnswerFormatValidator struct{}

// MathCheckValidator attempts to independently recompute the answer from
// the question text. Covers ~70% of MVP skills (pure arithmetic and
// fraction operations). Non-computable questions pass through silently.
type MathCheckValidator struct{}
```

### Custom Validators

Other components can implement `Validator` to add domain-specific checks. Examples:

```go
// A hypothetical grade-appropriateness validator from the session engine:
type GradeRangeValidator struct {
    MinGrade int
    MaxGrade int
}

func (v *GradeRangeValidator) Name() string { return "grade-range" }

func (v *GradeRangeValidator) Validate(q *Question, input GenerateInput) *ValidationError {
    if q.Difficulty < v.MinGrade || q.Difficulty > v.MaxGrade {
        return &ValidationError{
            Validator: v.Name(),
            Message:   fmt.Sprintf("difficulty %d outside allowed range [%d, %d]", q.Difficulty, v.MinGrade, v.MaxGrade),
            Retryable: true,
        }
    }
    return nil
}
```

---

## 4. Generator Config & Factory

### Config

```go
// internal/problemgen/config.go

// Config controls the behavior of the LLMGenerator.
type Config struct {
    // Validators is the ordered list of validators to run on every
    // generated question. They execute in order; the first failure
    // stops the pipeline.
    Validators []Validator

    // MaxTokens is the token budget for the LLM response.
    MaxTokens int

    // Temperature controls LLM output randomness (0.0-1.0).
    Temperature float64

    // MaxPriorQuestions is the maximum number of prior questions
    // to include in the prompt for deduplication.
    MaxPriorQuestions int

    // MaxRecentErrors is the maximum number of recent errors
    // to include in the prompt for context.
    MaxRecentErrors int
}

// DefaultConfig returns a Config with the standard validator chain
// and recommended defaults.
func DefaultConfig() Config {
    return Config{
        Validators: []Validator{
            &StructuralValidator{},
            &AnswerFormatValidator{},
            &MathCheckValidator{},
        },
        MaxTokens:        512,
        Temperature:      0.7,
        MaxPriorQuestions: 8,
        MaxRecentErrors:  5,
    }
}
```

Callers can customize the config before passing it to `New`:

```go
// Use default config:
gen := problemgen.New(provider, problemgen.DefaultConfig())

// Add a custom validator:
cfg := problemgen.DefaultConfig()
cfg.Validators = append(cfg.Validators, &myCustomValidator{})
gen := problemgen.New(provider, cfg)

// Remove math check (e.g., for a word-problem-only session):
cfg := problemgen.DefaultConfig()
cfg.Validators = cfg.Validators[:2] // Keep structural + answer-format only
gen := problemgen.New(provider, cfg)

// Override temperature:
cfg := problemgen.DefaultConfig()
cfg.Temperature = 0.5
gen := problemgen.New(provider, cfg)
```

---

## 5. Generator Interface & Implementation

### Interface

```go
// internal/problemgen/generator.go

package problemgen

import "context"

// Generator produces math questions using an LLM provider.
type Generator interface {
    // Generate produces a single question for the given input context.
    // Returns a validated Question or an error.
    // All configured validators are run before returning.
    Generate(ctx context.Context, input GenerateInput) (*Question, error)
}
```

### Implementation

```go
// internal/problemgen/llm_generator.go

// LLMGenerator implements Generator using the LLM provider.
type LLMGenerator struct {
    provider llm.Provider
    config   Config
}

// New creates a new LLMGenerator with the given provider and config.
func New(provider llm.Provider, cfg Config) *LLMGenerator {
    return &LLMGenerator{provider: provider, config: cfg}
}
```

The `LLMGenerator.Generate()` method:

1. Builds the system prompt and user message from `GenerateInput` (respecting `config.MaxPriorQuestions` and `config.MaxRecentErrors`)
2. Calls `provider.Generate()` with the question schema, `config.MaxTokens`, and `config.Temperature`
3. Parses the JSON response into a raw `questionOutput` struct
4. Converts to the `Question` type
5. Runs each `config.Validators[i].Validate(q, input)` in order; returns the first `ValidationError` if any fails
6. Returns the validated `Question`

---

## 6. Prompt Design

### System Prompt

The system prompt is constant across all question generation calls:

```
You are a math tutor creating practice problems for children in grades 3-5.

Rules:
- Generate a single math problem appropriate for the given skill, grade, and difficulty tier.
- Use plain ASCII text for all math. No LaTeX, no Unicode symbols. Use / for fractions, * for multiplication, and standard operators.
- The question text should be clear, self-contained, and age-appropriate.
- The answer must be correct and in the simplest form (reduce fractions, no trailing zeros on decimals).
- The explanation should show the solution step by step, suitable for a child.
- Choose "numeric" format for computation problems (the student types the answer).
- Choose "multiple_choice" format for conceptual, comparison, or identification problems (the student picks from 4 options).
- For multiple choice, provide exactly 4 options where exactly one is correct. Distractors should reflect common mistakes, not random values.
- If the difficulty tier is "learn", include a helpful hint. If "prove", leave the hint empty.
- Do not repeat any question from the "already asked" list.
```

### User Message Template

```
Skill: {skill.Name}
Description: {skill.Description}
Grade: {skill.GradeLevel}
Keywords: {skill.Keywords joined by ", "}
Tier: {tier label — "learn" or "prove"}
Hints allowed: {tier.HintsAllowed — true/false}

Already asked in this session:
{numbered list of PriorQuestions, or "None" if empty}

Recent errors by this student:
{numbered list of RecentErrors, or "None" if empty}
```

### Token Budget

| Field | Notes |
|-------|-------|
| `MaxTokens` | 512 |
| `Temperature` | 0.7 (for variety) |

The system prompt is ~200 tokens. The user message varies from ~50 tokens (no history) to ~300 tokens (with 8 prior questions and 5 errors). Total input stays well under 1K tokens. Output is typically 150-300 tokens.

---

## 7. JSON Schema

The LLM is constrained to return JSON matching this schema via the provider's native structured output mechanism.

```go
var QuestionSchema = &llm.Schema{
    Name:        "math-question",
    Description: "A single math practice question with answer and explanation",
    Definition: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "question_text": map[string]any{
                "type":        "string",
                "description": "The question prompt shown to the learner, in plain ASCII text",
            },
            "format": map[string]any{
                "type":        "string",
                "enum":        []any{"numeric", "multiple_choice"},
                "description": "How the learner answers: type a number or pick from choices",
            },
            "answer": map[string]any{
                "type":        "string",
                "description": "The correct answer. For numeric: the number as a string. For MC: the text of the correct option.",
            },
            "answer_type": map[string]any{
                "type":        "string",
                "enum":        []any{"integer", "decimal", "fraction"},
                "description": "The numeric type of the answer",
            },
            "choices": map[string]any{
                "type": "array",
                "items": map[string]any{
                    "type": "string",
                },
                "description": "Exactly 4 options for multiple_choice format. Empty array for numeric format.",
            },
            "hint": map[string]any{
                "type":        "string",
                "description": "A short scaffolding hint for the learner. Non-empty for learn tier, empty for prove tier.",
            },
            "difficulty": map[string]any{
                "type":        "integer",
                "minimum":     1,
                "maximum":     5,
                "description": "Self-assessed difficulty from 1 (easy) to 5 (hard)",
            },
            "explanation": map[string]any{
                "type":        "string",
                "description": "Step-by-step worked solution, age-appropriate for a child",
            },
        },
        "required":             []any{"question_text", "format", "answer", "answer_type", "choices", "hint", "difficulty", "explanation"},
        "additionalProperties": false,
    },
}
```

### Raw Output Struct

The JSON response is first unmarshalled into this internal struct before validation:

```go
// questionOutput is the raw LLM response before validation.
type questionOutput struct {
    QuestionText string   `json:"question_text"`
    Format       string   `json:"format"`
    Answer       string   `json:"answer"`
    AnswerType   string   `json:"answer_type"`
    Choices      []string `json:"choices"`
    Hint         string   `json:"hint"`
    Difficulty   int      `json:"difficulty"`
    Explanation  string   `json:"explanation"`
}
```

---

## 8. Built-in Validators

All built-in validators implement the `Validator` interface from section 3. They are included in `DefaultConfig()` and run in order: structural → answer-format → math-check.

### 8.1 StructuralValidator

**Name:** `"structural"`

Checks that required fields are present and within bounds:

- `question_text` is non-empty and at most 500 characters
- `explanation` is non-empty and at most 1000 characters
- `difficulty` is between 1 and 5
- `format` is one of `"numeric"`, `"multiple_choice"`
- `answer_type` is one of `"integer"`, `"decimal"`, `"fraction"`

All failures are `Retryable: true` — the LLM can produce different output on retry.

### 8.2 AnswerFormatValidator

**Name:** `"answer-format"`

Validates that the `answer` string matches the declared `answer_type`, and that multiple choice constraints are satisfied.

**Answer type checks:**

| AnswerType | Validation |
|-----------|------------|
| `integer` | Parseable as `int64` via `strconv.ParseInt`. No leading zeros (except "0" itself). |
| `decimal` | Parseable as `float64` via `strconv.ParseFloat`. No trailing zeros after decimal point (e.g., "3.5" not "3.50"). |
| `fraction` | Matches pattern `^-?\d+/\d+$`. Denominator > 0. Fraction is in lowest terms (GCD of numerator and denominator is 1). |

**Multiple choice checks** (when `format` is `"multiple_choice"`):

- `choices` has exactly 4 elements
- All choices are non-empty and distinct
- Exactly one choice matches `answer` (case-insensitive, whitespace-trimmed)

**Numeric format checks** (when `format` is `"numeric"`):

- `choices` is empty (length 0)

All failures are `Retryable: true`.

### 8.3 MathCheckValidator

**Name:** `"math-check"`

For skills where the answer is mechanically computable, this validator **independently recomputes the answer** from the question text and compares it to the LLM's answer. This catches the most dangerous class of LLM error: a well-formed question with a wrong answer.

```go
// internal/problemgen/mathcheck.go

type MathCheckValidator struct{}

func (v *MathCheckValidator) Name() string { return "math-check" }

func (v *MathCheckValidator) Validate(q *Question, input GenerateInput) *ValidationError {
    computed, err := computeAnswer(q.Text, q.AnswerType)
    if err != nil {
        // Question is not computable (word problem, comparison, etc.)
        // This is fine — pass through silently.
        return nil
    }
    if !answersEqual(computed, q.Answer, q.AnswerType) {
        return &ValidationError{
            Validator: v.Name(),
            Message:   fmt.Sprintf("computed %q but LLM claimed %q", computed, q.Answer),
            Retryable: true,
        }
    }
    return nil
}
```

**How `computeAnswer` works:**

1. A set of regex-based **extractors** attempt to parse the question text into a computable expression:
   - **Binary arithmetic**: `"What is 345 + 278?"` → `345 + 278` → compute → `623`
   - **Multi-step arithmetic**: `"345 + 278 - 100"` → left-to-right evaluation
   - **Fraction arithmetic**: `"What is 1/4 + 1/2?"` → `1/4 + 1/2` → compute → `3/4`
   - **Comparison**: `"Which is greater: 3/4 or 2/3?"` → not computable (returns error)
   - **Word problems**: `"A farmer has 345 apples..."` → not computable (returns error)

2. If an extractor matches, compute the result and return it as a normalized string.

3. If no extractor matches, return an error — the validator treats this as a pass (non-computable question).

**Supported operations:**

| Pattern | Example | Extraction |
|---------|---------|-----------|
| `a + b` | "What is 345 + 278?" | Integer/decimal addition |
| `a - b` | "567 - 289 = ?" | Integer/decimal subtraction |
| `a * b` or `a × b` | "What is 23 * 45?" | Integer/decimal multiplication |
| `a / b` or `a ÷ b` | "What is 144 / 12?" | Integer division (exact) |
| `a/b + c/d` | "What is 1/4 + 1/2?" | Fraction addition |
| `a/b - c/d` | "What is 3/4 - 1/3?" | Fraction subtraction |
| `a/b * c/d` | "What is 2/3 * 3/4?" | Fraction multiplication |
| `a/b ÷ c/d` | "What is 1/2 ÷ 1/4?" | Fraction division |

**Coverage estimate:** ~70% of the 52 MVP skills (all pure arithmetic and fraction computation skills). Skills not covered: comparison/ordering, word problems, measurement, place value identification, and conceptual skills. These pass through without math verification.

---

## 9. Validation Pipeline

The generator runs validators from `Config.Validators` in order. The first failure stops the pipeline.

```
LLM JSON response (schema-validated by llm package)
    │
    ▼
1. Parse into questionOutput
    │
    ▼
2. Convert to Question struct
    │
    ▼
3. Run Config.Validators in order:
    │
    ├─ StructuralValidator.Validate(q, input)
    │     ↓ (if *ValidationError → return error)
    ├─ AnswerFormatValidator.Validate(q, input)
    │     ↓ (if *ValidationError → return error)
    ├─ MathCheckValidator.Validate(q, input)
    │     ↓ (if *ValidationError → return error)
    ├─ ... (any custom validators)
    │
    ▼
4. Return validated Question
```

### Validation Failure & Retry Strategy

When a validator returns a `*ValidationError`, `Generate()` wraps it and returns it to the caller. The `ValidationError.Retryable` field tells callers whether regeneration is likely to fix the issue.

The generator itself does **not** retry internally — that's the caller's responsibility. This keeps the generator stateless and gives callers control over retry budgets. The recommended pattern for callers:

```go
var q *problemgen.Question
var err error
for attempt := 0; attempt < 3; attempt++ {
    q, err = generator.Generate(ctx, input)
    if err == nil {
        break
    }
    var valErr *problemgen.ValidationError
    if errors.As(err, &valErr) && !valErr.Retryable {
        break // Non-retryable validation failure
    }
    log.Printf("question generation attempt %d failed: %v", attempt+1, err)
}
if err != nil {
    // All attempts failed — surface error to learner or skip question
}
```

The LLM layer's retry decorator handles transient network/API errors (spec 04); the generator layer surfaces content quality issues via `ValidationError` for the caller to handle.

---

## 10. Answer Checking

The module also provides answer checking — comparing the learner's input against the correct answer.

```go
// CheckAnswer compares the learner's input against the correct answer.
// Returns true if the answer is correct.
//
// Normalization rules:
// - Whitespace is trimmed
// - Comparison is case-insensitive
// - For fractions: equivalent fractions are accepted (e.g., "2/4" matches "1/2")
// - For decimals: trailing zeros are ignored (e.g., "3.50" matches "3.5")
// - For integers: leading zeros are ignored (e.g., "007" matches "7")
// - For multiple choice: matches against the choice text
func CheckAnswer(learnerAnswer string, question *Question) bool
```

### Normalization Details

```go
// normalizeAnswer normalizes an answer string for comparison.
func normalizeAnswer(answer string, answerType AnswerType) (string, error)
```

| AnswerType | Normalization |
|-----------|---------------|
| `integer` | Parse to `int64`, format back to string. Strips leading zeros and whitespace. |
| `decimal` | Parse to `float64`, format with `strconv.FormatFloat(f, 'f', -1, 64)`. Strips trailing zeros. |
| `fraction` | Parse numerator and denominator. Reduce to lowest terms (divide by GCD). Normalize sign (negative sign on numerator only). Format as `"n/d"`. |

For **multiple choice**, comparison is done against the choices list — the learner's input is matched against the option index (1-4) or the option text. If the learner enters "1", "2", "3", or "4", it's matched by index. Otherwise, it's matched by text (case-insensitive, trimmed).

---

## 11. Deduplication

Deduplication is handled by including prior questions in the LLM prompt. This approach is simple and effective:

1. The caller tracks `PriorQuestions []string` — the text of every question already served in the current session for the current skill.
2. These are included in the user message under "Already asked in this session."
3. The system prompt instructs the LLM: "Do not repeat any question from the already asked list."

### Limits

- At most **`Config.MaxPriorQuestions`** prior questions are included in the prompt (default: 8, the most recent N). Beyond that, older questions are dropped. This keeps token cost bounded.
- If a session has more questions than the limit for a single skill, the dedup is best-effort for older questions.
- There is no post-generation duplicate check — the prompt-based approach is sufficient and avoids the complexity of semantic similarity detection.

### Token Budget Impact

Each prior question adds ~20-40 tokens to the prompt. With 8 prior questions, the total dedup overhead is ~160-320 tokens, well within the input budget.

---

## 12. Tier-Specific Behavior

The `Tier` in `GenerateInput` affects question generation:

| Aspect | Learn Tier | Prove Tier |
|--------|-----------|------------|
| Hint | Included (non-empty) | Empty string |
| Difficulty | LLM targets 1-3 | LLM targets 3-5 |
| Format | LLM chooses freely | LLM chooses freely |
| Temperature | 0.7 | 0.7 |

The tier label ("learn" or "prove") is passed in the user message. The system prompt instructs the LLM to generate hints only for the learn tier.

The difficulty targeting is **advisory** — the LLM is told the tier but not explicitly constrained. The self-assessed `difficulty` field is recorded for analytics but not used for gating. Over time, actual learner performance (accuracy, speed) is a better signal than the LLM's self-assessment.

---

## 13. Error Context

When `RecentErrors` is provided, the generator includes them in the prompt so the LLM can:

- Avoid generating questions that test the exact same thing the learner just failed
- Generate questions that approach the same concept from a different angle
- Adjust difficulty if the learner is struggling

### Error Format

Each entry in `RecentErrors` is a human-readable string describing what happened:

```
"Answered 623 for '345 + 289 = ?', correct answer was 634 (likely carried incorrectly)"
"Answered 1/3 for 'What is 1/4 + 1/4?', correct answer was 1/2 (added denominators)"
```

The caller (session engine) constructs these strings from the learner's response history. The format is intentionally flexible — it's natural language consumed by the LLM, not structured data.

### Limits

- At most **5 recent errors** are included. Older errors are dropped.
- Each error string should be at most ~100 characters. The session engine is responsible for truncation.
- Total error context overhead: ~250-500 tokens.

---

## 14. Package Structure

```
internal/
  problemgen/
    types.go            # Question, AnswerType, AnswerFormat, GenerateInput types
    validator.go        # Validator interface, ValidationError type
    config.go           # Config struct, DefaultConfig()
    generator.go        # Generator interface
    llm_generator.go    # LLMGenerator implementation (prompt building, LLM call, validation loop)
    schema.go           # QuestionSchema definition
    structural.go       # StructuralValidator (field presence, bounds, enums)
    answer_format.go    # AnswerFormatValidator (type parsing, MC constraints)
    mathcheck.go        # MathCheckValidator, regex extractors, arithmetic/fraction computation
    answer.go           # CheckAnswer, normalizeAnswer, fraction math (GCD, reduce)
    dedup.go            # buildDedup helper (formats prior questions for prompt)
    prompt.go           # System prompt constant, user message template builder
    generator_test.go   # Generator tests (using mock provider)
    validator_test.go   # Validator interface + pipeline tests
    structural_test.go  # StructuralValidator edge cases
    answer_format_test.go # AnswerFormatValidator + MC constraint tests
    mathcheck_test.go   # Math verification tests (correct/incorrect/unparseable)
    answer_test.go      # Answer checking and normalization tests
```

---

## 15. Dependencies

| Dependency | Direction | What's Used |
|-----------|-----------|------------|
| `internal/llm` | → imports | `Provider`, `Request`, `Message`, `Schema`, `Response`, `WithPurpose` |
| `internal/skillgraph` | → imports | `Skill`, `Tier`, `TierLearn`, `TierProve` |
| Session Engine (06) | ← consumed by | Calls `Generator.Generate()` and `CheckAnswer()` |
| Diagnostic Placement (03) | ← consumed by | Calls `Generator.Generate()` for probe questions |

The problem generation module has **no persistence dependency**. It does not read from or write to the database. The LLM logging decorator (spec 04) handles event recording transparently.

---

## 16. Testing Strategy

### 16.1 Generator Tests (Mock Provider)

Use `llm.MockProvider` to test the full generation flow without API calls.

```go
func TestGenerate_Numeric(t *testing.T) {
    // Setup: MockProvider returns valid numeric question JSON, DefaultConfig()
    // Act: Call Generate with a skill and Learn tier
    // Assert: Question has correct fields, format is numeric, hint is non-empty
}

func TestGenerate_MultipleChoice(t *testing.T) {
    // Setup: MockProvider returns valid MC question JSON
    // Act: Call Generate with a conceptual skill
    // Assert: Question has 4 choices, one matches answer
}

func TestGenerate_ProveTier_NoHint(t *testing.T) {
    // Setup: MockProvider returns question with empty hint
    // Act: Call Generate with Prove tier
    // Assert: Hint is empty string
}

func TestGenerate_ValidationFailure(t *testing.T) {
    // Setup: MockProvider returns JSON with invalid answer (e.g., "abc" for integer type)
    // Act: Call Generate with DefaultConfig()
    // Assert: Returns *ValidationError from "answer-format" validator
}

func TestGenerate_CustomValidator(t *testing.T) {
    // Setup: Config with DefaultConfig() + a custom validator that rejects difficulty > 3
    // Act: Call Generate, MockProvider returns question with difficulty 4
    // Assert: Returns *ValidationError from custom validator
}

func TestGenerate_ValidatorOrder(t *testing.T) {
    // Setup: Config with two validators, first rejects
    // Act: Call Generate
    // Assert: Error is from first validator; second validator was never called
}

func TestGenerate_NoValidators(t *testing.T) {
    // Setup: Config with empty Validators slice
    // Act: Call Generate with MockProvider returning valid JSON
    // Assert: Question returned without validation (no error)
}

func TestGenerate_PriorQuestionsInPrompt(t *testing.T) {
    // Setup: Provide 3 prior questions in GenerateInput
    // Act: Call Generate
    // Assert: MockProvider.Calls[0] user message contains all 3 prior questions
}

func TestGenerate_RecentErrorsInPrompt(t *testing.T) {
    // Setup: Provide 2 recent errors in GenerateInput
    // Act: Call Generate
    // Assert: MockProvider.Calls[0] user message contains error descriptions
}

func TestGenerate_PurposeLabel(t *testing.T) {
    // Assert: ctx passed to provider has purpose "question-gen"
}

func TestGenerate_ConfigOverrides(t *testing.T) {
    // Setup: Config with MaxTokens=256, Temperature=0.5
    // Act: Call Generate
    // Assert: MockProvider.Calls[0] request has MaxTokens=256, Temperature=0.5
}
```

### 16.2 StructuralValidator Tests

```go
func TestStructural_ValidQuestion(t *testing.T) {
    // Well-formed question → nil
}

func TestStructural_EmptyQuestionText(t *testing.T) {
    // question_text "" → ValidationError{Validator: "structural", Retryable: true}
}

func TestStructural_QuestionTextTooLong(t *testing.T) {
    // question_text > 500 chars → ValidationError
}

func TestStructural_DifficultyOutOfRange(t *testing.T) {
    // difficulty 0 or 6 → ValidationError
}

func TestStructural_UnknownFormat(t *testing.T) {
    // format "essay" → ValidationError
}

func TestStructural_UnknownAnswerType(t *testing.T) {
    // answer_type "boolean" → ValidationError
}
```

### 16.3 AnswerFormatValidator Tests

```go
func TestAnswerFormat_Integer(t *testing.T) {
    // Valid: "42", "0", "-5" → nil
    // Invalid: "3.5", "abc", "3/4", "007" → ValidationError
}

func TestAnswerFormat_Decimal(t *testing.T) {
    // Valid: "3.5", "0.75", "-2.1" → nil
    // Invalid: "abc", "3.50" (trailing zero) → ValidationError
}

func TestAnswerFormat_Fraction(t *testing.T) {
    // Valid: "3/4", "1/2", "-7/3" → nil
    // Invalid: "3/0" (zero denom), "2/4" (not reduced), "abc" → ValidationError
}

func TestAnswerFormat_MultipleChoice(t *testing.T) {
    // 4 unique choices with answer matching one → nil
    // 3 choices → ValidationError
    // Duplicate choices → ValidationError
    // Answer not in choices → ValidationError
}

func TestAnswerFormat_NumericWithChoices(t *testing.T) {
    // Numeric format with non-empty choices → ValidationError
}
```

### 16.4 MathCheckValidator Tests

```go
func TestMathCheck_Addition(t *testing.T) {
    // "What is 345 + 278?" with answer "623" → nil (correct)
    // "What is 345 + 278?" with answer "612" → ValidationError{Validator: "math-check"}
}

func TestMathCheck_Subtraction(t *testing.T) {
    // "567 - 289 = ?" with answer "278" → nil
    // "567 - 289 = ?" with answer "288" → ValidationError
}

func TestMathCheck_Multiplication(t *testing.T) {
    // "What is 23 * 45?" with answer "1035" → nil
    // "What is 23 * 45?" with answer "1025" → ValidationError
}

func TestMathCheck_Division(t *testing.T) {
    // "What is 144 / 12?" with answer "12" → nil
    // "What is 144 / 12?" with answer "11" → ValidationError
}

func TestMathCheck_FractionArithmetic(t *testing.T) {
    // "What is 1/4 + 1/2?" with answer "3/4" → nil
    // "What is 3/4 - 1/3?" with answer "5/12" → nil
    // "What is 2/3 * 3/4?" with answer "1/2" → nil
    // "What is 1/2 ÷ 1/4?" with answer "2" → nil
}

func TestMathCheck_FractionWrongAnswer(t *testing.T) {
    // "What is 1/4 + 1/2?" with answer "2/6" → ValidationError
}

func TestMathCheck_NonComputable(t *testing.T) {
    // "Which fraction is larger: 3/4 or 2/3?" → nil (skipped, not computable)
    // "A farmer has 345 apples..." → nil (skipped, word problem)
    // "What place value does 5 have in 5,432?" → nil (skipped, conceptual)
}

func TestMathCheck_LargeNumbers(t *testing.T) {
    // "What is 12345 + 67890?" with answer "80235" → nil
    // "What is 456 * 789?" with answer "359784" → nil
}
```

### 16.5 Answer Checking Tests

```go
func TestCheckAnswer_Integer(t *testing.T) {
    // "42" matches "42" → true
    // " 42 " matches "42" → true (whitespace)
    // "042" matches "42" → true (leading zeros)
    // "43" does not match "42" → false
}

func TestCheckAnswer_Decimal(t *testing.T) {
    // "3.5" matches "3.5" → true
    // "3.50" matches "3.5" → true (trailing zero)
    // "3.6" does not match "3.5" → false
}

func TestCheckAnswer_Fraction(t *testing.T) {
    // "1/2" matches "1/2" → true
    // "2/4" matches "1/2" → true (equivalent)
    // "3/6" matches "1/2" → true (equivalent)
    // "1/3" does not match "1/2" → false
}

func TestCheckAnswer_MultipleChoice_ByIndex(t *testing.T) {
    // Input "2" matches second choice → true (if second choice is correct)
}

func TestCheckAnswer_MultipleChoice_ByText(t *testing.T) {
    // Input "3/4" matches choice "3/4" → true
    // Case insensitive: "Three quarters" matches "three quarters" → true
}
```

### 16.6 Prompt Construction Tests

```go
func TestBuildUserMessage_MinimalContext(t *testing.T) {
    // No prior questions, no errors → message contains "None" for both
}

func TestBuildUserMessage_WithHistory(t *testing.T) {
    // 3 prior questions and 2 errors → all included in message
}

func TestBuildUserMessage_TruncatesPriorQuestions(t *testing.T) {
    // Config.MaxPriorQuestions=8, provide 12 → only most recent 8 included
}

func TestBuildUserMessage_TruncatesErrors(t *testing.T) {
    // Config.MaxRecentErrors=5, provide 8 → only most recent 5 included
}

func TestBuildUserMessage_CustomLimits(t *testing.T) {
    // Config.MaxPriorQuestions=3, provide 5 → only most recent 3 included
}
```

---

## 17. Example Flow

A complete question generation flow for a Grade 3 learner working on "Add 3-Digit Numbers" in the Learn tier:

**1. Session engine creates generator and calls Generate:**

```go
gen := problemgen.New(provider, problemgen.DefaultConfig())

input := problemgen.GenerateInput{
    Skill:          skillgraph.MustGetSkill("add-3digit"),
    Tier:           skillgraph.TierLearn,
    PriorQuestions: []string{"What is 234 + 156?", "What is 487 + 312?"},
    RecentErrors:   []string{"Answered 890 for '456 + 378 = ?', correct was 834 (carried tens but not hundreds)"},
}
q, err := gen.Generate(ctx, input)
```

**2. Generator builds the LLM request:**

System prompt: (see section 6)

User message:
```
Skill: Add 3-Digit Numbers
Description: Addition of three-digit numbers with regrouping
Grade: 3
Keywords: addition, carry, three-digit
Tier: learn
Hints allowed: true

Already asked in this session:
1. What is 234 + 156?
2. What is 487 + 312?

Recent errors by this student:
1. Answered 890 for '456 + 378 = ?', correct was 834 (carried tens but not hundreds)
```

**3. LLM returns structured JSON:**

```json
{
    "question_text": "What is 567 + 285?",
    "format": "numeric",
    "answer": "852",
    "answer_type": "integer",
    "choices": [],
    "hint": "Try adding the ones first: 7 + 5 = 12. Write down 2 and carry the 1 to the tens column.",
    "difficulty": 3,
    "explanation": "Step 1: Add ones: 7 + 5 = 12. Write 2, carry 1.\nStep 2: Add tens: 6 + 8 + 1 = 15. Write 5, carry 1.\nStep 3: Add hundreds: 5 + 2 + 1 = 8.\nAnswer: 852"
}
```

**4. Generator runs Config.Validators in order:**

- `StructuralValidator`: question_text non-empty, difficulty 3 in range, format valid — passed
- `AnswerFormatValidator`: "852" parses as valid integer, numeric format has no choices — passed
- `MathCheckValidator`: parses "567 + 285" from question text, computes 852, matches LLM answer — passed

**5. Generator returns Question to session engine.**

**6. Learner answers "852". Session engine calls:**

```go
correct := problemgen.CheckAnswer("852", q) // → true
```

---

## 18. Verification

The Problem Generation module is verified when:

- [ ] `internal/problemgen/validator.go` defines the `Validator` interface and `ValidationError` type
- [ ] `internal/problemgen/config.go` defines `Config` struct and `DefaultConfig()` returns the standard validator chain
- [ ] `internal/problemgen/types.go` defines `Question`, `AnswerType`, `AnswerFormat`, `GenerateInput`
- [ ] `internal/problemgen/generator.go` defines the `Generator` interface
- [ ] `internal/problemgen/llm_generator.go` implements `LLMGenerator` accepting `Config`, runs validators from config in order
- [ ] `internal/problemgen/schema.go` defines `QuestionSchema` with all required fields
- [ ] `internal/problemgen/prompt.go` builds system and user messages from `GenerateInput`, respecting `Config.MaxPriorQuestions` and `Config.MaxRecentErrors`
- [ ] `internal/problemgen/structural.go` implements `StructuralValidator` (field presence, bounds, enums)
- [ ] `internal/problemgen/answer_format.go` implements `AnswerFormatValidator` (type parsing, MC constraints)
- [ ] `internal/problemgen/mathcheck.go` implements `MathCheckValidator` with regex extractors for arithmetic and fraction operations
- [ ] `internal/problemgen/answer.go` implements `CheckAnswer` with normalization for integers, decimals, and fractions (including equivalent fraction matching)
- [ ] `internal/problemgen/dedup.go` formats prior questions for the prompt, respecting `Config.MaxPriorQuestions`
- [ ] Custom validators can be appended to `Config.Validators` and are invoked by the generator
- [ ] Generator tests pass using `MockProvider` for all question formats, tiers, and custom validator configurations
- [ ] Built-in validator tests cover all edge cases (invalid answers, malformed MC, out-of-range difficulty, wrong math)
- [ ] MathCheckValidator tests confirm correct answers pass, wrong answers are caught, and non-computable questions are skipped
- [ ] Answer checking tests verify normalization and equivalence for all answer types
- [ ] `CheckAnswer` correctly handles multiple choice by both index and text
- [ ] Prompt tests verify context inclusion and truncation limits from Config
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
- [ ] `go test ./internal/problemgen/...` passes
