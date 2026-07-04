import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, deviceToken } from '../api'
import {
  gameApi,
  type AnswerResult,
  type Expedition,
  type GameMap,
  type Question,
  type Spot,
} from '../game'

// The treasure map: the skill graph as islands. Solving AI-generated math
// digs treasure, collects gems, and lifts the fog on new territory.

type Phase = 'idle' | 'starting' | 'loading' | 'question' | 'feedback' | 'summary'

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
      setExpError(err instanceof Error ? err.message : String(err))
      setPhase('idle')
    }
  }

  async function nextQuestion(expId: string) {
    setPhase('loading')
    setResult(null)
    setHint(null)
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
          <span className="gem-counter">💎 {map?.gems.total ?? 0}</span>
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
          onSubmit={submit}
          onHint={showHint}
          onNext={() => expedition && void nextQuestion(expedition.id)}
          onClose={() => void sailHome()}
        />
      )}
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
  onSubmit,
  onHint,
  onNext,
  onClose,
}: {
  phase: Phase
  expedition: Expedition | null
  question: Question | null
  result: AnswerResult | null
  hint: string | null
  onSubmit: (answer: string) => void
  onHint: () => void
  onNext: () => void
  onClose: () => void
}) {
  const [answer, setAnswer] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (phase === 'question') {
      setAnswer('')
      inputRef.current?.focus()
    }
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
            {result.mastery?.to === 'learning' && result.mastery.from === 'new' && null}
            {result.mastery && result.mastery.to !== 'mastered' && result.mastery.from === 'learning' && (
              <p className="tier-up">🗝️ You found the vault — now prove it to open the chest!</p>
            )}
            <button className="btn btn-kid btn-block" onClick={onNext}>
              Next clue →
            </button>
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
