package billing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FakeProvider is the dev/test payment provider: checkout "succeeds"
// immediately by redirecting through a local completion endpoint, which
// posts normalized events back through the same webhook path a real
// provider would use. The entire purchase→grant→play loop is clickable
// with zero external services.
type FakeProvider struct {
	mu       sync.Mutex
	sessions map[string]CheckoutParams // token → what's being bought

	// BaseURL is the server's own origin (e.g. http://localhost:8080).
	BaseURL string
}

func NewFakeProvider(baseURL string) *FakeProvider {
	return &FakeProvider{sessions: make(map[string]CheckoutParams), BaseURL: baseURL}
}

func (f *FakeProvider) Name() string { return "fake" }

func (f *FakeProvider) CreateCheckout(_ context.Context, p CheckoutParams) (string, error) {
	if _, ok := PlanByID(p.PlanID); !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownPlan, p.PlanID)
	}
	token := uuid.NewString()
	f.mu.Lock()
	f.sessions[token] = p
	f.mu.Unlock()
	return f.BaseURL + "/api/v1/billing/fake/complete?token=" + url.QueryEscape(token), nil
}

func (f *FakeProvider) PortalURL(_ context.Context, _ string) (string, error) {
	return f.BaseURL + "/dashboard", nil
}

// ParseWebhook accepts the fake completion redirect (?token=...) and turns
// it into the events a real provider would deliver. Signature verification
// is the token being one we issued.
func (f *FakeProvider) ParseWebhook(r *http.Request) ([]Event, error) {
	token := r.URL.Query().Get("token")
	f.mu.Lock()
	params, ok := f.sessions[token]
	if ok {
		delete(f.sessions, token) // single use
	}
	f.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown fake checkout token")
	}

	plan, _ := PlanByID(params.PlanID)
	ev := Event{
		EventID:       token,
		FamilySpaceID: params.FamilySpaceID,
		PlanID:        params.PlanID,
		CustomerID:    "fake-customer-" + params.FamilySpaceID,
	}
	if plan.Subscription() {
		ev.Type = EventSubscriptionActivated
		ev.SubscriptionID = "fake-sub-" + params.FamilySpaceID
		ev.PeriodEnd = time.Now().Add(30 * 24 * time.Hour)
	} else {
		ev.Type = EventTopupPurchased
	}
	return []Event{ev}, nil
}
