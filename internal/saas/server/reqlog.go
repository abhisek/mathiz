package server

import (
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/abhisek/mathiz/internal/saas/logctx"
)

// Canonical request logging: ONE structured line per request, wide-event
// style. The middleware is the outermost wrapper (CORS preflights included),
// installs the logctx bag, recovers panics, and emits the line on completion
// with everything the request accumulated: status, duration, principal,
// error message, error detail.

// logWriter is the ResponseWriter shim: it captures status code and bytes
// written, and carries slots for the client-visible error message
// (writeError) and the internal error detail (writeServiceError et al.)
// so error paths land on the canonical line with zero call-site changes.
type logWriter struct {
	http.ResponseWriter

	status      int
	bytes       int64
	wroteHeader bool

	// mu guards the error slots only — handlers may record errors from
	// spawned goroutines; status/bytes are written on the serving goroutine.
	mu        sync.Mutex
	errMsg    string
	errDetail string
}

func (w *logWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *logWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// Flush passes streaming through (the analytics relay may stream).
func (w *logWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap supports http.ResponseController.
func (w *logWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *logWriter) setErr(msg string) {
	w.mu.Lock()
	w.errMsg = msg
	w.mu.Unlock()
}

func (w *logWriter) setErrDetail(msg string) {
	w.mu.Lock()
	w.errDetail = msg
	w.mu.Unlock()
}

func (w *logWriter) errSlots() (msg, detail string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.errMsg, w.errDetail
}

// recordErrDetail stashes the REAL underlying error on the canonical line
// while the HTTP body stays generic. Fail-open when w is not our shim
// (tests, handlers exercised standalone).
func recordErrDetail(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	if lw, ok := w.(*logWriter); ok {
		lw.setErrDetail(err.Error())
	}
}

// withRequestLog is the outermost middleware: shim + logctx bag + panic
// recovery + the one canonical line.
func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &logWriter{ResponseWriter: w, status: http.StatusOK}
		r = r.WithContext(logctx.Install(r.Context()))

		var panicked any
		var stack string
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					panicked = rec
					stack = trimmedStack()
					// A handler panic must never be a silent empty reply:
					// send a 500 JSON error if the response hasn't started.
					if !lw.wroteHeader {
						writeError(lw, http.StatusInternalServerError, "internal error")
					}
					lw.status = http.StatusInternalServerError
				}
			}()
			next.ServeHTTP(lw, r)
		}()

		// Go 1.22 ServeMux sets r.Pattern in place on match: group lines by
		// route pattern, not high-cardinality raw paths. Unmatched requests
		// (SPA fallback, preflights) log the raw path.
		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}

		level := slog.LevelDebug // SPA assets/pages: not wallpaper by default
		switch {
		case lw.status >= 500:
			level = slog.LevelError
		case lw.status >= 400:
			level = slog.LevelWarn
		case strings.HasPrefix(r.URL.Path, "/api/") ||
			r.URL.Path == relayPrefix ||
			strings.HasPrefix(r.URL.Path, relayPrefix+"/"):
			level = slog.LevelInfo
		}

		attrs := make([]slog.Attr, 0, 16)
		attrs = append(attrs,
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", lw.status),
			slog.Float64("dur_ms", float64(time.Since(start).Microseconds())/1000),
			slog.Int64("bytes", lw.bytes),
			slog.String("ip", s.clientIP(r)),
		)
		attrs = append(attrs, logctx.Attrs(r.Context())...)
		if errMsg, errDetail := lw.errSlots(); errMsg != "" || errDetail != "" {
			if errMsg != "" {
				attrs = append(attrs, slog.String("err", errMsg))
			}
			if errDetail != "" {
				attrs = append(attrs, slog.String("err_detail", errDetail))
			}
		}
		if panicked != nil {
			attrs = append(attrs,
				slog.Any("panic", panicked),
				slog.String("stack", stack),
			)
		}

		s.logger.LogAttrs(r.Context(), level, "request", attrs...)

		// http.ErrAbortHandler is the sanctioned "abort this response" panic
		// (the reverse proxy uses it on upstream copy failures): re-raise it
		// after logging so net/http aborts the connection as intended.
		if panicked == http.ErrAbortHandler {
			panic(panicked)
		}
	})
}

// trimmedStack is the current goroutine's stack, capped so a canonical line
// stays a line-ish.
func trimmedStack() string {
	buf := make([]byte, 8<<10)
	n := runtime.Stack(buf, false)
	const max = 2048
	if n > max {
		n = max
	}
	return string(buf[:n])
}
