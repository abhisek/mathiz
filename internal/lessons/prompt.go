package lessons

import (
	"fmt"
	"strings"
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

func buildProfileUserMessage(input ProfileInput) string {
	var b strings.Builder

	b.WriteString("Session Results:\n")
	for skillID, result := range input.PerSkillResults {
		var pct float64
		if result.Attempted > 0 {
			pct = float64(result.Correct) / float64(result.Attempted) * 100
		}
		b.WriteString(fmt.Sprintf("- %s: %d attempted, %d correct (%.0f%%)\n", skillID, result.Attempted, result.Correct, pct))
	}

	b.WriteString("\nMastery State:\n")
	for skillID, data := range input.MasteryData {
		b.WriteString(fmt.Sprintf("- %s: state=%s, fluency=%.2f\n", skillID, data.State, data.FluencyScore))
	}

	if len(input.ErrorHistory) > 0 {
		b.WriteString("\nError History:\n")
		for skillID, errors := range input.ErrorHistory {
			b.WriteString(fmt.Sprintf("### %s\n", skillID))
			for _, e := range errors {
				b.WriteString(fmt.Sprintf("- %s\n", e))
			}
		}
	}

	if input.PreviousProfile != nil {
		b.WriteString(fmt.Sprintf("\nPrevious Profile:\n%s\n", input.PreviousProfile.Summary))
		b.WriteString(fmt.Sprintf("Strengths: %s\n", strings.Join(input.PreviousProfile.Strengths, ", ")))
		b.WriteString(fmt.Sprintf("Weaknesses: %s\n", strings.Join(input.PreviousProfile.Weaknesses, ", ")))
	}

	b.WriteString(`
Instructions:
Create a concise learner profile:
1. Write a 3-5 sentence summary of the student's current abilities, focusing on what they know well and where they need work.
2. List 2-4 specific strengths (e.g., "solid multiplication facts", "good with simple fractions").
3. List 2-4 specific weaknesses (e.g., "struggles with carrying in addition", "confuses fraction denominators").
4. List 1-3 error patterns observed (e.g., "frequently rushes and makes careless mistakes", "consistently forgets to borrow in subtraction").

If a previous profile exists, update it with new evidence rather than starting fresh. Keep all entries concise (5-10 words each for strengths/weaknesses/patterns).`)

	return b.String()
}
