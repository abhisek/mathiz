# 03 - Skill Graph

## Overview

The Skill Graph is the structural backbone of Mathiz. It models math skills as a directed acyclic graph (DAG) where edges represent prerequisite relationships. The graph drives session planning (which skill to practice next), mastery tracking (what's unlocked, what's blocked), diagnostic placement, and the Skill Map screen.

Skills are **embedded in code** as Go structs, compiled into the binary. No external seed files or DB-driven editing. To add or modify skills, update the code and rebuild.

---

## Skill Node Schema

Each skill node carries rich metadata for grouping, filtering, LLM prompt generation, and UI display.

```go
type Skill struct {
    ID            string        // Unique kebab-case identifier, e.g. "add-3digit"
    Name          string        // Human-readable name, e.g. "Add 3-Digit Numbers"
    Description   string        // One-sentence description for UI and LLM context
    Strand        Strand        // Math strand (see below)
    GradeLevel    int           // Target grade level (3, 4, or 5 for MVP)
    CommonCoreID  string        // Common Core standard, e.g. "3.NBT.2" (optional, "" if none)
    EstimatedMins int           // Estimated minutes to learn (for session planning)
    Keywords      []string      // Terms for LLM prompt context, e.g. ["carry", "regrouping"]
    Prerequisites []string      // IDs of prerequisite skills (ALL must be mastered to unlock)
    Tiers         [2]TierConfig // Learn and Prove tier configurations
}
```

### Strands

```go
type Strand string

const (
    StrandNumberPlace   Strand = "number-and-place-value"
    StrandAddSub        Strand = "addition-and-subtraction"
    StrandMultDiv       Strand = "multiplication-and-division"
    StrandFractions     Strand = "fractions"
    StrandMeasurement   Strand = "measurement"
)
```

### Difficulty Tiers

Each skill has exactly **2 tiers**: Learn and Prove.

```go
type Tier int

const (
    TierLearn Tier = iota // Practice with hints available, untimed
    TierProve             // Timed assessment without hints, demonstrates mastery
)

type TierConfig struct {
    Tier              Tier
    ProblemsRequired  int     // Number of problems to attempt in this tier
    AccuracyThreshold float64 // Minimum accuracy to pass (0.0 - 1.0)
    TimeLimitSecs     int     // Per-problem time limit (0 = untimed, used in Prove tier)
    HintsAllowed      bool    // Whether hints are available
}
```

**Progression:**

1. **Learn tier** — Hints available, no time pressure. Learner practices until they reach the accuracy threshold. Purpose: build understanding.
2. **Prove tier** — No hints, per-problem time limit. Learner must demonstrate fluency under light pressure. Purpose: confirm mastery.

A skill is considered **mastered** only after passing the Prove tier. Passing Learn tier unlocks the Prove tier.

Default tier configurations (overridable per-skill):

| Tier  | Problems | Accuracy | Time Limit | Hints |
|-------|----------|----------|------------|-------|
| Learn | 8        | 0.75     | 0 (none)   | Yes   |
| Prove | 6        | 0.85     | 30s        | No    |

---

## Prerequisite Model

- Every prerequisite listed in `Prerequisites` must be **mastered** (Prove tier passed) before the skill unlocks.
- A skill with an empty `Prerequisites` slice is a **root skill** — available from the start.
- The graph must be a valid DAG (no cycles). This is enforced by a build-time validation function.

### Skill States (relative to learner)

A skill's state depends on the learner's progress:

| State        | Definition |
|--------------|------------|
| `locked`     | One or more prerequisites not yet mastered |
| `available`  | All prerequisites mastered; skill not yet started |
| `learning`   | Learn tier in progress |
| `proving`    | Learn tier passed; Prove tier in progress |
| `mastered`   | Prove tier passed |
| `rusty`      | Previously mastered but flagged by spaced repetition |

