package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// relayPrefix is the same-origin mount point for the PostHog relay. The SPA
// is handed this path as posthogHost via /api/v1/config, so the browser only
// ever talks to our origin — ad blockers keyed on analytics domains can't
// drop events, and the upstream host stays a server-side concern
// (specs/16-analytics.md).
const relayPrefix = "/relay"

// newPostHogRelay builds a reverse proxy that forwards /relay/* to the
// configured PostHog ingestion host: prefix stripped, Host header rewritten
// to the upstream (PostHog cloud requires it), query and body untouched.
// This also covers /relay/static/* — posthog-js fetches its remote-config
// bundle from api_host.
func newPostHogRelay(upstream string) (http.Handler, error) {
	target, err := url.Parse(upstream)
	if err != nil {
		return nil, err
	}
	// url.Parse happily accepts scheme-less values ("us.i.posthog.com") and
	// bare paths — those produce a proxy that fails obscurely per request.
	// Fail fast instead: the upstream must be an absolute http(s) URL.
	if (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, fmt.Errorf("upstream must be an absolute http(s) URL, got %q", upstream)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// SetURL routes to the upstream and rewrites the outbound Host
			// header to the target host.
			pr.SetURL(target)
			pr.Out.URL.Path = strings.TrimPrefix(pr.In.URL.Path, relayPrefix)
			pr.Out.URL.RawPath = ""
			if pr.Out.URL.Path == "" {
				pr.Out.URL.Path = "/"
			}
			pr.Out.URL.RawQuery = pr.In.URL.RawQuery
			// Never forward our origin's cookies to the analytics upstream.
			pr.Out.Header.Del("Cookie")
		},
		// Short upstream timeouts: a PostHog outage must never back-pressure
		// real API traffic through this mux.
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// Analytics is best-effort; the SPA fires and forgets. The
			// upstream error still lands on the canonical request line.
			recordErrDetail(w, err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}
	return proxy, nil
}
