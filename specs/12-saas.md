# SaaS Layer — Family Spaces, Supabase Auth, Hosted Learning

## 1. Overview

Mathiz today is a single-learner terminal app: one SQLite file, one child, one machine.
This spec adds a **SaaS layer** so that:

- **Parents sign up** (Supabase handles authentication — email/password or magic link).
- A parent creates a **Family Space** and adds **child profiles** (name, grade, optional PIN).
- Children get access via a **join code** — no email, no password, no Supabase account.
  The child redeems the code once on their device, picks their profile, enters their PIN,
  and receives a long-lived revocable **device token**.
- Children learn in the **browser**: the existing Bubble Tea TUI runs server-side and is
  streamed to an xterm.js terminal over a WebSocket. Zero TUI rewrite.
- Parents see per-child progress (sessions, mastery, gems) in a web dashboard.

The local CLI experience is unchanged: `mathiz` still runs fully offline against SQLite.

### Relationship to `ssh-server.md`

The earlier SSH/Wish spec solved "TUI over the network" with an SSH hop and per-profile
SQLite files. This spec supersedes it for the SaaS product:

| Concern | ssh-server.md | This spec |
|---|---|---|
| Transport | Browser → WS proxy → SSH → Wish | Browser → WebSocket → in-process `tea.Program` |
| Learning data | Per-profile SQLite files on disk | Single Postgres DB, owner-scoped rows |
| Identity | Any JWT IdP, profile = SSH username | Supabase (parents) + device tokens (children) |
| Authorization | None (filesystem isolation) | Explicit authz layer (`internal/saas/authz`) |

The in-process bridge removes two moving parts (SSH server, WS↔SSH proxy) and a whole
class of credential plumbing (JWT in the SSH password field).

## 2. Architecture

```
┌───────────────────────────── Browser ─────────────────────────────┐
│  Parent dashboard (React SPA)          Kid terminal (xterm.js)    │
│  supabase-js login → JWT               device token (localStorage)│
└───────────────┬───────────────────────────────┬───────────────────┘
                │ HTTPS /api/v1/* (Bearer JWT)   │ WSS /api/v1/terminal
┌───────────────▼───────────────────────────────▼───────────────────┐
│                        mathiz serve (one binary)                  │
│                                                                   │
│  auth: Supabase JWT verify (HS256 secret / JWKS) → Account        │
│        device token verify → ChildProfile                         │
│  authz: internal/saas/authz — all permission decisions            │
│  api:  family spaces, children, invites, devices, stats           │
│  termbridge: WS ⇄ tea.Program (input bytes / output frames /      │
│              resize control messages)                             │
│  webui: embedded React SPA (embed.FS)                             │
│                                                                   │
│  store: one shared *store.Store (Postgres)                        │
│         EventRepoFor(childProfileID) / SnapshotRepoFor(...)       │
│  llm:   provider from env, per-session logging decorator          │
└───────────────────────────────┬───────────────────────────────────┘
                                │
                     PostgreSQL (dev: local / prod: Supabase or any hosted PG)
```

## 3. Data Model

### 3.1 Data plane (existing event/snapshot tables)

Every event table (via `EventMixin`) and `Snapshot` gains:

```
owner_id string  — default "" — immutable — indexed (owner_id, sequence)
```

- `owner_id` is the **child profile ID** in SaaS mode, `""` in local single-user mode.
- Existing local databases auto-migrate: the column is added with default `""` and all
  queries filter on `owner_id = ""` — identical behavior, no data rewrite.
- `Store.EventRepo()` / `Store.SnapshotRepo()` keep the local (owner `""`) behavior.
  `Store.EventRepoFor(owner)` / `Store.SnapshotRepoFor(owner)` return tenant-scoped
  repos. Every append stamps the owner; every query filters by it. Domain packages
  (session, mastery, spacedrep, gems, lessons) are untouched — they already work
  against the repo interfaces.
- The global sequence counter stays global (not per-owner): per-owner ordering is a
  subset of global ordering, and a single counter is simpler and contention-free at
  this scale.

### 3.2 Control plane (new ent schemas)

Follows the codebase convention: no ent edges, plain UUID string keys + indexes.

