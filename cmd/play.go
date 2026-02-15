package cmd

import (
	"github.com/spf13/cobra"
)

var playCmd = &cobra.Command{
	Use:   "play",
	Short: "Start a practice session",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runApp(cmd)
	},
}
