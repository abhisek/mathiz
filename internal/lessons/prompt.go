package lessons

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

const lessonSystemPrompt = `You are a patient, encouraging math tutor for children in grades 3-5. A student is struggling with a math concept and needs a short, clear lesson.`

func buildLessonUserMessage(input LessonInput) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Skill: %s\n", input.Skill.Name))
	b.WriteString(fmt.Sprintf("Description: %s\n", input.Skill.Description))
	b.WriteString(fmt.Sprintf("Grade: %d\n", input.Skill.GradeLevel))
	b.WriteString(fmt.Sprintf("Student accuracy on this skill: %.0f%%\n", input.Accuracy*100))

	b.WriteString("\nRecent Errors:\n")
	if len(input.RecentErrors) == 0 {
		b.WriteString("None\n")
	} else {
		for _, e := range input.RecentErrors {
			b.WriteString(fmt.Sprintf("- %s\n", e))
		}
	}

	if input.LastDiagnosis != nil {
		b.WriteString(fmt.Sprintf("\nDiagnosed Issue:\nCategory: %s\n", input.LastDiagnosis.Category))
		if input.LastDiagnosis.MisconceptionID != "" {
			b.WriteString(fmt.Sprintf("Misconception: %s\n", input.LastDiagnosis.MisconceptionID))
		}
	}

	b.WriteString(`
Instructions:
Create a micro-lesson that:
1. Explains the concept clearly in 3-5 sentences. Use simple language a child would understand. Address the specific errors shown above.
2. Shows a complete worked example with numbered steps. Pick a problem similar to (but different from) the ones the student got wrong. Show every step.
3. Creates one practice question that is EASIER than the ones the student got wrong. The student should be able to solve it using the explanation and worked example above.
4. The practice question must have a single correct answer. Provide a brief explanation for the practice answer.
5. Use plain ASCII text for all math. No LaTeX, no Unicode symbols. Use / for fractions, * for multiplication.`)

	return b.String()
}

const compressionSystemPrompt = `You are summarizing a math student's error patterns on a specific skill. Create a concise summary that captures the key patterns without losing important details.`

func buildCompressionUserMessage(errors []string) string {
	var b strings.Builder

	b.WriteString("Errors:\n")
	for _, e := range errors {
		b.WriteString(fmt.Sprintf("- %s\n", e))
	}

	b.WriteString(`
Instructions:
Summarize these errors in 2-3 sentences. Focus on:
- What types of mistakes the student is making (e.g., forgetting to carry, confusing operations)
- Any patterns you see across multiple errors
- What the student seems to understand vs. what they're struggling with

Keep the summary concise and factual. Do not include encouragement or advice — this summary is used internally for generating better practice questions.`)

	return b.String()
}

const profileSystemPrompt = `You are creating a learner profile for a math tutoring system. This profile helps personalize future practice sessions for a student in grades 3-5.`

// maxSessionErrorsPerSkill caps this session's raw error lines. The error
// patterns section already carries the diagnosed signal across sessions, and
// a long list of raw lines lets one session dominate the prompt.
const maxSessionErrorsPerSkill = 3

