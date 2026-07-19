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

	"github.com/abhisek/mathiz/internal/llm"
	"github.com/abhisek/mathiz/internal/saas/activity"
	"github.com/abhisek/mathiz/internal/saas/auth"
	"github.com/abhisek/mathiz/internal/saas/billing"
	"github.com/abhisek/mathiz/internal/saas/credits"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/saas/game"
	"github.com/abhisek/mathiz/internal/saas/playslot"
	"github.com/abhisek/mathiz/internal/saas/quests"
	"github.com/abhisek/mathiz/internal/saas/server"
	"github.com/abhisek/mathiz/internal/saas/webui"
	"github.com/abhisek/mathiz/internal/store"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the Mathiz SaaS server (parent dashboard + browser learning sessions)",
	Long: `Starts the multi-tenant Mathiz server:

  - Parent dashboard API under /api/v1 (Supabase-authenticated)
  - Child join flow (join codes + device tokens)
  - Browser learning sessions: the treasure-map game API
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

	// The family service backing the API server.
	svc := family.New(st.Client())

	// Monetisation: enabled only when a billing provider is configured.
	// Without one, everything is free (local/self-hosted default).
	var (
		creditsSvc *credits.Service
		billingSvc *billing.Service
		charge     func(ctx context.Context, childUID, sessionID string) error
	)
	switch cfg.BillingProvider {
	case "":
		// billing disabled
	case "fake":
		baseURL := cfg.PublicBaseURL
		if baseURL == "" {
			baseURL = "http://localhost" + cfg.Addr
		}
		creditsSvc = credits.New(st.Client())
		billingSvc = billing.NewService(st.Client(), creditsSvc, billing.NewFakeProvider(baseURL))
	case "stripe":
		if cfg.PublicBaseURL == "" {
			return fmt.Errorf("stripe billing requires MATHIZ_PUBLIC_BASE_URL (checkout redirect target)")
		}
		provider, err := billing.NewStripeProvider(cfg.StripeSecretKey, cfg.StripeWebhookSecret, cfg.PublicBaseURL)
		if err != nil {
			return err
		}
		creditsSvc = credits.New(st.Client())
		billingSvc = billing.NewService(st.Client(), creditsSvc, provider)
	default:
		return fmt.Errorf("unsupported MATHIZ_BILLING_PROVIDER %q (available: fake, stripe; paddle possible later)", cfg.BillingProvider)
	}
	if creditsSvc != nil {
		// One credit per learning session, debited at start. The session ID
		// is the idempotency key; ErrNoCredits maps to a kid-friendly stop.
		charge = func(ctx context.Context, childUID, sessionID string) error {
			child, err := svc.Child(ctx, childUID)
			if err != nil {
				return err
			}
			// Families created before billing was enabled get their free
			// credits here, at the chokepoint — not only on dashboard views.
			if err := creditsSvc.EnsureStarterGrant(ctx, child.FamilySpaceID); err != nil {
				return err
			}
			err = creditsSvc.Debit(ctx, child.FamilySpaceID, 1, "session:"+sessionID)
			if errors.Is(err, credits.ErrInsufficient) {
				return game.ErrNoCredits
			}
			return err
		}
	}

	// One play slot per child: the game manager (and any future play
	// surface) acquires from this shared registry so two live sessions can
	// never write the same child's snapshot at once.
	slots := playslot.NewRegistry()

	// Parent quests: authoring + AI generation (specs/15-quests.md). The
	// generation debit goes through the same credit ledger; with billing
	// off (creditsSvc nil) generation is free. LLM provider from env, no
	// child event stream (generation is parent-initiated).
	questsSvc := quests.New(st.Client(), creditsSvc, func(ctx context.Context) (llm.Provider, error) {
		return llm.NewProviderFromEnv(ctx, nil)
	})

	// Activity timeline read model: merges the child's event streams for the
	// parent dashboard. Quest attribution resolves through the quests service
	// and author names through family accounts (display name, else email).
	activityReader := activity.NewReader(st, questsSvc, func(ctx context.Context, accountID string) (string, error) {
		a, err := svc.Account(ctx, accountID)
		if err != nil {
			return "", err
		}
		if a.DisplayName != "" {
			return a.DisplayName, nil
		}
		return a.Email, nil
	})

	gameMgr := game.NewManager(game.Config{
		Store:       st,
		IdleTimeout: cfg.SessionIdleTimeout,
		Charge:      charge,
		Slots:       slots,
		Quests:      questsSvc,
	})
	srv := server.New(server.Deps{
		Config:   cfg,
		Store:    st,
		Family:   svc,
		Verifier: verifier,
		WebUI:    webui.Handler(),
		Game:     gameMgr,
		Credits:  creditsSvc,
		Billing:  billingSvc,
		Quests:   questsSvc,
		Activity: activityReader,
	})

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
