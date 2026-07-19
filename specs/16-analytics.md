# 16 — Product Analytics (PostHog)

Status: implemented.

## Goals

Answer the product questions we actually have — does the landing page
convert, do parents finish onboarding (family → child → join code → kid
playing), do kids keep playing, do quests get used, does anyone hit the
paywall — with the least data we can get away with. Analytics is an
operator opt-in: a self-hosted Mathiz with no PostHog key configured ships
**zero** analytics code paths.

## Privacy stance (invariant — do not weaken)

- **Parents are identified.** They consented by creating an account.
  Distinct id = account UID (never the email); email / display name / role
  ride along as person properties. Family group (`family` group type, key =
  family space UID) carries the family name — set from parent surfaces only.
- **Children are NEVER identified.** No child names, no child profile UIDs,
  no PINs, no per-child `identify()` — not as distinct ids, not as event
  properties, not as group properties. Child usage is visible only as
  anonymous events attached to the `family` group (key only, no
  properties). Child surfaces run PostHog with `persistence: 'memory'`, so
  no analytics cookie or localStorage entry is ever written on a kid's
  device and the anonymous distinct id is throwaway per page load.
- If a feature seems to need per-child analytics, the answer is **no** —
  stop and raise it with the repo owner.

## Config flow

```
MATHIZ_POSTHOG_API_KEY  ──►  server.Config.PostHogAPIKey ──┬─► GET /api/v1/config
MATHIZ_POSTHOG_HOST     ──►  server.Config.PostHogHost ────┤     {posthogKey, posthogHost: "/relay"}
  (default https://us.i.posthog.com when key set)          └─► /relay/* reverse proxy (server-side)
                                                                         │
                                              web/src/analytics.ts ◄─────┘
                                              (dynamic import('posthog-js'))
```

- Key unset (default): `/api/v1/config` **omits** both fields, the `/relay`
  route is not mounted, and the SPA never downloads the posthog-js chunk.
  Tests and .env-less self-hosters see zero analytics.
- Key set: the SPA lazy-loads posthog-js on first `initAnalytics` /
  `ensureAnalyticsBooted` call. posthog-js stays in its own Vite chunk via
  dynamic `import()`.

### Same-origin relay (`/relay`)

The browser never talks to a `*.posthog.com` domain — ad-blocker domain
lists silently drop those requests, and the upstream host is a server-side
deployment concern anyway. Instead:

- The server mounts `internal/saas/server/posthog_proxy.go` at `/relay/`
  (only when the key is configured): an `httputil.ReverseProxy` to
  `MATHIZ_POSTHOG_HOST` that strips the `/relay` prefix, rewrites the
  `Host` header to the upstream (PostHog cloud requires it), forwards query
  and body untouched, drops our origin's cookies, and uses short upstream
  timeouts (~10s response-header) so a PostHog outage can never
  back-pressure real API traffic. `/relay/static/*` is covered too —
  posthog-js fetches its remote-config bundle from `api_host`.
- `/api/v1/config` serves `posthogHost: "/relay"`; analytics.ts passes it
  through as `api_host` unchanged (posthog-js accepts a relative path) and
  sets `ui_host: 'https://us.posthog.com'` so the PostHog toolbar works.
- Client rule: **never point `api_host` at a third-party domain.**

## The chokepoint: `web/src/analytics.ts`

The only file allowed to import `posthog-js`, and the only file allowed to
contain event-name strings. Everything else calls typed helpers:

- `initAnalytics(cfg, surface)` / `ensureAnalyticsBooted(surface)` — no-op
  without a key; single lazy init promise; double-init no-ops (a later
  `'child'` boot flips persistence to memory).
- `identifyParent(accountId, {email, name, role})` — parent surfaces only.
- `identifyFamilyGroup(familyId, familyName)` — parent surfaces only (the
  only place the family name enters).
- `attachChildToFamily(familyId)` — child surfaces; group key ONLY, no
  properties. `familyId` comes from the extended `/api/v1/child/me` and
  `/api/v1/join/redeem` responses.
- `resetAnalytics()` — sign-out.
- `track.*` — the event helpers below. All fire-and-forget: never throw,
  silently drop before init or with analytics off.

## Surface configs

| Surface | Routes | PostHog options (all: `autocapture:false`, `capture_pageview:false`, `disable_session_recording:true`, `respect_dnt:true`, `api_host:'/relay'`, `ui_host:'https://us.posthog.com'`) | Identity |
|---|---|---|---|
| public | `/`, `/pricing` | `person_profiles:'identified_only'` | anonymous |
| parent | `/login`, `/dashboard/*` | `person_profiles:'identified_only'` | `identifyParent` + `identifyFamilyGroup`; manual `$pageview` per route |
| child | `/join`, `/play` | `person_profiles:'identified_only'`, **`persistence:'memory'`** | NONE — `attachChildToFamily` group key only; no pageviews |
| — | `/terminal` | nothing — no analytics at all (surface removed 2026-07-19 with termbridge) | — |

## Event taxonomy

Naming: `snake_case`, past tense, verb-last. Properties minimal and
non-identifying.

| Event | Properties | Fired where |
|---|---|---|
| `landing_cta_clicked` | `persona: 'parent'\|'kid'` | Landing hero doors (`Landing.tsx`) |
| `pricing_viewed` | — | Pricing mount (`Pricing.tsx`) |
| `signin_completed` | — | OTP verify / password sign-in success (`Login.tsx`) |
| `family_created` | — | Create-family success (`dashboard/Layout.tsx`) |
| `child_added` | `grade` | Add-child modal success (`dashboard/Kids.tsx`) |
| `join_code_created` | — | Mint join code (`dashboard/Family.tsx`) |
| `coparent_invited` | — | Invite parent success (`dashboard/Family.tsx`) |
| `coparent_accepted` | — | Accept-invite banner success (`dashboard/Layout.tsx`) |
| `activity_viewed` | — | Activity page mount (`dashboard/Activity.tsx`) |
| `quest_created` | — | Create-quest modal success (`dashboard/Quests.tsx`) |
| `quest_ai_generated` | `count` | AI generation success (`dashboard/QuestEditor.tsx`) |
| `quest_published` | — | Publish success (`dashboard/QuestEditor.tsx`) |
| `billing_viewed` | — | Billing mount, owners only (`dashboard/Billing.tsx`) |
| `checkout_started` | `plan` | Checkout URL obtained (`dashboard/Billing.tsx`) |
| `join_redeemed` | — | Join redeem success (`Join.tsx`, child) |
| `expedition_started` | `kind: 'skill'\|'quest'` | Expedition start success (`Play.tsx`, child) |
| `expedition_completed` | `questions`, `correct`, `kind` | Final answer summary (`Play.tsx`, child) |
| `quest_completed_by_child` | — | Quest-done celebration (`Play.tsx`, child) |
| `out_of_credits_shown` | — | "Ship needs to rest" screen (`Play.tsx`, child) |
| `$pageview` | `$current_url` | Parent surfaces on route change (`App.tsx`); never child surfaces |

Adding an event = a typed helper in `analytics.ts` + a row in this table,
same PR. See `.claude/skills/analytics/SKILL.md`.

## Phase 2 (not implemented)

Server-side billing events (subscription started / renewed / canceled,
top-ups) captured from webhook handlers via a Go PostHog client, keyed to
the family group — client-side `checkout_started` only sees intent, not
money. Revisit when billing is live in prod.
