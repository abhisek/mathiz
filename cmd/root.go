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
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(previewCmd)
	rootCmd.AddCommand(skillCmd)
}

// resolveDBPath returns the database DSN using --db flag (highest priority),
// then MATHIZ_DB env var, then the default XDG path. Directory creation only
// applies to SQLite file paths — a postgres:// DSN is passed through as-is.
func resolveDBPath(cmd *cobra.Command) (string, error) {
	if p, _ := cmd.Flags().GetString("db"); p != "" {
		if store.IsPostgresDSN(p) {
			return p, nil
		}
		return p, store.EnsureDir(p)
	}
	return store.DefaultDBPath()
}
