package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all settings for `mathiz serve`.
type Config struct {
	// Addr is the HTTP listen address (host:port).
	Addr string

	// DatabaseURL is the PostgreSQL DSN (postgres://...). SQLite paths are
	// accepted too, which is handy for tests and single-box setups.
	DatabaseURL string

	// SupabaseURL is the Supabase project URL.
	SupabaseURL string

	// SupabaseAnonKey is served to the SPA via /api/v1/config (public by design).
	SupabaseAnonKey string

	// SupabaseJWTSecret enables HS256 token verification.
	SupabaseJWTSecret string

	// CORSOrigins optionally allows cross-origin SPA deployments.
	CORSOrigins []string

	// TrustProxy trusts the last X-Forwarded-For hop for client IPs (set it
	// when running behind a reverse proxy, or rate limiting keys on the
	// proxy's address and throttles everyone together).
	TrustProxy bool

	// MaxSessions caps concurrent terminal sessions.
	MaxSessions int

	// SessionIdleTimeout disconnects idle terminal sessions.
	SessionIdleTimeout time.Duration

	// BillingProvider enables monetisation: "" (off — everything free,
	// the self-hoster default), "fake" (dev), "stripe"/"paddle" (planned).
	BillingProvider string

	// PublicBaseURL is this server's externally reachable origin, used for
	// billing redirect URLs (defaults to http://localhost + Addr).
	PublicBaseURL string
}

// ConfigFromEnv builds a Config from MATHIZ_* environment variables.
func ConfigFromEnv() (*Config, error) {
	cfg := &Config{
		Addr:               envOr("MATHIZ_SERVER_ADDR", ":8080"),
		DatabaseURL:        os.Getenv("MATHIZ_DATABASE_URL"),
		SupabaseURL:        strings.TrimRight(os.Getenv("MATHIZ_SUPABASE_URL"), "/"),
		SupabaseAnonKey:    os.Getenv("MATHIZ_SUPABASE_ANON_KEY"),
		SupabaseJWTSecret:  os.Getenv("MATHIZ_SUPABASE_JWT_SECRET"),
		MaxSessions:        envIntOr("MATHIZ_MAX_SESSIONS", 100),
		SessionIdleTimeout: time.Duration(envIntOr("MATHIZ_SESSION_IDLE_MINUTES", 30)) * time.Minute,
		TrustProxy:         os.Getenv("MATHIZ_TRUST_PROXY") == "true",
		BillingProvider:    os.Getenv("MATHIZ_BILLING_PROVIDER"),
		PublicBaseURL:      os.Getenv("MATHIZ_PUBLIC_BASE_URL"),
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
	if c.SupabaseURL == "" && c.SupabaseJWTSecret == "" {
		return fmt.Errorf("configure MATHIZ_SUPABASE_URL and/or MATHIZ_SUPABASE_JWT_SECRET for parent auth")
	}
	return nil
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
