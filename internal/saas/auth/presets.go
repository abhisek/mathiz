package auth

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Provider names: the value of MATHIZ_AUTH_PROVIDER and of
// Account.auth_provider.
const (
	ProviderSupabase = "supabase"
	ProviderTest     = "test"
)

// TestIssuer is the iss of tokens the test preset accepts.
const TestIssuer = "mathiz-test"

// defaultAudience is what both Supabase and the test preset put in aud.
const defaultAudience = "authenticated"

// Config selects and configures one provider's preset.
type Config struct {
	Provider    string // ProviderSupabase | ProviderTest
	URL         string // provider base URL (Supabase: project URL)
	HS256Secret string // test: required; supabase: optional (self-hosted HS256 projects)
	Audience    string // overrides the preset's default audience
	AllowTest   bool   // the test preset refuses to load without it
}

// NewVerifier builds the verifier for cfg.Provider.
func NewVerifier(cfg Config) (*JWTVerifier, error) {
	var (
		p   Preset
		err error
	)
	switch cfg.Provider {
	case ProviderSupabase:
		p, err = SupabasePreset(cfg)
	case ProviderTest:
		p, err = TestPreset(cfg)
	default:
		return nil, fmt.Errorf("%w: unknown provider %q (available: %s, %s)",
			ErrMisconfigured, cfg.Provider, ProviderSupabase, ProviderTest)
	}
	if err != nil {
		return nil, err
	}
	return NewJWTVerifier(p)
}

// SupabasePreset verifies Supabase Auth access tokens. The project URL is
// required: it pins the issuer, so a token from another Supabase project
// signed with a shared secret can never pass.
//
// Supabase sessions count as email-verified: the deployment runbook
// requires "Confirm email" on, so Supabase issues no session for an
// unconfirmed address. A token that explicitly says
// user_metadata.email_verified=false is still refused, which covers a
// self-hosted project where that setting is off.
func SupabasePreset(cfg Config) (Preset, error) {
	base, err := providerURL(cfg.URL)
	if err != nil {
		return Preset{}, err
	}
	aud := cfg.Audience
	if aud == "" {
		aud = defaultAudience
	}
	return Preset{
		Name:        ProviderSupabase,
		Issuer:      base + "/auth/v1",
		JWKSURL:     base + "/auth/v1/.well-known/jwks.json",
		HS256Secret: cfg.HS256Secret,
		Audience:    aud,
		Profile: func(c jwt.MapClaims) Profile {
			meta, _ := c["user_metadata"].(map[string]any)
			email, _ := c["email"].(string)
			// A session without an email (a phone-only user) has no
			// address to vouch for.
			verified := email != ""
			if v, ok := meta["email_verified"].(bool); ok && !v {
				verified = false
			}
			return Profile{Email: email, EmailVerified: verified, DisplayName: metadataName(meta)}
		},
	}, nil
}

// TestPreset accepts HS256 tokens minted with a shared secret, for unit
// tests, the dev compose stack and browser E2E. Anyone with the secret can
// sign in as anyone, so it needs an explicit opt-in.
func TestPreset(cfg Config) (Preset, error) {
	if !cfg.AllowTest {
		return Preset{}, fmt.Errorf("%w: the test provider needs AllowTest", ErrMisconfigured)
	}
	if cfg.HS256Secret == "" {
		return Preset{}, fmt.Errorf("%w: the test provider needs an HS256 secret", ErrMisconfigured)
	}
	aud := cfg.Audience
	if aud == "" {
		aud = defaultAudience
	}
	return Preset{
		Name:        ProviderTest,
		Issuer:      TestIssuer,
		HS256Secret: cfg.HS256Secret,
		Audience:    aud,
		Profile: func(c jwt.MapClaims) Profile {
			meta, _ := c["user_metadata"].(map[string]any)
			email, _ := c["email"].(string)
			verified := email != ""
			if v, ok := c["email_verified"].(bool); ok && !v {
				verified = false
			}
			return Profile{Email: email, EmailVerified: verified, DisplayName: metadataName(meta)}
		},
	}, nil
}

// providerURL validates a provider base URL: an https origin, or plain
// http on a loopback host for local development. The JWKS is fetched from
// it, so plain http across a network would let anyone on the path serve
// their own signing keys.
func providerURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", fmt.Errorf("%w: the provider URL is required", ErrMisconfigured)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%w: provider URL %q must be a bare origin (scheme://host[:port])", ErrMisconfigured, raw)
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && isLoopback(u.Hostname()):
	default:
		return "", fmt.Errorf("%w: provider URL %q must use https (http only for localhost)", ErrMisconfigured, raw)
	}
	// Issuer comparison is exact; normalise the case-insensitive parts.
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func metadataName(meta map[string]any) string {
	for _, key := range []string{"full_name", "name", "display_name"} {
		if name, ok := meta[key].(string); ok && name != "" {
			return name
		}
	}
	return ""
}
