// Typed client for the treasure-map game API.
import { deviceToken } from './api'

export type SpotState = 'locked' | 'ready' | 'digging' | 'proving' | 'treasure' | 'sinking'

export interface Spot {
  id: string
  name: string
  description: string
  grade: number
  prerequisites: string[]
  state: SpotState
  progress: number
  reviewDue: boolean
}

export interface Island {
  id: string
  name: string
  spots: Spot[]
}

export interface GameMap {
  islands: Island[]
  gems: { total: number; byType: Record<string, number> }
}

export interface Expedition {
  id: string
  skillId: string
  skillName: string
  totalQuestions: number
  tier: 'learn' | 'prove'
  category: string
}

export interface Question {
  index: number
  total: number
  text: string
  format: 'numeric' | 'multiple_choice'
  choices?: string[]
  answerType: string
  tier: string
}

export interface GemAward {
  type: string
  rarity: string
  reason: string
}

export interface ExpeditionSummary {
  questions: number
  correct: number
  accuracy: number
  gems: GemAward[] | null
  mastered: boolean
}

export interface AnswerResult {
  correct: boolean
  correctAnswer: string
  explanation?: string
  hintAvailable: boolean
  streak: number
  gem?: GemAward
  mastery?: { from: string; to: string }
  unlockedSkillIds?: string[]
  questionsAnswered: number
  totalQuestions: number
  done: boolean
  summary?: ExpeditionSummary
}

class GameApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = deviceToken.get()
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const resp = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  const data = await resp.json().catch(() => ({}))
  if (!resp.ok) {
    throw new GameApiError(resp.status, (data as { error?: string }).error ?? `HTTP ${resp.status}`)
  }
  return data as T
}

export const gameApi = {
  map: () => call<GameMap>('GET', '/api/v1/game/map'),
  start: (skillId: string) => call<Expedition>('POST', '/api/v1/game/expeditions', { skillId }),
  question: (expId: string) => call<Question>('POST', `/api/v1/game/expeditions/${expId}/question`),
  answer: (expId: string, answer: string) =>
    call<AnswerResult>('POST', `/api/v1/game/expeditions/${expId}/answer`, { answer }),
  hint: (expId: string) => call<{ hint: string }>('POST', `/api/v1/game/expeditions/${expId}/hint`),
  end: (expId: string) => call<ExpeditionSummary>('POST', `/api/v1/game/expeditions/${expId}/end`),
}
