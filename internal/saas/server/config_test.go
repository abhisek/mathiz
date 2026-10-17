package server

import (
	"strings"
	"testing"
)

func TestConfigValidateAuth(t *testing.T) {
	base := func(mutate func(*Config)) *Config {
		c := &Config{DatabaseURL: "postgres://x", LogFormat: "text", LogLevel: "info"}
		mutate(c)
		return c
	}
	supabase := func(c *Config) {
		c.AuthProvider = "supabase"
		c.AuthURL = "https://p.supabase.co"
		c.AuthPublicKey = "anon"
	}
	testPreset := func(c *Config) {
		c.AuthProvider = "test"
		c.AuthHS256Secret = "secret"
		c.AuthAllowTest = true
	}

	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{"supabase ok", base(supabase), ""},
		{"test ok", base(testPreset), ""},
		{"test ok on http origin", base(func(c *Config) { testPreset(c); c.PublicBaseURL = "http://localhost:8080" }), ""},
		{"no provider", base(func(*Config) {}), "MATHIZ_AUTH_PROVIDER is required"},
		{"unknown provider", base(func(c *Config) { c.AuthProvider = "clerk" }), "unsupported MATHIZ_AUTH_PROVIDER"},
		{"supabase without URL", base(func(c *Config) { supabase(c); c.AuthURL = "" }), "MATHIZ_AUTH_URL is required"},
		{"supabase without key", base(func(c *Config) { supabase(c); c.AuthPublicKey = "" }), "MATHIZ_AUTH_PUBLIC_KEY is required"},
		{"test without opt-in", base(func(c *Config) { testPreset(c); c.AuthAllowTest = false }), "MATHIZ_AUTH_ALLOW_TEST=1"},
		{"test without secret", base(func(c *Config) { testPreset(c); c.AuthHS256Secret = "" }), "MATHIZ_AUTH_HS256_SECRET"},
		{"test on public https", base(func(c *Config) { testPreset(c); c.PublicBaseURL = "https://mathiz.example.com" }), "refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigFromEnvRejectsRemovedSupabaseVars(t *testing.T) {
	t.Setenv("MATHIZ_DATABASE_URL", "postgres://x")
	t.Setenv("MATHIZ_AUTH_PROVIDER", "supabase")
	t.Setenv("MATHIZ_AUTH_URL", "https://p.supabase.co")
	t.Setenv("MATHIZ_AUTH_PUBLIC_KEY", "anon")
	if _, err := ConfigFromEnv(); err != nil {
		t.Fatalf("clean env: %v", err)
	}
	for _, old := range []string{"MATHIZ_SUPABASE_URL", "MATHIZ_SUPABASE_ANON_KEY", "MATHIZ_SUPABASE_JWT_SECRET"} {
		t.Run(old, func(t *testing.T) {
			t.Setenv(old, "x")
			_, err := ConfigFromEnv()
			if err == nil || !strings.Contains(err.Error(), old+" was removed") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestConfigFromEnvAllowTestNeedsExactOne(t *testing.T) {
	t.Setenv("MATHIZ_DATABASE_URL", "postgres://x")
	t.Setenv("MATHIZ_AUTH_PROVIDER", "test")
	t.Setenv("MATHIZ_AUTH_HS256_SECRET", "secret")
	for _, v := range []string{"", "true", "yes", "0"} {
		t.Setenv("MATHIZ_AUTH_ALLOW_TEST", v)
		if _, err := ConfigFromEnv(); err == nil {
			t.Errorf("MATHIZ_AUTH_ALLOW_TEST=%q enabled the test provider", v)
		}
	}
	t.Setenv("MATHIZ_AUTH_ALLOW_TEST", "1")
	if _, err := ConfigFromEnv(); err != nil {
		t.Errorf("MATHIZ_AUTH_ALLOW_TEST=1: %v", err)
	}
}
