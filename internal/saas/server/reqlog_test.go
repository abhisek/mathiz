package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/abhisek/mathiz/internal/saas/logctx"
)

// ---- capture handler ----

// capturedRecord is a flattened slog record.
type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

// captureHandler collects every record at every level.
type captureHandler struct {
	mu      sync.Mutex
	records []capturedRecord
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := capturedRecord{level: r.Level, msg: r.Message, attrs: map[string]any{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) all() []capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]capturedRecord(nil), h.records...)
}

// newLogTestServer builds the minimal Server the middleware needs.
func newLogTestServer() (*Server, *captureHandler) {
	h := &captureHandler{}
	return &Server{
		cfg:    &Config{},
		logger: slog.New(h),
	}, h
}

func singleRecord(t *testing.T, h *captureHandler) capturedRecord {
	t.Helper()
	recs := h.all()
	if len(recs) != 1 {
		t.Fatalf("expected exactly 1 log record, got %d: %+v", len(recs), recs)
	}
	return recs[0]
}

// ---- middleware tests ----

func TestRequestLogBasicFields(t *testing.T) {
	s, h := newLogTestServer()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/things/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	})
	srv := s.withRequestLog(mux)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/things/42", nil))

	rec := singleRecord(t, h)
	if rec.level != slog.LevelInfo {
		t.Errorf("level = %v, want Info", rec.level)
	}
	if got := rec.attrs["method"]; got != "GET" {
		t.Errorf("method = %v", got)
	}
	// Route must be the mux pattern, not the raw path.
	if got := rec.attrs["route"]; got != "GET /api/v1/things/{id}" {
		t.Errorf("route = %v, want pattern", got)
	}
	if got := rec.attrs["status"]; got != int64(200) {
		t.Errorf("status = %v (%T)", got, got)
	}
	if _, ok := rec.attrs["dur_ms"]; !ok {
		t.Error("dur_ms missing")
	}
	if got, ok := rec.attrs["bytes"].(int64); !ok || got <= 0 {
		t.Errorf("bytes = %v", rec.attrs["bytes"])
	}
	if _, ok := rec.attrs["ip"]; !ok {
		t.Error("ip missing")
	}
	if _, ok := rec.attrs["err"]; ok {
		t.Error("unexpected err attr on success")
	}
}

func TestRequestLogWriteErrorSameLine(t *testing.T) {
	s, h := newLogTestServer()
	srv := s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusBadRequest, "malformed request body")
	}))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("POST", "/api/v1/family", nil))

	rec := singleRecord(t, h)
	if rec.level != slog.LevelWarn {
		t.Errorf("level = %v, want Warn for 4xx", rec.level)
	}
	if got := rec.attrs["status"]; got != int64(400) {
		t.Errorf("status = %v", got)
	}
	if got := rec.attrs["err"]; got != "malformed request body" {
		t.Errorf("err = %v, want the writeError message on the SAME line", got)
	}
}

func TestRequestLogServiceErrorDetail(t *testing.T) {
	s, h := newLogTestServer()
	srv := s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeServiceError(w, errors.New("pq: connection refused"))
	}))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/me", nil))

	rec := singleRecord(t, h)
	if rec.level != slog.LevelError {
		t.Errorf("level = %v, want Error for 5xx", rec.level)
	}
	if got := rec.attrs["err"]; got != "internal error" {
		t.Errorf("err = %v", got)
	}
	// The real cause reaches the line; the body stays generic.
	if got := rec.attrs["err_detail"]; got != "pq: connection refused" {
		t.Errorf("err_detail = %v", got)
	}
	if strings.Contains(rr.Body.String(), "connection refused") {
		t.Error("internal error detail leaked into the HTTP body")
	}
}

func TestRequestLogLogctxEnrichment(t *testing.T) {
	s, h := newLogTestServer()
	srv := s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logctx.Add(r.Context(), "principal", "parent")
		logctx.Add(r.Context(), "account", "acc_123")
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/me", nil))

	rec := singleRecord(t, h)
	if got := rec.attrs["principal"]; got != "parent" {
		t.Errorf("principal = %v", got)
	}
	if got := rec.attrs["account"]; got != "acc_123" {
		t.Errorf("account = %v", got)
	}
}

