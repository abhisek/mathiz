// Typed client for the treasure-map game API.
import { deviceToken, request } from './api'

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

// QuestMapItem is one parent-authored quest on the map, with progress.
// Kid-facing: never carries prices, balances, or any monetisation data.
export interface QuestMapItem {
  id: string
  name: string
  emoji?: string
  total: number
  correct: number
  done: boolean
}

export interface GameMap {
  islands: Island[]
  gems: { total: number; byType: Record<string, number> }
  quests?: QuestMapItem[]
}

export interface Expedition {
  id: string
  skillId: string
  skillName: string
  totalQuestions: number
  tier: 'learn' | 'prove'
  category: string
  questId?: string
}

export interface Question {
  index: number
  total: number
  text: string
  format: 'numeric' | 'multiple_choice'
  choices?: string[]
  answerType: string
  tier: string
  timeLimitSecs?: number
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
  questId?: string
  questComplete?: boolean
}

// correctAnswer/explanation are absent on a WRONG answer to a quest
// question: quest questions repeat until solved, so the server keeps the
// answer sealed until the question can never gate again. Map digs always
// carry both on a wrong answer.
export interface AnswerResult {
  correct: boolean
  correctAnswer?: string
  explanation?: string
  hintAvailable: boolean
  streak: number
  gem?: GemAward
  mastery?: { from: string; to: string }
  unlockedSkillIds?: string[]
  questionsAnswered: number
  totalQuestions: number
  done: boolean
  lessonPending?: boolean
  summary?: ExpeditionSummary
}

export interface Lesson {
  ready: boolean
  title?: string
  explanation?: string
  workedExample?: string
  practice?: { text: string; answerType: string }
}

export interface LessonGrade {
  correct: boolean
  correctAnswer: string
  explanation?: string
}

export interface NotebookTip {
  skillId: string
  skillName: string
  islandId: string
  islandName: string
  at: string
  title: string
  explanation: string
  workedExample?: string
  practiceText?: string
  practiceAnswer?: string
  practiceExplanation?: string
}

export interface Notebook {
  tips: NotebookTip[]
}

// One fetch wrapper for the whole SPA: game calls reuse api.ts's request
// (and its error type) with the device token instead of a parent JWT.
export { ApiError as GameApiError } from './api'

function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  return request<T>(method, path, deviceToken.get(), body)
}

export const gameApi = {
  map: () => call<GameMap>('GET', '/api/v1/game/map'),
  notebook: () => call<Notebook>('GET', '/api/v1/game/notebook'),
  start: (skillId: string) => call<Expedition>('POST', '/api/v1/game/expeditions', { skillId }),
  startQuest: (questId: string) =>
    call<Expedition>('POST', `/api/v1/game/quests/${questId}/expeditions`),
  question: (expId: string) => call<Question>('POST', `/api/v1/game/expeditions/${expId}/question`),
  answer: (expId: string, answer: string) =>
    call<AnswerResult>('POST', `/api/v1/game/expeditions/${expId}/answer`, { answer }),
  hint: (expId: string) => call<{ hint: string }>('POST', `/api/v1/game/expeditions/${expId}/hint`),
  lesson: (expId: string) => call<Lesson>('POST', `/api/v1/game/expeditions/${expId}/lesson`),
  answerLesson: (expId: string, answer: string, skip: boolean) =>
    call<LessonGrade>('POST', `/api/v1/game/expeditions/${expId}/lesson/answer`, { answer, skip }),
  end: (expId: string) => call<ExpeditionSummary>('POST', `/api/v1/game/expeditions/${expId}/end`),
}
