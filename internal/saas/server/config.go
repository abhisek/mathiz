package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/abhisek/mathiz/internal/saas/auth"
)

// Config holds all settings for `mathiz serve`.
type Config struct {
	// Addr is the HTTP listen address (host:port).
	Addr string

	// DatabaseURL is the PostgreSQL DSN (postgres://...). SQLite paths are
	// accepted too, which is handy for tests and single-box setups.
	DatabaseURL string

	// AuthProvider selects the parent auth preset: "supabase" or "test"
	// (specs/17-auth-providers.md). One provider per deployment.
	AuthProvider string

	// AuthURL is the provider's base URL (Supabase: the project URL). It
	// pins the token issuer and is served to the SPA.
	AuthURL string

	// AuthPublicKey is the provider's public client key (Supabase: the
	// anon/publishable key), served to the SPA via /api/v1/config.
	AuthPublicKey string

	// AuthHS256Secret enables HS256 verification: required by the test
	// preset, optional for Supabase projects that sign with a shared secret.
	AuthHS256Secret string

	// AuthAudience overrides the preset's expected aud claim.
	AuthAudience string

	// AuthAllowTest must be set for the test preset, whose tokens anyone
	// holding the secret can mint.
	AuthAllowTest bool

	// CORSOrigins optionally allows cross-origin SPA deployments.
	CORSOrigins []string

	// TrustProxy trusts the last X-Forwarded-For hop for client IPs (set it
	// when running behind a reverse proxy, or rate limiting keys on the
	// proxy's address and throttles everyone together).
	TrustProxy bool

	// SessionIdleTimeout ends idle learning sessions (game expeditions).
	SessionIdleTimeout time.Duration

	// BillingProvider enables monetisation: "" (off — everything free,
	// the self-hoster default), "fake" (dev), "stripe"/"paddle" (planned).
	BillingProvider string

	// StripeSecretKey / StripeWebhookSecret configure the stripe provider
	// (secret API key; webhook signing secret from the endpoint config).
	StripeSecretKey     string
	StripeWebhookSecret string

	// PublicBaseURL is this server's externally reachable origin, used for
	// billing redirect URLs (defaults to http://localhost + Addr).
	PublicBaseURL string

	// PostHogAPIKey enables product analytics in the SPA (served via
	// /api/v1/config, public by design — PostHog project keys are client
	// keys). Empty = analytics fully off: the config response omits the
	// posthog fields, the /relay proxy is not mounted, and the SPA never
	// loads the analytics module.
	PostHogAPIKey string

	// PostHogHost is the PostHog ingestion host the server-side /relay
	// proxy forwards to; defaults to the US cloud when a key is set but the
	// host is empty. Never served to the browser — the SPA is handed the
	// same-origin /relay path instead.
	PostHogHost string

	// LogFile optionally tees the structured log to an append-mode file in
	// addition to stdout. Unset = stdout only. If the file can't be opened,
	// startup fails. No built-in rotation — rotate externally.
	LogFile string

	// LogFormat selects the slog handler: "text" (default; canonical
	// key=value) or "json".
	LogFormat string

	// LogLevel is the minimum level: debug|info|warn|error (default info).
	LogLevel string
}

