// Typed client for the Mathiz SaaS API.

export interface BootConfig {
  supabaseUrl: string
  supabaseAnonKey: string
  // Present only when the operator configured analytics; absent = fully off.
  // posthogHost is the server's same-origin relay path ("/relay"), never an
  // external domain. Consumed exclusively by src/analytics.ts.
  posthogKey?: string
  posthogHost?: string
}

export interface Account {
  id: string
  email: string
  displayName: string
}

export interface FamilySpace {
  id: string
  name: string
  createdAt: string
}

export interface ChildProfile {
  id: string
  name: string
  grade: number
  hasPin: boolean
  archived: boolean
  createdAt: string
}

export interface ChildSummary {
  masteredSkills: number
  learningSkills: number
  totalSkills: number
  gems: number
  lastSessionAt: string | null
}

export interface ChildWithSummary {
  profile: ChildProfile
  summary: ChildSummary
}

export interface Invite {
  id: string
  code: string
  expiresAt: string
  createdAt: string
}

// ---- Co-parents (specs/12-saas.md, "Co-parents") ----

export interface ParentMember {
  accountId: string
  email: string
  displayName: string
  role: string // 'owner' | 'parent'
  createdAt: string
}

export interface ParentInvite {
  id: string
  email: string
  status: string
  createdAt: string
}

// Surfaced on /me when the signed-in account has no family but a pending
// co-parent invite matches its email — drives the dashboard accept banner.
export interface PendingParentInvite {
  id: string
  familyName: string
  invitedBy: string
}

export interface Device {
  id: string
  label: string
  createdAt: string
  lastUsedAt: string | null
}

export interface SkillStat {
  id: string
  name: string
  strand: string
  grade: number
  state: string
  accuracy: number
  attempts: number
}

export interface SessionStat {
  at: string
  questions: number
  correct: number
  durationSecs: number
  gems: number
}

export interface StrandStat {
  id: string
  name: string
  mastered: number
  total: number
}

export interface ChildStats {
  mastery: {
    mastered: number
    learning: number
    rusty: number
    total: number
    strands: StrandStat[] | null
    skills: SkillStat[] | null
  }
  learnerProfile: {
    summary: string
    strengths: string[]
    weaknesses: string[]
    patterns: string[]
  } | null
  recentSessions: SessionStat[]
  gems: { total: number; byType: Record<string, number> }
}

// ---- Public curriculum (GET /api/v1/curriculum, no auth) ----
// The skill graph rendered for humans: islands in canonical order, each
// island's skills ordered by grade. Static per binary — cache freely.

export interface CurriculumSkill {
  id: string
  name: string
  grade: number
  prereqs: string[]
}

export interface CurriculumIsland {
  id: string
  name: string
  skills: CurriculumSkill[]
}

export interface CurriculumInfo {
  islands: CurriculumIsland[]
}

// ---- Parent quests (specs/15-quests.md) ----

export type QuestStatus = 'draft' | 'active' | 'archived'

// Per-child completion on the quest list: a single entry for child-targeted
// quests, one per active child (name-ordered) otherwise.
export interface QuestProgress {
  childId: string
  name: string
  correct: number
  total: number
  done: boolean
}

export interface Quest {
  id: string
  name: string
  emoji?: string
  skillId: string
  childId: string // "" = all children
  status: QuestStatus
  questionCount: number
  createdAt: string
  progress?: QuestProgress[] // present on the family list endpoint
}

export interface QuestQuestion {
  id: string
  position: number
  text: string
  answer: string
  answerType: string
  format: 'numeric' | 'multiple_choice'
  choices?: string[]
  hint?: string
  explanation?: string
  generated: boolean // saved by AI generation — review me
}

export interface QuestQuestionInput {
  text: string
  answer: string
  answerType: string
  format: string
  choices: string[]
  hint: string
  explanation: string
}

// Saving a question can succeed with a non-empty warning (the math checker
// computed a different answer — probably a typo in the answer key).
export interface QuestQuestionResult {
  question: QuestQuestion
  warning: string
}

// ---- Activity timeline (parent dashboard) ----
// A per-child feed of expeditions, mastery milestones, and micro-lessons,
// paged newest-first by event sequence number.

