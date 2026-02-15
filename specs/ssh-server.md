# SSH Server — Web-Accessible Mathiz via Wish

## 1. Overview

Mathiz is a terminal-native TUI. To serve it on the web without rewriting the UI, we run Mathiz server-side behind Charm's **Wish** SSH framework. Each authenticated session gets an isolated Bubble Tea program backed by its own SQLite database.

Authentication is **decoupled**: an external identity provider (Auth0, Clerk, Supabase, etc.) issues JWTs. The web frontend handles login, obtains a JWT, and passes it to Mathiz via the SSH password field. Mathiz verifies the token signature, extracts the user identity, and combines it with a **profile name** (the SSH username) to derive a per-profile database path.

**Profiles** allow multiple learners under one authenticated account — a parent logs in once and each child connects with their own profile name (e.g. `alice`, `bob`, `john`), getting fully independent progress tracking.

**Design goals:**

- **Zero TUI changes** — existing screens, session engine, and domain packages work unmodified.
- **Auth-agnostic** — Mathiz only verifies JWT signatures. Swap providers without touching Mathiz.
- **Per-profile isolation** — each identity + profile combination gets its own SQLite file. No multi-tenant queries.
- **Secure by default** — LLM API keys stay server-side. No credentials in the browser.
- **Family-friendly** — multiple profiles per identity, no limit, no extra auth per profile.

### Consumers

| Component | Role |
|-----------|------|
| **Web frontend** | Authenticates the user, obtains JWT, opens WebSocket to SSH proxy |
| **WebSocket-to-SSH proxy** | Bridges the browser to the Wish SSH server (out of scope for this spec) |
| **Wish SSH server** | Verifies JWT, resolves identity + profile, opens per-profile DB, spawns Bubble Tea |

### Out of Scope

- Web frontend implementation
- WebSocket-to-SSH proxy implementation
- User registration / password management (handled by the identity provider)
- JWT issuance

---

## 2. Architecture

```
┌──────────────────────────────────────┐
│           Web Frontend               │
│  1. User authenticates (OAuth)       │
│  2. Receives JWT                     │
│  3. User picks/types profile name    │
│  4. Opens WebSocket connection       │
│     username = "alice"               │
│     password = JWT                   │
└──────────────┬───────────────────────┘
               │ WebSocket
               │ (out of scope)
┌──────────────▼───────────────────────┐
│    WebSocket-to-SSH Proxy            │
│    username = profile name           │
│    password = JWT                    │
└──────────────┬───────────────────────┘
               │ SSH (TCP)
┌──────────────▼───────────────────────┐
│       Mathiz Wish Server             │
│                                      │
│  5. WithPasswordAuth callback        │
│     → verify JWT signature           │
│     → extract identity claim (sub)   │
│     → read profile from ctx.User()   │
│  6. Derive namespace: SHA-256(sub)   │
│  7. Normalize profile: trim+lower    │
│  8. Encode profile: URL-safe base64  │
│  9. Open data/{ns}/{encoded}.db      │
│  10. Build app.Options               │
│  11. Spawn tea.Program               │
│                                      │
│  On disconnect:                      │
│  12. tea.Program exits               │
│  13. Store.Close()                   │
└──────────────────────────────────────┘
```

### Data Flow per Connection

1. SSH handshake completes. The SSH username carries the profile name, the password carries the JWT.
2. Wish calls the `PasswordHandler` with the JWT string. The profile name is available via `ctx.User()`.
3. The handler verifies the JWT signature using the configured JWKS endpoint.
4. On success, the configured identity claim (default `sub`) is extracted.
5. The claim value is hashed with SHA-256 to produce the **namespace** directory (64 hex chars).
6. The profile name is normalized (trimmed whitespace, lowercased) and encoded to a filesystem-safe form (URL-safe base64, no padding).
7. The handler stashes both the identity and profile in the SSH context.
8. The Bubble Tea middleware reads identity + profile from context and opens `{data_dir}/{namespace}/{encoded_profile}.db`.
9. Dependencies are wired (same as `cmd/run.go`) and a `tea.Program` is spawned.
10. When the SSH session ends, the program exits and the DB connection is closed.

---

## 3. JWT Verification

### 3.1 JWKS-Based Verification

Mathiz fetches the JSON Web Key Set from the identity provider's well-known endpoint. Keys are cached and refreshed on rotation.

**Config:**

