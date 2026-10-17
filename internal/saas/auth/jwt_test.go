package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "super-secret-jwt-token-with-at-least-32-characters"

func baseClaims(iss string) jwt.MapClaims {
	return jwt.MapClaims{
		"sub":   "user-123",
		"aud":   "authenticated",
		"iss":   iss,
		"email": "parent@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"user_metadata": map[string]any{
			"full_name": "Pat Parent",
		},
	}
}

func signHS256(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func testToken(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()
	c := baseClaims(TestIssuer)
	if mutate != nil {
		mutate(c)
	}
	return signHS256(t, testSecret, c)
}

func newTestVerifier(t *testing.T) *JWTVerifier {
	t.Helper()
	v, err := NewVerifier(Config{Provider: ProviderTest, HS256Secret: testSecret, AllowTest: true})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return v
}

func TestTestPresetIdentity(t *testing.T) {
	v := newTestVerifier(t)
	id, err := v.Verify(context.Background(), testToken(t, nil))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	want := Identity{Provider: ProviderTest, Subject: "user-123", Email: "parent@example.com", EmailVerified: true, DisplayName: "Pat Parent"}
	if *id != want {
		t.Errorf("identity = %+v, want %+v", *id, want)
	}
	if v.Provider() != ProviderTest {
		t.Errorf("Provider() = %q", v.Provider())
	}

	id, err = v.Verify(context.Background(), testToken(t, func(c jwt.MapClaims) { c["email_verified"] = false }))
	if err != nil {
		t.Fatalf("verify unverified: %v", err)
	}
	if id.EmailVerified {
		t.Error("email_verified=false must map to EmailVerified=false")
	}
}

func TestVerifyRejections(t *testing.T) {
	v := newTestVerifier(t)
	ctx := context.Background()

	noneTok := func() string {
		c := baseClaims(TestIssuer)
		s, _ := jwt.NewWithClaims(jwt.SigningMethodNone, c).SignedString(jwt.UnsafeAllowNoneSignatureType)
		return s
	}

	cases := map[string]string{
		"expired":        testToken(t, func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }),
		"no exp":         testToken(t, func(c jwt.MapClaims) { delete(c, "exp") }),
		"wrong audience": testToken(t, func(c jwt.MapClaims) { c["aud"] = "anon" }),
		"no audience":    testToken(t, func(c jwt.MapClaims) { delete(c, "aud") }),
		"wrong issuer":   testToken(t, func(c jwt.MapClaims) { c["iss"] = "https://evil.example.com" }),
		"no issuer":      testToken(t, func(c jwt.MapClaims) { delete(c, "iss") }),
		"missing sub":    testToken(t, func(c jwt.MapClaims) { delete(c, "sub") }),
		"wrong secret":   signHS256(t, "some-other-secret-that-is-long-enough!", baseClaims(TestIssuer)),
		"alg none":       noneTok(),
		"garbage":        "not.a.jwt",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
				t.Errorf("got %v, want ErrUnauthenticated", err)
			}
		})
	}
}

// jwksServer serves a mutable JWKS at the Supabase path and counts fetches.
type jwksServer struct {
	*httptest.Server
	mu      sync.Mutex
	keys    []map[string]any
	fail    bool
	fetches int
}

func newJWKSServer(t *testing.T) *jwksServer {
	t.Helper()
	js := &jwksServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/v1/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		js.mu.Lock()
		defer js.mu.Unlock()
		js.fetches++
		if js.fail {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": js.keys})
	})
	js.Server = httptest.NewServer(mux)
	t.Cleanup(js.Close)
	return js
}

func (js *jwksServer) set(keys ...map[string]any) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.keys = keys
}

func ecJWK(kid string, pub *ecdsa.PublicKey) map[string]any {
	return map[string]any{
		"kty": "EC", "crv": "P-256", "kid": kid, "alg": "ES256", "use": "sig",
		"x": base64.RawURLEncoding.EncodeToString(padCoord(pub.X)),
		"y": base64.RawURLEncoding.EncodeToString(padCoord(pub.Y)),
	}
}