func TestRequestLogPanicRecovery(t *testing.T) {
	s, h := newLogTestServer()
	srv := s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest("GET", "/api/v1/game/map", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "internal error") {
		t.Errorf("body = %q, want JSON error", rr.Body.String())
	}
	rec := singleRecord(t, h)
	if rec.level != slog.LevelError {
		t.Errorf("level = %v, want Error", rec.level)
	}
	if got := rec.attrs["panic"]; got != "boom" {
		t.Errorf("panic = %v", got)
	}
	if stack, _ := rec.attrs["stack"].(string); !strings.Contains(stack, "goroutine") {
		t.Errorf("stack missing or malformed: %q", stack)
	}
	if got := rec.attrs["status"]; got != int64(500) {
		t.Errorf("status = %v", got)
	}
}

func TestRequestLogLevels(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		status int
		want   slog.Level
	}{
		{"spa asset is Debug", "/dashboard", http.StatusOK, slog.LevelDebug},
		{"api 2xx is Info", "/api/v1/config", http.StatusOK, slog.LevelInfo},
		{"relay 2xx is Info", "/relay/e", http.StatusOK, slog.LevelInfo},
		{"4xx is Warn", "/api/v1/nope", http.StatusNotFound, slog.LevelWarn},
		{"4xx on spa path is Warn", "/no-such-page", http.StatusNotFound, slog.LevelWarn},
		{"5xx is Error", "/api/v1/me", http.StatusBadGateway, slog.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, h := newLogTestServer()
			srv := s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", tt.path, nil))
			if rec := singleRecord(t, h); rec.level != tt.want {
				t.Errorf("level = %v, want %v", rec.level, tt.want)
			}
		})
	}
}

func TestRequestLogFallsBackToRawPath(t *testing.T) {
	// No mux → r.Pattern stays empty → the raw path is the route.
	s, h := newLogTestServer()
	srv := s.withRequestLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/assets/app.js", nil))
	if got := singleRecord(t, h).attrs["route"]; got != "/assets/app.js" {
		t.Errorf("route = %v, want raw path fallback", got)
	}
}

// The shim must keep streaming (Flusher) and ResponseController working.
func TestLogWriterFlushAndUnwrap(t *testing.T) {
	rr := httptest.NewRecorder() // implements http.Flusher
	lw := &logWriter{ResponseWriter: rr, status: http.StatusOK}

	if _, ok := any(lw).(http.Flusher); !ok {
		t.Fatal("logWriter must implement http.Flusher")
	}
	lw.Flush()
	if !rr.Flushed {
		t.Error("Flush did not pass through")
	}
	if lw.Unwrap() != http.ResponseWriter(rr) {
		t.Error("Unwrap must return the wrapped writer")
	}
}

// ---- logger construction ----

func TestNewLoggerFileTee(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mathiz.log")
	var stdout bytes.Buffer
	logger, closeFn, err := NewLogger(&stdout, &Config{LogFile: path, LogFormat: "text", LogLevel: "info"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Info("hello", "k", "v")
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if !strings.Contains(stdout.String(), "hello") || !strings.Contains(stdout.String(), "k=v") {
		t.Errorf("stdout missing line: %q", stdout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello") || !strings.Contains(string(data), "k=v") {
		t.Errorf("file missing line: %q", string(data))
	}
}

func TestNewLoggerBadFileFails(t *testing.T) {
	_, _, err := NewLogger(io.Discard, &Config{LogFile: filepath.Join(t.TempDir(), "no", "such", "dir", "x.log")})
	if err == nil {
		t.Fatal("expected error for unopenable log file")
	}
}

func TestNewLoggerRejectsBadFormatAndLevel(t *testing.T) {
	if _, _, err := NewLogger(io.Discard, &Config{LogFormat: "yaml"}); err == nil {
		t.Error("expected error for bad format")
	}
	if _, _, err := NewLogger(io.Discard, &Config{LogLevel: "verbose"}); err == nil {
		t.Error("expected error for bad level")
	}
}

func TestNewLoggerJSONAndLevelFilter(t *testing.T) {
	var out bytes.Buffer
	logger, closeFn, err := NewLogger(&out, &Config{LogFormat: "json", LogLevel: "warn"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer closeFn()
	logger.Info("dropped")
	logger.Warn("kept")
	s := out.String()
	if strings.Contains(s, "dropped") {
		t.Error("info line should be filtered at warn level")
	}
	if !strings.Contains(s, `"msg":"kept"`) {
		t.Errorf("want JSON warn line, got %q", s)
	}
}
