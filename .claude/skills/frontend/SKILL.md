---
name: frontend
description: Mathiz web SPA (web/) patterns — data fetching, loading UX, routing, role gating, kid-surface rules, CSS conventions, and the verification gate. Use BEFORE writing or changing any code under web/.
---

# Frontend patterns (web/)

**Maintenance rule (read first): this file is a map, not the territory.**
The code is the source of truth; this skill records the rules and points at
living exemplars. If your PR changes one of these patterns, update this
file IN THE SAME PR — a stale skill misleads every future agent. Keep it
minimal: rules + pointers, never copied code snippets.

Stack: Vite + React + react-router. **No data-fetching framework by
explicit decision** — no react-query/SWR/Redux. Don't introduce one.

## Data fetching

- `web/src/api.ts` `request()` is the ONLY fetch chokepoint. Every call
  goes through the typed `api` object; never call `fetch` directly. This
  is load-bearing: `request()` maintains the in-flight counter that drives
  the global `BusyBar` (`web/src/components/BusyBar.tsx`), so any direct
  fetch is invisible to the activity UI.
- Add new endpoints as typed methods on `api` with interfaces beside them.
  Optional server features (billing, quests, activity) 404 when not
  configured — callers treat 404 as "feature off", not an error wall.

## Loading & feedback

- Every mutation button uses `useAction` from `web/src/hooks.ts` —
  `[run, busy]` with a re-entrancy guard. No bare async onClick handlers.
- First loads render `Skeleton` (`web/src/components/Skeleton.tsx`) with a
  per-section first-load flag; empty-state copy is gated behind that flag
  so it never flashes before data arrives.
- After a mutation, refetch only what changed (targeted refetch), not the
  whole page. Exemplar for all three: `web/src/pages/dashboard/Activity.tsx`.
- Stale-response discipline: any fetch whose inputs can change mid-flight
  (filters, route params) guards with a generation/id check before
  applying results — see the `fetchGeneration` ref in `Activity.tsx` and
  the `activeQuestId` guard in `QuestEditor.tsx`.

## Structure & navigation

- The parent dashboard is a routed shell: `pages/dashboard/Layout.tsx`
  fetches `/me` + children ONCE and shares `{token, family, role,
  children, refresh*}` via outlet context (`pages/dashboard/context.ts`,
  `useDashboard()`). New parent pages = new route under the shell reading
  that context; don't re-fetch what the layout already has.
- **Modals are for transactions, pages are for workloads.** A short,
  few-field, one-commit interaction may be a modal (Add Child, Create
  Quest). Anything with an unbounded list, an editing session, or a
  review flow gets a route (see `QuestEditor.tsx` — it was a modal once,
  and that was a mistake).
- Public marketing/legal pages (`Landing`, `Pricing`, `Legal`) are
  Supabase-free — routed OUTSIDE `ParentArea` in `App.tsx`. Never make
  the front door pay the auth boot cost.

## Roles & authz

- Role threading is fail-closed: `role ?? 'parent'` (least privilege) —
  never default to owner. Owner-only UI (billing, parent management) is
  hidden for co-parents AND guarded on direct navigation (redirect), but
  the UI is cosmetic — the server is the enforcement point. Never build
  UI that assumes hiding equals security.

## Kid surfaces (/join, /play)

- NEVER show prices, balances, plans, upsell, or links to pricing. Zero
  credits reads as "The ship needs to rest! ⛵ Ask your grown-up".
- Errors are playful and blame-free; no technical copy. A second session
  is "politely refused", not "409 Conflict".

## Analytics

- Read the `analytics` skill BEFORE touching anything tracking-related.
  Chokepoint rule: only `web/src/analytics.ts` imports `posthog-js`
  (dynamically) or contains event-name strings — everything else calls the
  typed `track.*` helpers. Child identity NEVER enters analytics.

## CSS

- One stylesheet: `web/src/index.css`. Extend the existing vocabulary
  (`.btn`, `.chip`, `.card`, `.muted`, section classes) with new
  namespaced rules appended at the end; never restyle existing rules for
  a new feature.
- `.btn` dresses both `<button>` and `<a>/<Link>` — it already kills the
  anchor underline. Don't re-patch per component; if a base class lacks
  something every user needs, fix the base class.
- Mobile is real: the shell collapses to a tab bar under ~45rem; new
  pages must work at 390px width (no horizontal scroll).

## Verification gate (before calling anything done)

```bash
cd web && npx tsc --noEmit && npm run lint && npm run build && cd ..
```

- `npm run build` emits into `internal/saas/webui/dist` and DELETES the
  tracked `dist/.gitkeep` — restore it before committing (`git checkout
  internal/saas/webui/dist/.gitkeep`). Losing it breaks `go:embed` on
  fresh checkouts.
- The Go binary embeds a stale SPA unless you `touch
  internal/saas/webui/webui.go` after a web build.
- Browser-level verification: use the `saas-e2e` skill.