> Note: `learning`, `proving`, `mastered`, and `rusty` are tracked by the Mastery module (spec 07). The Skill Graph module is responsible for computing `locked` vs `available` based on prerequisite edges.

---

## Graph Traversal API

The following functions operate on the in-memory skill graph. They accept a set of mastered skill IDs (from the persistence layer) and return computed results.

```go
// GetSkill returns a skill by ID, or error if not found.
func GetSkill(id string) (Skill, error)

// AllSkills returns all skills in the graph.
func AllSkills() []Skill

// ByStrand returns all skills in a given strand, ordered by grade then topological position.
func ByStrand(strand Strand) []Skill

// ByGrade returns all skills for a given grade level, ordered by strand then topological position.
func ByGrade(grade int) []Skill

// RootSkills returns all skills with no prerequisites.
func RootSkills() []Skill

// Prerequisites returns the direct prerequisite skills for a given skill ID.
func Prerequisites(id string) []Skill

// Dependents returns skills that directly depend on the given skill ID.
func Dependents(id string) []Skill

// IsUnlocked returns true if all prerequisites for the given skill are in the mastered set.
func IsUnlocked(id string, mastered map[string]bool) bool

// AvailableSkills returns all skills that are unlocked but not yet mastered.
func AvailableSkills(mastered map[string]bool) []Skill

// FrontierSkills returns the subset of available skills that are "next" for the learner --
// skills whose prerequisites were most recently mastered.
func FrontierSkills(mastered map[string]bool) []Skill

// BlockedSkills returns all skills that have at least one unmastered prerequisite.
func BlockedSkills(mastered map[string]bool) []Skill

// TopologicalOrder returns all skills in a valid topological order.
func TopologicalOrder() []Skill

// Validate checks the graph for cycles, dangling prerequisite references,
// and other structural issues. Called at init time.
func Validate() error
```

---

## Diagnostic Placement

On first launch (no learner data exists), Mathiz runs a **top-down probing** diagnostic quiz to skip the learner past already-known skills.

### Algorithm

1. Collect all skills and group by strand.
2. Within each strand, sort by grade level descending (hardest first).
3. For each strand, start probing from the highest-grade skills:
   a. Present 2-3 problems for the current skill (generated via LLM, Learn-tier difficulty).
   b. If the learner answers >= 2/3 correctly, mark that skill and all its transitive prerequisites as **mastered** (skip them).
   c. If the learner answers < 2/3 correctly, move to the next lower skill in that strand.
   d. Stop probing a strand when a skill is failed or root skills are reached.
4. After all strands are probed, the learner's initial mastered set is established.

### Constraints

- Target: **10-15 questions total** across all strands.
- Maximum: 3 questions per skill probed.
- The diagnostic can be skipped (learner starts from root skills).
- Results are persisted as mastery events (same format as normal session results).

---

## Skill Map Screen

The Skill Map is a **grouped list** view showing all skills organized by strand, with visual indicators of the learner's progress.

### Layout

```
╭─ Skill Map ──────────────────────────────────────────╮
│                                                      │
│  NUMBER & PLACE VALUE                                │
│  ✅ Place Value to 1,000          Grade 3   Mastered │
│  ✅ Compare & Order to 1,000      Grade 3   Mastered │
│  🔓 Place Value to 10,000         Grade 4  Available │
│  🔒 Place Value to 1,000,000      Grade 5    Locked  │
│                                                      │
│  ADDITION & SUBTRACTION                              │
│  ✅ Add 2-Digit Numbers           Grade 3   Mastered │
│  📖 Add 3-Digit Numbers           Grade 3  Learning  │
│  🔒 Add 4-Digit Numbers           Grade 4    Locked  │
│  ...                                                 │
│                                                      │
│  FRACTIONS                                           │
│  🔒 Fraction Concepts             Grade 3    Locked  │
│  ...                                                 │
│                                                      │
│  ↑/↓ Navigate  Enter: Start Practice  q: Back       │
╰──────────────────────────────────────────────────────╯
```

### State Icons

