package cmd

import (
	"github.com/abhisek/mathiz/internal/store"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mathiz",
	Short: "AI math tutor for kids",
	Long:  "Mathiz — AI-native terminal app that helps children (grades 3-5) build math mastery.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApp(cmd)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().String("db", "", "Path to SQLite database file (overrides MATHIZ_DB env var)")

	rootCmd.AddCommand(playCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(llmCmd)
}

// resolveDBPath returns the database path using --db flag (highest priority),
// then MATHIZ_DB env var, then the default XDG path.
func resolveDBPath(cmd *cobra.Command) (string, error) {
	if p, _ := cmd.Flags().GetString("db"); p != "" {
		return p, store.EnsureDir(p)
	}
	return store.DefaultDBPath()
}
