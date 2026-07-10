// Package billing connects a payment provider to the credit ledger.
//
// The Provider abstraction is deliberately thin — checkout URL, portal URL,
// webhook parsing into a tiny normalized event set. Entitlements live in
// the credit ledger and billing_states, never in the provider; anything
// beyond these three operations (proration, tax, invoices) is the
// provider's problem and is intentionally NOT abstracted.
package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/abhisek/mathiz/ent"
	"github.com/abhisek/mathiz/ent/billingstate"
	"github.com/abhisek/mathiz/internal/saas/credits"
)

// EventType is the normalized set of provider happenings we care about.
type EventType string

const (
	EventSubscriptionActivated EventType = "subscription_activated"
	EventSubscriptionRenewed   EventType = "subscription_renewed"
	EventSubscriptionCanceled  EventType = "subscription_canceled"
	EventTopupPurchased        EventType = "topup_purchased"
)

// Event is a normalized billing event. EventID must be stable per provider
// event — it is the ledger idempotency key.
type Event struct {
	Type           EventType
	EventID        string
	FamilySpaceID  string
	PlanID         string // plans and top-up packs
	CustomerID     string
	SubscriptionID string
	PeriodEnd      time.Time // subscriptions
}

// CheckoutParams describes what the parent is buying.
type CheckoutParams struct {
	FamilySpaceID string
	PlanID        string // plan or top-up pack ID from the catalog
	SuccessURL    string
	CancelURL     string
}

// Provider is the payment-provider abstraction. Implementations: fake
// (dev/tests), stripe, paddle.
type Provider interface {
	Name() string
	CreateCheckout(ctx context.Context, p CheckoutParams) (string, error)
	PortalURL(ctx context.Context, customerID string) (string, error)
	// ParseWebhook verifies the request's signature and returns normalized
	// events. Implementations must reject unsigned/invalid payloads.
	ParseWebhook(r *http.Request) ([]Event, error)
}

var ErrUnknownPlan = errors.New("unknown plan")

// Service applies normalized events to the ledger + billing state.
type Service struct {
	client   *ent.Client
	credits  *credits.Service
	provider Provider
}

func NewService(client *ent.Client, creditsSvc *credits.Service, provider Provider) *Service {
	return &Service{client: client, credits: creditsSvc, provider: provider}
}

func (s *Service) Provider() Provider { return s.provider }

// Apply is the single write path from provider events into our state.
// Idempotent: the ledger dedups on EventID and state updates are
// last-write-wins on the same values.
func (s *Service) Apply(ctx context.Context, ev Event) error {
	plan, planKnown := PlanByID(ev.PlanID)

	switch ev.Type {
	case EventSubscriptionActivated, EventSubscriptionRenewed:
		if !planKnown || plan.MonthlyCredits == 0 {
			return fmt.Errorf("%w: %q", ErrUnknownPlan, ev.PlanID)
		}
		// Previous period's leftover plan credits retire when the new
		// period's grant lands (top-ups are never touched). Expiry + grant
		// are one idempotent unit keyed on the event ID: a replayed webhook
		// must not re-expire the grant its first delivery created.
		if err := s.credits.RenewPlanCredits(ctx, ev.FamilySpaceID,
			plan.MonthlyCredits, nil, "sub:"+ev.EventID); err != nil {
			return err
		}
		return s.updateState(ctx, ev, "active")

	case EventSubscriptionCanceled:
		return s.updateState(ctx, ev, "canceled")

	case EventTopupPurchased:
		if !planKnown || plan.TopupCredits == 0 {
			return fmt.Errorf("%w: %q", ErrUnknownPlan, ev.PlanID)
		}
		return s.credits.Grant(ctx, ev.FamilySpaceID, credits.KindTopup,
			plan.TopupCredits, nil, "topup:"+ev.EventID)

	default:
		return fmt.Errorf("unknown billing event type %q", ev.Type)
	}
}

// State returns the billing state for a space (a zero-value "none" state if
// the family has never subscribed).
func (s *Service) State(ctx context.Context, spaceUID string) (*ent.BillingState, error) {
	st, err := s.client.BillingState.Query().
		Where(billingstate.FamilySpaceID(spaceUID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return &ent.BillingState{FamilySpaceID: spaceUID, Status: "none"}, nil
	}
	return st, err
}

func (s *Service) updateState(ctx context.Context, ev Event, status string) error {
	st, err := s.client.BillingState.Query().
		Where(billingstate.FamilySpaceID(ev.FamilySpaceID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		create := s.client.BillingState.Create().
			SetUID(uuid.NewString()).
			SetFamilySpaceID(ev.FamilySpaceID).
			SetProvider(s.provider.Name()).
			SetCustomerID(ev.CustomerID).
			SetSubscriptionID(ev.SubscriptionID).
			SetPlanID(ev.PlanID).
			SetStatus(status)
		if !ev.PeriodEnd.IsZero() {
			create.SetCurrentPeriodEnd(ev.PeriodEnd)
		}
		if err := create.Exec(ctx); err != nil && !ent.IsConstraintError(err) {
			return fmt.Errorf("create billing state: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("query billing state: %w", err)
	}

	upd := st.Update().
		SetProvider(s.provider.Name()).
		SetStatus(status)
	if ev.CustomerID != "" {
		upd.SetCustomerID(ev.CustomerID)
	}
	if ev.SubscriptionID != "" {
		upd.SetSubscriptionID(ev.SubscriptionID)
	}
	if ev.PlanID != "" {
		upd.SetPlanID(ev.PlanID)
	}
	if !ev.PeriodEnd.IsZero() {
		upd.SetCurrentPeriodEnd(ev.PeriodEnd)
	}
	return upd.Exec(ctx)
}