| Icon | State     |
|------|-----------|
| `🔒` | Locked    |
| `🔓` | Available |
| `📖` | Learning  |
| `📝` | Proving   |
| `✅` | Mastered  |
| `🔄` | Rusty     |

### Interactions

- **Arrow keys**: Navigate the skill list
- **Enter** on an available/learning/proving/rusty skill: Start a practice session for that skill
- **Enter** on a locked skill: Show which prerequisites are needed (inline or tooltip)
- **Enter** on a mastered skill: Show stats (accuracy, speed, last practiced) or start review
- **Tab**: Cycle between strands (jump to next strand header)
- **q / Esc**: Return to Home screen

### Grouping & Sorting

Skills are grouped by strand (in the order defined above: Number & Place Value, Addition & Subtraction, Multiplication & Division, Fractions, Measurement). Within each strand, skills are sorted by grade level ascending, then by topological order within the same grade.

---

## Full MVP Seed Graph

Below is the complete skill graph for the MVP. Each entry specifies: `ID`, `Name`, `Grade`, `Strand`, `CommonCoreID`, `EstimatedMins`, `Keywords`, and `Prerequisites`.

### Number & Place Value (8 nodes)

| # | ID | Name | Grade | CC ID | Est. Min | Keywords | Prerequisites |
|---|----|------|-------|-------|----------|----------|---------------|
| 1 | `pv-hundreds` | Place Value to 1,000 | 3 | 2.NBT.1 | 10 | ones, tens, hundreds, place value | _(root)_ |
| 2 | `compare-1000` | Compare & Order to 1,000 | 3 | 2.NBT.4 | 8 | greater than, less than, order | `pv-hundreds` |
| 3 | `round-nearest-10-100` | Round to Nearest 10 or 100 | 3 | 3.NBT.1 | 8 | round, nearest, estimate | `pv-hundreds` |
| 4 | `pv-ten-thousands` | Place Value to 10,000 | 4 | 3.NBT.1 | 10 | thousands, ten-thousands, expanded form | `pv-hundreds` |
| 5 | `compare-10000` | Compare & Order to 10,000 | 4 | 4.NBT.2 | 8 | compare, order, place value | `pv-ten-thousands` |
| 6 | `round-nearest-1000` | Round to Nearest 1,000 | 4 | 4.NBT.3 | 8 | round, estimate, nearest thousand | `pv-ten-thousands`, `round-nearest-10-100` |
| 7 | `pv-millions` | Place Value to 1,000,000 | 5 | 4.NBT.1 | 12 | hundred-thousands, millions, expanded form | `pv-ten-thousands` |
| 8 | `compare-millions` | Compare & Order to 1,000,000 | 5 | 4.NBT.2 | 8 | compare, order, large numbers | `pv-millions` |

### Addition & Subtraction (10 nodes)

| # | ID | Name | Grade | CC ID | Est. Min | Keywords | Prerequisites |
|---|----|------|-------|-------|----------|----------|---------------|
| 9 | `add-2digit` | Add 2-Digit Numbers | 3 | 2.NBT.5 | 10 | addition, carry, regrouping | `pv-hundreds` |
| 10 | `sub-2digit` | Subtract 2-Digit Numbers | 3 | 2.NBT.5 | 10 | subtraction, borrow, regrouping | `pv-hundreds` |
| 11 | `add-3digit` | Add 3-Digit Numbers | 3 | 3.NBT.2 | 12 | addition, carry, three-digit | `add-2digit` |
| 12 | `sub-3digit` | Subtract 3-Digit Numbers | 3 | 3.NBT.2 | 12 | subtraction, borrow, three-digit | `sub-2digit` |
| 13 | `add-sub-estimation` | Estimate Sums & Differences | 3 | 3.OA.8 | 8 | estimate, round, approximate | `round-nearest-10-100`, `add-2digit`, `sub-2digit` |
| 14 | `add-4digit` | Add 4-Digit Numbers | 4 | 4.NBT.4 | 12 | addition, four-digit, carry | `add-3digit`, `pv-ten-thousands` |
| 15 | `sub-4digit` | Subtract 4-Digit Numbers | 4 | 4.NBT.4 | 12 | subtraction, four-digit, borrow | `sub-3digit`, `pv-ten-thousands` |
| 16 | `add-sub-word-3digit` | Addition & Subtraction Word Problems (3-digit) | 3 | 3.OA.8 | 15 | word problem, context, multi-step | `add-3digit`, `sub-3digit` |
| 17 | `add-5digit` | Add 5+ Digit Numbers | 5 | 5.NBT.5 | 12 | large number addition, carry | `add-4digit`, `pv-millions` |
| 18 | `sub-5digit` | Subtract 5+ Digit Numbers | 5 | 5.NBT.5 | 12 | large number subtraction, borrow | `sub-4digit`, `pv-millions` |

