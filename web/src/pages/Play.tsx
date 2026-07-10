import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, deviceToken } from '../api'
import {
  gameApi,
  GameApiError,
  type AnswerResult,
  type Expedition,
  type GameMap,
  type Lesson,
  type LessonGrade,
  type Notebook,
  type NotebookTip,
  type Question,
  type Spot,
} from '../game'

// The treasure map: the skill graph as islands. Solving AI-generated math
// digs treasure, collects gems, and lifts the fog on new territory.

type Phase = 'idle' | 'starting' | 'loading' | 'question' | 'feedback' | 'lesson' | 'summary'

const GEM_META: Record<string, { icon: string; label: string }> = {
  mastery: { icon: '🏆', label: 'Mastery gems' },
  streak: { icon: '🔥', label: 'Streak gems' },
  session: { icon: '⛵', label: 'Expedition gems' },
  recovery: { icon: '💪', label: 'Comeback gems' },
  retention: { icon: '🛡️', label: 'Keeper gems' },
}

const SPOT_ICON: Record<string, string> = {
  locked: '🌫️',
  ready: '✕',
  digging: '⛏️',
  proving: '🏅',
  treasure: '💰',
  sinking: '🌊',
}

const SPOT_LABEL: Record<string, string> = {
  locked: 'Hidden in the fog',
  ready: 'X marks the spot — dig here!',
  digging: 'Keep digging!',
  proving: 'Prove it to open the chest!',
  treasure: 'Treasure secured!',
  sinking: 'Treasure sinking — rescue it!',
}

