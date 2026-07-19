package server

import (
	"log"
	"net/http"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/internal/saas/authz"
	"github.com/abhisek/mathiz/internal/saas/billing"
)

// Billing API — parent-only. The kid surface never sees prices or balances.

func (s *Server) handleGetBilling(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageBilling(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}

	// Families created before billing existed still get their starter
	// credits (idempotent).
	if err := s.credits.EnsureStarterGrant(r.Context(), spaceID); err != nil {
		writeServiceError(w, err)
		return
	}

	balance, err := s.credits.Balance(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	state, err := s.billing.State(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	resp := map[string]any{
		"balance": balance,
		"plan":    state.PlanID,
		"status":  state.Status,
		"plans":   billing.Plans(),
	}
	if state.CurrentPeriodEnd != nil {
		resp["periodEnd"] = rfc3339(*state.CurrentPeriodEnd)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBillingCheckout(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageBilling(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	var req struct {
		PlanID string `json:"planId"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, ok := billing.PlanByID(req.PlanID); !ok {
		writeError(w, http.StatusBadRequest, "unknown plan")
		return
	}
	url, err := s.billing.Provider().CreateCheckout(r.Context(), billing.CheckoutParams{
		FamilySpaceID: spaceID,
		PlanID:        req.PlanID,
		SuccessURL:    "/dashboard?billing=success",
		CancelURL:     "/dashboard",
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (s *Server) handleBillingPortal(w http.ResponseWriter, r *http.Request, p authz.Principal, acct *ent.Account) {
	spaceID := r.PathValue("id")
	if err := s.checker.CanManageBilling(r.Context(), p, spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	state, err := s.billing.State(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	url, err := s.billing.Provider().PortalURL(r.Context(), state.CustomerID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// applyProviderEvents parses the provider request and applies its events.
// Shared by the webhook and the fake completion redirect so error handling
// can't drift between them. Returns false when an error response was written.
func (s *Server) applyProviderEvents(w http.ResponseWriter, r *http.Request, parseErrMsg string) bool {
	events, err := s.billing.Provider().ParseWebhook(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, parseErrMsg)
		return false
	}
	for _, ev := range events {
		if err := s.billing.Apply(r.Context(), ev); err != nil {
			log.Printf("billing: apply %s (%s): %v", ev.Type, ev.EventID, err)
			writeError(w, http.StatusInternalServerError, "event application failed")
			return false
		}
	}
	return true
}

// handleBillingWebhook is the provider's server-to-server channel. The
// provider implementation verifies authenticity (signature / issued token);
// event application is idempotent via ledger sources.
func (s *Server) handleBillingWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.applyProviderEvents(w, r, "invalid webhook") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleFakeBillingComplete simulates the provider redirecting the parent
// back after a successful payment (fake provider / dev only).
func (s *Server) handleFakeBillingComplete(w http.ResponseWriter, r *http.Request) {
	if !s.applyProviderEvents(w, r, "invalid or used checkout token") {
		return
	}
	http.Redirect(w, r, "/dashboard?billing=success", http.StatusSeeOther)
}
