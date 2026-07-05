// Package server is the HTTP layer of the Mathiz SaaS: parent dashboard API,
// child join/redeem flow, and mounting points for the terminal WebSocket and
// the embedded web UI.
package server

import (
	"net/http"

	"github.com/abhisek/mathiz/internal/saas/auth"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/saas/game"
	"github.com/abhisek/mathiz/internal/store"
)

// Server wires config, services, and routes.
type Server struct {
	cfg      *Config
	st       *store.Store
	family   *family.Service
	checker  *authz.Checker
	verifier *auth.SupabaseVerifier

	// terminal serves the learning session WebSocket. Optional.
	terminal http.Handler
	// webui serves the embedded SPA. Optional.
	webui http.Handler
	// game is the treasure-map play manager. Optional.
	game *game.Manager

	joinLimiter *ipLimiter
	handler     http.Handler
}

// New builds a Server around a shared family service. terminal, webui, and
// gameMgr may be nil (their routes 404 / fall through).
func New(cfg *Config, st *store.Store, svc *family.Service, verifier *auth.SupabaseVerifier, terminal, webui http.Handler, gameMgr *game.Manager) *Server {
	s := &Server{
		cfg:      cfg,
		st:       st,
		family:   svc,
		checker:  authz.NewChecker(svc),
		verifier: verifier,
		terminal: terminal,
		webui:    webui,
		game:     gameMgr,
		// Join endpoints are unauthenticated: keep brute force slow.
		joinLimiter: newIPLimiter(1, 10),
	}
	s.handler = s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Public.
	mux.HandleFunc("GET /api/v1/config", s.handleBootConfig)
	mux.Handle("POST /api/v1/join/preview", s.rateLimited(http.HandlerFunc(s.handleJoinPreview)))
	mux.Handle("POST /api/v1/join/redeem", s.rateLimited(http.HandlerFunc(s.handleJoinRedeem)))

	// Parent (Supabase JWT).
	mux.Handle("GET /api/v1/me", s.withParent(s.handleMe))
	mux.Handle("POST /api/v1/family", s.withParent(s.handleCreateFamily))
	mux.Handle("PATCH /api/v1/family/{id}", s.withParent(s.handleRenameFamily))
	mux.Handle("POST /api/v1/family/{id}/children", s.withParent(s.handleAddChild))
	mux.Handle("GET /api/v1/family/{id}/children", s.withParent(s.handleListChildren))
	mux.Handle("POST /api/v1/family/{id}/invites", s.withParent(s.handleCreateInvite))
	mux.Handle("GET /api/v1/family/{id}/invites", s.withParent(s.handleListInvites))
	mux.Handle("DELETE /api/v1/invites/{id}", s.withParent(s.handleRevokeInvite))
	mux.Handle("PATCH /api/v1/children/{id}", s.withParent(s.handleUpdateChild))
	mux.Handle("GET /api/v1/children/{id}/stats", s.withParent(s.handleChildStats))
	mux.Handle("GET /api/v1/children/{id}/devices", s.withParent(s.handleListDevices))
	mux.Handle("DELETE /api/v1/devices/{id}", s.withParent(s.handleRevokeDevice))

	// Child (device token).
	mux.Handle("GET /api/v1/child/me", s.withChild(s.handleChildMe))

	// Treasure-map game.
	if s.game != nil {
		mux.Handle("GET /api/v1/game/map", s.withChild(s.handleGameMap))
		mux.Handle("GET /api/v1/game/notebook", s.withChild(s.handleGameNotebook))
		mux.Handle("POST /api/v1/game/expeditions", s.withChild(s.handleExpeditionStart))
		mux.Handle("POST /api/v1/game/expeditions/{id}/question", s.withChild(s.handleExpeditionQuestion))
		mux.Handle("POST /api/v1/game/expeditions/{id}/answer", s.withChild(s.handleExpeditionAnswer))
		mux.Handle("POST /api/v1/game/expeditions/{id}/hint", s.withChild(s.handleExpeditionHint))
		mux.Handle("POST /api/v1/game/expeditions/{id}/lesson", s.withChild(s.handleExpeditionLesson))
		mux.Handle("POST /api/v1/game/expeditions/{id}/lesson/answer", s.withChild(s.handleExpeditionLessonAnswer))
		mux.Handle("POST /api/v1/game/expeditions/{id}/end", s.withChild(s.handleExpeditionEnd))
	}

	// Terminal WebSocket (authenticates in-protocol via first message).
	if s.terminal != nil {
		mux.Handle("GET /api/v1/terminal", s.terminal)
	}

	// Embedded SPA for everything else.
	if s.webui != nil {
		mux.Handle("/", s.webui)
	}

	return s.withCORS(mux)
}

// withCORS adds CORS headers for explicitly allowed origins. The embedded
// SPA is same-origin, so by default this is a no-op.
func (s *Server) withCORS(next http.Handler) http.Handler {
	if len(s.cfg.CORSOrigins) == 0 {
		return next
	}
	allowed := make(map[string]bool, len(s.cfg.CORSOrigins))
	for _, o := range s.cfg.CORSOrigins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