export default function Play() {
  const navigate = useNavigate()
  const [childName, setChildName] = useState('')
  const [map, setMap] = useState<GameMap | null>(null)
  const [mapError, setMapError] = useState<string | null>(null)

  // Expedition state machine.
  const [phase, setPhase] = useState<Phase>('idle')
  const [expedition, setExpedition] = useState<Expedition | null>(null)
  const [question, setQuestion] = useState<Question | null>(null)
  const [result, setResult] = useState<AnswerResult | null>(null)
  const [hint, setHint] = useState<string | null>(null)
  const [lesson, setLesson] = useState<Lesson | null>(null)
  const [lessonGrade, setLessonGrade] = useState<LessonGrade | null>(null)
  const [vaultOpen, setVaultOpen] = useState(false)
  const [notebook, setNotebook] = useState<Notebook | null>(null)
  const [notebookOpen, setNotebookOpen] = useState(false)
  const [shipResting, setShipResting] = useState(false)
  const [expError, setExpError] = useState<string | null>(null)

  const refreshMap = useCallback(async () => {
    try {
      setMap(await gameApi.map())
      setMapError(null)
    } catch (err) {
      setMapError(err instanceof Error ? err.message : String(err))
    }
  }, [])

  useEffect(() => {
    if (!deviceToken.get()) {
      navigate('/join')
      return
    }
    void api
      .childMe(deviceToken.get()!)
      .then((me) => setChildName(me.profile.name))
      .catch(() => {
        deviceToken.clear()
        navigate('/join')
      })
    void refreshMap()
  }, [navigate, refreshMap])

  async function dig(spot: Spot) {
    if (spot.state === 'locked' || phase !== 'idle') return
    setPhase('starting')
    setExpError(null)
    setResult(null)
    setHint(null)
    try {
      const exp = await gameApi.start(spot.id)
      setExpedition(exp)
      await nextQuestion(exp.id)
    } catch (err) {
      setPhase('idle')
      if (err instanceof GameApiError && err.status === 402) {
        // Out of credits: kid-friendly, no prices, no meter.
        setShipResting(true)
        return
      }
      setExpError(err instanceof Error ? err.message : String(err))
    }
  }

  async function nextQuestion(expId: string) {
    setPhase('loading')
    setResult(null)
    setHint(null)
    setLesson(null)
    setLessonGrade(null)
    try {
      const q = await gameApi.question(expId)
      setQuestion(q)
      setPhase('question')
    } catch (err) {
      setExpError(err instanceof Error ? err.message : String(err))
      setPhase('idle')
      setExpedition(null)
      await refreshMap()
    }
  }

  async function submit(answer: string) {
    if (!expedition) return
    setPhase('loading')
    try {
      const res = await gameApi.answer(expedition.id, answer)
      setResult(res)
      setPhase(res.done ? 'summary' : 'feedback')
      if (res.done) await refreshMap()
    } catch (err) {
      setExpError(err instanceof Error ? err.message : String(err))
      setPhase('question')
    }
  }

  async function showHint() {
    if (!expedition) return
    try {
      const h = await gameApi.hint(expedition.id)
      setHint(h.hint)
    } catch {
      // hint already used — ignore
    }
  }

  // openLesson polls until the guide finishes writing (or gives up and
  // moves on — lessons are best-effort, never a blocker).
  async function openLesson() {
    if (!expedition) return
    setPhase('lesson')
    setLesson(null)
    setLessonGrade(null)
    for (let attempt = 0; attempt < 8; attempt++) {
      try {
        const l = await gameApi.lesson(expedition.id)
        if (l.ready) {
          setLesson(l)
          return
        }
      } catch {
        break // lesson no longer available
      }
      await new Promise((r) => setTimeout(r, 1200))
    }
    await nextQuestion(expedition.id)
  }

  async function submitLesson(answer: string, skip: boolean) {
    if (!expedition) return
    try {
      setLessonGrade(await gameApi.answerLesson(expedition.id, answer, skip))
    } catch {
      await nextQuestion(expedition.id)
    }
  }

  async function toggleNotebook() {
    if (notebookOpen) {
      setNotebookOpen(false)
      return
    }
    setVaultOpen(false)
    setNotebookOpen(true)
    try {
      setNotebook(await gameApi.notebook())
    } catch {
      setNotebook({ tips: [] })
    }
  }

  async function sailHome() {
    if (expedition && phase !== 'summary') {
      try {
        await gameApi.end(expedition.id)
      } catch {
        // already finished server-side
      }
    }
    setExpedition(null)
    setQuestion(null)
    setResult(null)
    setPhase('idle')
    await refreshMap()
  }

  return (
    <div className="game-page">
      <header className="game-bar">
        <div className="brand brand-small brand-dark">
          <span className="brand-mark">∑</span>
          <span>Mathiz</span>
          {childName && <span className="game-captain">Captain {childName}</span>}
        </div>
        <div className="game-bar-right">
          <button className="gem-counter" onClick={() => void toggleNotebook()}>
            🧭
          </button>
          <button
            className="gem-counter"
            onClick={() => {
              setNotebookOpen(false)
              setVaultOpen((v) => !v)
            }}
          >
            💎 {map?.gems.total ?? 0}
          </button>
          <button
            className="btn btn-ghost btn-ghost-dark"
            onClick={() => {
              deviceToken.clear()
              navigate('/join')
            }}
          >
            Switch player
          </button>
        </div>
      </header>

      {mapError && <p className="form-error game-error">{mapError}</p>}
      {expError && <p className="form-error game-error">{expError}</p>}

      {shipResting && (
        <div className="expedition-backdrop" onClick={() => setShipResting(false)}>
          <div className="expedition rest-card" onClick={(e) => e.stopPropagation()}>
            <div className="summary-big">⛵💤</div>
            <h3>The ship needs to rest!</h3>
            <p>
              You've explored so much today. Ask your grown-up to send the ship
              back out on more expeditions.
            </p>
            <button className="btn btn-kid btn-block" onClick={() => setShipResting(false)}>
              Back to the map
            </button>
          </div>
        </div>
      )}

      {notebookOpen && (
        <NotebookDrawer notebook={notebook} onClose={() => setNotebookOpen(false)} />
      )}

      {vaultOpen && map && (
        <div className="vault">
          <h3>💎 Your gem vault</h3>
          {map.gems.total === 0 ? (
            <p className="vault-empty">No gems yet — go dig some treasure!</p>
          ) : (
            <ul>
              {Object.entries(map.gems.byType)
                .sort(([, a], [, b]) => b - a)
                .map(([type, count]) => (
                  <li key={type}>
                    <span>{GEM_META[type]?.icon ?? '💎'}</span>
                    <span>{GEM_META[type]?.label ?? type}</span>
                    <strong>× {count}</strong>
                  </li>
                ))}
            </ul>
          )}
        </div>
      )}

      <main className="sea">
        {!map && !mapError && <div className="boot boot-dark">Charting the map…</div>}
        {map?.islands.map((island) => (
          <section key={island.id} className="island">
            <h2 className="island-name">🏝️ {island.name}</h2>
            <div className="spots">
              {island.spots.map((spot) => (
                <button
                  key={spot.id}
                  className={`spot spot-${spot.state}`}
                  onClick={() => void dig(spot)}
                  disabled={spot.state === 'locked' || phase !== 'idle'}
                  title={`${spot.name} — ${SPOT_LABEL[spot.state]}`}
                >
                  <span className="spot-marker">
                    {spot.state === 'digging' || spot.state === 'proving' ? (
                      <ProgressRing progress={spot.progress} icon={SPOT_ICON[spot.state]} />
                    ) : (
                      <span className="spot-icon">{SPOT_ICON[spot.state]}</span>
                    )}
                  </span>
                  <span className="spot-name">{spot.name}</span>
                  <span className="spot-grade">G{spot.grade}</span>
                </button>
              ))}
            </div>
          </section>
        ))}
      </main>

      {phase !== 'idle' && (
        <ExpeditionOverlay
          phase={phase}
          expedition={expedition}
          question={question}
          result={result}
          hint={hint}
          lesson={lesson}
          lessonGrade={lessonGrade}
          onSubmit={submit}
          onHint={showHint}
          onLesson={() => void openLesson()}
          onLessonAnswer={(a, skip) => void submitLesson(a, skip)}
          onNext={() => expedition && void nextQuestion(expedition.id)}
          onClose={() => void sailHome()}
        />
      )}
    </div>
  )
}