| Env Var | Required | Description |
|---------|----------|-------------|
| `MATHIZ_SSH_SERVER_JWKS_URL` | Yes | JWKS endpoint URL (e.g. `https://example.us.auth0.com/.well-known/jwks.json`) |
| `MATHIZ_SSH_SERVER_JWT_AUDIENCE` | No | Expected `aud` claim. If set, tokens without this audience are rejected. |
| `MATHIZ_SSH_SERVER_JWT_ISSUER` | No | Expected `iss` claim. If set, tokens from other issuers are rejected. |
| `MATHIZ_SSH_SERVER_JWT_IDENTITY_CLAIM` | No | Claim to use as user identity. Default: `sub`. |

### 3.2 Verification Steps

1. Parse the JWT header to extract the `kid` (key ID).
2. Look up the signing key from the cached JWKS. If not found, refresh the JWKS and retry once.
3. Verify the signature using the matching public key (RS256 or ES256).
4. Validate standard claims: `exp` (not expired), `iat` (not in future), `nbf` (not before).
5. If `MATHIZ_SSH_SERVER_JWT_AUDIENCE` is set, verify `aud` contains the expected value.
6. If `MATHIZ_SSH_SERVER_JWT_ISSUER` is set, verify `iss` matches.
7. Extract the identity claim (`sub` by default, configurable via `MATHIZ_SSH_SERVER_JWT_IDENTITY_CLAIM`).
8. Return the claim value as the verified identity.

### 3.3 Error Handling

| Condition | Behavior |
|-----------|----------|
| JWKS fetch fails on startup | Server refuses to start (fail-fast) |
| JWKS refresh fails at runtime | Use cached keys; log warning |
| Invalid/expired JWT | SSH auth rejected (connection closed) |
| Missing identity claim | SSH auth rejected |

---

## 4. Identity & Profile to Database Mapping

### 4.1 Two-Level Path: Namespace + Profile

The database path is derived from two independent inputs:

```
{data_dir}/{namespace}/{encoded_profile}.db
```

| Component | Source | Derivation |
|-----------|--------|------------|
| **Namespace** | JWT identity claim (`sub`) | `hex(SHA-256(claim_value))` — 64 hex chars |
| **Profile** | SSH username (`ctx.User()`) | Trim whitespace → lowercase → URL-safe base64 (no padding) |

**Example:**

```
JWT sub:           "google-oauth2|12345"
SSH username:      "Alice"

Namespace:         hex(SHA-256("google-oauth2|12345"))
                   → "a1b2c3d4e5f6...64 hex chars"

Profile (display): "alice"           (trimmed + lowercased)
Profile (encoded): "YWxpY2U"         (base64url of "alice")

DB path:           /var/lib/mathiz/data/a1b2c3d4.../YWxpY2U.db
```

A parent authenticating once can have children connect as different profiles:

```
/var/lib/mathiz/data/
  a1b2c3d4.../                    ← namespace (one Google account)
    YWxpY2U.db                    ← alice's progress
    Ym9i.db                       ← bob's progress
    am9obg.db                     ← john's progress
```

### 4.2 Namespace Derivation (Identity)

```
namespace = hex(SHA-256(identity_claim_value))
```

**Properties:**
- Fixed 64-character hex string — no filesystem escaping needed.
- No PII in directory names — the raw identity cannot be recovered from the hash.
- Deterministic — same identity always maps to the same directory.
- Collision-resistant — SHA-256 collision probability is negligible.

### 4.3 Profile Name Handling

**Normalization** (applied before any use):
1. `strings.TrimSpace(username)` — strip leading/trailing whitespace
2. `strings.ToLower(normalized)` — case-insensitive ("Alice" and "alice" are the same profile)

**Filesystem encoding** (applied to the normalized name for the DB filename):
```
encoded = base64.RawURLEncoding.EncodeToString([]byte(normalized))
```

- URL-safe base64 without padding — safe on all filesystems.
- Supports any Unicode input (e.g. names in non-Latin scripts).
- The normalized (human-readable) form is stored in the `Identity` struct for display/logging.
- The encoded form is used only for the filename.

**Validation:**
- Empty profile name (after trimming) → SSH auth rejected with an error.

### 4.4 Database Directory

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `MATHIZ_SSH_SERVER_DATA_DIR` | No | `/var/lib/mathiz/data` | Root directory for per-profile SQLite files |

