package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	stripe "github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/billingportal/session"
	checkout "github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/client"
	"github.com/stripe/stripe-go/v81/webhook"
)

// StripeProvider is the real-money adapter. Per specs/14-monetisation.md §4:
// subscriptions + one-time payments ONLY — no metered billing, no Stripe
// credit grants. Stripe is a money pipe; entitlements live in our ledger.
//
// Event mapping (deliberate): subscription credits flow exclusively from
// invoice.paid — billing_reason subscription_create activates, later
// invoices renew. checkout.session.completed is used only for top-up packs
// (mode=payment). Mapping activation to the checkout event as well would
// double-grant a brand-new subscription, because Stripe sends BOTH the
// checkout completion and the first invoice within seconds.
type StripeProvider struct {
	api           *client.API
	webhookSecret string
	baseURL       string // our public origin for success/cancel/return URLs
}

func NewStripeProvider(secretKey, webhookSecret, baseURL string) (*StripeProvider, error) {
	if secretKey == "" || webhookSecret == "" {
		return nil, fmt.Errorf("stripe: MATHIZ_STRIPE_SECRET_KEY and MATHIZ_STRIPE_WEBHOOK_SECRET are required")
	}
	api := &client.API{}
	api.Init(secretKey, nil)
	return &StripeProvider{api: api, webhookSecret: webhookSecret, baseURL: baseURL}, nil
}

func (s *StripeProvider) Name() string { return "stripe" }

// metadata keys carried through checkout → subscription → webhook events.
const (
	mdFamilySpace = "family_space_id"
	mdPlan        = "plan_id"
)

func (s *StripeProvider) CreateCheckout(ctx context.Context, p CheckoutParams) (string, error) {
	plan, ok := PlanByID(p.PlanID)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownPlan, p.PlanID)
	}
	if plan.ProviderPriceID == "" {
		return "", fmt.Errorf("stripe: plan %q has no price ID (set MATHIZ_BILLING_PRICE_*)", p.PlanID)
	}

	md := map[string]string{mdFamilySpace: p.FamilySpaceID, mdPlan: p.PlanID}
	params := &stripe.CheckoutSessionParams{
		SuccessURL:        stripe.String(s.baseURL + p.SuccessURL),
		CancelURL:         stripe.String(s.baseURL + p.CancelURL),
		ClientReferenceID: stripe.String(p.FamilySpaceID),
		Metadata:          md,
		// Early-adopter comps are Stripe promotion codes (100%-off etc.);
		// the checkout page must show the code entry field.
		AllowPromotionCodes: stripe.Bool(true),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Price:    stripe.String(plan.ProviderPriceID),
			Quantity: stripe.Int64(1),
		}},
	}
	if plan.Subscription() {
		params.Mode = stripe.String(string(stripe.CheckoutSessionModeSubscription))
		// The webhook reads family/plan from the SUBSCRIPTION's metadata
		// (invoice.paid doesn't carry checkout-session metadata).
		params.SubscriptionData = &stripe.CheckoutSessionSubscriptionDataParams{Metadata: md}
	} else {
		params.Mode = stripe.String(string(stripe.CheckoutSessionModePayment))
	}

	sess, err := checkout.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe: create checkout: %w", err)
	}
	return sess.URL, nil
}

func (s *StripeProvider) PortalURL(ctx context.Context, customerID string) (string, error) {
	if customerID == "" {
		return "", fmt.Errorf("stripe: no customer on file yet — subscribe first")
	}
	sess, err := session.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(s.baseURL + "/dashboard"),
	})
	if err != nil {
		return "", fmt.Errorf("stripe: portal session: %w", err)
	}
	return sess.URL, nil
}

// ParseWebhook verifies the Stripe-Signature header and normalizes the
// event. Unmapped event types return no events (204 upstream) — Stripe
// sends many types we don't model. Every returned Event carries Stripe's
// own event ID, which becomes the ledger idempotency source.
func (s *StripeProvider) ParseWebhook(r *http.Request) ([]Event, error) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("stripe: read payload: %w", err)
	}
	ev, err := webhook.ConstructEventWithOptions(payload, r.Header.Get("Stripe-Signature"), s.webhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true})
	if err != nil {
		return nil, fmt.Errorf("stripe: verify signature: %w", err)
	}

	switch ev.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(ev.Data.Raw, &cs); err != nil {
			return nil, fmt.Errorf("stripe: decode checkout session: %w", err)
		}
		// Top-ups only; subscription credits flow via invoice.paid.
		if cs.Mode != stripe.CheckoutSessionModePayment {
			return nil, nil
		}
		out := Event{
			Type:          EventTopupPurchased,
			EventID:       ev.ID,
			FamilySpaceID: cs.Metadata[mdFamilySpace],
			PlanID:        cs.Metadata[mdPlan],
		}
		if cs.Customer != nil {
			out.CustomerID = cs.Customer.ID
		}
		return validated(out)

	case "invoice.paid":
		var inv stripe.Invoice
		if err := json.Unmarshal(ev.Data.Raw, &inv); err != nil {
			return nil, fmt.Errorf("stripe: decode invoice: %w", err)
		}
		md := map[string]string{}
		if inv.SubscriptionDetails != nil {
			md = inv.SubscriptionDetails.Metadata
		}
		out := Event{
			EventID:       ev.ID,
			FamilySpaceID: md[mdFamilySpace],
			PlanID:        md[mdPlan],
		}
		if inv.BillingReason == stripe.InvoiceBillingReasonSubscriptionCreate {
			out.Type = EventSubscriptionActivated
		} else {
			out.Type = EventSubscriptionRenewed
		}
		if inv.Customer != nil {
			out.CustomerID = inv.Customer.ID
		}
		if inv.Subscription != nil {
			out.SubscriptionID = inv.Subscription.ID
		}
		if inv.Lines != nil && len(inv.Lines.Data) > 0 && inv.Lines.Data[0].Period != nil {
			out.PeriodEnd = time.Unix(inv.Lines.Data[0].Period.End, 0)
		}
		return validated(out)

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(ev.Data.Raw, &sub); err != nil {
			return nil, fmt.Errorf("stripe: decode subscription: %w", err)
		}
		out := Event{
			Type:           EventSubscriptionCanceled,
			EventID:        ev.ID,
			FamilySpaceID:  sub.Metadata[mdFamilySpace],
			PlanID:         sub.Metadata[mdPlan],
			SubscriptionID: sub.ID,
		}
		if sub.Customer != nil {
			out.CustomerID = sub.Customer.ID
		}
		return validated(out)

	default:
		return nil, nil // unmodeled event type — acknowledge, do nothing
	}
}

// validated refuses events that lost their family linkage (e.g. a
// subscription created outside our checkout flow) rather than letting a
// grant with an empty family_space_id into Service.Apply.
func validated(ev Event) ([]Event, error) {
	if ev.FamilySpaceID == "" || ev.PlanID == "" {
		return nil, fmt.Errorf("stripe: event %s missing family/plan metadata", ev.EventID)
	}
	return []Event{ev}, nil
}
