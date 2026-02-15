package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/abhisek/mathiz/internal/selfupdate"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update mathiz to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		checker := selfupdate.NewChecker(selfupdate.WithTimeout(2 * time.Minute))

		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()

		err := checker.Update(ctx, &selfupdate.UpdateInput{
			CurrentVersion: version,
		}, func(p selfupdate.UpdateProgress) {
			fmt.Println(p.Message)
		})

		if err == nil {
			return nil
		}

		if errors.Is(err, selfupdate.ErrDevBuild) {
			fmt.Println("Cannot update a development build. Install a release build first.")
			return nil
		}
		if errors.Is(err, selfupdate.ErrAlreadyLatest) {
			fmt.Println("Already running the latest version.")
			return nil
		}
		if os.IsPermission(err) {
			return fmt.Errorf("%w\n\nTry running: sudo mathiz update", err)
		}

		return err
	},
}
