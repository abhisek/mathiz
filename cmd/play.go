package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/abhisek/mathiz/internal/app"
	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
	"github.com/abhisek/mathiz/internal/store"
	"github.com/spf13/cobra"
)

var playCmd = &cobra.Command{
	Use:   "play",
	Short: "Start a practice session",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// Open store for event logging.
		dbPath, err := store.DefaultDBPath()
		if err != nil {
			return fmt.Errorf("resolve DB path: %w", err)
		}
		st, err := store.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer st.Close()

		// Build LLM provider (optional — app works without it).
		opts := app.Options{
			EventRepo:    st.EventRepo(),
			SnapshotRepo: st.SnapshotRepo(),
		}
		provider, err := llm.NewProviderFromEnv(ctx, st.EventRepo())
		if err != nil {
			fmt.Fprintln(os.Stderr, "LLM provider not configured:", err)
			fmt.Fprintln(os.Stderr, "AI features will be unavailable.")
		} else {
			opts.LLMProvider = provider
			opts.Generator = problemgen.New(provider, problemgen.DefaultConfig())
			diagService := diagnosis.NewService(provider)
			defer diagService.Close()
			opts.DiagnosisService = diagService
			opts.LessonService = lessons.NewService(provider, lessons.DefaultConfig())
			opts.Compressor = lessons.NewCompressor(provider, lessons.DefaultCompressorConfig())
		}

		return app.Run(opts)
	},
}

func init() {
	// Context for provider initialization.
	playCmd.SetContext(context.Background())
}
