package problemgen

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are a math tutor creating practice problems for children in grades 3-5.

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
- Do not repeat any question from the "already asked" list.`

// buildUserMessage constructs the user message from GenerateInput and Config limits.
func buildUserMessage(input GenerateInput, cfg Config) string {
	tierLabel := "learn"
	hintsAllowed := true
	if input.Tier == 1 { // TierProve
		tierLabel = "prove"
		hintsAllowed = false
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Skill: %s\n", input.Skill.Name)
	fmt.Fprintf(&b, "Description: %s\n", input.Skill.Description)
	fmt.Fprintf(&b, "Grade: %d\n", input.Skill.GradeLevel)
	fmt.Fprintf(&b, "Keywords: %s\n", strings.Join(input.Skill.Keywords, ", "))
	fmt.Fprintf(&b, "Tier: %s\n", tierLabel)
	fmt.Fprintf(&b, "Hints allowed: %t\n", hintsAllowed)

	b.WriteString("\nAlready asked in this session:\n")
	b.WriteString(buildDedup(input.PriorQuestions, cfg.MaxPriorQuestions))

	b.WriteString("\nRecent errors by this student:\n")
	b.WriteString(buildErrors(input.RecentErrors, cfg.MaxRecentErrors))

	if input.LearnerProfile != "" {
		b.WriteString("\n\nLearner profile:\n")
		b.WriteString(input.LearnerProfile)
	}

	return b.String()
}

// buildErrors formats recent errors for the prompt, respecting the max limit.
func buildErrors(errors []string, max int) string {
	if len(errors) == 0 {
		return "None"
	}

	// Keep only the most recent N errors.
	if max > 0 && len(errors) > max {
		errors = errors[len(errors)-max:]
	}

	var b strings.Builder
	for i, e := range errors {
		fmt.Fprintf(&b, "%d. %s\n", i+1, e)
	}
	return strings.TrimRight(b.String(), "\n")
}
