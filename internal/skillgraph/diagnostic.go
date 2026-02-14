package skillgraph

// DiagnosticResult holds the outcome of a diagnostic placement quiz.
// Diagnostic placement logic will be implemented in a future spec.
// The algorithm (top-down probing across strands) requires the LLM
// problem generation module (specs 04/05) to generate diagnostic questions.
type DiagnosticResult struct {
	MasteredSkillIDs []string
	QuestionsAsked   int
}
