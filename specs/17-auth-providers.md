# Pluggable Parent Auth — Supabase out of the core

**Status:** implemented, with the `supabase` and `test` presets. No Clerk
code. The decisions in §2 are the owner's (2026-10-17) and override the
first draft of this spec.

## 1. Why

Parent auth was Supabase-shaped all the way down: `auth.SupabaseVerifier`,
`Account.supabase_user_id`, `MATHIZ_SUPABASE_*`, `/api/v1/config` returning
`supabaseUrl`/`supabaseAnonKey`, and supabase-js in the SPA. We want:

- **Core storage and logic with no vendor in it.** Data lives on our own
  PostgreSQL (specs/18). Nothing outside an auth preset or an SPA auth
  adapter names a vendor.
- **Credentials stay with a hosted provider.** We never store passwords,
  send OTP/magic-link email, or run sign-in security ourselves. The provider
  is an identity pipe, the same way billing is a money pipe (specs/14).
- **Supabase is one preset among future ones.** A provider that issues
  asymmetric-signed JWTs with a JWKS (Clerk, Auth0, WorkOS, a self-hosted
  OIDC server) is one preset constructor plus one SPA adapter file.

Non-goals: first-party auth (password storage, SMTP), OAuth/social login,
MFA, account deletion, child auth. Kids keep join codes + device tokens
(specs/12 §4.2); **no child ever has a provider account**.

## 2. Decisions

- **One auth provider per deployment.** No account linking across
  providers, no "same email = same parent" merging (an account-takeover
  vector). A switch is an explicit, offline remap (§6).
- **One generic JWT verifier, presets per provider.** Vendor knowledge is
  limited to a preset: issuer, keys, audience vs authorized-party check,
  claim mapping. No vendor SDK in the Go core.
- **Bearer tokens stay.** No cookies, so no CSRF surface, and CORS stays as
  is. The SPA asks for a fresh token per request (§5).
- **Verified email is enforced by us.** Co-parent invite acceptance matches
  on `Account.Email` (`family/members.go` AcceptInvite), so an unverified
  email is an invite takeover. `withParent` refuses an identity whose email
  is not verified, with the same 401 as a bad token.
- **Supabase sessions count as verified.** The deployment runbook requires
  "Confirm email" ON (docs/saas.md), so Supabase issues no session for an
  unconfirmed address. As a second line, a token whose
  `user_metadata.email_verified` is explicitly `false` is refused, and so is
  a session without an email (a phone-only user).
- **The provider URL is compulsory** for the `supabase` preset. Missing URL
  = the server does not start. There is no issuer-less, secret-only mode.
- **No back-compat and no data import.** The Supabase test deployment is
  abandoned. Production starts on a fresh database (specs/18). The
  `MATHIZ_SUPABASE_*` names, the `supabaseUrl`/`supabaseAnonKey` boot
  fields and `Account.supabase_user_id` are removed, not deprecated.
- **No Clerk code now.** The abstraction is built and tested with
  `supabase` and `test`.

## 3. Backend

### 3.1 Verifier