### Multiplication & Division (14 nodes)

| # | ID | Name | Grade | CC ID | Est. Min | Keywords | Prerequisites |
|---|----|------|-------|-------|----------|----------|---------------|
| 19 | `mult-concept` | Multiplication Concept | 3 | 3.OA.1 | 10 | groups of, repeated addition, array | `add-2digit` |
| 20 | `mult-facts-2-5-10` | Multiply by 2, 5, and 10 | 3 | 3.OA.7 | 12 | times tables, skip counting, facts | `mult-concept` |
| 21 | `mult-facts-3-4-6` | Multiply by 3, 4, and 6 | 3 | 3.OA.7 | 12 | times tables, facts, memorization | `mult-facts-2-5-10` |
| 22 | `mult-facts-7-8-9` | Multiply by 7, 8, and 9 | 3 | 3.OA.7 | 15 | times tables, facts, memorization | `mult-facts-3-4-6` |
| 23 | `mult-properties` | Multiplication Properties | 3 | 3.OA.5 | 10 | commutative, associative, distributive | `mult-facts-2-5-10` |
| 24 | `div-concept` | Division Concept | 3 | 3.OA.2 | 10 | sharing, grouping, inverse of multiplication | `mult-concept` |
| 25 | `div-facts` | Division Facts (÷1 through ÷9) | 3 | 3.OA.7 | 15 | division facts, inverse, fluency | `div-concept`, `mult-facts-7-8-9` |
| 26 | `mult-div-word` | Multiplication & Division Word Problems | 3 | 3.OA.3 | 15 | word problem, context, operation choice | `mult-facts-7-8-9`, `div-facts` |
| 27 | `mult-2d-by-1d` | Multiply 2-Digit by 1-Digit | 4 | 4.NBT.5 | 12 | partial products, area model | `mult-facts-7-8-9`, `pv-ten-thousands` |
| 28 | `mult-2d-by-2d` | Multiply 2-Digit by 2-Digit | 4 | 4.NBT.5 | 15 | partial products, standard algorithm | `mult-2d-by-1d` |
| 29 | `div-2d-by-1d` | Divide 2-Digit by 1-Digit | 4 | 4.NBT.6 | 12 | long division, remainder | `div-facts`, `pv-ten-thousands` |
| 30 | `div-3d-by-1d` | Divide 3-Digit by 1-Digit | 4 | 4.NBT.6 | 15 | long division, multi-step | `div-2d-by-1d` |
| 31 | `mult-3d-by-2d` | Multiply 3-Digit by 2-Digit | 5 | 5.NBT.5 | 15 | standard algorithm, large multiplication | `mult-2d-by-2d` |
| 32 | `div-4d-by-2d` | Divide 4-Digit by 2-Digit | 5 | 5.NBT.6 | 15 | long division, two-digit divisor | `div-3d-by-1d`, `mult-2d-by-2d` |

### Fractions (12 nodes)

