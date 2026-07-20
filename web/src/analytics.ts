// Product analytics (PostHog) — THE single chokepoint for the whole SPA.
//
// Rules (specs/16-analytics.md, .claude/skills/analytics/SKILL.md):
// - Only this module may import posthog-js, and only via dynamic import()
//   gated on a server-provided key: key unset = zero analytics execution.
// - String event names exist ONLY in this file. Every event is a typed
//   helper on the exported `track` object.
// - CHILD IDENTITY NEVER ENTERS ANALYTICS. No child names, profile UIDs,
//   or PINs; no identify() on child surfaces. Children are anonymous usage
//   attributed to their family group, with memory-only persistence so no
//   cross-session fingerprint is ever written on a kid's device.
// - Every helper is fire-and-forget: never throws, silently drops before
//   init or when analytics is off.

import type { PostHog } from 'posthog-js'
import { api } from './api'

export type AnalyticsSurface = 'public' | 'parent' | 'child'

interface AnalyticsBootConfig {
  posthogKey?: string
  posthogHost?: string
}

// Module singleton: one client, one lazy init promise. Double-init no-ops.
let client: PostHog | null = null
let initPromise: Promise<void> | null = null
let currentSurface: AnalyticsSurface | null = null

// initAnalytics wires PostHog from boot config. No key → no-op (and the
// posthog-js module is never even downloaded — it stays a lazy chunk).
export function initAnalytics(cfg: AnalyticsBootConfig, surface: AnalyticsSurface): Promise<void> {
  if (!cfg.posthogKey) return Promise.resolve()
  if (initPromise) {
    // Already booted (or booting). Persistence must follow the surface in
    // BOTH directions within one tab: a kid surface must never persist
    // (landing → /join flips to memory-only), and a later parent/public
    // surface must get durable persistence back (kid plays, parent signs
    // in — without the flip-back the parent identity would silently drop
    // on every refresh).
    if (surface !== currentSurface) {
      currentSurface = surface
      const persistence = surface === 'child' ? ('memory' as const) : ('localStorage+cookie' as const)
      void initPromise.then(() => {
        try {
          client?.set_config({ persistence })
        } catch {
          // analytics must never break the app
        }
      })
    }
    return initPromise
  }
  currentSurface = surface
  const key = cfg.posthogKey
  // The server hands us its same-origin relay path ("/relay"), never a
  // third-party domain — events go through our origin so ad-blocker domain
  // lists can't drop them. Do NOT point api_host at *.posthog.com.
  const host = cfg.posthogHost || '/relay'
  initPromise = import('posthog-js')
    .then(({ default: posthog }) => {
      posthog.init(key, {
        api_host: host,
        ui_host: 'https://us.posthog.com', // keeps the PostHog toolbar working
        autocapture: false,
        capture_pageview: false, // SPA — routes are tracked manually
        disable_session_recording: true,
        respect_dnt: true,
        person_profiles: 'identified_only',
        // Kid surfaces: memory-only persistence — no cookies/localStorage,
        // the anonymous distinct id is throwaway by design.
        ...(surface === 'child' ? { persistence: 'memory' as const } : {}),
      })
      client = posthog
    })
    .catch(() => {
      // Loading analytics failed (offline, blocker) — stay silently off.
    })
  return initPromise
}

// ensureAnalyticsBooted fetches /api/v1/config itself (once, cached here)
// and inits — for surfaces that don't otherwise touch boot config, like
// the static Landing page or the kid surfaces.
let bootCfgPromise: Promise<AnalyticsBootConfig> | null = null
export function ensureAnalyticsBooted(surface: AnalyticsSurface): Promise<void> {
  bootCfgPromise ??= api.bootConfig().catch(() => ({}) as AnalyticsBootConfig)
  return bootCfgPromise.then((cfg) => initAnalytics(cfg, surface)).catch(() => {})
}

// safe runs an analytics call if the client is up — never throws.
function safe(fn: (ph: PostHog) => void) {
  try {
    if (client) fn(client)
  } catch {
    // analytics must never break the app
  }
}

// ---- Identity (parent surfaces ONLY) ----

// identifyParent: parents consented by signing up. Distinct id is the
// account UID — never the email (emails are person properties, not ids).
export function identifyParent(
  accountId: string,
  props: { email?: string; name?: string; role?: string },
) {
  safe((ph) => ph.identify(accountId, { email: props.email, name: props.name, role: props.role }))
}

// identifyFamilyGroup: parent-side family group. This is the ONLY place the
// family name may enter analytics.
export function identifyFamilyGroup(familyId: string, familyName?: string) {
  safe((ph) => ph.group('family', familyId, familyName ? { name: familyName } : undefined))
}

// attachChildToFamily: child-side family group — group key ONLY, no
// properties (the child side never sends the family name, and never any
// child identity at all).
export function attachChildToFamily(familyId: string) {
  safe((ph) => ph.group('family', familyId))
}

// resetAnalytics: on sign-out — drops the identified distinct id.
export function resetAnalytics() {
  safe((ph) => ph.reset())
}

// ---- Events ----
// snake_case, past tense. Add new events HERE (typed helper) and to the
// taxonomy table in specs/16-analytics.md — never inline a string name
// anywhere else.

function capture(event: string, props?: Record<string, unknown>) {
  safe((ph) => ph.capture(event, props))
}

export const track = {
  // Public funnel
  landingCtaClicked: (persona: 'parent' | 'kid') => capture('landing_cta_clicked', { persona }),
  pricingViewed: () => capture('pricing_viewed'),
  // Fires when the public /how-it-works page mounts.
  howItWorksViewed: () => capture('how_it_works_viewed'),
  signinCompleted: () => capture('signin_completed'),

  // Parent dashboard
  familyCreated: () => capture('family_created'),
  childAdded: (grade: number) => capture('child_added', { grade }),
  joinCodeCreated: () => capture('join_code_created'),
  coparentInvited: () => capture('coparent_invited'),
  coparentAccepted: () => capture('coparent_accepted'),
  activityViewed: () => capture('activity_viewed'),
  // Fires when the parent /dashboard/curriculum page mounts.
  curriculumViewed: () => capture('curriculum_viewed'),
  questCreated: () => capture('quest_created'),
  questAiGenerated: (count: number) => capture('quest_ai_generated', { count }),
  questPublished: () => capture('quest_published'),
  billingViewed: () => capture('billing_viewed'),
  checkoutStarted: (plan: string) => capture('checkout_started', { plan }),

  // Child surfaces — anonymous, family-group attributed, NEVER identified.
  joinRedeemed: () => capture('join_redeemed'),
  expeditionStarted: (kind: 'skill' | 'quest') => capture('expedition_started', { kind }),
  expeditionCompleted: (questions: number, correct: number, kind: 'skill' | 'quest') =>
    capture('expedition_completed', { questions, correct, kind }),
  questCompletedByChild: () => capture('quest_completed_by_child'),
  outOfCreditsShown: () => capture('out_of_credits_shown'),

  // Manual SPA pageview (parent/public surfaces only — never on child ones).
  pageview: (path: string) => capture('$pageview', { $current_url: window.location.origin + path }),
}