```go
// internal/saas/auth
type Identity struct {
    Provider      string // preset name: "supabase" | "test"
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

`server.Deps.Verifier` takes the interface. `withParent`: verify → refuse
an unverified email → `EnsureAccount(family.Identity)` →
`authz.Principal{AccountID: acct.UID}`. Every token failure is a generic
401 (`ErrUnauthenticated`). One exception: when the provider's keys cannot
be fetched, `Verify` returns `ErrUnavailable` and the server answers 503,
because the SPA signs out on a 401 and a provider blip must not sign out
every parent.

`auth.JWTVerifier` is the single implementation, configured by a `Preset`:

| Field | Meaning |
|---|---|
| `Issuer` | exact `iss` match; always required |
| `JWKSURL` | key set for asymmetric algorithms; fetched lazily, cached, refetched on unknown `kid` |
| `HS256Secret` | symmetric key (32+ bytes); `test`, or a Supabase project that signs with a shared secret |
| `Audience` | if set, `aud` must contain it |
| `AuthorizedParties` | if set, `azp` must be one of them |
| `Profile` | maps verified claims to email, email-verified, display name |

Rules the verifier enforces for every preset: at least one of `Audience` /
`AuthorizedParties`; an algorithm is allowed only when its key is
configured (`jwt.WithValidMethods`), never `none`; `exp` required; `iat`
not in the future; 30 s leeway; tokens on the JWKS path must carry a `kid`.
A failed first JWKS fetch returns its error, so the verifier's own 30 s
retry runs (keyfunc's default would build an empty set and refetch only
every 5 minutes). Each failed attempt cancels its refresh goroutine. JWKS
fetches time out after 5 s, because the fetch runs under a mutex.

### 3.2 Presets

| | `supabase` | `test` |
|---|---|---|
| Issuer | `{URL}/auth/v1` | `mathiz-test` |
| Keys | JWKS `{URL}/auth/v1/.well-known/jwks.json`, + optional HS256 secret | HS256 secret (required) |
| Audience | `authenticated` (overridable) | `authenticated` (overridable) |
| Email verified | true, unless `user_metadata.email_verified=false` or no email | true, unless `email_verified=false` (top level or `user_metadata`) or no email |
| Display name | `user_metadata.full_name\|name\|display_name` | same |

The provider URL must be an https origin with no path, query or user info
(plain http only on a loopback host for local development): the JWKS is
fetched from it. Scheme and host are lower-cased, because the issuer check
is exact.

`test` needs an explicit opt-in (`MATHIZ_AUTH_ALLOW_TEST=1`), is refused
when `MATHIZ_PUBLIC_BASE_URL` is https, and the compose `prod` profile
blanks the opt-in. It exists for unit tests, the dev compose stack and the
`saas-e2e` skill.

A later preset such as Clerk adds a constructor (issuer = the Frontend API
URL, JWKS, `AuthorizedParties` = the SPA origins, claims from a custom
session template) and the `Config` fields it reads. The verifier does not
change.

### 3.3 Data model

`Account` is keyed on `auth_provider` + `auth_subject` (both required, a
unique index on the pair). `supabase_user_id` is gone. `EnsureAccount(ctx,
family.Identity)` looks up by the pair, creates with a fresh UUID, refreshes
email and name, and tolerates the unique-constraint race. It is never keyed
on email.

ent auto-migrate never drops columns. On a database from before this
change, the old `supabase_user_id NOT NULL` column would fail every new
account insert, and adding the new NOT NULL columns to existing rows fails
in auto-migrate itself. So `mathiz serve` checks for that column **before**
auto-migrate (`store.BeforeMigrate`) and refuses to start with the fix: use
a fresh database, or drop the column if `accounts` is empty. On SQLite,
auto-migrate rebuilds an empty legacy table by itself, so a local CLI file
upgrades without help (the CLI never writes accounts).

### 3.4 Startup checks (fail fast)

In order:
1. The config validates (§3.5).
2. The verifier builds from the preset, before the database is touched.
3. Before auto-migrate: the legacy-column refusal (§3.3).
4. **Provider pin:** if any account has an `auth_provider` other than the
   configured one, refuse to start:
   `auth: N accounts belong to provider "supabase", configured "test": run
   "mathiz auth remap --from supabase --to test --map <csv>" (specs/17 §6)`.
5. A best-effort JWKS fetch, logged on failure but not fatal.

### 3.5 Config

| Var | Notes |
|---|---|
| `MATHIZ_AUTH_PROVIDER` | required: `supabase` \| `test` |
| `MATHIZ_AUTH_URL` | required for `supabase`: the project URL |
| `MATHIZ_AUTH_PUBLIC_KEY` | required for `supabase`: the anon key, sent to the SPA |
| `MATHIZ_AUTH_HS256_SECRET` | required for `test`; optional for `supabase` |
| `MATHIZ_AUTH_AUDIENCE` | overrides the preset's audience |
| `MATHIZ_AUTH_ALLOW_TEST` | must be exactly `1` for `test` |

A set `MATHIZ_SUPABASE_URL`, `MATHIZ_SUPABASE_ANON_KEY` or
`MATHIZ_SUPABASE_JWT_SECRET` stops the server with the name of its
replacement, so an old `.env` cannot boot half-configured.

### 3.6 Boot config

`GET /api/v1/config` returns:

```json
{ "auth": { "provider": "supabase", "url": "https://x.supabase.co", "publicKey": "…" },
  "posthogKey": "…", "posthogHost": "/relay" }
