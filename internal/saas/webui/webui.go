// Package webui embeds the built React SPA (web/ → dist here) into the
// binary so `mathiz serve` is a single deployable. Build it with `make web`.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

const notBuiltPage = `<!doctype html><html><head><title>Mathiz</title></head>
<body style="font-family: system-ui; max-width: 40rem; margin: 4rem auto; line-height: 1.6">
<h1>Mathiz server is running</h1>
<p>The web UI was not bundled into this binary. Build it with:</p>
<pre>make web &amp;&amp; make mathiz</pre>
<p>The API is live under <code>/api/v1</code>.</p>
</body></html>`

// Handler serves the SPA with client-side routing support: unknown paths
// fall back to index.html. Whether the SPA was bundled is fixed at compile
// time, so the check happens once, not per request.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		sub = dist
	}

	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(notBuiltPage))
		})
	}

	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(sub, path); err != nil {
			// Client-side route (e.g. /dashboard) → serve the app shell.
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