The server creates the root directory on startup if it doesn't exist (mode `0755`). Namespace subdirectories are created on first connection for that identity. Database files are created on first connection for that profile.

### 4.5 Database Lifecycle

- **First connection**: The namespace directory and SQLite file are created. `store.Open()` runs auto-migration.
- **Subsequent connections**: `store.Open()` opens the existing file. Event sourcing ensures continuity.
- **Concurrent connections (same profile)**: SQLite WAL mode supports concurrent readers. Two simultaneous sessions for the same profile will share the same DB file. This is safe because Mathiz's event-sourcing model uses append-only writes with a sequence counter.
- **Different profiles (same identity)**: Completely independent DB files in the same namespace directory. No interference.
- **Cleanup**: Database files are never auto-deleted. An admin tool or script can remove inactive namespace directories based on filesystem timestamps.

---

## 5. Server Configuration

### 5.1 Environment Variables

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `MATHIZ_SSH_SERVER_HOST` | No | `0.0.0.0` | SSH listen address |
| `MATHIZ_SSH_SERVER_PORT` | No | `2222` | SSH listen port |
| `MATHIZ_SSH_SERVER_HOST_KEY_PATH` | No | `mathiz_ed25519` | Path to SSH host key (auto-generated if missing) |
| `MATHIZ_SSH_SERVER_DATA_DIR` | No | `/var/lib/mathiz/data` | Per-user database directory |
| `MATHIZ_SSH_SERVER_JWKS_URL` | Yes | — | JWKS endpoint for JWT verification |
| `MATHIZ_SSH_SERVER_JWT_AUDIENCE` | No | — | Expected JWT audience |
| `MATHIZ_SSH_SERVER_JWT_ISSUER` | No | — | Expected JWT issuer |
| `MATHIZ_SSH_SERVER_JWT_IDENTITY_CLAIM` | No | `sub` | JWT claim for user identity |
| `MATHIZ_LLM_PROVIDER` | Yes | — | LLM provider (existing) |
| `MATHIZ_*_API_KEY` | Yes | — | LLM API key (existing) |

### 5.2 Startup Sequence

1. Load and validate configuration (fail-fast on missing `MATHIZ_SSH_SERVER_JWKS_URL`).
2. Fetch JWKS from the configured URL (fail-fast on error).
3. Create `MATHIZ_SSH_SERVER_DATA_DIR` if it doesn't exist.
4. Initialize the LLM provider (shared across all sessions).
5. Load or generate SSH host key.
6. Start the Wish SSH server.

### 5.3 Shared vs Per-Session Resources

| Resource | Shared / Per-Session | Reason |
|----------|---------------------|--------|
| LLM provider | Shared | Stateless HTTP client; safe for concurrent use |
| JWKS cache | Shared | One cache, refreshed on key rotation |
| SSH host key | Shared | Single server identity |
| SQLite database | Per-profile | Isolated by namespace (identity) + profile |
| `store.Store` | Per-session | Owns DB connection and ent client |
| `app.Options` | Per-session | Built from per-profile store + shared provider |
| `tea.Program` | Per-session | Wish spawns one per SSH connection |
| `diagnosis.Service` | Per-session | Holds buffered channel for async LLM calls |
| `gems.Service` | Per-session | Backed by per-profile EventRepo |

---

## 6. Wish Integration

### 6.1 Package Structure

```
cmd/
  serve.go              # "mathiz serve" cobra command
internal/
  sshserver/
    server.go           # Wish server setup, middleware chain
    auth.go             # JWT verification, JWKS cache, identity extraction
    identity.go         # DB key derivation, context helpers
    config.go           # Server config from env vars
    server_test.go      # Server integration tests
    auth_test.go        # JWT verification unit tests
    identity_test.go    # DB key derivation tests
```

### 6.2 Server Setup (`server.go`)

```go
func Start(ctx context.Context, cfg *Config) error {
    jwks, err := NewJWKSCache(cfg.JWKSURL)
    if err != nil {
        return fmt.Errorf("fetch JWKS: %w", err)
    }

    provider, err := llm.NewProviderFromEnv(ctx, nil) // no event logging for shared provider
    if err != nil {
        return fmt.Errorf("init LLM provider: %w", err)
    }

    srv, err := wish.NewServer(
        wish.WithAddress(net.JoinHostPort(cfg.Host, cfg.Port)),
        wish.WithHostKeyPath(cfg.HostKeyPath),
        wish.WithPasswordAuth(passwordHandler(jwks, cfg)),
        wish.WithMiddleware(
            bubbletea.Middleware(teaHandler(cfg, provider)),
            activeterm.Middleware(),
        ),
    )
    if err != nil {
        return fmt.Errorf("create server: %w", err)
    }

    // Graceful shutdown on context cancellation.
    go func() {
        <-ctx.Done()
        srv.Shutdown(ctx)
    }()

    log.Printf("Mathiz SSH server listening on %s:%s", cfg.Host, cfg.Port)
    return srv.ListenAndServe()
}
```

