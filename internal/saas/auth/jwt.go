// Package auth verifies caller credentials for the SaaS layer.
//
// Parents present a JWT from the deployment's hosted auth provider. One
// generic verifier checks every provider's tokens; a Preset holds the only
// vendor knowledge (issuer, keys, audience, claim names). Children present
// opaque device tokens, verified by internal/saas/family.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrUnauthenticated = errors.New("invalid or missing credentials")
	ErrMisconfigured   = errors.New("auth provider misconfigured")
	// ErrUnavailable means the provider's keys could not be fetched. It is
	// not the caller's fault, so it must not read as a bad token: a client
	// that signs out on 401 would sign out every parent during a blip.
	ErrUnavailable = errors.New("auth provider unavailable")
)

// Identity is a verified parent identity. Subject is opaque and unique only
// within Provider: accounts are keyed on the pair, never on Email.
type Identity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	DisplayName   string
}

// Verifier checks a parent's bearer token. Every failure is
// ErrUnauthenticated, so callers can't leak verification details, except
// ErrUnavailable when the provider's keys cannot be fetched.
type Verifier interface {
	Verify(ctx context.Context, token string) (*Identity, error)
	Provider() string
}

// Profile is what a preset reads from verified claims.
type Profile struct {
	Email         string
	EmailVerified bool
	DisplayName   string
}

// Preset is everything one provider needs. Adding a provider whose tokens
// are signed JWTs with a JWKS means one Preset constructor plus any Config
// fields it reads (Clerk, for example, needs an issuer and authorized
// parties); the verifier itself does not change.
type Preset struct {
	Name   string
	Issuer string // exact iss match; always required

	// Keys: a JWKS for asymmetric algorithms, and/or a shared secret for
	// HS256. A token may only use an algorithm whose key is configured.
	JWKSURL     string
	HS256Secret string

	// At least one is required, so a token minted for another app at the
	// same issuer is rejected.
	Audience          string
	AuthorizedParties []string

	// Profile maps verified claims to the identity fields.
	Profile func(jwt.MapClaims) Profile
}

// leeway tolerates clock skew between us and the provider.
const leeway = 30 * time.Second

var asymmetricAlgs = []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}

// JWTVerifier is the only Verifier implementation.
type JWTVerifier struct {
	p    Preset
	algs []string

	jwksRetry   time.Duration
	jwksMu      sync.Mutex
	jwks        keyfunc.Keyfunc
	jwksStop    context.CancelFunc // ends keyfunc's refresh goroutine
	jwksLastTry time.Time
	jwksLastErr error
}

var _ Verifier = (*JWTVerifier)(nil)

// jwksRetryInterval bounds how often a failed JWKS fetch is retried, so a
// burst of logins during an outage doesn't hammer the endpoint.
const jwksRetryInterval = 30 * time.Second

// jwksHTTPTimeout and jwksRateLimitWait bound how long a login waits on the
// provider. keyfunc's defaults are a minute each, and the fetch runs under
// jwksMu, so every parent request would queue behind a hung provider.
const (
	jwksHTTPTimeout   = 5 * time.Second
	jwksRateLimitWait = 2 * time.Second
)

// MinHS256SecretLen is the shortest shared secret a preset accepts
// (Supabase's own minimum). Short HMAC keys can be brute-forced offline
// from any one captured token.
const MinHS256SecretLen = 32

// NewJWTVerifier validates the preset. The JWKS is fetched lazily on the
// first asymmetric token (or by Warm), so a provider outage at boot doesn't
// stop the server.
func NewJWTVerifier(p Preset) (*JWTVerifier, error) {
	switch {
	case p.Name == "":
		return nil, fmt.Errorf("%w: preset has no name", ErrMisconfigured)
	case p.Issuer == "":
		return nil, fmt.Errorf("%w: %s preset has no issuer", ErrMisconfigured, p.Name)
	case p.JWKSURL == "" && p.HS256Secret == "":
		return nil, fmt.Errorf("%w: %s preset has no JWKS URL or HS256 secret", ErrMisconfigured, p.Name)
	case p.Audience == "" && len(p.AuthorizedParties) == 0:
		return nil, fmt.Errorf("%w: %s preset checks neither audience nor authorized party", ErrMisconfigured, p.Name)
	case p.Profile == nil:
		return nil, fmt.Errorf("%w: %s preset has no claim mapping", ErrMisconfigured, p.Name)
	case p.HS256Secret != "" && len(p.HS256Secret) < MinHS256SecretLen:
		return nil, fmt.Errorf("%w: %s preset HS256 secret is shorter than %d bytes", ErrMisconfigured, p.Name, MinHS256SecretLen)
	}
	var algs []string
	if p.JWKSURL != "" {
		algs = append(algs, asymmetricAlgs...)
	}
	if p.HS256Secret != "" {
		algs = append(algs, "HS256")
	}
	return &JWTVerifier{p: p, algs: algs, jwksRetry: jwksRetryInterval}, nil
}

