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
- `termbridge` session start — charges **1 credit** per terminal session
  (v1 simplification; terminal sessions are longer than expeditions, revisit
  toward per-5-questions metering if terminal usage becomes material).

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
- `stripe` / `paddle` (planned): adapter files implementing the same three
  methods. Choosing between them is a deployment decision
  (`MATHIZ_BILLING_PROVIDER`), not an architectural one.

`billing_states` (one row per family space): provider, customer ID,
subscription ID, plan, status, current period end — updated only by
webhook events, read by the dashboard.

## 5. API

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET  /api/v1/family/{id}/billing` | parent | balance, plan, status, period end, catalog |
| `POST /api/v1/family/{id}/billing/checkout {planId}` | parent | checkout URL (plan or top-up) |
| `POST /api/v1/family/{id}/billing/portal` | parent | provider portal URL |
| `POST /api/v1/billing/webhook` | provider signature | normalized events → ledger + state |
| `GET  /api/v1/billing/fake/complete` | dev only (fake provider) | simulates provider success + redirect |

## 6. Testing

- Ledger: grant/debit/FIFO-expiry ordering, idempotent sources, insufficient
  balance, expiry exclusion.
- Gating: expedition start without credits → 402 + no expedition; with
  credits → exactly one debit per session ID (idempotent on retry).
- Webhook flow with the fake provider: checkout → events → balance grows,
  plan state updates; replayed events don't double-grant.
- Starter grant: created exactly once per family space.