| Entity | Fields |
|---|---|
| `Account` | `id` (uuid), `supabase_user_id` (unique), `email`, `display_name`, `created_at` |
| `FamilySpace` | `id`, `owner_account_id` (indexed), `name`, `created_at` |
| `ChildProfile` | `id`, `family_space_id` (indexed), `name`, `grade` (2–5), `pin_hash` (optional, bcrypt), `created_at`, `archived` (bool) |
| `Invite` | `id`, `family_space_id` (indexed), `code` (unique, human-friendly e.g. `TIGER-4207`), `expires_at`, `revoked`, `created_at` |
| `DeviceToken` | `id`, `child_profile_id` (indexed), `family_space_id`, `token_hash` (SHA-256, unique), `device_label`, `created_at`, `last_used_at`, `revoked` |

Notes:
- Join codes are **family-space scoped** (not per child): the child enters the code,
  then picks *their* profile from the family list and confirms with their PIN. One code
  handed to three siblings works for all of them.
- Join codes are stored plaintext (parents need to re-read them from the dashboard) but
  they are low-privilege: redeeming one only ever yields a *child* device token, and only
  with the profile's PIN if one is set. They expire (default 7 days) and are revocable.
- Device tokens are real bearer credentials → only the SHA-256 hash is stored. The
  plaintext (`mzd_` prefix + 32 random bytes, base64url) is shown/stored client-side once.
- PINs are 4–6 digits, bcrypt-hashed. They gate profile selection at redeem time so
  siblings can't impersonate each other; they are not meant to resist adults.

## 4. Authentication

### 4.1 Parents — Supabase

The SPA authenticates with `supabase-js` and sends `Authorization: Bearer <access_token>`
on every API call. The server verifies locally (no network call per request):

- **HS256** with `MATHIZ_SUPABASE_JWT_SECRET` (classic Supabase JWT secret), and/or
- **RS256/ES256** via JWKS at `{MATHIZ_SUPABASE_URL}/auth/v1/.well-known/jwks.json`
  (new Supabase asymmetric signing keys). Keys cached, refreshed on unknown `kid`.

Claims validated: `exp`/`nbf`/`iat` (30s skew), `iss == {url}/auth/v1` (when URL set),
`aud == authenticated` (configurable). `sub` = Supabase user id.

**Account provisioning is implicit**: the first authenticated API call upserts an
`Account` for the `sub` (email + name from claims). There is no separate signup endpoint;
Supabase *is* signup.

### 4.2 Children — device tokens

- `POST /api/v1/join/preview {code}` → family space name + child profiles (id, name,
  grade, `has_pin`). No auth.
- `POST /api/v1/join/redeem {code, child_profile_id, pin?, device_label}` →
  `{token}` (plaintext, once). Validates code liveness + PIN.
- Subsequent child API calls send `Authorization: Bearer mzd_...`.
- The terminal WebSocket authenticates with the same token as its **first message**
  (never in the URL — query strings end up in access logs).

## 5. Authorization — `internal/saas/authz`

All permission decisions live in one package. Handlers never inline policy.

```go
type Principal struct {
    Kind           PrincipalKind // KindParent | KindChild
    AccountID      string        // parents
    ChildProfileID string        // children
    FamilySpaceID  string        // children (resolved from device token)
}
```

Policy (v1):
- A parent may manage (read/write) a family space iff they are its `owner_account_id`.
- A parent may manage children, invites, devices, and read stats of spaces they own.
- A child may access exactly one thing: **their own** learning session and profile info.
- Children can never read other profiles, stats, or control-plane objects.