func (v *JWTVerifier) Provider() string { return v.p.Name }

// Warm fetches the JWKS ahead of the first login, so a bad URL shows up in
// the boot log. A failure is not fatal: verification retries lazily.
func (v *JWTVerifier) Warm() error {
	if v.p.JWKSURL == "" {
		return nil
	}
	_, err := v.jwksKeyfunc()
	return err
}

// jwksKeyfunc returns the cached JWKS, fetching it on first use. Transient
// fetch failures are NOT cached permanently — they retry (rate-limited), so
// one network blip at boot can't disable parent auth until restart. The
// fetch uses a background context because keyfunc's refresh goroutine must
// outlive the request that triggered it (key rotation, unknown kid).
func (v *JWTVerifier) jwksKeyfunc() (keyfunc.Keyfunc, error) {
	v.jwksMu.Lock()
	defer v.jwksMu.Unlock()

	if v.jwks != nil {
		return v.jwks, nil
	}
	if v.jwksLastErr != nil && time.Since(v.jwksLastTry) < v.jwksRetry {
		return nil, v.jwksLastErr
	}

	v.jwksLastTry = time.Now()
	// keyfunc's default builds an empty key set when the first fetch fails
	// and then refetches at most every 5 minutes. Surfacing the error lets
	// the retry above recover within jwksRetryInterval instead. jwkset
	// starts its refresh goroutine before that first fetch, so a failed
	// attempt must cancel its context or each retry leaks a goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	noEmptyFirstFetch := false
	kf, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{v.p.JWKSURL}, keyfunc.Override{
		NoErrorReturnFirstHTTPReq: &noEmptyFirstFetch,
		HTTPTimeout:               jwksHTTPTimeout,
		RateLimitWaitMax:          jwksRateLimitWait,
	})
	if err != nil {
		cancel()
		v.jwksLastErr = fmt.Errorf("fetch JWKS: %w", err)
		slog.Error("auth: fetch JWKS", "provider", v.p.Name, "err", err)
		return nil, v.jwksLastErr
	}
	v.jwks, v.jwksStop, v.jwksLastErr = kf, cancel, nil
	return kf, nil
}

// Close stops the background JWKS refresh. The verifier stays usable: the
// next asymmetric token fetches the key set again.
func (v *JWTVerifier) Close() {
	v.jwksMu.Lock()
	defer v.jwksMu.Unlock()
	if v.jwksStop != nil {
		v.jwksStop()
	}
	v.jwks, v.jwksStop = nil, nil
}

// Verify checks the signature and the standard claims, then maps the
// identity through the preset.
//
// Only a key set that could not be fetched at all is ErrUnavailable. A kid
// missing from a fetched set stays a 401 even if keyfunc's refetch fails:
// anyone can put a random kid in a forged token, and turning that into a
// 503 would let them make sign-in look down.
func (v *JWTVerifier) Verify(ctx context.Context, tokenString string) (*Identity, error) {
	claims := jwt.MapClaims{}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods(v.algs),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(leeway),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(v.p.Issuer),
	}
	if v.p.Audience != "" {
		opts = append(opts, jwt.WithAudience(v.p.Audience))
	}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		alg, _ := t.Header["alg"].(string)
		switch {
		case alg == "HS256" && v.p.HS256Secret != "":
			return []byte(v.p.HS256Secret), nil
		case v.p.JWKSURL != "" && slices.Contains(asymmetricAlgs, alg):
			// Without a kid, keyfunc tries every key in the set and skips
			// the per-key alg check. Every supported provider sets kid.
			if kid, _ := t.Header["kid"].(string); kid == "" {
				return nil, errors.New("token has no kid")
			}
			kf, err := v.jwksKeyfunc()
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
			}
			return kf.KeyfuncCtx(ctx)(t)
		default:
			return nil, fmt.Errorf("disallowed signing algorithm %q", alg)
		}
	}, opts...)
	if errors.Is(err, ErrUnavailable) {
		return nil, ErrUnavailable
	}
	if err != nil || !token.Valid {
		return nil, ErrUnauthenticated
	}

	if len(v.p.AuthorizedParties) > 0 {
		azp, _ := claims["azp"].(string)
		if !slices.Contains(v.p.AuthorizedParties, azp) {
			return nil, ErrUnauthenticated
		}
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, ErrUnauthenticated
	}

	prof := v.p.Profile(claims)
	return &Identity{
		Provider:      v.p.Name,
		Subject:       sub,
		Email:         prof.Email,
		EmailVerified: prof.EmailVerified,
		DisplayName:   prof.DisplayName,
	}, nil
}

// BearerToken extracts a bearer token from an Authorization header value.
func BearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):]), true
	}
	return "", false
}