func rsaJWK(kid string, pub *rsa.PublicKey) map[string]any {
	return map[string]any{
		"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

func padCoord(v *big.Int) []byte {
	b := v.Bytes()
	if len(b) >= 32 {
		return b
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func signWith(t *testing.T, method jwt.SigningMethod, kid string, key crypto.Signer, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func newSupabaseVerifier(t *testing.T, url, secret string) *JWTVerifier {
	t.Helper()
	v, err := NewVerifier(Config{Provider: ProviderSupabase, URL: url, HS256Secret: secret})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return v
}

func TestSupabasePresetJWKS(t *testing.T) {
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	js := newJWKSServer(t)
	js.set(ecJWK("ec", &ecKey.PublicKey), rsaJWK("rsa", &rsaKey.PublicKey))
	v := newSupabaseVerifier(t, js.URL+"/", "")
	iss := js.URL + "/auth/v1"
	ctx := context.Background()

	for name, tok := range map[string]string{
		"ES256": signWith(t, jwt.SigningMethodES256, "ec", ecKey, baseClaims(iss)),
		"RS256": signWith(t, jwt.SigningMethodRS256, "rsa", rsaKey, baseClaims(iss)),
	} {
		id, err := v.Verify(ctx, tok)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if id.Provider != ProviderSupabase || id.Subject != "user-123" || !id.EmailVerified || id.DisplayName != "Pat Parent" {
			t.Errorf("%s: identity = %+v", name, id)
		}
	}

	otherKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	unverified := baseClaims(iss)
	unverified["user_metadata"] = map[string]any{"email_verified": false}
	id, err := v.Verify(ctx, signWith(t, jwt.SigningMethodES256, "ec", ecKey, unverified))
	if err != nil {
		t.Fatalf("unverified: %v", err)
	}
	if id.EmailVerified {
		t.Error("user_metadata.email_verified=false must map to EmailVerified=false")
	}

	rejected := map[string]string{
		"wrong issuer": signWith(t, jwt.SigningMethodES256, "ec", ecKey, baseClaims("https://evil.example.com/auth/v1")),
		"wrong key":    signWith(t, jwt.SigningMethodES256, "ec", otherKey, baseClaims(iss)),
		// No secret configured: an HS256 token is an alg-confusion attempt.
		"HS256 without secret": signHS256(t, testSecret, baseClaims(iss)),
	}
	for name, tok := range rejected {
		if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%s: got %v, want ErrUnauthenticated", name, err)
		}
	}
}

func TestSupabasePresetHS256(t *testing.T) {
	js := newJWKSServer(t)
	v := newSupabaseVerifier(t, js.URL, testSecret)
	iss := js.URL + "/auth/v1"
	if _, err := v.Verify(context.Background(), signHS256(t, testSecret, baseClaims(iss))); err != nil {
		t.Fatalf("HS256 with secret: %v", err)
	}
	// The issuer is still pinned for HS256 projects.
	if _, err := v.Verify(context.Background(), signHS256(t, testSecret, baseClaims("https://other.supabase.co/auth/v1"))); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("HS256 from another project: got %v", err)
	}
}

func TestJWKSUnknownKidRefetch(t *testing.T) {
	oldKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	newKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	js := newJWKSServer(t)
	js.set(ecJWK("old", &oldKey.PublicKey))
	v := newSupabaseVerifier(t, js.URL, "")
	iss := js.URL + "/auth/v1"

	if _, err := v.Verify(context.Background(), signWith(t, jwt.SigningMethodES256, "old", oldKey, baseClaims(iss))); err != nil {
		t.Fatalf("old key: %v", err)
	}
	// The provider rotates its key: a token with a kid we have not seen
	// triggers a refetch instead of failing until restart.
	js.set(ecJWK("old", &oldKey.PublicKey), ecJWK("new", &newKey.PublicKey))
	if _, err := v.Verify(context.Background(), signWith(t, jwt.SigningMethodES256, "new", newKey, baseClaims(iss))); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
}

func TestJWKSOutageRetries(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	js := newJWKSServer(t)
	js.set(ecJWK("k", &key.PublicKey))
	js.fail = true
	v := newSupabaseVerifier(t, js.URL, "")
	tok := signWith(t, jwt.SigningMethodES256, "k", key, baseClaims(js.URL+"/auth/v1"))

	if err := v.Warm(); err == nil {
		t.Fatal("Warm during an outage should report the error")
	}
	// An outage is not a bad token: a client that signs out on 401 would
	// sign out every parent.
	if _, err := v.Verify(context.Background(), tok); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("during outage: got %v, want ErrUnavailable", err)
	}
	js.mu.Lock()
	fetched := js.fetches
	js.fail = false
	js.mu.Unlock()
	if fetched != 1 {
		t.Errorf("fetches during the retry window = %d, want 1 (rate-limited)", fetched)
	}

	v.jwksRetry = 0 // skip the rate limit instead of sleeping 30s
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("after recovery: %v", err)
	}

	v.Close()
	v.Close() // idempotent
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("after Close: %v", err)
	}
	v.Close()
}

