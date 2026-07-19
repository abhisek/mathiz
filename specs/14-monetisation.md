# Monetisation — Credits, Plans, and the Billing Abstraction

## 1. Model

Mathiz resells tokens; pricing follows the token economy, but denominated in
a unit parents and kids understand: **credits, where 1 credit = 1 expedition**
(5 AI-generated questions, hints and the guide's micro-lesson included).

- **Family Space creation is free** and grants **starter credits**
  (default 30, expiring in 30 days) — kids play within minutes, no card.
- **Plans** grant monthly credits at decreasing unit cost (Explorer →
  Voyager → Armada). Plan credits refresh each billing period and do not
  accumulate beyond one period.
- **Top-ups** are one-time credit packs for usage beyond the plan.
  Purchased credits do not auto-expire (consumer-protection posture).
- **Overage is opt-in**: default behavior at zero balance is a hard stop
  with a parent-facing prompt — never a surprise bill.

### Buyer ≠ user

The parent buys; the child plays. Therefore:
- The **kid never sees the meter**. At zero balance the game says
  "The ship needs to rest! ⛵ Ask your grown-up" — no prices, no paywall
  aimed at a child.
- Balance, plans, and purchase flows live only in the parent dashboard.
- A started expedition is never interrupted mid-question by the meter
  (the debit happens at start).

### Default catalog (tunable in `internal/saas/billing/plans.go`)

| Plan | Price | Credits/month | Per-credit |
|---|---|---|---|
| Starter (free, once) | $0 | 30 (30-day expiry) | — |
| Explorer | $5/mo | 150 | 3.3¢ |
| Voyager | $10/mo | 400 | 2.5¢ |
| Armada | $20/mo | 1,000 | 2.0¢ |
| Top-up pack | $5 | 100 (no expiry) | 5¢ |

## 2. Credit ledger (source of truth)

Event-sourced entries on the **FamilySpace** (`credit_entries`):

- Grants (`starter|plan|topup`): positive `amount`, a `remaining` counter,
  optional `expires_at`.
- Debits (`debit`): negative `amount`, consumed FIFO from unexpired grants
  ordered by soonest expiry (use-it-or-lose-it credits burn first).
- Every entry has a unique `source` (e.g. `starter:<spaceID>`,
  `sub:<providerEventID>`, `expedition:<sessionID>`) — **idempotency is
  structural**: replayed webhooks and retried requests cannot double-grant
  or double-charge.

`Balance = Σ remaining of unexpired grants`. Debits run in a transaction;
expired remainders are simply ignored by the balance query (lazy expiry —
no background job).

## 3. Enforcement points

Exactly the two chokepoints that start LLM-consuming sessions:

- `game.Manager.Start` — charges **1 credit** per expedition (source =
  session ID). Mastery ending an expedition early still costs 1 (generous).
- `termbridge` session start — charges **1 credit per device per UTC
  hour** (source `terminal:<deviceUID>:<yyyymmddhh>`), so reconnects and
  page reloads within the hour are idempotent no-ops instead of fresh
  debits. Open-ended terminal time is bounded at 1 credit/hour; revisit
  toward per-5-questions metering if terminal usage becomes material.

Both accept a nil `Charge` hook → free. Local CLI, self-hosters, and tests
are unaffected; only `mathiz serve` wires the hook (and only when billing
is enabled). Insufficient credits → HTTP **402** with `out_of_credits`.

## 4. Billing provider abstraction (deliberately thin)

```go
type Provider interface {
    Name() string
    // CreateCheckout returns a URL to send the parent to (plan or top-up).
    CreateCheckout(ctx, CheckoutParams) (url string, err error)
    // PortalURL returns the provider's self-service portal for a customer.
    PortalURL(ctx, customerID string) (string, error)
    // ParseWebhook verifies the request signature and normalizes it.
    ParseWebhook(r *http.Request) ([]Event, error)
}
```

Normalized events: `subscription_activated`, `subscription_renewed`,
`subscription_canceled`, `topup_purchased`. That's the entire surface —
**entitlements live in our ledger + `billing_states`, never in the
provider**. Plan catalog lives in our code; provider price IDs come from
env (`MATHIZ_BILLING_PRICE_<PLAN>`). Anything beyond this (proration, tax,
invoices, seat management) is explicitly the provider's problem and is NOT
abstracted.

Implementations:
- `fake` (shipped): checkout "succeeds" immediately via a local completion
  endpoint — the entire purchase→grant→play loop is clickable in dev with
  zero external services. Also the test double.
- `stripe` (shipped, `internal/saas/billing/stripe.go`): checkout sessions
  (`mode=subscription`/`payment`, `allow_promotion_codes` on), hosted
  Customer Portal, signature-verified webhook. Subscription credits flow
  EXCLUSIVELY from `invoice.paid` (`billing_reason=subscription_create` →
  activated, else renewed); `checkout.session.completed` maps only top-ups
  — mapping activation there too would double-grant new subscriptions,
  since Stripe delivers both events on signup. Config:
  `MATHIZ_STRIPE_SECRET_KEY`, `MATHIZ_STRIPE_WEBHOOK_SECRET`,
  `MATHIZ_PUBLIC_BASE_URL`, `MATHIZ_BILLING_PRICE_*`.
- `paddle` (possible later): same three methods; a deployment decision,
  not an architectural one.

### Provider decision: Stripe — subscriptions + one-time payments only

Decided 2026-07. How the catalog maps onto Stripe:

| Ours | Stripe |
|---|---|
| Plan subscribe | Checkout Session `mode=subscription`, recurring licensed price (`MATHIZ_BILLING_PRICE_{EXPLORER,VOYAGER,ARMADA}`) |
| Monthly renewal → expire + regrant plan credits | `invoice.paid` webhook → `subscription_renewed` |
| Cancel / plan change / card update | hosted Customer Portal (our `PortalURL`) |
| Top-up pack | Checkout Session `mode=payment`, one-time price (`MATHIZ_BILLING_PRICE_TOPUP_100`) |
| Activation / top-up delivery | `checkout.session.completed` → `subscription_activated` or `topup_purchased` |
| Cancellation | `customer.subscription.deleted` → `subscription_canceled` |
| Idempotency | Stripe event ID → `Event.EventID` → the ledger `source` column |

**Explicitly rejected — do not add later without revisiting this spec:**
- **Stripe metered/usage-based billing.** It bills in arrears; buyer ≠ user
  means a kid's usage could produce a surprise parent bill, and we gate at
  zero balance so there is never unbilled overage to meter anyway.
- **Stripe Billing credit grants.** Would make Stripe a second entitlement
  ledger, breaking the invariant above and making any future Paddle adapter
  asymmetric. Stripe is a money pipe, nothing more.

The "usage-based beyond the plan" leg of the pricing is realized as
**prepaid top-up packs**. Future, opt-in **auto top-up** stays within this
model: save the card at first checkout (`setup_future_usage: 'off_session'`),
then charge an off-session PaymentIntent when *our* ledger balance crosses a
parent-configured threshold — the trigger lives in `credits`, not Stripe,
and spending stays prepaid and capped. Not built yet; requires a
parent-facing toggle before shipping.

`billing_states` (one row per family space): provider, customer ID,
subscription ID, plan, status, current period end — updated only by
webhook events, read by the dashboard.

## 5. API

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET  /api/v1/pricing` | public | plan catalog + `billingEnabled` + starter credits (served even with billing off) |
| `GET  /api/v1/family/{id}/billing` | parent | balance, plan, status, period end, catalog |
| `POST /api/v1/family/{id}/billing/checkout {planId}` | parent | checkout URL (plan or top-up) |
| `POST /api/v1/family/{id}/billing/portal` | parent | provider portal URL |
| `POST /api/v1/billing/webhook` | provider signature | normalized events → ledger + state |
| `GET  /api/v1/billing/fake/complete` | dev only (fake provider) | simulates provider success + redirect |

### Public pricing page & beta state

The plan catalog is served read-only via the public `GET /api/v1/pricing`
regardless of provider configuration — the route is registered
unconditionally, unlike the parent billing routes, which only exist when a
provider is wired. The response carries a `billingEnabled` flag
(`true` iff a billing provider is configured), the starter-credit count, and
the catalog (`ProviderPriceID` never serializes).

`billingEnabled` drives all beta messaging in the SPA: `/pricing` shows a
"public beta — everything is free right now" banner over the indicative plan
cards, and the dashboard billing page (whose API 404s with billing off)
shows a friendly beta card linking to `/pricing`. Flipping the provider on
in production flips all of that messaging with no frontend change. As
everywhere else, none of this ever reaches a child surface.

## 6. Testing

- Ledger: grant/debit/FIFO-expiry ordering, idempotent sources, insufficient
  balance, expiry exclusion.
- Gating: expedition start without credits → 402 + no expedition; with
  credits → exactly one debit per session ID (idempotent on retry).
- Webhook flow with the fake provider: checkout → events → balance grows,
  plan state updates; replayed events don't double-grant.
- Starter grant: created exactly once per family space.
