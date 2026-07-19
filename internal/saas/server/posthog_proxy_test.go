package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestPostHogRelayForwards asserts the same-origin relay's contract: the
// /relay prefix is stripped, the Host header is rewritten to the upstream
// (PostHog cloud requires it), and query + body pass through untouched —
// including the /relay/static/* path posthog-js uses for its remote config.
func TestPostHogRelayForwards(t *testing.T) {
	type seen struct {
		path, rawQuery, host, cookie, body string
	}
	var got seen
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = seen{
			path:     r.URL.Path,
			rawQuery: r.URL.RawQuery,
			host:     r.Host,
			cookie:   r.Header.Get("Cookie"),
			body:     string(b),
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":1}`))
	}))
	t.Cleanup(upstream.Close)
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	e := newTestEnvWith(t, func(cfg *Config) {
		cfg.PostHogAPIKey = "phc_test"
		cfg.PostHogHost = upstream.URL
	})

	cases := []struct {
		name, method, path, body string
		wantPath, wantQuery      string
	}{
		{
			name: "capture batch", method: "POST",
			path: "/relay/batch/?compression=gzip-js", body: `{"api_key":"phc_test"}`,
			wantPath: "/batch/", wantQuery: "compression=gzip-js",
		},
		{
			name: "remote config bundle", method: "GET",
			path:     "/relay/static/array.js",
			wantPath: "/static/array.js",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			req, err := http.NewRequest(tc.method, e.ts.URL+tc.path, body)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			// Our origin's cookies must not leak upstream.
			req.Header.Set("Cookie", "mathiz=secret")
			resp, err := e.ts.Client().Do(req)
			if err != nil {
				t.Fatalf("relay request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if got.path != tc.wantPath {
				t.Errorf("upstream path = %q, want %q (prefix not stripped?)", got.path, tc.wantPath)
			}
			if got.rawQuery != tc.wantQuery {
				t.Errorf("upstream query = %q, want %q", got.rawQuery, tc.wantQuery)
			}
			if got.host != upstreamHost {
				t.Errorf("upstream Host = %q, want %q (Host header not rewritten)", got.host, upstreamHost)
			}
			if got.body != tc.body {
				t.Errorf("upstream body = %q, want %q", got.body, tc.body)
			}
			if got.cookie != "" {
				t.Errorf("upstream saw Cookie %q, want none", got.cookie)
			}
		})
	}
}

// TestPostHogRelayOffWithoutKey: no analytics key → the relay route is not
// mounted at all.
func TestPostHogRelayOffWithoutKey(t *testing.T) {
	e := newTestEnv(t)
	resp, err := e.ts.Client().Get(e.ts.URL + "/relay/batch/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when analytics is unconfigured", resp.StatusCode)
	}
}

// Guard against accidentally re-parsing the relay prefix into something the
// mux can't mount.
func TestRelayPrefixIsValidURLPath(t *testing.T) {
	if _, err := url.Parse(relayPrefix); err != nil || !strings.HasPrefix(relayPrefix, "/") {
		t.Fatalf("relayPrefix %q must be an absolute path", relayPrefix)
	}
}

// TestPostHogRelayRejectsBadUpstream: url.Parse accepts scheme-less hosts and
// bare paths without error — the relay must fail fast on anything that is not
// an absolute http(s) URL.
func TestPostHogRelayRejectsBadUpstream(t *testing.T) {
	for _, bad := range []string{"us.i.posthog.com", "/some/path", "ftp://x.example", ""} {
		if _, err := newPostHogRelay(bad); err == nil {
			t.Errorf("newPostHogRelay(%q): want error, got nil", bad)
		}
	}
	if _, err := newPostHogRelay("https://us.i.posthog.com"); err != nil {
		t.Errorf("valid upstream rejected: %v", err)
	}
}

// TestPostHogBadHostForcesAnalyticsOff: when the relay can't be built the
// server must stop advertising analytics — a config that hands the SPA
// posthogKey + "/relay" while the route 404s would strand every event.
func TestPostHogBadHostForcesAnalyticsOff(t *testing.T) {
	e := newTestEnvWith(t, func(cfg *Config) {
		cfg.PostHogAPIKey = "phc_test"
		cfg.PostHogHost = "us.i.posthog.com" // scheme-less: relay init fails
	})

	var cfg map[string]any
	e.call(t, http.MethodGet, "/api/v1/config", "", nil, &cfg)
	if _, ok := cfg["posthogKey"]; ok {
		t.Errorf("config still advertises posthogKey with a broken relay: %v", cfg)
	}
	if _, ok := cfg["posthogHost"]; ok {
		t.Errorf("config still advertises posthogHost with a broken relay: %v", cfg)
	}

	resp, err := e.ts.Client().Post(e.ts.URL+"/relay/e/", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("relay probe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/relay with broken upstream: want 404, got %d", resp.StatusCode)
	}
}