func TestSupabaseClaimEdgeCases(t *testing.T) {
	ecKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	js := newJWKSServer(t)
	js.set(ecJWK("ec", &ecKey.PublicKey), rsaJWK("rsa", &rsaKey.PublicKey))
	v := newSupabaseVerifier(t, js.URL, "")
	t.Cleanup(v.Close)
	iss := js.URL + "/auth/v1"
	ctx := context.Background()
	es := func(mutate func(jwt.MapClaims)) string {
		c := baseClaims(iss)
		mutate(c)
		return signWith(t, jwt.SigningMethodES256, "ec", ecKey, c)
	}

	accepted := map[string]string{
		"aud array":        es(func(c jwt.MapClaims) { c["aud"] = []string{"other", "authenticated"} }),
		"no user_metadata": es(func(c jwt.MapClaims) { delete(c, "user_metadata") }),
	}
	for name, tok := range accepted {
		id, err := v.Verify(ctx, tok)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !id.EmailVerified {
			t.Errorf("%s: EmailVerified = false", name)
		}
	}

	id, err := v.Verify(ctx, es(func(c jwt.MapClaims) { delete(c, "email") }))
	if err != nil {
		t.Fatalf("no email: %v", err)
	}
	if id.EmailVerified {
		t.Error("a session without an email must not count as email-verified")
	}

	kidlessTok, _ := jwt.NewWithClaims(jwt.SigningMethodES256, baseClaims(iss)).SignedString(ecKey)
	rejected := map[string]string{
		"nbf in the future":    es(func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }),
		"iat in the future":    es(func(c jwt.MapClaims) { c["iat"] = time.Now().Add(time.Hour).Unix() }),
		"no kid":               kidlessTok,
		"alg differs from JWK": signWith(t, jwt.SigningMethodRS256, "ec", rsaKey, baseClaims(iss)),
		"unknown kid":          signWith(t, jwt.SigningMethodES256, "nope", ecKey, baseClaims(iss)),
	}
	for name, tok := range rejected {
		if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%s: got %v, want ErrUnauthenticated", name, err)
		}
	}
}

func TestTestPresetRefusesAsymmetricTokens(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	v := newTestVerifier(t)
	tok := signWith(t, jwt.SigningMethodES256, "k", key, baseClaims(TestIssuer))
	if _, err := v.Verify(context.Background(), tok); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("ES256 token on the HS256-only preset: got %v", err)
	}
}

