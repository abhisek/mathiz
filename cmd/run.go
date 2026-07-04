package cmd

import (
	"fmt"
	"os"

	"github.com/abhisek/mathiz/internal/app"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/selfupdate"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/spf13/cobra"
)

// runApp opens the store, builds dependencies, and launches the TUI.
func runApp(cmd *cobra.Command) error {
	ctx := cmd.Context()
	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return fmt.Errorf("resolve DB path: %w", err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// Start async version check (non-blocking).
	checker := selfupdate.NewChecker()
	updateCh := checker.CheckAsync(ctx, &selfupdate.CheckInput{Version: version})

	eventRepo := st.EventRepo()
	provider, err := llm.NewProviderFromEnv(ctx, eventRepo)
	if err != nil {
		if isDirectSession(cmd) {
			return errNoLLM
		}
		fmt.Fprintln(os.Stderr, "LLM provider not configured:", err)
		fmt.Fprintln(os.Stderr, "AI features will be unavailable.")
		provider = nil
	}

	opts, cleanup := app.BuildOptions(eventRepo, st.SnapshotRepo(), provider)
	defer cleanup()
	opts.UpdateCh = updateCh
	opts.DirectSession = isDirectSession(cmd)

	return app.Run(opts)
}
