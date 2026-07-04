// Package auth verifies caller credentials for the SaaS layer.
//
// Parents present Supabase-issued JWTs (verified locally — HS256 with the
// project JWT secret and/or RS256/ES256 via the project's JWKS endpoint).
// Children present opaque device tokens (verified by internal/saas/family).
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrUnauthenticated = errors.New("invalid or missing credentials")
	ErrMisconfigured   = errors.New("no Supabase JWT verification method configured")
)

// SupabaseConfig configures JWT verification for a Supabase project.
type SupabaseConfig struct {
	// ProjectURL is the Supabase project URL (https://xyz.supabase.co).
	// When set, the issuer claim is validated against {url}/auth/v1 and the
	// JWKS endpoint {url}/auth/v1/.well-known/jwks.json serves asymmetric keys.
	ProjectURL string

	// JWTSecret enables HS256 verification (the classic Supabase JWT secret).
	JWTSecret string

	// Audience is the expected aud claim. Defaults to "authenticated".
	Audience string
}

// Identity is the verified parent identity extracted from a Supabase JWT.
type Identity struct {
	SupabaseUserID string // sub
	Email          string
	DisplayName    string // best-effort from user_metadata
}

// allowed asymmetric algorithms; symmetric HS256 is only accepted when the
// shared secret is configured (prevents alg-confusion attacks).
var asymmetricAlgs = map[string]bool{
	"RS256": true, "RS384": true, "RS512": true,
	"ES256": true, "ES384": true, "ES512": true,
}

// SupabaseVerifier verifies Supabase access tokens locally.
type SupabaseVerifier struct {
	cfg SupabaseConfig

	jwksMu      sync.Mutex
	jwks        keyfunc.Keyfunc
	jwksLastTry time.Time
	jwksLastErr error
}

// jwksRetryInterval bounds how often a failed JWKS fetch is retried, so a
// burst of logins during an outage doesn't hammer the endpoint.
const jwksRetryInterval = 30 * time.Second

// NewSupabaseVerifier validates the configuration. The JWKS is fetched
// lazily on the first asymmetric token so HS256-only projects (whose JWKS
// endpoint may be empty) work without network access.
func NewSupabaseVerifier(cfg SupabaseConfig) (*SupabaseVerifier, error) {
	if cfg.JWTSecret == "" && cfg.ProjectURL == "" {
		return nil, ErrMisconfigured
	}
	if cfg.Audience == "" {
		cfg.Audience = "authenticated"
	}
	cfg.ProjectURL = strings.TrimRight(cfg.ProjectURL, "/")
	return &SupabaseVerifier{cfg: cfg}, nil
}

// jwksKeyfunc returns the cached JWKS, fetching it on first use. Transient
// fetch failures are NOT cached permanently — they retry (rate-limited), so
// one network blip at boot can't disable parent auth until restart. The
// fetch uses a background context because keyfunc's refresh goroutine must
// outlive the request that triggered it (key rotation support).
func (v *SupabaseVerifier) jwksKeyfunc() (keyfunc.Keyfunc, error) {
	v.jwksMu.Lock()
	defer v.jwksMu.Unlock()

	if v.jwks != nil {
		return v.jwks, nil
	}
	if v.cfg.ProjectURL == "" {
		return nil, fmt.Errorf("asymmetric token but no Supabase project URL configured")
	}
	if v.jwksLastErr != nil && time.Since(v.jwksLastTry) < jwksRetryInterval {
		return nil, v.jwksLastErr
	}

	v.jwksLastTry = time.Now()
	url := v.cfg.ProjectURL + "/auth/v1/.well-known/jwks.json"
	kf, err := keyfunc.NewDefaultCtx(context.Background(), []string{url})
	if err != nil {
		v.jwksLastErr = fmt.Errorf("fetch JWKS: %w", err)
		return nil, v.jwksLastErr
	}
	v.jwks, v.jwksLastErr = kf, nil
	return kf, nil
}

// Verify checks the token signature and standard claims, returning the
// parent identity. All failures collapse into ErrUnauthenticated so the API
// layer can't accidentally leak verification details.
func (v *SupabaseVerifier) Verify(ctx context.Context, tokenString string) (*Identity, error) {
	claims := jwt.MapClaims{}

	parserOpts := []jwt.ParserOption{
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30 * time.Second),
		jwt.WithAudience(v.cfg.Audience),
	}
	if v.cfg.ProjectURL != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(v.cfg.ProjectURL+"/auth/v1"))
	}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		alg, _ := t.Header["alg"].(string)
		switch {
		case alg == "HS256" && v.cfg.JWTSecret != "":
			return []byte(v.cfg.JWTSecret), nil
		case asymmetricAlgs[alg]:
			kf, err := v.jwksKeyfunc()
			if err != nil {
				return nil, err
			}
			return kf.Keyfunc(t)
		default:
			return nil, fmt.Errorf("disallowed signing algorithm %q", alg)
		}
	}, parserOpts...)
	if err != nil || !token.Valid {
		return nil, ErrUnauthenticated
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, ErrUnauthenticated
	}

	id := &Identity{SupabaseUserID: sub}
	id.Email, _ = claims["email"].(string)
	if meta, ok := claims["user_metadata"].(map[string]any); ok {
		for _, key := range []string{"full_name", "name", "display_name"} {
			if name, ok := meta[key].(string); ok && name != "" {
				id.DisplayName = name
				break
			}
		}
	}
	return id, nil
}

// BearerToken extracts a bearer token from an Authorization header value.
func BearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):]), true
	}
	return "", false
}
