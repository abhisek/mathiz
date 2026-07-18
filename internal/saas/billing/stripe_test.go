package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testWebhookSecret = "whsec_test_secret"

func newTestStripe(t *testing.T) *StripeProvider {
	t.Helper()
	p, err := NewStripeProvider("sk_test_x", testWebhookSecret, "https://mathiz.example.com")
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return p
}

func stripeSig(payload, secret string, at time.Time) string {
	ts := fmt.Sprintf("%d", at.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + payload))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func parseSigned(t *testing.T, p *StripeProvider, payload string) ([]Event, error) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/billing/webhook", strings.NewReader(payload))
	req.Header.Set("Stripe-Signature", stripeSig(payload, testWebhookSecret, time.Now()))
	return p.ParseWebhook(req)
}

func stripeEnvelope(id, typ, obj string) string {
	return fmt.Sprintf(`{"id":%q,"type":%q,"api_version":"2024-06-20","data":{"object":%s}}`, id, typ, obj)
}

func TestStripeWebhookInvoicePaid(t *testing.T) {
	p := newTestStripe(t)

	invoice := func(reason string) string {
		return `{"object":"invoice","billing_reason":"` + reason + `",
			"customer":{"id":"cus_1"},"subscription":{"id":"sub_1"},
			"subscription_details":{"metadata":{"family_space_id":"fam-1","plan_id":"explorer"}},
			"lines":{"data":[{"period":{"end":1893456000}}]}}`
	}

	events, err := parseSigned(t, p, stripeEnvelope("evt_create", "invoice.paid", invoice("subscription_create")))
	if err != nil || len(events) != 1 {
		t.Fatalf("subscription_create: %v (%d events)", err, len(events))
	}
	ev := events[0]
	if ev.Type != EventSubscriptionActivated || ev.EventID != "evt_create" ||
		ev.FamilySpaceID != "fam-1" || ev.PlanID != "explorer" ||
		ev.CustomerID != "cus_1" || ev.SubscriptionID != "sub_1" || ev.PeriodEnd.IsZero() {
		t.Errorf("activated event = %+v", ev)
	}

	events, err = parseSigned(t, p, stripeEnvelope("evt_cycle", "invoice.paid", invoice("subscription_cycle")))
	if err != nil || len(events) != 1 || events[0].Type != EventSubscriptionRenewed {
		t.Fatalf("subscription_cycle: %v %+v", err, events)
	}
}

func TestStripeWebhookCheckoutCompleted(t *testing.T) {
	p := newTestStripe(t)

	topup := `{"object":"checkout.session","mode":"payment",
		"customer":{"id":"cus_1"},
		"metadata":{"family_space_id":"fam-1","plan_id":"topup-100"}}`
	events, err := parseSigned(t, p, stripeEnvelope("evt_top", "checkout.session.completed", topup))
	if err != nil || len(events) != 1 {
		t.Fatalf("topup: %v (%d)", err, len(events))
	}
	if ev := events[0]; ev.Type != EventTopupPurchased || ev.PlanID != "topup-100" || ev.EventID != "evt_top" {
		t.Errorf("topup event = %+v", ev)
	}

	// Subscription checkouts emit nothing here — credits flow from
	// invoice.paid, or a new subscription would double-grant.
	sub := `{"object":"checkout.session","mode":"subscription",
		"metadata":{"family_space_id":"fam-1","plan_id":"explorer"}}`
	events, err = parseSigned(t, p, stripeEnvelope("evt_sub", "checkout.session.completed", sub))
	if err != nil || len(events) != 0 {
		t.Fatalf("subscription checkout: %v (%d events, want 0)", err, len(events))
	}
}

func TestStripeWebhookCanceledAndUnmapped(t *testing.T) {
	p := newTestStripe(t)

	sub := `{"object":"subscription","id":"sub_1","customer":{"id":"cus_1"},
		"metadata":{"family_space_id":"fam-1","plan_id":"explorer"}}`
	events, err := parseSigned(t, p, stripeEnvelope("evt_del", "customer.subscription.deleted", sub))
	if err != nil || len(events) != 1 || events[0].Type != EventSubscriptionCanceled {
		t.Fatalf("canceled: %v %+v", err, events)
	}

	// Unmodeled types are acknowledged with zero events.
	events, err = parseSigned(t, p, stripeEnvelope("evt_x", "customer.updated", `{"object":"customer"}`))
	if err != nil || len(events) != 0 {
		t.Fatalf("unmapped: %v (%d)", err, len(events))
	}
}

func TestStripeWebhookRejectsBadSignatureAndMissingMetadata(t *testing.T) {
	p := newTestStripe(t)
	payload := stripeEnvelope("evt_1", "invoice.paid", `{"object":"invoice"}`)

	// Tampered signature (wrong secret).
	req := httptest.NewRequest("POST", "/api/v1/billing/webhook", strings.NewReader(payload))
	req.Header.Set("Stripe-Signature", stripeSig(payload, "whsec_wrong", time.Now()))
	if _, err := p.ParseWebhook(req); err == nil {
		t.Fatal("tampered signature accepted")
	}

	// No signature at all.
	req = httptest.NewRequest("POST", "/api/v1/billing/webhook", strings.NewReader(payload))
	if _, err := p.ParseWebhook(req); err == nil {
		t.Fatal("unsigned payload accepted")
	}

	// Valid signature but no family/plan metadata → refused, not granted.
	if _, err := parseSigned(t, p, payload); err == nil {
		t.Fatal("event without family metadata accepted")
	}
}
