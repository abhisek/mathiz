---
name: saas-e2e
description: Run the full Mathiz SaaS stack (PostgreSQL + mathiz serve + stub LLM) and drive browser E2E flows with Playwright in a sandbox — no real Supabase project or LLM API key required. Use when verifying changes to internal/saas/**, web/**, cmd/serve.go, or the store's Postgres path.
---

# Full-stack SaaS E2E in a sandbox

Everything runs locally: Postgres bootstrapped from the distro package, the
LLM stubbed with an OpenAI-compatible fake, parent auth via a self-minted
HS256 JWT. Children never need Supabase at all (join codes + device tokens).

## 1. Start PostgreSQL (no Docker needed)

```bash
bash .claude/skills/saas-e2e/assets/pg-local.sh   # idempotent; port 5433, db mathiz_e2e
```

## 2. Start the stub LLM + the server

```bash
node .claude/skills/saas-e2e/assets/llmstub.mjs &   # OpenAI-compatible on :9993
make web && make mathiz                              # SPA + binary

MATHIZ_DATABASE_URL="postgres://postgres@127.0.0.1:5433/mathiz_e2e?sslmode=disable" \
MATHIZ_SUPABASE_URL="https://dummy.supabase.co" \
MATHIZ_SUPABASE_ANON_KEY="dummy-anon-key" \
MATHIZ_SUPABASE_JWT_SECRET="e2e-test-secret-with-plenty-of-length!!" \
MATHIZ_SERVER_ADDR=":8091" \
MATHIZ_LLM_PROVIDER=openai \
MATHIZ_OPENAI_API_KEY=stub-key \
MATHIZ_OPENAI_BASE_URL="http://127.0.0.1:9993/v1" \
./bin/mathiz serve &
```

The stub answers every question with "What is 12 + 7?" (answer **19**) and
serves a canned micro-lesson (practice answer **19**) when the request's
JSON-schema name is `micro-lesson`. Diagnosis/profile calls fail to parse
harmlessly (they're async best-effort).

## 3. Parent onboarding via API (mint a JWT, no Supabase)

```bash
JWT=$(node .claude/skills/saas-e2e/assets/mint-jwt.mjs)   # same secret as above
curl -s -H "Authorization: Bearer $JWT" http://localhost:8091/api/v1/me
# POST /api/v1/family {name} → POST /api/v1/family/{id}/children {name,grade,pin}
# → POST /api/v1/family/{id}/invites {} → note the join code (e.g. TIGER-4207)
```

## 4. Browser flows (Playwright, preinstalled Chromium)

```js
const browser = await chromium.launch({ executablePath: '/opt/pw-browsers/chromium' })
```

Key selectors, in flow order:
- **Join**: `/join` → `.code-input` → `button:has-text("go!")` →
  `.profile-tile:has-text("<name>")` → (PIN) `input[type="password"]` →
  `button:has-text("Start playing")` → lands on `/play`.
- **Map**: `.island`, spots by state: `.spot-ready` (glowing X), `.spot-locked`,
  `.spot-digging`, `.spot-proving`, `.spot-treasure`, `.spot-sinking`.
  Fresh child ⇒ exactly 1 ready + 53 locked.
- **Expedition**: click a spot → `.quest-text`, `.answer-input`,
  `button:has-text("Dig!")` → `.feedback-yes` / `.feedback-no` →
  `button:has-text("Next clue")`. Prove tier shows `.timer-bar`.
- **Guide (micro-lesson)**: two wrong answers → feedback shows
  `button:has-text("guide has a tip")` → `.lesson h3`, `.lesson .answer-input`,
  `button:has-text("Try it!")` → `button:has-text("Back to the hunt")`.
- **Summary / vault / notebook**: `.summary`, `button:has-text("Back to the map")`;
  header `🧭` button → `.notebook` / `.notebook-tip-head`; gem counter → `.vault`.
- **Terminal fallback**: `/terminal` streams the TUI (xterm.js); `.pill-live`
  when connected; ctrl+c ends the session.

## 5. Cleanup

Kill by exact process name or listening port — `pkill -f <pattern>` matches
your own compound shell command and kills your shell (exit 144):

```bash
kill $(pgrep -x mathiz)
kill $(ss -tlnp | grep 9993 | grep -oP 'pid=\K[0-9]+' | head -1)
```

## Notes

- Postgres data persists between runs in this sandbox; reuse the family +
  join code you created, or create fresh children for clean state.
- Server logs: check the file you redirected `mathiz serve` output into —
  500s log the underlying error there.
- The store's Postgres suite (unit level) is cheaper than full E2E:
  `MATHIZ_TEST_DATABASE_URL="postgres://postgres@127.0.0.1:5433/mathiz_test?sslmode=disable" go test ./internal/store/`.
