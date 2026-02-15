package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mathiz",
	Short: "AI math tutor for kids",
	Long:  "Mathiz — AI-native terminal app that helps children (grades 3-5) build math mastery.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApp(cmd.Context())
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(playCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(statsCmd)
}