// NotebookDrawer shows every tip the guide has given, grouped by island.
function NotebookDrawer({
  notebook,
  onClose,
}: {
  notebook: Notebook | null
  onClose: () => void
}) {
  const [openTip, setOpenTip] = useState<string | null>(null)

  const byIsland = new Map<string, NotebookTip[]>()
  for (const tip of notebook?.tips ?? []) {
    const key = tip.islandName || 'Somewhere at sea'
    byIsland.set(key, [...(byIsland.get(key) ?? []), tip])
  }

  return (
    <div className="notebook">
      <div className="notebook-head">
        <h3>🧭 The guide's notebook</h3>
        <button className="btn btn-ghost" onClick={onClose}>
          Close
        </button>
      </div>
      {!notebook && <p className="vault-empty">Opening the notebook…</p>}
      {notebook && notebook.tips.length === 0 && (
        <p className="vault-empty">
          No tips yet! The guide writes one down whenever a spot gets tricky.
        </p>
      )}
      {[...byIsland.entries()].map(([island, tips]) => (
        <div key={island} className="notebook-island">
          <h4>🏝️ {island}</h4>
          {tips.map((tip, i) => {
            const key = `${island}-${i}`
            const open = openTip === key
            return (
              <div key={key} className="notebook-tip">
                <button
                  className="notebook-tip-head"
                  onClick={() => setOpenTip(open ? null : key)}
                >
                  <strong>{tip.title}</strong>
                  <span className="muted">{tip.skillName}</span>
                </button>
                {open && (
                  <div className="notebook-tip-body">
                    <p>{tip.explanation}</p>
                    {tip.workedExample && <div className="worked">{tip.workedExample}</div>}
                    {tip.practiceText && (
                      <p className="muted">
                        {tip.practiceText}{' '}
                        {tip.practiceAnswer && <strong>→ {tip.practiceAnswer}</strong>}
                      </p>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}

function ProgressRing({ progress, icon }: { progress: number; icon: string }) {
  const r = 26
  const c = 2 * Math.PI * r
  return (
    <span className="ring-wrap">
      <svg viewBox="0 0 64 64" className="ring">
        <circle cx="32" cy="32" r={r} className="ring-bg" />
        <circle
          cx="32"
          cy="32"
          r={r}
          className="ring-fg"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - Math.max(0.05, progress))}
        />
      </svg>
      <span className="spot-icon ring-icon">{icon}</span>
    </span>
  )
}

function ExpeditionOverlay({
  phase,
  expedition,
  question,
  result,
  hint,
  lesson,
  lessonGrade,
  onSubmit,
  onHint,
  onLesson,
  onLessonAnswer,
  onNext,
  onClose,
}: {
  phase: Phase
  expedition: Expedition | null
  question: Question | null
  result: AnswerResult | null
  hint: string | null
  lesson: Lesson | null
  lessonGrade: LessonGrade | null
  onSubmit: (answer: string) => void
  onHint: () => void
  onLesson: () => void
  onLessonAnswer: (answer: string, skip: boolean) => void
  onNext: () => void
  onClose: () => void
}) {
  const [answer, setAnswer] = useState('')
  const [practiceAnswer, setPracticeAnswer] = useState('')
  const [secondsLeft, setSecondsLeft] = useState<number | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (phase === 'question') {
      setAnswer('')
      inputRef.current?.focus()
    }
  }, [phase, question])

  // Prove-tier countdown: a nudge for speed, never a guillotine — answers
  // are accepted after it runs out.
  useEffect(() => {
    if (phase !== 'question' || !question?.timeLimitSecs) {
      setSecondsLeft(null)
      return
    }
    setSecondsLeft(question.timeLimitSecs)
    const timer = setInterval(() => {
      setSecondsLeft((s) => (s === null || s <= 0 ? s : s - 1))
    }, 1000)
    return () => clearInterval(timer)
  }, [phase, question])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (answer.trim()) onSubmit(answer.trim())
  }

  return (
    <div className="expedition-backdrop">
      <div className="expedition">
        <div className="expedition-head">
          <strong>⛏️ {expedition?.skillName ?? 'Expedition'}</strong>
          <button className="btn btn-ghost btn-ghost-dark" onClick={onClose}>
            Sail home
          </button>
        </div>

        {question && phase !== 'summary' && (
          <div className="quest-dots">
            {Array.from({ length: question.total }, (_, i) => (
              <span
                key={i}
                className={`dot${i < question.index - 1 ? ' dot-done' : ''}${
                  i === question.index - 1 ? ' dot-now' : ''
                }`}
              />
            ))}
          </div>
        )}

        {(phase === 'starting' || phase === 'loading') && (
          <div className="quest-loading">
            <span className="compass">🧭</span>
            <p>{phase === 'starting' ? 'Setting sail…' : 'Consulting the map…'}</p>
          </div>
        )}

        {phase === 'question' && question && (
          <div className="quest">
            {question.timeLimitSecs && secondsLeft !== null ? (
              <div className="timer">
                <div
                  className={`timer-bar${secondsLeft <= 5 ? ' timer-low' : ''}`}
                  style={{ width: `${(secondsLeft / question.timeLimitSecs) * 100}%` }}
                />
                {secondsLeft === 0 && (
                  <p className="timer-up">⏰ Time's up — give it your best guess!</p>
                )}
              </div>
            ) : null}
            <p className="quest-text">{question.text}</p>
            {question.format === 'multiple_choice' && question.choices ? (
              <div className="choice-grid">
                {question.choices.map((c, i) => (
                  <button key={i} className="choice" onClick={() => onSubmit(c)}>
                    {c}
                  </button>
                ))}
              </div>
            ) : (
              <form onSubmit={handleSubmit} className="answer-form">
                <input
                  ref={inputRef}
                  className="answer-input"
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                  placeholder="?"
                  inputMode={question.answerType === 'fraction' ? 'text' : 'decimal'}
                  autoComplete="off"
                />
                <button className="btn btn-kid" disabled={!answer.trim()}>
                  Dig! ⛏️
                </button>
              </form>
            )}
          </div>
        )}

        {phase === 'feedback' && result && (
          <div className={`feedback ${result.correct ? 'feedback-yes' : 'feedback-no'}`}>
            {result.correct ? (
              <>
                <div className="feedback-big">
                  {result.streak >= 3 ? `🔥 ${result.streak} in a row!` : '✨ Treasure found!'}
                </div>
                {result.gem && (
                  <div className="gem-pop">
                    💎 You earned a <strong>{result.gem.rarity}</strong> gem!
                  </div>
                )}
              </>
            ) : (
              <>
                <div className="feedback-big">🌊 Not quite!</div>
                <p className="feedback-answer">
                  The treasure was <strong>{result.correctAnswer}</strong>
                </p>
                {result.explanation && <p className="feedback-explain">{result.explanation}</p>}
                {result.hintAvailable && !hint && (
                  <button className="btn btn-secondary" onClick={onHint}>
                    🗺️ Show me a clue
                  </button>
                )}
                {hint && <p className="hint-box">🗺️ {hint}</p>}
              </>
            )}
            {result.mastery && result.mastery.to !== 'mastered' && result.mastery.from === 'learning' && (
              <p className="tier-up">🗝️ You found the vault — now prove it to open the chest!</p>
            )}
            {result.lessonPending && !result.done ? (
              <>
                <button className="btn btn-kid btn-block" onClick={onLesson}>
                  🧭 The guide has a tip for you!
                </button>
                <button className="linklike" onClick={onNext}>
                  Skip the tip →
                </button>
              </>
            ) : (
              <button className="btn btn-kid btn-block" onClick={onNext}>
                Next clue →
              </button>
            )}
          </div>
        )}

        {phase === 'lesson' && !lesson && (
          <div className="quest-loading">
            <span className="compass">🧭</span>
            <p>The guide is drawing a picture for you…</p>
          </div>
        )}

        {phase === 'lesson' && lesson && (
          <div className="lesson">
            <h3>🧭 {lesson.title}</h3>
            <p className="lesson-explain">{lesson.explanation}</p>
            {lesson.workedExample && <div className="worked">{lesson.workedExample}</div>}

            {lesson.practice && !lessonGrade && (
              <form
                onSubmit={(e) => {
                  e.preventDefault()
                  if (practiceAnswer.trim()) onLessonAnswer(practiceAnswer.trim(), false)
                }}
              >
                <div className="lesson-practice">
                  <p className="quest-text">{lesson.practice.text}</p>
                  <div className="answer-form">
                    <input
                      className="answer-input"
                      value={practiceAnswer}
                      onChange={(e) => setPracticeAnswer(e.target.value)}
                      placeholder="?"
                      inputMode="decimal"
                      autoComplete="off"
                      autoFocus
                    />
                    <button className="btn btn-kid" disabled={!practiceAnswer.trim()}>
                      Try it!
                    </button>
                  </div>
                  <button
                    type="button"
                    className="linklike"
                    onClick={() => onLessonAnswer('', true)}
                  >
                    Skip practice →
                  </button>
                </div>
              </form>
            )}

            {lessonGrade && (
              <div className={`feedback ${lessonGrade.correct ? 'feedback-yes' : 'feedback-no'}`}>
                <div className="feedback-big">
                  {lessonGrade.correct ? '🌟 You got it!' : '💙 Good try!'}
                </div>
                {!lessonGrade.correct && lessonGrade.correctAnswer && (
                  <p className="feedback-answer">
                    The answer was <strong>{lessonGrade.correctAnswer}</strong>
                  </p>
                )}
                {lessonGrade.explanation && (
                  <p className="feedback-explain">{lessonGrade.explanation}</p>
                )}
                <button className="btn btn-kid btn-block" onClick={onNext}>
                  Back to the hunt →
                </button>
              </div>
            )}
          </div>
        )}

        {phase === 'summary' && result?.summary && (
          <div className="summary">
            {result.summary.mastered ? (
              <>
                <div className="summary-big chest-open">💰</div>
                <h3>Treasure chest opened!</h3>
                {result.unlockedSkillIds && result.unlockedSkillIds.length > 0 && (
                  <p className="unlock-note">
                    🗺️ The fog lifted on {result.unlockedSkillIds.length} new{' '}
                    {result.unlockedSkillIds.length === 1 ? 'spot' : 'spots'}!
                  </p>
                )}
              </>
            ) : (
              <>
                <div className="summary-big">⛵</div>
                <h3>Expedition complete!</h3>
              </>
            )}
            <p>
              You dug up <strong>{result.summary.correct}</strong> of{' '}
              <strong>{result.summary.questions}</strong> treasures.
            </p>
            {result.summary.gems && result.summary.gems.length > 0 && (
              <div className="summary-gems">
                {result.summary.gems.map((g, i) => (
                  <span key={i} className={`gem-chip gem-${g.rarity}`}>
                    💎 {g.rarity} {g.type}
                  </span>
                ))}
              </div>
            )}
            <button className="btn btn-kid btn-block" onClick={onClose}>
              Back to the map
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