Every check returns `authz.ErrDenied` which the API maps to 403 (404 for existence
probes on objects the caller can't see).

## 6. Terminal Bridge — `internal/saas/termbridge`

> **Removed (2026-07-19).** The browser terminal proved unusable for
> children on the web and was unneeded attack surface; the termbridge
> package, the `/terminal` page, and the `WS /api/v1/terminal` endpoint
> were deleted. The treasure-map game (spec 13) is the only hosted kid
> surface. The local CLI TUI is unaffected. The section below — and every
> other terminal/xterm/WebSocket mention elsewhere in this spec (the
> architecture diagram, auth flow, endpoint table, security notes) — is
> historical design record, not current behavior.

The existing TUI runs unmodified server-side, one `tea.Program` per WebSocket:

- **Client → server**: binary frames = raw terminal input bytes (xterm.js `onData`).
  Text frames = JSON control: `{"type":"auth","token":...}`, `{"type":"resize","cols":C,"rows":R}`.
- **Server → client**: binary frames = ANSI output from the Bubble Tea renderer.
- Program wiring: `tea.WithInput(wsReader)`, `tea.WithOutput(wsWriter)`,
  `tea.WithEnvironment("TERM=xterm-256color", "COLORTERM=truecolor")`, initial + live
  `tea.WindowSizeMsg` from resize messages.
- Per connection: owner-scoped repos from the shared Postgres store + a per-session LLM
  logging decorator (LLM usage events land in the child's event stream, so `mathiz llm`
  style auditing works per child).
- Lifecycle: WS close → program kill → snapshot already saved by session flow;
  program quit → WS close. Idle timeout (default 30 min) and a concurrent session cap
  (default 100) protect the host.

## 7. HTTP API (v1)

All under `/api/v1`. Parent endpoints require Supabase JWT; child endpoints device token.

| Method & Path | Auth | Purpose |
|---|---|---|
| `GET  /config` | none | Public boot config for SPA (Supabase URL + anon key) |
| `GET  /curriculum` | none | Static skill-graph curriculum: islands (strands, canonical order) → skills (`id`, `name`, `grade`, `prereqs`), cacheable (`Cache-Control: public, max-age=3600`) |
| `GET  /me` | parent | Account (auto-provisioned) + owned family space |
| `POST /family` | parent | Create family space `{name}` (one per account, v1) |
| `PATCH /family/{id}` | parent | Rename |
| `POST /family/{id}/children` | parent | Add child `{name, grade, pin?}` |
| `GET  /family/{id}/children` | parent | List children (+ summary stats) |
| `PATCH /children/{id}` | parent | Update name/grade/PIN, archive |
| `GET  /children/{id}/stats` | parent | Mastery overview, recent sessions, gems |
| `POST /family/{id}/invites` | parent | Mint join code (default 7-day expiry) |
| `GET  /family/{id}/invites` | parent | List active codes |
| `DELETE /invites/{id}` | parent | Revoke code |
| `GET  /children/{id}/devices` | parent | List child devices |
| `DELETE /devices/{id}` | parent | Revoke device token |
| `POST /join/preview` | none | Code → space name + profiles |
| `POST /join/redeem` | none | Code + profile + PIN → device token |
| `GET  /child/me` | child | Own profile (name, grade, space name) |
| `WS   /terminal` | child (first msg) | Learning session stream |

## 8. Configuration

| Env var | Required (serve) | Purpose |
|---|---|---|
| `MATHIZ_DATABASE_URL` | yes | Postgres DSN (`postgres://...`). Dev: local PG; prod: Supabase/hosted PG |
| `MATHIZ_SUPABASE_URL` | yes | Project URL — JWKS + issuer validation + SPA boot config |
| `MATHIZ_SUPABASE_ANON_KEY` | yes | Served to the SPA via `/api/v1/config` (public by design) |
| `MATHIZ_SUPABASE_JWT_SECRET` | no | Enables HS256 verification (legacy Supabase keys) |
| `MATHIZ_SERVER_ADDR` | no (`:8080`) | Listen address |
| `MATHIZ_LLM_PROVIDER`, `MATHIZ_*_API_KEY` | yes | Existing LLM config, server-side only |

`mathiz serve` fails fast if the database is unreachable or no Supabase verification
method is configured.

## 9. Web App (`web/`)

Vite + React + TypeScript, embedded into the binary via `embed.FS` (single deployable).
`npm run build` output lands in `internal/saas/webui/dist` (gitignored; `make web`).
The SPA fetches `/api/v1/config` at boot — no per-environment rebuilds.

Routes:
- `/` — sign in / sign up (supabase-js)
- `/dashboard` — family space, children cards, add child, join codes, device revocation,
  per-child progress
- `/join` — kid flow: code → profile picker → PIN → token saved locally
- `/play` — full-screen xterm.js terminal

## 10. Security Considerations

- Supabase JWTs verified cryptographically on every request; `alg` allow-list
  (HS256 only when the shared secret is configured; RS/ES from JWKS); `none` rejected.
- Device tokens: 256-bit random, stored hashed, revocable, `last_used_at` tracked.
- Join codes: expiring, revocable, family-scoped, PIN-gated profile selection, and
  redemption is rate-limited per IP (simple token bucket) to slow brute force.
- Authorization is deny-by-default and centralized; handlers cannot "forget" a check
  because repos for control-plane objects are only reachable through authz-wrapped
  service methods.
- The LLM API key never leaves the server. Children's terminal sessions can only reach
  their own owner-scoped repos.
- CORS: same-origin by default (SPA is embedded); `MATHIZ_CORS_ORIGINS` opt-in for
  split deployments.

## 11. Testing Strategy

- Store: owner-isolation tests (two owners, no cross-reads) on SQLite; the same suite
  runs against Postgres when `MATHIZ_TEST_DATABASE_URL` is set.
- Auth: HS256 tokens signed with a test secret; ES256 via generated key + `httptest`
  JWKS endpoint; expiry/issuer/audience/alg-confusion cases.
- Family service + authz: table-driven over an in-memory SQLite store.
- API: `httptest` end-to-end — parent onboarding → add child → mint code → preview →
  redeem → child stats visible to parent; cross-tenant denial cases.
- Termbridge: unit tests for the framing/resize protocol with a fake program transport.

## Co-parents (membership model)

A Family Space supports multiple parent accounts via `family_member` rows
(space, account [unique — one family per account in v1], role).

- **Roles (exactly two):** `owner` — everything, plus billing and managing
  parents (the Stripe customer is the payer's identity; purchase actions
  and parent removal stay with them). `parent` — everything else: progress,
  quests, join codes, child management. Kid surfaces are role-agnostic.
- **Invites without email infra:** the owner records a co-parent's email
  (`parent_invite` row, pending, revocable). The invitee signs in normally
  (OTP); when `/me` finds a pending invite matching the account email, the
  dashboard shows an accept banner; accepting creates the membership.
  A typo'd email simply never matches. No SMTP anywhere.
- **Migration is lazy:** on `/me`, an owner without a member row gets one.
- **Authz:** membership checks replace owner checks at the single
  `internal/saas/authz` chokepoint; `CanManageBilling` / `CanManageParents`
  are owner-only. Cross-family denials remain 404.
- Quests record `created_by` (account UID) for the future activity log.

## Activity timeline (read model)

`internal/saas/activity.Reader` is a pure read model over the child's
owner-scoped event streams, serving the parent dashboard's "what has my kid
been doing" view. It writes nothing and reads only via
`Store.EventRepoFor(childUID)`.

- **Granularity:** the timeline shows expedition rows (session "end" events,
  hydrated with the plan from the matching "start" event, gem count, and
  skill names), mastery transitions worth a parent's attention (to
  `mastered` or `rusty` only — learning-state churn is filtered out), and
  micro-lessons. Individual answers are NOT timeline items; they live behind
  an expandable per-session detail
  (`GET /api/v1/children/{id}/activity/sessions/{sessionId}` → answers in
  question order + hint count) so the feed stays scannable. Each expedition
  item also carries an optional `category` — the first plan slot's category
  (`frontier` | `review` | `booster`), i.e. why the engine scheduled the
  expedition — omitted when the stored plan is empty.
- **Pagination:** one `before` cursor over the global event sequence, applied
  to every underlying stream; each stream contributes up to `limit` items,
  the merge keeps the newest `limit` overall. `nextBefore` is the lowest
  sequence in the returned page and is omitted when the page came back short
  (no more data).
- **Quest attribution:** untagged quest sessions plan the synthetic
  `quest:<uid>` skill ID; the reader resolves it through the quests service
  (name, emoji, authoring parent's display name) into a `quest` object on the
  expedition item. Lookups degrade — deleted quest or missing member name
  yields a bare ID / empty name, never a failed timeline. **Limitation:**
  skill-tagged quest sessions plan the real skill ID and are therefore
  indistinguishable from normal digs; they appear without quest attribution.
- **Authz:** `CanManageChild` — any family member (owner or co-parent) may
  read; strangers get 404. Endpoints:
  `GET /api/v1/children/{id}/activity` (`before`, `limit` ≤ 50 default 20,
  `kinds=expedition,mastery,lesson`, `from`/`to` RFC3339) and the session
  detail above.