### 6.3 Password Handler (`auth.go`)

The password handler verifies the JWT and validates the profile name. Both must succeed for the connection to be accepted.

```go
func passwordHandler(jwks *JWKSCache, cfg *Config) func(ssh.Context, string) bool {
    return func(ctx ssh.Context, token string) bool {
        // Verify JWT and extract identity claim.
        identity, err := verifyAndExtract(jwks, cfg, token)
        if err != nil {
            log.Printf("auth failed: %v", err)
            return false
        }

        // Normalize and validate profile name from SSH username.
        profile, err := NormalizeProfile(ctx.User())
        if err != nil {
            log.Printf("invalid profile name: %v", err)
            return false
        }

        identity.Profile = profile
        SetIdentity(ctx, identity)
        return true
    }
}
```

### 6.4 Bubble Tea Handler

```go
func teaHandler(cfg *Config, provider llm.Provider) func(ssh.Session) (tea.Model, []tea.ProgramOption) {
    return func(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
        identity := GetIdentity(sess.Context())
        dbPath := ProfileDBPath(cfg.DataDir, identity)

        // Ensure the namespace directory exists.
        if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
            log.Printf("create namespace dir for %s: %v", identity.Namespace, err)
            return errorModel(err), nil
        }

        st, err := store.Open(dbPath)
        if err != nil {
            log.Printf("open store for %s/%s: %v", identity.Namespace, identity.Profile, err)
            return errorModel(err), nil
        }

        // Register cleanup on session end.
        go func() {
            <-sess.Context().Done()
            st.Close()
        }()

        eventRepo := st.EventRepo()
        diagService := diagnosis.NewService(provider)

        go func() {
            <-sess.Context().Done()
            diagService.Close()
        }()

        opts := app.Options{
            EventRepo:        eventRepo,
            SnapshotRepo:     st.SnapshotRepo(),
            GemService:       gems.NewService(eventRepo),
            LLMProvider:      provider,
            Generator:        problemgen.New(provider, problemgen.DefaultConfig()),
            DiagnosisService: diagService,
            LessonService:    lessons.NewService(provider, lessons.DefaultConfig()),
            Compressor:       lessons.NewCompressor(provider, lessons.DefaultCompressorConfig()),
        }

        return app.NewAppModel(opts), nil
    }
}
```

### 6.5 Identity, Profile & DB Path (`identity.go`)

```go
// Identity holds the verified user identity and selected profile.
type Identity struct {
    Claim     string // raw identity claim value (e.g. "google-oauth2|12345")
    Namespace string // SHA-256 hex of claim — used as directory name
    Profile   string // normalized profile name (trimmed, lowercased)
}

// DeriveNamespace computes the namespace directory from an identity claim.
func DeriveNamespace(claimValue string) string {
    h := sha256.Sum256([]byte(claimValue))
    return hex.EncodeToString(h[:])
}

// NormalizeProfile trims and lowercases the SSH username.
// Returns an error if the result is empty.
func NormalizeProfile(username string) (string, error) {
    p := strings.ToLower(strings.TrimSpace(username))
    if p == "" {
        return "", fmt.Errorf("profile name is empty")
    }
    return p, nil
}

// EncodeProfile produces a filesystem-safe filename from a normalized profile name.
// Uses URL-safe base64 without padding to handle any Unicode input.
func EncodeProfile(normalized string) string {
    return base64.RawURLEncoding.EncodeToString([]byte(normalized))
}

// ProfileDBPath returns the full database path for an identity + profile.
//   {dataDir}/{namespace}/{encoded_profile}.db
func ProfileDBPath(dataDir string, id Identity) string {
    return filepath.Join(dataDir, id.Namespace, EncodeProfile(id.Profile)+".db")
}

// Context helpers using a package-level key.
type ctxKey struct{}

func SetIdentity(ctx ssh.Context, id Identity) {
    ctx.SetValue(ctxKey{}, id)
}

func GetIdentity(ctx context.Context) Identity {
    return ctx.Value(ctxKey{}).(Identity)
}
```

