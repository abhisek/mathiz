// Typed client for the Mathiz SaaS API.

export interface BootConfig {
  supabaseUrl: string
  supabaseAnonKey: string
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

// ---- Parent quests (specs/15-quests.md) ----

export type QuestStatus = 'draft' | 'active' | 'archived'

export interface Quest {
  id: string
  name: string
  emoji?: string
  skillId: string
  childId: string // "" = all children
  status: QuestStatus
  questionCount: number
  createdAt: string
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

  // Parent (Supabase JWT)
  me: (token: string) =>
    request<{ account: Account; family: FamilySpace | null }>('GET', '/api/v1/me', token),
  createFamily: (token: string, name: string) =>
    request<FamilySpace>('POST', '/api/v1/family', token, { name }),
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
  joinRedeem: (code: string, childProfileId: string, pin: string, deviceLabel: string) =>
    request<{ token: string; child: ChildProfile }>('POST', '/api/v1/join/redeem', null, {
      code,
      childProfileId,
      pin,
      deviceLabel,
    }),

  // Child (device token)
  childMe: (deviceToken: string) =>
    request<{ profile: ChildProfile; familyName: string }>('GET', '/api/v1/child/me', deviceToken),
}

// Device token persistence for the kid's browser.
const DEVICE_TOKEN_KEY = 'mathiz.deviceToken'
export const deviceToken = {
  get: () => localStorage.getItem(DEVICE_TOKEN_KEY),
  set: (t: string) => localStorage.setItem(DEVICE_TOKEN_KEY, t),
  clear: () => localStorage.removeItem(DEVICE_TOKEN_KEY),
}