export interface ActivitySkillRef {
  id: string
  name: string
}

export interface ActivityQuestRef {
  id: string
  name: string
  emoji: string
  createdBy: string
}

export interface ActivityExpedition {
  sessionId: string
  questions: number
  correct: number
  durationSecs: number
  gems: number
  skills: ActivitySkillRef[]
  quest?: ActivityQuestRef
  // Why the engine picked this expedition (may be absent on older events):
  // frontier = a new skill, review = spaced-rep re-check, booster = an
  // easier confidence run.
  category?: 'frontier' | 'review' | 'booster'
}

export interface ActivityMastery {
  skillId: string
  skillName: string
  fromState: string
  toState: string
}

export interface ActivityLesson {
  skillId: string
  skillName: string
  title: string
}

export interface ActivityItem {
  kind: 'expedition' | 'mastery' | 'lesson'
  seq: number
  at: string
  expedition?: ActivityExpedition
  mastery?: ActivityMastery
  lesson?: ActivityLesson
}

export interface ActivityFeed {
  items: ActivityItem[]
  nextBefore?: number | null
}

export interface ActivitySessionAnswer {
  seq: number
  at: string
  skillId: string
  skillName: string
  questionText: string
  learnerAnswer: string
  correctAnswer: string
  correct: boolean
  timeMs: number
}