---

## 7. CLI Command

### 7.1 `mathiz serve`

A new Cobra subcommand starts the SSH server.

```
mathiz serve [flags]

Flags:
  --host         SSH listen address (default: $MATHIZ_SSH_HOST or "0.0.0.0")
  --port         SSH listen port (default: $MATHIZ_SSH_PORT or "2222")
  --data-dir     Per-user database directory (default: $MATHIZ_SSH_SERVER_DATA_DIR or "/var/lib/mathiz/data")
  --host-key     SSH host key path (default: $MATHIZ_SSH_SERVER_HOST_KEY_PATH or "mathiz_ed25519")
```

Environment variables take precedence over defaults; CLI flags take precedence over both.

The `serve` command does **not** use the `--db` flag from the root command. The database path is derived per-user from the JWT identity.

---

## 8. Refactoring `cmd/run.go`

The dependency-wiring logic currently in `runApp()` is split into a reusable function that both `runApp()` (local mode) and the SSH server's tea handler can call.

### 8.1 Extracted Function

```go
// BuildOptions constructs app.Options from a store and optional LLM provider.
// If provider is nil, AI features are disabled.
func BuildOptions(st *store.Store, provider llm.Provider) app.Options {
    eventRepo := st.EventRepo()
    opts := app.Options{
        EventRepo:    eventRepo,
        SnapshotRepo: st.SnapshotRepo(),
        GemService:   gems.NewService(eventRepo),
    }
    if provider != nil {
        opts.LLMProvider = provider
        opts.Generator = problemgen.New(provider, problemgen.DefaultConfig())
        opts.DiagnosisService = diagnosis.NewService(provider)
        opts.LessonService = lessons.NewService(provider, lessons.DefaultConfig())
        opts.Compressor = lessons.NewCompressor(provider, lessons.DefaultCompressorConfig())
    }
    return opts
}
```

`runApp()` is updated to call `BuildOptions()` instead of inlining the wiring. The SSH tea handler also calls `BuildOptions()`.

---

## 9. Security Considerations

### 9.1 JWT Security

- **Signature verification is mandatory.** Tokens are never trusted without cryptographic verification.
- **Clock skew tolerance**: 30 seconds for `exp`/`nbf`/`iat` validation.
- **Algorithm restriction**: Only allow RS256, RS384, RS512, ES256, ES384, ES512. Reject `none` and symmetric algorithms (HS256) since we only have public keys.
- **JWKS refresh**: Cache keys for up to 1 hour. Refresh on unknown `kid`. Rate-limit refresh attempts to 1 per minute to prevent abuse.

### 9.2 Database Isolation

