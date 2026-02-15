package cmd

import (
	"fmt"
	"os"

	"github.com/abhisek/mathiz/internal/app"
	"github.com/abhisek/mathiz/internal/diagnosis"
	"github.com/abhisek/mathiz/internal/gems"
	"github.com/abhisek/mathiz/internal/lessons"
	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/problemgen"
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

	eventRepo := st.EventRepo()
	opts := app.Options{
		EventRepo:    eventRepo,
		SnapshotRepo: st.SnapshotRepo(),
		GemService:   gems.NewService(eventRepo),
	}

	provider, err := llm.NewProviderFromEnv(ctx, eventRepo)
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
}
