package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
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

  - Parent dashboard API under /api/v1 (auth-provider JWTs)
  - Child join flow (join codes + device tokens)
  - Browser learning sessions: the treasure-map game API
  - Embedded web app (when built with 'make web')

Configuration is environment-driven:

  MATHIZ_DATABASE_URL         PostgreSQL DSN (required)
  MATHIZ_AUTH_PROVIDER        Parent auth provider: supabase | test (required)
  MATHIZ_AUTH_URL             Provider URL (supabase: the project URL)
  MATHIZ_AUTH_PUBLIC_KEY      Provider client key, served to the SPA (supabase: anon key)
  MATHIZ_AUTH_HS256_SECRET    Shared signing secret (test; HS256 Supabase projects)
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

	// Structured logging: stdout always, plus an optional file tee
	// (MATHIZ_LOG_FILE). Fails fast on an unopenable file — a silently
	// dropped log path is worse than a crash at boot.
	logger, closeLog, err := server.NewLogger(os.Stdout, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeLog() }()
	// Bridge the stdlib log package too: stray log.Printf from dependencies
	// lands in the same stream instead of vanishing to stderr.
	slog.SetDefault(logger)

	st, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	verifier, err := auth.NewVerifier(cfg.VerifierConfig())
	if err != nil {
		return err
	}
	defer verifier.Close()

	// The family service backing the API server.
	svc := family.New(st.Client())

	if err := checkAccounts(ctx, st, svc, verifier.Provider()); err != nil {
		return err
	}
	if err := verifier.Warm(); err != nil {
		// Not fatal: the provider may be briefly down, and logins retry.
		logger.Warn("auth: provider keys unreachable at boot", "provider", verifier.Provider(), "err", err)
	}

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
	// off (creditsSvc nil) generation is free.
	//
	// Usage is logged to a family-space-scoped event stream: generation is
	// parent-initiated so there is no child to attribute it to, but leaving
	// it unlogged made bad AI output impossible to investigate after the
	// fact and hid a billed action from cost accounting.
	questsSvc := quests.New(st.Client(), creditsSvc, func(ctx context.Context, familySpaceID string) (llm.Provider, error) {
		return llm.NewProviderFromEnv(ctx, st.EventRepoFor(quests.LLMOwnerID(familySpaceID)))
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
		Logger:   logger,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Canonical startup summary: address, database kind, and which optional
	// services this process is running.
	dbKind := "sqlite"
	if strings.HasPrefix(cfg.DatabaseURL, "postgres://") || strings.HasPrefix(cfg.DatabaseURL, "postgresql://") {
		dbKind = "postgres"
	}
	billingProvider := "off"
	if billingSvc != nil {
		billingProvider = billingSvc.Provider().Name()
	}
	logger.Info("mathiz serve listening",
		"addr", cfg.Addr,
		"db", dbKind,
		"game", gameMgr != nil,
		"quests", questsSvc != nil,
		"billing", billingSvc != nil,
		"billing_provider", billingProvider,
		"auth_provider", verifier.Provider(),
		"analytics", cfg.PostHogAPIKey != "",
		"activity", activityReader != nil,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// checkAccounts refuses to start on an accounts table this release can't
// serve: a column from before pluggable auth, or families that belong to a
// provider other than the configured one (a config typo would otherwise
// lock every parent out of their family).
func checkAccounts(ctx context.Context, st *store.Store, svc *family.Service, provider string) error {
	// Auto-migrate never drops columns, and this one is NOT NULL: every new
	// account insert would fail.
	legacy, err := st.ColumnExists(ctx, "accounts", "supabase_user_id")
	if err != nil {
		return err
	}
	if legacy {
		return errors.New(`database predates pluggable auth (specs/17): run "ALTER TABLE accounts DROP COLUMN supabase_user_id;" or start on a fresh database`)
	}

	foreign, err := svc.CountForeignAccounts(ctx, provider)
	if err != nil {
		return err
	}
	if len(foreign) > 0 {
		other := slices.Sorted(maps.Keys(foreign))[0]
		return fmt.Errorf(`auth: %d accounts belong to provider %q, configured %q: run "mathiz auth remap --from %s --to %s" (specs/17 §6)`,
			foreign[other], other, provider, other, provider)
	}
	return nil
}
