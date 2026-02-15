package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var playCmd = &cobra.Command{
	Use:   "play",
	Short: "Start a practice session directly",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApp(cmd)
	},
}

func init() {
	// Mark play command so runApp can detect it.
	playCmd.Annotations = map[string]string{"direct_session": "true"}
}

// isDirectSession returns true when the command requests a direct session.
func isDirectSession(cmd *cobra.Command) bool {
	_, ok := cmd.Annotations["direct_session"]
	return ok
}

// errNoLLM is returned when play is invoked without a configured LLM provider.
var errNoLLM = fmt.Errorf("mathiz play requires a configured LLM provider.\n" +
	"Set MATHIZ_LLM_PROVIDER and the corresponding API key environment variable.\n" +
	"Example: MATHIZ_LLM_PROVIDER=anthropic MATHIZ_ANTHROPIC_API_KEY=sk-...")
