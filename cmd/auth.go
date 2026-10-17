package cmd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/store"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage parent auth for a mathiz serve deployment",
}

var authRemapCmd = &cobra.Command{
	Use:   "remap",
	Short: "Move every parent account from one auth provider to another",
	Long: `Moves parent accounts from one auth provider to another, in one
transaction (specs/17-auth-providers.md §6). Stop mathiz serve first.

The map is a CSV of old_subject,new_subject (an optional header row with
those names is skipped): each parent's user ID at the old provider and at
the new one. Every account of --from must be in the map, or nothing is
written. A rerun with the same map changes nothing.

Reads MATHIZ_DATABASE_URL.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Flags parsed: from here an error is about the data, not usage.
		cmd.SilenceUsage = true
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		mapPath, _ := cmd.Flags().GetString("map")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		dsn := os.Getenv("MATHIZ_DATABASE_URL")
		if dsn == "" {
			return errors.New("MATHIZ_DATABASE_URL is required")
		}
		f, err := os.Open(mapPath)
		if err != nil {
			return fmt.Errorf("open map: %w", err)
		}
		defer f.Close()
		pairs, err := readRemapCSV(f)
		if err != nil {
			return err
		}

		st, err := store.Open(dsn)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer st.Close()

		res, err := family.New(st.Client()).RemapProvider(cmd.Context(), from, to, pairs, dryRun)
		if err != nil {
			return err
		}
		verb := "remapped"
		if dryRun {
			verb = "would remap"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %d accounts from %s to %s (%d rows already done)\n",
			verb, res.Remapped, from, to, res.AlreadyDone)
		return err
	},
}

// readRemapCSV parses old_subject,new_subject rows.
func readRemapCSV(r io.Reader) ([]family.RemapPair, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = 2
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read map: %w", err)
	}
	if len(records) > 0 && strings.EqualFold(records[0][0], "old_subject") && strings.EqualFold(records[0][1], "new_subject") {
		records = records[1:]
	}
	if len(records) == 0 {
		return nil, errors.New("map has no rows")
	}
	pairs := make([]family.RemapPair, len(records))
	for i, rec := range records {
		pairs[i] = family.RemapPair{OldSubject: strings.TrimSpace(rec[0]), NewSubject: strings.TrimSpace(rec[1])}
	}
	return pairs, nil
}

func init() {
	authRemapCmd.Flags().String("from", "", "provider the accounts belong to now (required)")
	authRemapCmd.Flags().String("to", "", "provider to move them to (required)")
	authRemapCmd.Flags().String("map", "", "CSV of old_subject,new_subject (required)")
	authRemapCmd.Flags().Bool("dry-run", false, "check the map and print the counts without writing")
	for _, f := range []string{"from", "to", "map"} {
		_ = authRemapCmd.MarkFlagRequired(f)
	}
	authCmd.AddCommand(authRemapCmd)
	rootCmd.AddCommand(authCmd)
}
