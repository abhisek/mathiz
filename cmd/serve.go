package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/abhisek/mathiz/internal/saas/auth"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/saas/server"
	"github.com/abhisek/mathiz/internal/saas/termbridge"
	"github.com/abhisek/mathiz/internal/saas/webui"
	"github.com/abhisek/mathiz/internal/store"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the Mathiz SaaS server (parent dashboard + browser learning sessions)",
	Long: `Starts the multi-tenant Mathiz server:

  - Parent dashboard API under /api/v1 (Supabase-authenticated)
  - Child join flow (join codes + device tokens)
  - Browser learning sessions: the Mathiz TUI streamed over WebSocket
  - Embedded web app (when built with 'make web')

Configuration is environment-driven:

  MATHIZ_DATABASE_URL         PostgreSQL DSN (required)
  MATHIZ_SUPABASE_URL         Supabase project URL
  MATHIZ_SUPABASE_ANON_KEY    Supabase anon key (served to the SPA)
  MATHIZ_SUPABASE_JWT_SECRET  Legacy HS256 JWT secret (optional)
  MATHIZ_SERVER_ADDR          Listen address (default :8080)
  MATHIZ_*_API_KEY            LLM provider credentials (as for local mode)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServe(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(ctx context.Context) error {
	cfg, err := server.ConfigFromEnv()
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	verifier, err := auth.NewSupabaseVerifier(auth.SupabaseConfig{
		ProjectURL: cfg.SupabaseURL,
		JWTSecret:  cfg.SupabaseJWTSecret,
	})
	if err != nil {
		return err
	}

	// One family service shared by the API server and the terminal bridge.
	svc := family.New(st.Client())
	bridge := termbridge.New(termbridge.Options{
		Store:       st,
		Family:      svc,
		Checker:     authz.NewChecker(svc),
		IdleTimeout: cfg.SessionIdleTimeout,
		MaxSessions: cfg.MaxSessions,
	})
	srv := server.New(cfg, st, svc, verifier, bridge, webui.Handler())

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("mathiz serve listening on %s", cfg.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
