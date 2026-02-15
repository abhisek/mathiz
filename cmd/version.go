package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set via -ldflags at build time.
var version = "(devel)"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the current version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("mathiz", version)
	},
}