// buildProfileUserMessage puts lifetime history first and labels this
// session as the newest evidence. With the session first and alone, the
// model rewrote the whole-child profile from five answers on one skill.
func buildProfileUserMessage(input ProfileInput) string {
	var b strings.Builder

	b.WriteString("Learner History (all sessions):\n")
	if input.SessionCount > 0 {
		fmt.Fprintf(&b, "Sessions completed: %d\n", input.SessionCount)
	}
	states := map[string]int{}
	for _, d := range input.MasteryData {
		states[d.State]++
	}
	fmt.Fprintf(&b, "Skills mastered: %d, learning: %d, rusty: %d\n",
		states["mastered"], states["learning"], states["rusty"])
	b.WriteString("Per skill (lifetime answers; skills never attempted are not listed):\n")
	for _, id := range sortedSkills(input.MasteryData) {
		d := input.MasteryData[id]
		if d.Attempts == 0 {
			continue
		}
		// Answers can outlive the snapshot entry (legacy snapshots), and a
		// blank "state=" reads as a value.
		state, tier := cmp.Or(d.State, "unknown"), cmp.Or(d.Tier, "unknown")
		fmt.Fprintf(&b, "- %s: grade %d, state=%s, tier=%s, %d/%d correct (%.0f%%), fluency=%.2f\n",
			skillLabel(id, d.Name), d.Grade, state, tier, d.Correct, d.Attempts,
			float64(d.Correct)/float64(d.Attempts)*100, d.FluencyScore)
	}

	if len(input.RecentSessions) > 0 {
		b.WriteString("\nRecent Sessions (newest first):\n")
		for _, s := range input.RecentSessions {
			kind := ""
			if s.Quest {
				kind = " (quest)"
			}
			fmt.Fprintf(&b, "- %s%s: %s, %d/%d correct\n",
				s.EndedAt.UTC().Format(time.DateOnly), kind, strings.Join(s.Skills, ", "), s.Correct, s.Questions)
		}
	}

	if len(input.ErrorPatterns) > 0 {
		b.WriteString("\nDiagnosed Error Patterns (recent wrong answers, across sessions):\n")
		for _, e := range input.ErrorPatterns {
			kind := e.Category
			if e.Misconception != "" {
				kind += ": " + e.Misconception
			}
			fmt.Fprintf(&b, "- %s: %s (%d times)\n", skillLabel(e.SkillID, e.SkillName), kind, e.Count)
		}
	}

	b.WriteString("\nThis Session (newest evidence only):\n")
	for _, id := range sortedSkills(input.PerSkillResults) {
		result := input.PerSkillResults[id]
		// A skill with no attempts carries no signal; rendering it as "0%"
		// reads to the model as failure rather than absence.
		if result.Attempted == 0 {
			continue
		}
		pct := float64(result.Correct) / float64(result.Attempted) * 100
		fmt.Fprintf(&b, "- %s: %d attempted, %d correct (%.0f%%)\n",
			skillLabel(id, result.Name), result.Attempted, result.Correct, pct)
		errs := input.ErrorHistory[id]
		if len(errs) > maxSessionErrorsPerSkill {
			errs = errs[len(errs)-maxSessionErrorsPerSkill:]
		}
		for _, e := range errs {
			fmt.Fprintf(&b, "  - error: %s\n", e)
		}
	}

	if input.PreviousProfile != nil {
		b.WriteString("\nPrevious Profile")
		if !input.PreviousProfile.GeneratedAt.IsZero() {
			fmt.Fprintf(&b, " (from %s)", input.PreviousProfile.GeneratedAt.UTC().Format(time.DateOnly))
		}
		fmt.Fprintf(&b, ":\n%s\n", input.PreviousProfile.Summary)
		fmt.Fprintf(&b, "Strengths: %s\n", strings.Join(input.PreviousProfile.Strengths, ", "))
		fmt.Fprintf(&b, "Weaknesses: %s\n", strings.Join(input.PreviousProfile.Weaknesses, ", "))
		// Patterns are demanded back out by the schema, so withholding the
		// previous ones made them churn from scratch every session.
		fmt.Fprintf(&b, "Patterns: %s\n", strings.Join(input.PreviousProfile.Patterns, ", "))
	}

	b.WriteString(`
Instructions:
Create a concise learner profile of the whole student, based on the learner history above:
1. Write a 3-5 sentence summary of the student's current abilities, focusing on what they know well and where they need work.
2. List 2-4 specific strengths (e.g., "solid multiplication facts", "good with simple fractions").
3. List 2-4 specific weaknesses (e.g., "struggles with carrying in addition", "confuses fraction denominators").
4. List 1-3 error patterns observed (e.g., "frequently rushes and makes careless mistakes", "consistently forgets to borrow in subtraction").

Base strengths and weaknesses on lifetime evidence. Use this session to confirm, refine or flag a change. Do not add or remove a strength or weakness because of this session alone when the lifetime evidence disagrees. Name only skills listed above, and say nothing about skills the student has not attempted.
If a previous profile exists, update it with new evidence rather than starting fresh. Keep all entries concise (5-10 words each for strengths/weaknesses/patterns).`)

	return b.String()
}

// sortedSkills orders skill IDs so the same evidence renders as the same
// prompt: map order is random, and a reordered prompt reads as a change.
func sortedSkills[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

// skillLabel renders a skill for the model: its name when known, with the ID
// alongside for traceability. A bare ID like "mult-2digit" is not something
// the model can reason about.
func skillLabel(id, name string) string {
	if name == "" {
		return id
	}
	return fmt.Sprintf("%s (%s)", name, id)
}