func TestProviderURL(t *testing.T) {
	ok := map[string]string{
		"https://abc.supabase.co":       "https://abc.supabase.co",
		"https://abc.supabase.co/":      "https://abc.supabase.co",
		"HTTPS://ABC.Supabase.CO":       "https://abc.supabase.co",
		"https://auth.example.com:8443": "https://auth.example.com:8443",
		"http://localhost:54321":        "http://localhost:54321",
		"http://127.0.0.1:54321":        "http://127.0.0.1:54321",
	}
	for in, want := range ok {
		got, err := providerURL(in)
		if err != nil || got != want {
			t.Errorf("providerURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{
		"", "abc.supabase.co", "http://abc.supabase.co", "ftp://abc.supabase.co",
		"https://user:pw@abc.supabase.co", "https://abc.supabase.co/auth/v1",
		"https://abc.supabase.co?x=1", "https://abc.supabase.co#frag",
	} {
		if _, err := providerURL(bad); !errors.Is(err, ErrMisconfigured) {
			t.Errorf("providerURL(%q) accepted", bad)
		}
	}
}

func TestAuthorizedParties(t *testing.T) {
	v, err := NewJWTVerifier(Preset{
		Name:              "custom",
		Issuer:            "https://issuer.example.com",
		HS256Secret:       testSecret,
		AuthorizedParties: []string{"https://mathiz.example.com"},
		Profile:           func(jwt.MapClaims) Profile { return Profile{EmailVerified: true} },
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	claims := func(azp any) jwt.MapClaims {
		c := baseClaims("https://issuer.example.com")
		delete(c, "aud")
		if azp != nil {
			c["azp"] = azp
		}
		return c
	}
	if _, err := v.Verify(context.Background(), signHS256(t, testSecret, claims("https://mathiz.example.com"))); err != nil {
		t.Fatalf("allowed azp: %v", err)
	}
	for name, azp := range map[string]any{"other app": "https://evil.example.com", "missing": nil} {
		if _, err := v.Verify(context.Background(), signHS256(t, testSecret, claims(azp))); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%s azp: got %v", name, err)
		}
	}
}

func TestNewJWTVerifierValidation(t *testing.T) {
	profile := func(jwt.MapClaims) Profile { return Profile{} }
	valid := Preset{Name: "p", Issuer: "iss", HS256Secret: testSecret, Audience: "a", Profile: profile}
	cases := map[string]func(p *Preset){
		"no name":            func(p *Preset) { p.Name = "" },
		"no issuer":          func(p *Preset) { p.Issuer = "" },
		"no keys":            func(p *Preset) { p.HS256Secret = "" },
		"no audience or azp": func(p *Preset) { p.Audience = "" },
		"no claim mapping":   func(p *Preset) { p.Profile = nil },
		"short HS256 secret": func(p *Preset) { p.HS256Secret = "too-short" },
	}
	if _, err := NewJWTVerifier(valid); err != nil {
		t.Fatalf("valid preset: %v", err)
	}
	for name, mutate := range cases {
		p := valid
		mutate(&p)
		if _, err := NewJWTVerifier(p); !errors.Is(err, ErrMisconfigured) {
			t.Errorf("%s: got %v, want ErrMisconfigured", name, err)
		}
	}
}

func TestNewVerifierConfig(t *testing.T) {
	cases := map[string]Config{
		"unknown provider":      {Provider: "clerk"},
		"empty provider":        {},
		"supabase without URL":  {Provider: ProviderSupabase},
		"supabase bad URL":      {Provider: ProviderSupabase, URL: "not a url"},
		"supabase non-http URL": {Provider: ProviderSupabase, URL: "ftp://x.supabase.co"},
		"test without opt-in":   {Provider: ProviderTest, HS256Secret: testSecret},
		"test without secret":   {Provider: ProviderTest, AllowTest: true},
	}
	for name, cfg := range cases {
		if _, err := NewVerifier(cfg); !errors.Is(err, ErrMisconfigured) {
			t.Errorf("%s: got %v, want ErrMisconfigured", name, err)
		}
	}
}

func TestBearerToken(t *testing.T) {
	if tok, ok := BearerToken("Bearer abc123"); !ok || tok != "abc123" {
		t.Errorf("basic: %q %v", tok, ok)
	}
	if tok, ok := BearerToken("bearer abc123"); !ok || tok != "abc123" {
		t.Errorf("case-insensitive: %q %v", tok, ok)
	}
	if _, ok := BearerToken("Basic abc123"); ok {
		t.Error("basic scheme accepted")
	}
	if _, ok := BearerToken(""); ok {
		t.Error("empty accepted")
	}
}