// ConfigFromEnv builds a Config from MATHIZ_* environment variables.
func ConfigFromEnv() (*Config, error) {
	cfg := &Config{
		Addr:                envOr("MATHIZ_SERVER_ADDR", ":8080"),
		DatabaseURL:         os.Getenv("MATHIZ_DATABASE_URL"),
		AuthProvider:        os.Getenv("MATHIZ_AUTH_PROVIDER"),
		AuthURL:             strings.TrimRight(os.Getenv("MATHIZ_AUTH_URL"), "/"),
		AuthPublicKey:       os.Getenv("MATHIZ_AUTH_PUBLIC_KEY"),
		AuthHS256Secret:     os.Getenv("MATHIZ_AUTH_HS256_SECRET"),
		AuthAudience:        os.Getenv("MATHIZ_AUTH_AUDIENCE"),
		AuthAllowTest:       os.Getenv("MATHIZ_AUTH_ALLOW_TEST") == "1",
		SessionIdleTimeout:  time.Duration(envIntOr("MATHIZ_SESSION_IDLE_MINUTES", 30)) * time.Minute,
		TrustProxy:          os.Getenv("MATHIZ_TRUST_PROXY") == "true",
		BillingProvider:     os.Getenv("MATHIZ_BILLING_PROVIDER"),
		StripeSecretKey:     os.Getenv("MATHIZ_STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("MATHIZ_STRIPE_WEBHOOK_SECRET"),
		PublicBaseURL:       os.Getenv("MATHIZ_PUBLIC_BASE_URL"),
		PostHogAPIKey:       os.Getenv("MATHIZ_POSTHOG_API_KEY"),
		PostHogHost:         strings.TrimRight(os.Getenv("MATHIZ_POSTHOG_HOST"), "/"),
		LogFile:             os.Getenv("MATHIZ_LOG_FILE"),
		LogFormat:           envOr("MATHIZ_LOG_FORMAT", "text"),
		LogLevel:            envOr("MATHIZ_LOG_LEVEL", "info"),
	}
	if cfg.PostHogAPIKey != "" && cfg.PostHogHost == "" {
		cfg.PostHogHost = "https://us.i.posthog.com"
	}
	// The MATHIZ_SUPABASE_* names are gone. An old .env must fail loudly,
	// not boot into a half-configured state.
	for old, repl := range removedAuthVars {
		if os.Getenv(old) != "" {
			return nil, fmt.Errorf("%s was removed: use %s (see .env.example)", old, repl)
		}
	}
	if origins := os.Getenv("MATHIZ_CORS_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if o = strings.TrimSpace(o); o != "" {
				cfg.CORSOrigins = append(cfg.CORSOrigins, o)
			}
		}
	}
	return cfg, cfg.Validate()
}

// Validate enforces the fail-fast requirements from the spec.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("MATHIZ_DATABASE_URL is required (postgres://... for SaaS mode)")
	}
	switch c.AuthProvider {
	case auth.ProviderSupabase:
		if c.AuthURL == "" {
			return fmt.Errorf("MATHIZ_AUTH_URL is required for the supabase provider (the project URL)")
		}
		if c.AuthPublicKey == "" {
			return fmt.Errorf("MATHIZ_AUTH_PUBLIC_KEY is required for the supabase provider (the anon key)")
		}
	case auth.ProviderTest:
		if !c.AuthAllowTest {
			return fmt.Errorf("the test auth provider needs MATHIZ_AUTH_ALLOW_TEST=1")
		}
		if c.AuthHS256Secret == "" {
			return fmt.Errorf("the test auth provider needs MATHIZ_AUTH_HS256_SECRET")
		}
		// A public HTTPS origin is a real deployment: test tokens there
		// would let anyone with the secret sign in as any parent.
		if strings.HasPrefix(c.PublicBaseURL, "https://") {
			return fmt.Errorf("the test auth provider is refused when MATHIZ_PUBLIC_BASE_URL is https")
		}
	case "":
		return fmt.Errorf("MATHIZ_AUTH_PROVIDER is required (available: %s, %s)", auth.ProviderSupabase, auth.ProviderTest)
	default:
		return fmt.Errorf("unsupported MATHIZ_AUTH_PROVIDER %q (available: %s, %s)", c.AuthProvider, auth.ProviderSupabase, auth.ProviderTest)
	}
	switch c.LogFormat {
	case "", "text", "json":
	default:
		return fmt.Errorf("unsupported MATHIZ_LOG_FORMAT %q (available: text, json)", c.LogFormat)
	}
	switch c.LogLevel {
	case "", "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("unsupported MATHIZ_LOG_LEVEL %q (available: debug, info, warn, error)", c.LogLevel)
	}
	return nil
}

// removedAuthVars maps each removed variable to its replacement.
var removedAuthVars = map[string]string{
	"MATHIZ_SUPABASE_URL":        "MATHIZ_AUTH_PROVIDER=supabase and MATHIZ_AUTH_URL",
	"MATHIZ_SUPABASE_ANON_KEY":   "MATHIZ_AUTH_PUBLIC_KEY",
	"MATHIZ_SUPABASE_JWT_SECRET": "MATHIZ_AUTH_HS256_SECRET",
}

// VerifierConfig is the auth preset configuration for this deployment.
func (c *Config) VerifierConfig() auth.Config {
	return auth.Config{
		Provider:    c.AuthProvider,
		URL:         c.AuthURL,
		HS256Secret: c.AuthHS256Secret,
		Audience:    c.AuthAudience,
		AllowTest:   c.AuthAllowTest,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
