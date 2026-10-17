# Pluggable Parent Auth — Supabase out of the core

## 1. Why

Parent auth is Supabase-shaped all the way down: `auth.SupabaseVerifier`,
`Account.supabase_user_id`, `MATHIZ_SUPABASE_*`, `/api/v1/config` returning
`supabaseUrl`/`supabaseAnonKey`, and supabase-js in the SPA. Production data
also lives on a Supabase Postgres cluster. We want:

- **Core storage and logic with no Supabase in it.** Data lives on our own
  PostgreSQL; nothing outside an auth adapter names a vendor.
- **Credentials stay with a hosted provider.** We never store passwords,
  send OTP/magic-link email, or run sign-in security ourselves. The provider
  is an identity pipe, the same way billing is a money pipe (specs/14).
- **Supabase is one adapter among several.** Clerk is the first new one;
  Auth0 / WorkOS / Firebase should be config presets, not new code paths.

Non-goals: building first-party auth (password storage, SMTP), OAuth/social
login, MFA, account deletion, and child auth. Kids keep join codes + device
tokens (specs/12 §4.2) untouched; **no child ever has a provider account**
(also required by Clerk's terms on under-16 data).

## 2. Decisions

- **One auth provider per deployment.** No account linking across
  providers, no "same email = same parent" merging (that is an
  account-takeover vector). Switching providers is an explicit, offline
  remap (§6), never implicit.
- **One generic JWT verifier, presets per provider.** Every supported
  provider issues asymmetric-signed JWTs with a JWKS. Vendor knowledge is
  limited to a preset: issuer/JWKS URL shape, audience vs authorized-party
  check, claim names. No vendor SDK in the Go core (no `clerk-sdk-go`).
- **Bearer tokens stay.** No cookies → no CSRF surface and CORS stays as is.
  The SPA fetches a fresh token per request (§5).
- **Verified email is enforced by us**, not assumed from the provider.
  Co-parent invite acceptance matches on `Account.Email`
  (`family/members.go` AcceptInvite), so an unverified email is an invite
  takeover. Today nothing checks `email_verified`; this spec fixes that.

## 3. Backend

### 3.1 Verifier

```go
// internal/saas/auth
type Identity struct {
    Provider      string // preset name: "supabase" | "clerk" | "test"
    Subject       string // provider user id (JWT sub), opaque
    Email         string
    EmailVerified bool
    DisplayName   string
}

type Verifier interface {
    Verify(ctx context.Context, token string) (*Identity, error)
    Provider() string
}
```

`server.Deps.Verifier` takes the interface (today it is the concrete
`*auth.SupabaseVerifier`). `withParent` is otherwise unchanged:
verify → `EnsureAccount(identity)` → `authz.Principal{AccountID: acct.UID}`.
Every failure, including an unverified email or a wrong-provider token,
is a generic 401 (`ErrUnauthenticated`).

`auth.JWTVerifier` is the single implementation, configured by a preset:

| Field | Meaning |
|---|---|
| `Issuer` | exact `iss` match (required, except `test`) |
| `JWKSURL` | key set; fetched lazily, cached, refetched on unknown `kid` (existing keyfunc logic) |
| `HS256Secret` | symmetric key; only `test` and legacy Supabase |
| `Audience` | if set, `aud` must contain it |
| `AuthorizedParties` | if set, `azp` must be one of them |
| `Algs` | allow-list; never `none`, HS* only when a secret is set |
| `Claims` | `email`, `email_verified`, `display_name` claim paths |
| `Leeway` | 30s (unchanged) |

At least one of `Audience` / `AuthorizedParties` must be set, so a token
minted for another app at the same issuer is rejected.

### 3.2 Presets

| | `supabase` | `clerk` | `test` |
|---|---|---|---|
| Issuer | `{URL}/auth/v1` | Clerk Frontend API URL | `mathiz-test` |
| Keys | JWKS `{URL}/auth/v1/.well-known/jwks.json`, + optional legacy HS256 secret | JWKS `{issuer}/.well-known/jwks.json` (RS256) | HS256 secret |
| Audience check | `aud = authenticated` | none (Clerk sets no `aud`) | `aud = authenticated` |
| Authorized parties | — | SPA origin(s), default `MATHIZ_PUBLIC_BASE_URL` | — |
| Email | `email` | custom session claim `email` | `email` |
| Email verified | `email_confirmed_at`/`user_metadata.email_verified` (see §8) | custom session claim `email_verified` (see §8) | `email_verified`, default true |
| Display name | `user_metadata.full_name\|name\|display_name` | custom session claim `name` | `user_metadata.full_name` |

The Clerk instance must have a custom session-claims template that adds
`email`, `email_verified` and `name`. Without it every token fails
`email_verified` and the startup self-check (§3.4) logs it. Clerk session
tokens live 60s, so `Leeway` covers clock skew only. Long requests are fine
because verification happens at request start.

`test` refuses to start unless `MATHIZ_AUTH_ALLOW_TEST=1`. It exists for
unit tests, docker-compose and the `saas-e2e` skill, and must never be
reachable in production by default.

### 3.3 Data model

`Account` gains provider-neutral identity columns and loses the Supabase
one. ent auto-migrate (`store.Open` → `Schema.Create`) adds columns but never
renames or drops them, so this is staged:

1. **Add** `auth_provider` (string, Optional, Nillable) and
   `auth_subject` (string, Optional, Nillable), with a unique index on
   (`auth_provider`, `auth_subject`). They are nullable so the index can be
   created before backfill: NULLs don't collide in either PostgreSQL or
   SQLite.
2. **Relax** `supabase_user_id` to Optional + Nillable and stop writing it.
3. **Backfill** in `store.Open` after `Schema.Create`, idempotently:
   `UPDATE accounts SET auth_provider='supabase', auth_subject=supabase_user_id
   WHERE auth_subject IS NULL AND supabase_user_id IS NOT NULL`. This is
   portable SQL with no placeholders. Accounts aren't owner-scoped, so the
   owner guard doesn't apply.
4. A later release removes the `supabase_user_id` field from the schema and
   drops the column with a one-line manual SQL step in the release notes.

`EnsureAccount(ctx, Identity)` looks up by (`Provider`, `Subject`) and
otherwise keeps today's behaviour: create with a fresh UUID, refresh
email/name, and tolerate the unique-constraint race. It is never keyed on
email. Everything downstream already uses `Account.UID`, so nothing else
changes.

### 3.4 Startup checks (fail fast)

- The preset resolves and the config validates (replaces the current
  `config.go` "Supabase URL or JWT secret" rule).
- **Provider pin:** if any account has `auth_provider` set to something
  other than the configured preset, refuse to start:
  `auth: N accounts belong to provider "supabase", configured "clerk" — run
  "mathiz auth remap" (specs/17 §6)`. This stops a config typo from
  silently orphaning every family.
- Best-effort JWKS fetch at boot, logged via slog on failure but not fatal
  (the provider may be briefly unreachable, and verification retries
  lazily).

### 3.5 Config

| Var | Notes |
|---|---|
| `MATHIZ_AUTH_PROVIDER` | `supabase` \| `clerk` \| `test` |
| `MATHIZ_AUTH_ISSUER` | Clerk Frontend API URL; Supabase derives it from the URL |
| `MATHIZ_AUTH_AUDIENCE`, `MATHIZ_AUTH_AUTHORIZED_PARTIES` | override preset defaults (comma-separated) |
| `MATHIZ_AUTH_PUBLIC_KEY` | public, sent to the SPA: Supabase anon key / Clerk publishable key |
| `MATHIZ_AUTH_HS256_SECRET` | `test`, or legacy Supabase |
| `MATHIZ_AUTH_ALLOW_TEST` | required for `test` |

**Back-compat:** when `MATHIZ_AUTH_PROVIDER` is unset and `MATHIZ_SUPABASE_*`
are set, the server behaves as `supabase` and logs a deprecation warning.
The old names are removed one release after this ships.

### 3.6 Boot config

`GET /api/v1/config` returns:

```json
{ "auth": { "provider": "clerk", "publicKey": "pk_live_…", "issuer": "https://clerk.example.com" },
  "posthogKey": "…", "posthogHost": "…" }
```

`supabaseUrl` / `supabaseAnonKey` are kept alongside it for one release, so
a cached old SPA keeps working during deploy, then removed.

## 4. What does not change

Authz (`internal/saas/authz`, Principal-only), 404-not-403 on cross-tenant
denial, tenant isolation of learning data, child join/device-token auth
(parent tokens are still rejected on child routes and vice versa), the
billing/credits model, canonical log lines (UIDs only, never emails), and
the PostHog relay cookie strip.

## 5. Frontend

`web/src/auth/` holds a small adapter. Each provider is one file, loaded
lazily by `auth.provider` so a Supabase deployment never downloads Clerk and
vice versa:

```ts
interface AuthAdapter {
  init(cfg: AuthBootConfig): Promise<void>
  getToken(): Promise<string | null>  // fresh per call; the adapter caches/refreshes
  user(): { email: string } | null
  onChange(cb: () => void): () => void   // returns unsubscribe
  signOut(): Promise<void>
  SignIn: React.ComponentType          // rendered by /login
}
```

- **Supabase adapter:** today's `supa.ts` + `Login.tsx` moved behind the
  interface, with identical behaviour: email OTP first, magic link to `/login`,
  password fallback. `getToken()` returns `getSession()`'s access token.
- **Clerk adapter:** `@clerk/react` (Core 3), embedded `<SignIn/>` themed
  with `appearance` to match Mathiz. Email code + password configured in the
  Clerk dashboard. `getToken()` = Clerk's `getToken()`.
- **`api.ts`:** parent calls take a token *getter* instead of a string.
  `request()` awaits it per call. On a 401 it calls `getToken()` once more
  and retries once; a second 401 calls `signOut()`, which routes to `/login`.
  The token stops travelling through `DashboardContext`.
- **`App.tsx` / `Layout.tsx`:** use `onChange` / `user()` instead of the
  supabase-js `Session`. The guards are unchanged.
- **Kid surfaces never load an auth adapter.** `/join`, `/play` and the
  public pages stay provider-free.
- **Copy:** the processors named in the Legal privacy text follow the
  configured provider.

Update `.claude/skills/frontend/SKILL.md` with the adapter pattern in the
same PR.

## 6. Switching a deployment's provider

`mathiz auth remap --from supabase --to clerk --map map.csv [--dry-run]`

- `map.csv` is `old_subject,new_subject`. For Clerk it comes from the
  import result: each imported user has `external_id` = old Supabase id and
  `id` = new Clerk id.
- It runs in one transaction. It rewrites `auth_provider`/`auth_subject` only
  for accounts listed in the map. Any account not in the map makes the
  remap fail (`--allow-unmapped` leaves those accounts unmapped; their
  owners lose access until mapped).
- It is idempotent: re-running with the same map is a no-op.

Runbook (Supabase → Clerk):
1. Export Supabase `auth.users` with Clerk's `migration-tool`
   (`supabase` transformer). bcrypt hashes import as-is, so parents keep
   their passwords and OTP users simply sign in by code.
2. Import into the Clerk production instance with `external_id` set.
3. Stop the server, run `remap`, set `MATHIZ_AUTH_PROVIDER=clerk`, and start.
   Parents' Supabase sessions end, and they sign in once via Clerk.

## 7. Data plane: off the Supabase cluster

Moved to [18-self-hosted-postgres.md](18-self-hosted-postgres.md). It is
independent of this spec and can ship first. Supabase's own `auth` schema
stays with Supabase while it serves as the auth provider (§6).

## 8. Open questions

- **Clerk `email_verified` claim:** confirm the session-claim template can
  express primary-email verification status. If it can't, the Clerk preset
  falls back to a Backend API lookup by `sub` when an account is first
  created or its email changes, cached on the account row. That would be the
  only server→provider call, and its result must not be used for anything
  other than verification.
- **Supabase verified signal:** current Supabase access tokens carry no
  `email_verified` claim; check whether `user_metadata.email_verified`
  is reliably present, or whether we rely on "Supabase issues a session only
  after confirmation" (true for OTP; for password signup only when
  "Confirm email" is on). If it isn't reliable, the preset treats sessions
  as verified and the deployment runbook requires "Confirm email" on.
