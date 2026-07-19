// Package server is the HTTP layer of the Mathiz SaaS: parent dashboard API,
// child join/redeem flow, and mounting points for the terminal WebSocket and
// the embedded web UI.
package server

import (
	"net/http"

	"github.com/abhisek/mathiz/internal/saas/activity"
	"github.com/abhisek/mathiz/internal/saas/auth"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/billing"
	"github.com/abhisek/mathiz/internal/saas/credits"
	"github.com/abhisek/mathiz/internal/saas/family"
	"github.com/abhisek/mathiz/internal/saas/game"
	"github.com/abhisek/mathiz/internal/saas/quests"
	"github.com/abhisek/mathiz/internal/store"
)

// Deps carries everything a Server needs. Terminal, WebUI, Game, Credits,
// Billing, and Quests are optional — their routes 404 / fall through when nil.
type Deps struct {
	Config   *Config
	Store    *store.Store
	Family   *family.Service
	Verifier *auth.SupabaseVerifier
	Terminal http.Handler
	WebUI    http.Handler
	Game     *game.Manager
	Credits  *credits.Service
	Billing  *billing.Service
	Quests   *quests.Service
	Activity *activity.Reader
}

// Server wires config, services, and routes.
type Server struct {
	cfg      *Config
	st       *store.Store
	family   *family.Service
	checker  *authz.Checker
	verifier *auth.SupabaseVerifier

	terminal http.Handler
	webui    http.Handler
	game     *game.Manager
	credits  *credits.Service
	billing  *billing.Service
	quests   *quests.Service
	activity *activity.Reader

	joinLimiter *ipLimiter
	handler     http.Handler
}

// New builds a Server from its dependencies.
func New(d Deps) *Server {
	s := &Server{
		cfg:      d.Config,
		st:       d.Store,
		family:   d.Family,
		checker:  authz.NewChecker(d.Family),
		verifier: d.Verifier,
		terminal: d.Terminal,
		webui:    d.WebUI,
		game:     d.Game,
		credits:  d.Credits,
		billing:  d.Billing,
		quests:   d.Quests,
		activity: d.Activity,
		// Join endpoints are unauthenticated: keep brute force slow.
		joinLimiter: newIPLimiter(1, 10),
	}
	if d.Quests != nil {
		s.checker.SetQuests(d.Quests)
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
	mux.Handle("GET /api/v1/family/{id}/parents", s.withParent(s.handleListParents))
	mux.Handle("POST /api/v1/family/{id}/parents", s.withParent(s.handleInviteParent))
	mux.Handle("DELETE /api/v1/family/{id}/parents/{accountId}", s.withParent(s.handleRemoveParent))
	mux.Handle("DELETE /api/v1/parent-invites/{id}", s.withParent(s.handleRevokeParentInvite))
	mux.Handle("POST /api/v1/invites/parent/{id}/accept", s.withParent(s.handleAcceptParentInvite))
	mux.Handle("PATCH /api/v1/children/{id}", s.withParent(s.handleUpdateChild))
	mux.Handle("GET /api/v1/children/{id}/stats", s.withParent(s.handleChildStats))
	if s.activity != nil {
		mux.Handle("GET /api/v1/children/{id}/activity", s.withParent(s.handleChildActivity))
		mux.Handle("GET /api/v1/children/{id}/activity/sessions/{sessionId}", s.withParent(s.handleChildActivitySession))
	}
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

	// Parent quests (specs/15-quests.md).
	if s.quests != nil {
		mux.Handle("POST /api/v1/family/{id}/quests", s.withParent(s.handleCreateQuest))
		mux.Handle("GET /api/v1/family/{id}/quests", s.withParent(s.handleListQuests))
		mux.Handle("GET /api/v1/quests/{id}", s.withParent(s.handleGetQuest))
		mux.Handle("PATCH /api/v1/quests/{id}", s.withParent(s.handleUpdateQuest))
		mux.Handle("DELETE /api/v1/quests/{id}", s.withParent(s.handleDeleteQuest))
		mux.Handle("POST /api/v1/quests/{id}/questions", s.withParent(s.handleAddQuestQuestion))
		mux.Handle("PATCH /api/v1/quests/{id}/questions/{qid}", s.withParent(s.handleUpdateQuestQuestion))
		mux.Handle("DELETE /api/v1/quests/{id}/questions/{qid}", s.withParent(s.handleDeleteQuestQuestion))
		mux.Handle("POST /api/v1/quests/{id}/generate", s.withParent(s.handleGenerateQuestQuestions))
		mux.Handle("POST /api/v1/quests/{id}/publish", s.withParent(s.handlePublishQuest))
		if s.game != nil {
			mux.Handle("POST /api/v1/game/quests/{id}/expeditions", s.withChild(s.handleQuestExpeditionStart))
		}
	}

	// Billing (only when a provider is configured).
	if s.billing != nil {
		mux.Handle("GET /api/v1/family/{id}/billing", s.withParent(s.handleGetBilling))
		mux.Handle("POST /api/v1/family/{id}/billing/checkout", s.withParent(s.handleBillingCheckout))
		mux.Handle("POST /api/v1/family/{id}/billing/portal", s.withParent(s.handleBillingPortal))
		mux.HandleFunc("POST /api/v1/billing/webhook", s.handleBillingWebhook)
		if s.billing.Provider().Name() == "fake" {
			// Dev-only: the fake provider's "payment succeeded" redirect.
			mux.HandleFunc("GET /api/v1/billing/fake/complete", s.handleFakeBillingComplete)
		}
	}

	// Terminal WebSocket (authenticates in-protocol via first message).
	if s.terminal != nil {
		mux.Handle("GET /api/v1/terminal", s.terminal)
	}

	// Unmatched API paths must 404 as JSON, never fall through to the SPA:
	// a 200 index.html for a missing endpoint (e.g. billing routes when
	// billing is off) silently poisons API clients with HTML.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

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
