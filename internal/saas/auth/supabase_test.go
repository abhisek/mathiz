package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "super-secret-jwt-token-with-at-least-32-characters"

func hs256Token(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   "user-123",
		"aud":   "authenticated",
		"email": "parent@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"user_metadata": map[string]any{
			"full_name": "Pat Parent",
		},
	}
	if mutate != nil {
		mutate(claims)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func newHSVerifier(t *testing.T) *SupabaseVerifier {
	t.Helper()
	v, err := NewSupabaseVerifier(SupabaseConfig{JWTSecret: testSecret})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return v
}

func TestVerifyHS256(t *testing.T) {
	v := newHSVerifier(t)
	id, err := v.Verify(context.Background(), hs256Token(t, nil))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if id.SupabaseUserID != "user-123" || id.Email != "parent@example.com" || id.DisplayName != "Pat Parent" {
		t.Errorf("identity = %+v", id)
	}
}

func TestVerifyRejections(t *testing.T) {
	v := newHSVerifier(t)
	ctx := context.Background()

	cases := map[string]string{
		"expired": hs256Token(t, func(c jwt.MapClaims) {
			c["exp"] = time.Now().Add(-time.Hour).Unix()
		}),
		"wrong audience": hs256Token(t, func(c jwt.MapClaims) {
			c["aud"] = "anon"
		}),
		"missing sub": hs256Token(t, func(c jwt.MapClaims) {
			delete(c, "sub")
		}),
		"no exp": hs256Token(t, func(c jwt.MapClaims) {
			delete(c, "exp")
		}),
		"garbage": "not-a-jwt",
		"empty":   "",
	}
	for name, tok := range cases {
		if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%s: got %v, want ErrUnauthenticated", name, err)
		}
	}

	// Wrong secret.
	other := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-123", "aud": "authenticated", "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, _ := other.SignedString([]byte("some-other-secret-that-is-long-enough!"))
	if _, err := v.Verify(ctx, s); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("wrong secret: got %v", err)
	}

	// alg=none is always rejected.
	noneTok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "user-123", "aud": "authenticated", "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, _ = noneTok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := v.Verify(ctx, s); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("alg none: got %v", err)
	}
}

func TestVerifyHS256DisallowedWithoutSecret(t *testing.T) {
	// A verifier configured for JWKS-only must refuse HS256 tokens even if
	// signed with a "secret" — prevents alg-confusion downgrades.
	srv := newJWKSServer(t, nil)
	defer srv.Close()
	v, err := NewSupabaseVerifier(SupabaseConfig{ProjectURL: srv.URL})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	tok := hs256Token(t, func(c jwt.MapClaims) {
		c["iss"] = srv.URL + "/auth/v1"
	})
	if _, err := v.Verify(context.Background(), tok); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("HS256 without configured secret: got %v", err)
	}
}

// newJWKSServer serves a JWKS containing the public half of key (ES256).
// Passing nil generates a throwaway key that signs nothing.
func newJWKSServer(t *testing.T, key *ecdsa.PrivateKey) *httptest.Server {
	t.Helper()
	if key == nil {
		var err error
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
	}
	pub := key.PublicKey
	jwks := map[string]any{
		"keys": []map[string]any{{
			"kty": "EC",
			"crv": "P-256",
			"kid": "test-key",
			"alg": "ES256",
			"use": "sig",
			"x":   base64.RawURLEncoding.EncodeToString(padCoord(pub.X)),
			"y":   base64.RawURLEncoding.EncodeToString(padCoord(pub.Y)),
		}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/v1/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	})
	return httptest.NewServer(mux)
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

func TestVerifyES256ViaJWKS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	srv := newJWKSServer(t, key)
	defer srv.Close()

	v, err := NewSupabaseVerifier(SupabaseConfig{ProjectURL: srv.URL})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}

	makeToken := func(mutate func(jwt.MapClaims)) string {
		claims := jwt.MapClaims{
			"sub":   "user-es",
			"aud":   "authenticated",
			"iss":   srv.URL + "/auth/v1",
			"email": "es@example.com",
			"exp":   time.Now().Add(time.Hour).Unix(),
		}
		if mutate != nil {
			mutate(claims)
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
		tok.Header["kid"] = "test-key"
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return s
	}

	id, err := v.Verify(context.Background(), makeToken(nil))
	if err != nil {
		t.Fatalf("verify ES256: %v", err)
	}
	if id.SupabaseUserID != "user-es" {
		t.Errorf("identity = %+v", id)
	}

	// Wrong issuer is rejected when the project URL is configured.
	badIss := makeToken(func(c jwt.MapClaims) { c["iss"] = "https://evil.example.com/auth/v1" })
	if _, err := v.Verify(context.Background(), badIss); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("wrong issuer: got %v", err)
	}

	// Token signed by a different key is rejected.
	otherKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"sub": "user-es", "aud": "authenticated",
		"iss": srv.URL + "/auth/v1", "exp": time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "test-key"
	s, _ := tok.SignedString(otherKey)
	if _, err := v.Verify(context.Background(), s); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("wrong key: got %v", err)
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

func TestNewSupabaseVerifierRequiresConfig(t *testing.T) {
	if _, err := NewSupabaseVerifier(SupabaseConfig{}); !errors.Is(err, ErrMisconfigured) {
		t.Errorf("empty config: got %v", err)
	}
}