| # | ID | Name | Grade | CC ID | Est. Min | Keywords | Prerequisites |
|---|----|------|-------|-------|----------|----------|---------------|
| 33 | `frac-concept` | Fraction Concepts | 3 | 3.NF.1 | 12 | numerator, denominator, part of whole | `div-concept` |
| 34 | `frac-on-number-line` | Fractions on a Number Line | 3 | 3.NF.2 | 10 | number line, position, unit fraction | `frac-concept` |
| 35 | `frac-equivalent` | Equivalent Fractions | 3 | 3.NF.3 | 12 | same value, simplify, multiply numerator denominator | `frac-concept` |
| 36 | `frac-compare` | Compare Fractions | 3 | 3.NF.3 | 10 | greater than, less than, same denominator, benchmark | `frac-equivalent`, `frac-on-number-line` |
| 37 | `frac-equiv-generate` | Generate Equivalent Fractions | 4 | 4.NF.1 | 12 | multiply, divide, visual model | `frac-equivalent`, `mult-facts-7-8-9` |
| 38 | `frac-add-same-denom` | Add Fractions (Same Denominator) | 4 | 4.NF.3 | 10 | add numerators, same denominator | `frac-compare` |
| 39 | `frac-sub-same-denom` | Subtract Fractions (Same Denominator) | 4 | 4.NF.3 | 10 | subtract numerators, same denominator | `frac-compare` |
| 40 | `frac-mixed-numbers` | Mixed Numbers & Improper Fractions | 4 | 4.NF.3 | 12 | mixed number, improper, convert | `frac-add-same-denom` |
| 41 | `frac-add-diff-denom` | Add Fractions (Different Denominators) | 5 | 5.NF.1 | 15 | common denominator, LCD, unlike fractions | `frac-add-same-denom`, `frac-equiv-generate` |
| 42 | `frac-sub-diff-denom` | Subtract Fractions (Different Denominators) | 5 | 5.NF.1 | 15 | common denominator, LCD, unlike fractions | `frac-sub-same-denom`, `frac-equiv-generate` |
| 43 | `frac-mult` | Multiply Fractions | 5 | 5.NF.4 | 12 | multiply numerators, multiply denominators | `frac-mixed-numbers`, `mult-facts-7-8-9` |
| 44 | `frac-div` | Divide Fractions | 5 | 5.NF.7 | 12 | reciprocal, invert and multiply | `frac-mult`, `div-facts` |

### Measurement (8 nodes)

| # | ID | Name | Grade | CC ID | Est. Min | Keywords | Prerequisites |
|---|----|------|-------|-------|----------|----------|---------------|
| 45 | `meas-length` | Measure Length (cm, m, in, ft) | 3 | 3.MD.4 | 10 | ruler, centimeter, meter, inch, foot | `pv-hundreds` |
| 46 | `meas-time` | Tell Time & Elapsed Time | 3 | 3.MD.1 | 12 | clock, hour, minute, elapsed, duration | `add-2digit`, `sub-2digit` |
| 47 | `meas-mass-volume` | Mass & Volume (g, kg, mL, L) | 3 | 3.MD.2 | 10 | gram, kilogram, liter, milliliter, measure | `pv-hundreds` |
| 48 | `meas-perimeter` | Perimeter | 3 | 3.MD.8 | 10 | perimeter, side lengths, add sides | `add-3digit` |
| 49 | `meas-area-concept` | Area Concept (Counting Squares) | 3 | 3.MD.5 | 10 | area, square unit, count, cover | `mult-concept` |
| 50 | `meas-area-formula` | Area by Multiplication | 3 | 3.MD.7 | 10 | length times width, rectangle, formula | `meas-area-concept`, `mult-facts-2-5-10` |
| 51 | `meas-unit-conversion` | Unit Conversions | 4 | 4.MD.1 | 12 | convert, metric, customary, table | `meas-length`, `meas-mass-volume`, `mult-2d-by-1d` |
| 52 | `meas-area-perimeter-word` | Area & Perimeter Word Problems | 4 | 4.MD.3 | 15 | word problem, real-world, area, perimeter | `meas-area-formula`, `meas-perimeter`, `mult-2d-by-1d` |