- SHA-256 namespace ensures no user can predict or target another identity's directory.
- Profile names within a namespace are not secret (they're chosen by the user), but the namespace hash prevents cross-identity access.
- Profiles are scoped to an identity — "alice" under identity A is completely separate from "alice" under identity B.
- Database files are created with mode `0600` (owner read/write only).
- The `MATHIZ_SSH_SERVER_DATA_DIR` should be on a filesystem with quotas or monitoring to prevent disk exhaustion.

### 9.3 SSH Transport

- Wish uses modern SSH ciphers by default.
- SSH password auth is disabled for non-JWT use. The password field exclusively carries JWT tokens.
- Public key auth is disabled by default to enforce JWT-only authentication.
- Rate limiting: Wish does not provide built-in rate limiting. Deploy behind a reverse proxy or firewall for connection-rate limits.

### 9.4 Resource Limits

- **Max concurrent sessions**: Configurable (default 100). Reject new connections beyond this limit.
- **Session timeout**: Configurable idle timeout (default 30 minutes). Disconnect idle sessions.
- **DB connection pool**: Each session opens one SQLite connection. SQLite's lightweight footprint means 100 concurrent connections are feasible.

---

## 10. WebSocket Proxy Contract

While the proxy is out of scope, the SSH server expects this contract:

1. The proxy receives an authenticated WebSocket connection from the web frontend.
2. The web frontend includes the JWT and a profile name (e.g., as query parameters, headers, or initial message).
3. The proxy opens an SSH connection to `MATHIZ_SSH_HOST:MATHIZ_SSH_SERVER_PORT`.
4. The proxy passes the **profile name as the SSH username** and the **JWT as the SSH password**.
5. The proxy bridges WebSocket frames ↔ SSH channel bytes bidirectionally.
6. Terminal resize events from the browser are forwarded as SSH window-change requests.
7. On WebSocket close, the proxy closes the SSH connection (and vice versa).

**Example SSH connection from the proxy's perspective:**
```
ssh -o "User=alice" -o "Password=eyJhbGci..." mathiz-server:2222
```

---

## 11. Testing Strategy

### 11.1 Unit Tests

| Test | What it verifies |
|------|-----------------|
| `TestDeriveNamespace` | SHA-256 hex output is deterministic and 64 chars |
| `TestDeriveNamespace_DifferentInputs` | Different claim values produce different namespaces |
| `TestNormalizeProfile` | Trims whitespace, lowercases, rejects empty |
| `TestNormalizeProfile_Unicode` | Non-Latin names are preserved after lowercasing |
| `TestEncodeProfile` | URL-safe base64, no padding, reversible |
| `TestProfileDBPath` | Correct `{dir}/{namespace}/{encoded}.db` construction |
| `TestProfileDBPath_SameIdentityDifferentProfiles` | Different profiles → different paths |
| `TestProfileDBPath_DifferentIdentitySameProfile` | Same profile name under different identities → different paths |
| `TestVerifyJWT_ValidToken` | Valid token returns correct identity |
| `TestVerifyJWT_ExpiredToken` | Expired token is rejected |
| `TestVerifyJWT_WrongSignature` | Token signed with wrong key is rejected |
| `TestVerifyJWT_MissingClaim` | Token without identity claim is rejected |
| `TestVerifyJWT_CustomClaim` | Configurable claim extraction works |
| `TestVerifyJWT_AlgorithmNone` | `alg: none` tokens are rejected |
| `TestConfigFromEnv` | Config loads from env vars with correct defaults |
| `TestConfigValidation` | Missing JWKS URL fails validation |

### 11.2 Integration Tests

| Test | What it verifies |
|------|-----------------|
| `TestSSHSession_ValidJWT` | Full flow: connect → auth → Bubble Tea renders → disconnect |
| `TestSSHSession_InvalidJWT` | Connection rejected with invalid token |
| `TestSSHSession_EmptyProfile` | Connection rejected when username is empty/whitespace |
| `TestSSHSession_ProfileIsolation` | Same identity, different profiles → separate DB files, independent state |
| `TestSSHSession_IdentityIsolation` | Different identities, same profile name → separate namespace dirs |
| `TestSSHSession_ProfileCaseInsensitive` | "Alice" and "alice" resolve to the same DB |
| `TestSSHSession_Reconnect` | Same identity + profile reconnects and sees persisted state |
| `TestSSHSession_ConcurrentProfiles` | Multiple profiles under one identity active simultaneously |

Integration tests use an in-process Wish server on a random port with a test JWKS endpoint (httptest.Server serving a test key set).

---

## 12. Dependencies

### New Go Dependencies

| Package | Purpose |
|---------|---------|
| `charm.land/wish/v2` | SSH server framework with Bubble Tea middleware |
| `github.com/golang-jwt/jwt/v5` | JWT parsing and validation |
| `github.com/MicahParks/keyfunc/v3` | JWKS fetching, caching, and key selection |

### Existing Dependencies (No Changes)

All existing packages (`internal/app`, `internal/store`, `internal/session`, etc.) are used as-is via `app.Options` dependency injection.

---

## 13. Verification Checklist

- [ ] `mathiz serve` starts and listens on configured host:port
- [ ] Server refuses to start without `MATHIZ_SSH_SERVER_JWKS_URL`
- [ ] Valid JWT + profile name → SSH session established → Bubble Tea renders home screen
- [ ] Expired JWT → connection rejected
- [ ] Wrong signature → connection rejected
- [ ] `alg: none` → connection rejected
- [ ] Empty profile name → connection rejected
- [ ] Same identity, different profiles → distinct `.db` files in same namespace directory
- [ ] Different identities, same profile → distinct namespace directories
- [ ] "Alice" and "alice" → same database file
- [ ] Same identity + profile reconnects and sees prior session data
- [ ] Server shuts down gracefully on SIGINT/SIGTERM (active sessions drain)
- [ ] `go test ./internal/sshserver/...` passes
- [ ] `CGO_ENABLED=0 go build ./...` succeeds