```

`url` and `publicKey` are omitted when empty (the `test` provider has
neither).

## 4. What does not change

Authz (`internal/saas/authz`, Principal-only), 404-not-403 on cross-tenant
denial, tenant isolation of learning data, child join/device-token auth
(parent tokens are still rejected on child routes and vice versa), the
billing/credits model, canonical log lines (UIDs only, never emails), and
the PostHog relay cookie strip.

## 5. Frontend

`web/src/auth/` holds the adapter. Each provider is a factory file plus a
sign-in form file, loaded through one static `import()` per provider, so a
deployment downloads only its own provider's code:

```ts
interface AuthAdapter {
  getToken(fresh?: boolean): Promise<string | null> // fresh=true forces a refresh
  user(): { id: string; email: string } | null
  onChange(cb: () => void): () => void
  signOut(): Promise<void>
  SignIn: React.ComponentType
}
```

- **Supabase adapter** (`supabase.ts`, `SupabaseSignIn.tsx`): the earlier
  client and forms, unchanged in behaviour: email code first, magic link to
  `/login`, password fallback. These two files are the only Supabase
  imports in the SPA.
- **Test adapter** (`test.ts`, `TestSignIn.tsx`): paste a token minted for
  the `test` preset. The parent dashboard is now drivable in a browser
  without any provider.
- **`api.ts`:** parent calls take a `TokenGetter`. `request()` asks for a
  token per call. On a 401 it retries once, and only with a token that is
  actually new. A second 401 calls the one `signOut()`, which resets
  analytics and is shared by concurrent callers; `/login` then says the
  session ended. A 503 is "sign-in unavailable", never a sign-out.
- **`App.tsx` / `Layout.tsx`:** use `onChange` / `user()`. The dashboard is
  keyed on `user().id`, so a different parent signing in from another tab
  gets a fresh dashboard. A failed first `/me` shows a retry panel, not
  "create your Family Space".
- **Kid surfaces and public pages never load an adapter.**
- **Copy:** the privacy page names "our hosted authentication provider".

## 6. Switching a deployment's provider

`mathiz auth remap --from X --to Y --map map.csv [--dry-run]`

- `map.csv` is `old_subject,new_subject` (an optional header row with those
  names is skipped).
- One transaction. Every account of `--from` must be in the map, or nothing
  is written: a parent left behind would lose their family. There is no
  `--allow-unmapped`.
- Every row is checked before the first write. A new subject that already
  belongs to another `--to` account, an old subject with no account,
  duplicate rows and empty subjects each fail with the row named.
- A rerun with the same map is a no-op (rows whose new subject already
  exists on `--to` count as done).

Runbook: import the parents into the new provider and export the CSV of
old and new user IDs; stop the server; run `remap --dry-run`, then `remap`;
set `MATHIZ_AUTH_PROVIDER`; start. Parents sign in once with the new
provider.

## 7. Data plane: off the Supabase cluster

See [18-self-hosted-postgres.md](18-self-hosted-postgres.md).

## 8. Open questions — resolved

- **Supabase verified signal:** sessions count as verified; the runbook
  requires "Confirm email" ON; an explicit `email_verified=false` and a
  missing email are refused (§2).
- **Clerk** (`email_verified` claim template, US-only data residency):
  deferred with Clerk itself.
- **Email change:** `Account.Email` refreshes from each token, so pending
  invites for an old address stop matching. Intended.

## 9. Testing (as built)

- `auth`: table-driven tests per preset with self-signed ES256/RS256 keys
  behind an httptest JWKS: wrong `iss`/`aud`/`azp`, absent `aud`/`iss`,
  expired, `nbf`/`iat` in the future, alg `none`, HS256 without a secret,
  an asymmetric token on the HS256-only preset, a kid-less token, a token
  alg that differs from its JWK, unknown-`kid` refetch, JWKS outage →
  `ErrUnavailable` with a rate-limited retry, `Close`, provider URL rules,
  short secrets, both email-verified claim shapes.
- `server`: tokens minted for the `test` preset; wrong-provider token,
  unverified email → 401; provider outage → 503; config validation and the
  removed-variable refusal; boot config shape.
- `family`: `EnsureAccount` keyed on (provider, subject), never email;
  `CountForeignAccounts`; remap success, dry run, idempotent rerun, and
  every refusal leaves the table unchanged.
- `cmd`: the provider pin; a pre-change `accounts` table with a row is
  refused before migrate and left unchanged; CSV parsing.
- `store`: `ColumnExists` on SQLite and PostgreSQL.
- E2E (`saas-e2e`): browser login with a pasted test token, dashboard,
  sign-out, forced sign-out after a forged token; the Supabase adapter
  renders its forms and loads only its own chunk.

## 10. Rollout

1. Done: the abstraction with `supabase` and `test`, the fresh-database
   data model, the SPA adapter, remap.
2. Ops: production starts on the self-hosted PostgreSQL (specs/18) with
   `MATHIZ_AUTH_PROVIDER=supabase` and "Confirm email" ON.
3. Later, if wanted: a Clerk (or other) preset + adapter, tried on staging
   with an imported copy of users, then §6.