**Total: 52 skill nodes**

---

## Graph Validation

At application init (`init()` or startup), `Validate()` runs and panics on failure. It checks:

1. **No cycles** — Topological sort completes without detecting a back edge.
2. **No dangling prerequisites** — Every ID referenced in `Prerequisites` exists in the graph.
3. **No duplicate IDs** — Every skill has a unique ID.
4. **At least one root** — At least one skill has no prerequisites.
5. **All strands populated** — Every declared strand has at least one skill.
6. **Tier configs valid** — `ProblemsRequired > 0`, `0 < AccuracyThreshold <= 1.0`, `TimeLimitSecs >= 0`.

---

## Package Structure

```
internal/
  skillgraph/
    skill.go         // Skill struct, Strand type, TierConfig, Tier constants
    graph.go         // Graph struct, traversal API functions
    seed.go          // All 52 skill definitions as Go literals
    validate.go      // Validate() function, cycle detection, structural checks
    graph_test.go    // Tests for traversal API
    validate_test.go // Tests for validation rules
    diagnostic.go    // Diagnostic placement quiz logic
    diagnostic_test.go
```

---

## Interface with Other Modules

| Consumer | What it uses |
|----------|-------------|
| **Persistence (02)** | Stores per-skill mastery state; queries graph for skill metadata when recording events |
| **LLM / Problem Gen (04, 05)** | Uses `Skill.Keywords`, `Skill.Description`, `Skill.Strand`, tier config to construct prompts |
| **Session Planner (06)** | Calls `FrontierSkills()`, `AvailableSkills()` to build session mix |
| **Mastery (07)** | Reads tier configs to determine pass/fail; uses `Prerequisites()` for unlock checks |
| **Spaced Repetition (08)** | Queries graph for all mastered skills to schedule reviews |
| **Skill Map Screen (01, 03)** | Uses `AllSkills()`, `ByStrand()`, computes state per skill for display |
| **Diagnostic** | Uses `RootSkills()`, `ByStrand()`, `Prerequisites()` for top-down probing |

---

## Testing Strategy

### Unit Tests

- **Graph structure**: Validate returns no error for the seed graph.
- **Cycle detection**: Inject a cycle, assert Validate catches it.
- **Dangling reference**: Add a prerequisite to a nonexistent ID, assert error.
- **RootSkills**: Returns exactly the skills with no prerequisites.
- **IsUnlocked**: True when all prereqs mastered, false when any missing.
- **AvailableSkills**: Correct set for various mastered combinations.
- **FrontierSkills**: Subset of available, prioritized by recency.
- **BlockedSkills**: Complement of unlocked.
- **TopologicalOrder**: Valid topological ordering (every skill appears after its prerequisites).
- **ByStrand / ByGrade**: Correct grouping and ordering.

### Diagnostic Tests

- **Full skip**: Learner aces all probes; all skills marked mastered.
- **No skip**: Learner fails all probes; only root skills available.
- **Partial placement**: Learner passes some strands, fails others; correct mastered set.
- **Transitive closure**: Passing a high-level skill marks all transitive prerequisites as mastered.

---

## Open Questions / Future Considerations

- **Graph versioning**: When new skills are added in a code update, how to handle learners who already have progress? Likely: new skills appear as `locked` or `available` based on existing mastery. No migration needed since the graph is code-embedded and skill IDs are stable.
- **Multiple paths**: Some skills could reasonably have OR-prerequisites (e.g., "know fractions OR decimals"). The current model is AND-only for simplicity. Revisit if needed.
- **Skill retirement**: If a skill is removed from the graph, mastery events for it remain in the event log but are ignored. No special handling needed.
- **K-2 and 6+ expansion**: The architecture supports any number of skills and grades. Add nodes, connect prerequisites, rebuild.