export interface ActivitySessionDetail {
  answers: ActivitySessionAnswer[]
  hintCount: number
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export interface BillingPlan {
  id: string
  name: string
  priceUsdCents: number
  monthlyCredits?: number
  topupCredits?: number
  blurb: string
}

export interface BillingInfo {
  balance: number
  plan: string
  status: string
  periodEnd?: string
  plans: BillingPlan[]
}

// Public pricing catalog (GET /api/v1/pricing — served even when the server
// runs without a billing provider; billingEnabled drives beta messaging).
export interface PricingInfo {
  billingEnabled: boolean
  starterCredits: number
  plans: BillingPlan[]
}

// ---- Global API activity (drives the BusyBar) ----
// Module-level in-flight counter: every request() bumps it, and subscribers
// are notified only on 0↔1 transitions, so the UI sees "anything in flight?"
// rather than individual calls.

let inflight = 0
const activityListeners = new Set<(active: boolean) => void>()

export function subscribeApiActivity(cb: (active: boolean) => void): () => void {
  activityListeners.add(cb)
  return () => {
    activityListeners.delete(cb)
  }
}

function notifyActivity(active: boolean) {
  for (const cb of activityListeners) cb(active)
}

export async function request<T>(
  method: string,
  path: string,
  token: string | null,
  body?: unknown,
): Promise<T> {
  inflight++
  if (inflight === 1) notifyActivity(true)
  try {
    const headers: Record<string, string> = {}
    if (token) headers['Authorization'] = `Bearer ${token}`
    if (body !== undefined) headers['Content-Type'] = 'application/json'
    const resp = await fetch(path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
    if (resp.status === 204) return undefined as T
    const data = await resp.json().catch(() => ({}))
    if (!resp.ok) {
      throw new ApiError(resp.status, (data as { error?: string }).error ?? `HTTP ${resp.status}`)
    }
    return data as T
  } finally {
    inflight--
    if (inflight === 0) notifyActivity(false)
  }
}

export const api = {
  bootConfig: () => request<BootConfig>('GET', '/api/v1/config', null),
  // Public: plan catalog + beta flag for the /pricing page (no auth).
  pricing: () => request<PricingInfo>('GET', '/api/v1/pricing', null),
  // Public: the human-readable skill graph (no auth). Prefer
  // cachedCurriculum() below — the catalog is static per binary.
  curriculum: () => request<CurriculumInfo>('GET', '/api/v1/curriculum', null),

  // Parent (Supabase JWT)
  me: (token: string) =>
    request<{
      account: Account
      family: FamilySpace | null
      role?: string // 'owner' | 'parent', present only with a family
      pendingInvite?: PendingParentInvite // present only without a family
    }>('GET', '/api/v1/me', token),
  createFamily: (token: string, name: string) =>
    request<FamilySpace>('POST', '/api/v1/family', token, { name }),
  renameFamily: (token: string, familyId: string, name: string) =>
    request<FamilySpace>('PATCH', `/api/v1/family/${familyId}`, token, { name }),
  listChildren: (token: string, familyId: string) =>
    request<{ children: ChildWithSummary[] }>('GET', `/api/v1/family/${familyId}/children`, token),
  addChild: (token: string, familyId: string, name: string, grade: number, pin: string) =>
    request<ChildProfile>('POST', `/api/v1/family/${familyId}/children`, token, { name, grade, pin }),
  updateChild: (
    token: string,
    childId: string,
    patch: Partial<{ name: string; grade: number; pin: string; archived: boolean }>,
  ) => request<ChildProfile>('PATCH', `/api/v1/children/${childId}`, token, patch),
  childStats: (token: string, childId: string) =>
    request<ChildStats>('GET', `/api/v1/children/${childId}/stats`, token),
  createInvite: (token: string, familyId: string, ttlHours = 0) =>
    request<Invite>('POST', `/api/v1/family/${familyId}/invites`, token, { ttlHours }),
  listInvites: (token: string, familyId: string) =>
    request<{ invites: Invite[] }>('GET', `/api/v1/family/${familyId}/invites`, token),
  revokeInvite: (token: string, inviteId: string) =>
    request<void>('DELETE', `/api/v1/invites/${inviteId}`, token),
  // Co-parents (reading the roster is open to any member; invite/revoke/
  // remove are owner-only — the server enforces, the UI just hides them)
  listParents: (token: string, familyId: string) =>
    request<{ parents: ParentMember[]; invites: ParentInvite[] }>(
      'GET',
      `/api/v1/family/${familyId}/parents`,
      token,
    ),
  inviteParent: (token: string, familyId: string, email: string) =>
    request<ParentInvite>('POST', `/api/v1/family/${familyId}/parents`, token, { email }),
  removeParent: (token: string, familyId: string, accountId: string) =>
    request<void>('DELETE', `/api/v1/family/${familyId}/parents/${accountId}`, token),
  revokeParentInvite: (token: string, inviteId: string) =>
    request<void>('DELETE', `/api/v1/parent-invites/${inviteId}`, token),
  acceptParentInvite: (token: string, inviteId: string) =>
    request<{ family: FamilySpace; role: string }>(
      'POST',
      `/api/v1/invites/parent/${inviteId}/accept`,
      token,
    ),
  // Activity timeline (see specs — GET .../activity pages newest-first by seq)
  activity: (
    token: string,
    childId: string,
    opts?: {
      before?: number
      limit?: number
      kinds?: string[]
      from?: string
      to?: string
      // Only expedition items for this quest UID (kinds are moot when set).
      quest?: string
    },
  ) => {
    const params = new URLSearchParams()
    if (opts?.before !== undefined) params.set('before', String(opts.before))
    if (opts?.limit !== undefined) params.set('limit', String(opts.limit))
    if (opts?.kinds && opts.kinds.length > 0) params.set('kinds', opts.kinds.join(','))
    if (opts?.quest) params.set('quest', opts.quest)
    if (opts?.from) params.set('from', opts.from)
    if (opts?.to) params.set('to', opts.to)
    const qs = params.toString()
    return request<ActivityFeed>(
      'GET',
      `/api/v1/children/${childId}/activity${qs ? `?${qs}` : ''}`,
      token,
    )
  },
  activitySession: (token: string, childId: string, sessionId: string) =>
    request<ActivitySessionDetail>(
      'GET',
      `/api/v1/children/${childId}/activity/sessions/${sessionId}`,
      token,
    ),
  listDevices: (token: string, childId: string) =>
    request<{ devices: Device[] }>('GET', `/api/v1/children/${childId}/devices`, token),
  revokeDevice: (token: string, deviceId: string) =>
    request<void>('DELETE', `/api/v1/devices/${deviceId}`, token),

  // Parent quests (404s when the server runs without quests)
  createQuest: (
    token: string,
    familyId: string,
    input: { name: string; emoji: string; skillId: string; childId: string },
  ) => request<Quest>('POST', `/api/v1/family/${familyId}/quests`, token, input),
  listQuests: (token: string, familyId: string) =>
    request<{ quests: Quest[] }>('GET', `/api/v1/family/${familyId}/quests`, token),
  getQuest: (token: string, questId: string) =>
    request<{ quest: Quest; questions: QuestQuestion[] }>('GET', `/api/v1/quests/${questId}`, token),
  updateQuest: (
    token: string,
    questId: string,
    patch: Partial<{ name: string; emoji: string; skillId: string; childId: string; status: string }>,
  ) => request<Quest>('PATCH', `/api/v1/quests/${questId}`, token, patch),
  deleteQuest: (token: string, questId: string) =>
    request<void>('DELETE', `/api/v1/quests/${questId}`, token),
  publishQuest: (token: string, questId: string) =>
    request<Quest>('POST', `/api/v1/quests/${questId}/publish`, token),
  addQuestQuestion: (token: string, questId: string, input: QuestQuestionInput) =>
    request<QuestQuestionResult>('POST', `/api/v1/quests/${questId}/questions`, token, input),
  updateQuestQuestion: (token: string, questId: string, qid: string, input: QuestQuestionInput) =>
    request<QuestQuestionResult>('PATCH', `/api/v1/quests/${questId}/questions/${qid}`, token, input),
  deleteQuestQuestion: (token: string, questId: string, qid: string) =>
    request<void>('DELETE', `/api/v1/quests/${questId}/questions/${qid}`, token),
  generateQuestQuestions: (
    token: string,
    questId: string,
    brief: string,
    count: number,
    clientKey: string,
  ) =>
    request<{ questions: QuestQuestion[]; replayed: boolean }>(
      'POST',
      `/api/v1/quests/${questId}/generate`,
      token,
      { brief, count, clientKey },
    ),

  // Billing (404s when the server runs without a billing provider)
  billing: (token: string, familyId: string) =>
    request<BillingInfo>('GET', `/api/v1/family/${familyId}/billing`, token),
  billingCheckout: (token: string, familyId: string, planId: string) =>
    request<{ url: string }>('POST', `/api/v1/family/${familyId}/billing/checkout`, token, { planId }),
  billingPortal: (token: string, familyId: string) =>
    request<{ url: string }>('POST', `/api/v1/family/${familyId}/billing/portal`, token),

  // Child join flow (public)
  joinPreview: (code: string) =>
    request<{ familyName: string; children: ChildProfile[] }>('POST', '/api/v1/join/preview', null, {
      code,
    }),
  // familyId (both responses below): the child's own family space UID — used
  // only as the family-level analytics group key (never any child identity).
  joinRedeem: (code: string, childProfileId: string, pin: string, deviceLabel: string) =>
    request<{ token: string; child: ChildProfile; familyId: string }>(
      'POST',
      '/api/v1/join/redeem',
      null,
      {
        code,
        childProfileId,
        pin,
        deviceLabel,
      },
    ),

  // Child (device token)
  childMe: (deviceToken: string) =>
    request<{ profile: ChildProfile; familyName: string; familyId: string }>(
      'GET',
      '/api/v1/child/me',
      deviceToken,
    ),
}

// cachedCurriculum shares one curriculum fetch across the whole SPA — the
// catalog is static per server binary, so every consumer (quest skill
// pickers, /how-it-works, /dashboard/curriculum) reuses the same promise.
// A failed fetch clears the cache so the next consumer can retry.
let curriculumPromise: Promise<CurriculumInfo> | null = null
export function cachedCurriculum(): Promise<CurriculumInfo> {
  curriculumPromise ??= api.curriculum().catch((err) => {
    curriculumPromise = null
    throw err
  })
  return curriculumPromise
}

// Device token persistence for the kid's browser.
const DEVICE_TOKEN_KEY = 'mathiz.deviceToken'
export const deviceToken = {
  get: () => localStorage.getItem(DEVICE_TOKEN_KEY),
  set: (t: string) => localStorage.setItem(DEVICE_TOKEN_KEY, t),
  clear: () => localStorage.removeItem(DEVICE_TOKEN_KEY),
}
