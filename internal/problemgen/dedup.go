package problemgen

import (
	"fmt"
	"strings"
)

// buildDedup formats prior questions for the prompt, respecting the max limit.
// Returns "None" if there are no prior questions.
func buildDedup(priorQuestions []string, max int) string {
	if len(priorQuestions) == 0 {
		return "None"
	}

	// Keep only the most recent N questions.
	if max > 0 && len(priorQuestions) > max {
		priorQuestions = priorQuestions[len(priorQuestions)-max:]
	}

	var b strings.Builder
	for i, q := range priorQuestions {
		fmt.Fprintf(&b, "%d. %s\n", i+1, q)
	}
	return strings.TrimRight(b.String(), "\n")
}