- **Clerk data residency:** US-only storage. Acceptable for current users?
- **Email change:** both providers let a parent change their email. We
  refresh `Account.Email` from the token on each request, so pending invites
  for the old email stop matching, which is the intended behaviour.

## 9. Testing

- `auth`: table-driven tests per preset with self-signed RS256 keys behind
  an httptest JWKS: wrong `iss`/`aud`/`azp`, expired, unverified email, alg
  `none`, HS256 without a secret, unknown `kid` refetch. Keep the existing
  Supabase HS256/ES256 cases.
- `server`: `parentToken` helper mints via the `test` preset. Add tests for
  the wrong-provider token → 401, and that a parent token is rejected on
  child routes (existing).
- `store`: backfill is idempotent on SQLite and PostgreSQL
  (`MATHIZ_TEST_DATABASE_URL`); two accounts with NULL subjects coexist.
- `family`: `EnsureAccount` is keyed by (provider, subject), never email.
- `cmd`: provider-pin refusal at startup; `remap` dry-run, unmapped-account
  failure, idempotent rerun.
- E2E (`saas-e2e`): switch to the `test` preset; `mint-jwt.mjs` keeps
  working. Add a `test` SPA adapter (enabled only with `ALLOW_TEST`) that
  takes a pasted token, so the browser parent dashboard becomes
  E2E-testable without Supabase or Clerk, which isn't possible today.

## 10. Rollout

1. Data move off the Supabase cluster (specs/18).
2. §3 + §5 with the Supabase preset only, shipped with no visible change.
   Back-compat env and boot-config fields stay in place.
3. Clerk preset + adapter, tried on a staging deployment with an imported
   copy of users.
4. Production cutover via §6 if we choose Clerk.
5. Remove the deprecated env names, boot fields and `supabase_user_id`
   column.

Docs to update alongside: specs/12-saas.md §4.1 (point here),
docs/personas.md (operator config), docs/saas.md, docs/development.md,
`.env.example`, docker-compose, CLAUDE.md env-var list and the `saas-e2e` skill.
