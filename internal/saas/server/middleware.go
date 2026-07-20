package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/auth"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/saas/logctx"
)

// parentHandler receives the verified principal and provisioned account.
type parentHandler func(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account)

// childHandler receives the verified principal and child profile.
type childHandler func(w http.ResponseWriter, r *http.Request, p authz.Principal, child *ent.ChildProfile)

// withParent verifies the Supabase JWT, provisions the account, and invokes
// the handler with a parent principal.
func (s *Server) withParent(h parentHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		id, err := s.verifier.Verify(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		acct, err := s.family.EnsureAccount(r.Context(), id.SupabaseUserID, id.Email, id.DisplayName)
		if err != nil {
			recordErrDetail(w, fmt.Errorf("ensure account: %w", err))
			writeError(w, http.StatusInternalServerError, "account provisioning failed")
			return
		}
		// Canonical-line identity: UIDs only, never emails or names.
		logctx.Add(r.Context(), "principal", "parent")
		logctx.Add(r.Context(), "account", acct.UID)
		p := authz.Principal{Kind: authz.KindParent, AccountID: acct.UID}
		h(w, r, p, acct)
	})
}

// withChild authenticates a child device token.
func (s *Server) withChild(h childHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		_, child, err := s.family.ResolveDeviceToken(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		// Canonical-line identity: UIDs only, never names.
		logctx.Add(r.Context(), "principal", "child")
		logctx.Add(r.Context(), "child", child.UID)
		h(w, r, authz.ChildPrincipal(child), child)
	})
}

// ---- JSON helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are gone; all we can do is put it on the canonical line
		// (typically the client hung up mid-body).
		recordErrDetail(w, fmt.Errorf("encode response: %w", err))
	}
}

type errorBody struct {
	Error string `json:"error"`
}

// writeError sends the JSON error body AND records the message into the
// request-log shim so it lands on the canonical line as err=<msg>. Fail-open
// when w is not our shim (tests exercising handlers directly).
func writeError(w http.ResponseWriter, status int, msg string) {
	if lw, ok := w.(*logWriter); ok {
		lw.setErr(msg)
	}
	writeJSON(w, status, errorBody{Error: msg})
}

// decodeJSON reads a bounded JSON body.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		recordErrDetail(w, err)
		writeError(w, http.StatusBadRequest, "malformed request body")
		return false
	}
	return true
}

// writeServiceError maps family/authz errors onto HTTP statuses.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, authz.ErrDenied):
		// Existence probes get 404, not 403 — do not confirm object IDs the
		// caller cannot see.
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, family.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, family.ErrSpaceExists),
		errors.Is(err, family.ErrAlreadyMember),
		errors.Is(err, family.ErrAlreadyInvited):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, family.ErrInviteInvalid),
		errors.Is(err, family.ErrParentInviteInvalid),
		errors.Is(err, family.ErrPINRequired),
		errors.Is(err, family.ErrPINMismatch),
		errors.Is(err, family.ErrArchived):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, family.ErrBadPIN),
		errors.Is(err, family.ErrBadGrade),
		errors.Is(err, family.ErrBadName),
		errors.Is(err, family.ErrBadEmail),
		errors.Is(err, family.ErrOwnerRemoval):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		// The HTTP body stays generic; the REAL error rides the canonical
		// request line as err_detail.
		recordErrDetail(w, err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// ---- Per-IP rate limiting for unauthenticated join endpoints ----

type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipEntry
	rps      rate.Limit
	burst    int
}

type ipEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

func newIPLimiter(rps float64, burst int) *ipLimiter {
	return &ipLimiter{
		limiters: make(map[string]*ipEntry),
		rps:      rate.Limit(rps),
		burst:    burst,
	}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	// Opportunistic cleanup keeps the map bounded without a background goroutine.
	if len(l.limiters) > 10_000 {
		for k, e := range l.limiters {
			if now.Sub(e.seen) > 10*time.Minute {
				delete(l.limiters, k)
			}
		}
	}
	e, ok := l.limiters[ip]
	if !ok {
		e = &ipEntry{lim: rate.NewLimiter(l.rps, l.burst)}
		l.limiters[ip] = e
	}
	e.seen = now
	return e.lim.Allow()
}

func (s *Server) rateLimited(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.joinLimiter.allow(s.clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "slow down")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP identifies the caller for rate limiting. Behind a trusted proxy
// the last X-Forwarded-For hop (appended by our own proxy) is the client;
// otherwise trusting the header would let anyone spoof their bucket.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
