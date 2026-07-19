import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  api,
  type ActivityItem,
  type ActivitySessionDetail,
} from '../../api'
import { useAction } from '../../hooks'
import Skeleton from '../../components/Skeleton'
import { useDashboard } from './context'

const PAGE_SIZE = 30

// Kind toggles: UI label → API kind value.
const KIND_TOGGLES = [
  { kind: 'expedition', label: 'Expeditions' },
  { kind: 'lesson', label: 'Lessons' },
  { kind: 'mastery', label: 'Milestones' },
] as const

type RangeKey = '7' | '30' | 'all'

// Activity — a per-child timeline of expeditions, micro-lessons, and
// mastery milestones, newest first, paged by the seq cursor.
export default function Activity() {
  const { token, children, childrenLoading } = useDashboard()
  const kids = useMemo(
    () => children.filter((c) => !c.profile.archived).map((c) => c.profile),
    [children],
  )

  const [childId, setChildId] = useState<string | null>(null)
  // First child is the default pick once the roster lands.
  useEffect(() => {
    if (childId && kids.some((k) => k.id === childId)) return
    setChildId(kids[0]?.id ?? null)
  }, [kids, childId])

  const [kinds, setKinds] = useState<string[]>(KIND_TOGGLES.map((t) => t.kind))
  const [range, setRange] = useState<RangeKey>('30')

  const [items, setItems] = useState<ActivityItem[]>([])
  const [nextBefore, setNextBefore] = useState<number | null>(null)
  // Expanded-expedition detail cache, keyed by `${childId}:${sessionId}` —
  // survives filter changes and "load more" so a re-expand never refetches,
  // and the childId prefix means a (however unlikely) session-ID collision
  // between siblings can never show one child's answers under the other.
  const [details, setDetails] = useState<Record<string, ActivitySessionDetail>>({})
  // True while the first fetch for the CURRENT child+filters is in flight —
  // changing any filter starts a fresh timeline (and fresh skeletons).
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fromParam = useMemo(() => {
    if (range === 'all') return undefined
    const days = range === '7' ? 7 : 30
    return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString()
  }, [range])

  const kindsKey = kinds.join(',')

  // Bumped whenever child/kind/range changes so an in-flight "Load more"
  // started under the OLD filters can never splice its stale page into the
  // fresh timeline (the first-page effect below has its own `cancelled`
  // guard; this covers the loadMore path, which outlives the effect).
  const fetchGeneration = useRef(0)

  useEffect(() => {
    fetchGeneration.current += 1
    if (!childId || kinds.length === 0) {
      setItems([])
      setNextBefore(null)
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    setItems([])
    setNextBefore(null)
    setError(null)
    api
      .activity(token, childId, {
        limit: PAGE_SIZE,
        // All kinds on = no filter param — the server default.
        kinds: kinds.length === KIND_TOGGLES.length ? undefined : kinds,
        from: fromParam,
      })
      .then((res) => {
        if (cancelled) return
        setItems(res.items ?? [])
        setNextBefore(res.nextBefore ?? null)
      })
      .catch((err) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, childId, kindsKey, fromParam])

  const [loadMore, loadingMore] = useAction(async () => {
    if (!childId || nextBefore === null) return
    const generation = fetchGeneration.current
    try {
      const res = await api.activity(token, childId, {
        before: nextBefore,
        limit: PAGE_SIZE,
        kinds: kinds.length === KIND_TOGGLES.length ? undefined : kinds,
        from: fromParam,
      })
      // Filters changed while this page was in flight — drop it.
      if (fetchGeneration.current !== generation) return
      setItems((cur) => [...cur, ...(res.items ?? [])])
      setNextBefore(res.nextBefore ?? null)
      setError(null)
    } catch (err) {
      if (fetchGeneration.current !== generation) return
      setError(err instanceof Error ? err.message : String(err))
    }
  })

  function toggleKind(kind: string) {
    setKinds((cur) =>
      cur.includes(kind) ? cur.filter((k) => k !== kind) : [...cur, kind],
    )
  }

  if (!childrenLoading && kids.length === 0) {
    return (
      <section className="activity">
        <div className="section-head">
          <h3>Activity</h3>
        </div>
        <div className="empty">
          <p>
            No explorers aboard yet — <Link to="/dashboard">add a child</Link> and
            their adventures will show up here.
          </p>
        </div>
      </section>
    )
  }

  return (
    <section className="activity">
      <div className="section-head">
        <h3>Activity</h3>
      </div>

      <div className="activity-chips">
        {childrenLoading &&
          [0, 1].map((i) => <Skeleton key={i} width="6rem" height="2rem" />)}
        {kids.map((k) => (
          <button
            key={k.id}
            className={`chip${childId === k.id ? ' chip-active' : ''}`}
            onClick={() => setChildId(k.id)}
          >
            {k.name}
          </button>
        ))}
      </div>

      <div className="activity-filters">
        <div className="activity-kinds">
          {KIND_TOGGLES.map((t) => (
            <button
              key={t.kind}
              className={`chip chip-small${kinds.includes(t.kind) ? ' chip-active' : ''}`}
              aria-pressed={kinds.includes(t.kind)}
              onClick={() => toggleKind(t.kind)}
            >
              {t.label}
            </button>
          ))}
        </div>
        <label className="invite-ttl">
          Showing{' '}
          <select value={range} onChange={(e) => setRange(e.target.value as RangeKey)}>
            <option value="7">last 7 days</option>
            <option value="30">last 30 days</option>
            <option value="all">all time</option>
          </select>
        </label>
      </div>

      {error && <p className="form-error">{error}</p>}

      {loading || childrenLoading ? (
        <div className="timeline" aria-hidden="true">
          {[0, 1, 2].map((i) => (
            <div key={i} className="timeline-row" style={{ cursor: 'default' }}>
              <Skeleton width="3rem" height="0.8rem" />
              <Skeleton circle width="1.6rem" height="1.6rem" />
              <div className="timeline-main" style={{ gap: '0.35rem' }}>
                <Skeleton width="12rem" height="0.95rem" />
                <Skeleton width="9rem" height="0.7rem" />
              </div>
            </div>
          ))}
        </div>
      ) : kinds.length === 0 ? (
        <p className="muted">Pick at least one activity type above.</p>
      ) : items.length === 0 ? (
        <div className="empty">
          <p>
            Nothing here {range === 'all' ? 'yet' : 'in this period'} — quiet seas.
            Adventures show up as soon as they happen.
          </p>
        </div>
      ) : (
        <>
          <Timeline
            token={token}
            childId={childId!}
            items={items}
            details={details}
            onDetail={(cacheKey, detail) =>
              setDetails((cur) => ({ ...cur, [cacheKey]: detail }))
            }
          />
          {nextBefore !== null && (
            <div className="activity-more">
              <button
                className="btn btn-secondary"
                disabled={loadingMore}
                onClick={() => void loadMore()}
              >
                {loadingMore ? 'Loading…' : 'Load more'}
              </button>
            </div>
          )}
        </>
      )}
    </section>
  )
}

// ---- Timeline rendering ----

function dayLabel(at: string): string {
  const d = new Date(at)
  const today = new Date()
  const yesterday = new Date(today)
  yesterday.setDate(today.getDate() - 1)
  if (d.toDateString() === today.toDateString()) return 'Today'
  if (d.toDateString() === yesterday.toDateString()) return 'Yesterday'
  const opts: Intl.DateTimeFormatOptions = {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  }
  if (d.getFullYear() !== today.getFullYear()) opts.year = 'numeric'
  return d.toLocaleDateString(undefined, opts)
}

function timeLabel(at: string): string {
  return new Date(at).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

function durationLabel(secs: number): string {
  if (secs < 60) return `${secs}s`
  return `${Math.round(secs / 60)} min`
}

interface DetailCacheProps {
  // Keyed by `${childId}:${sessionId}` (see the cache in Activity).
  details: Record<string, ActivitySessionDetail>
  onDetail: (cacheKey: string, detail: ActivitySessionDetail) => void
}

function Timeline({
  token,
  childId,
  items,
  details,
  onDetail,
}: {
  token: string
  childId: string
  items: ActivityItem[]
} & DetailCacheProps) {
  // Group consecutive items (already newest-first) under day headings.
  const groups: { label: string; items: ActivityItem[] }[] = []
  for (const item of items) {
    const label = dayLabel(item.at)
    const last = groups[groups.length - 1]
    if (last && last.label === label) last.items.push(item)
    else groups.push({ label, items: [item] })
  }

  return (
    <div className="timeline">
      {groups.map((g) => (
        <div key={`${g.label}-${g.items[0].seq}`} className="timeline-day">
          <h4 className="timeline-day-head">{g.label}</h4>
          {g.items.map((item) => (
            <TimelineRow
              key={`${item.kind}-${item.seq}`}
              token={token}
              childId={childId}
              item={item}
              details={details}
              onDetail={onDetail}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

function TimelineRow({
  token,
  childId,
  item,
  details,
  onDetail,
}: {
  token: string
  childId: string
  item: ActivityItem
} & DetailCacheProps) {
  if (item.kind === 'expedition' && item.expedition) {
    return (
      <ExpeditionRow
        token={token}
        childId={childId}
        item={item}
        details={details}
        onDetail={onDetail}
      />
    )
  }
  if (item.kind === 'mastery' && item.mastery) {
    const m = item.mastery
    const rusty = m.toState === 'rusty'
    const mastered = m.toState === 'mastered'
    return (
      <div className="timeline-row timeline-milestone">
        <span className="timeline-time muted">{timeLabel(item.at)}</span>
        <span className="timeline-icon" aria-hidden="true">
          {mastered ? '🏆' : rusty ? '🌧️' : '🧭'}
        </span>
        <div className="timeline-main">
          <strong>
            {mastered
              ? `Mastered ${m.skillName}`
              : rusty
                ? `${m.skillName} got rusty — review due`
                : `${m.skillName}: ${m.fromState} → ${m.toState}`}
          </strong>
        </div>
      </div>
    )
  }
  if (item.kind === 'lesson' && item.lesson) {
    const l = item.lesson
    return (
      <div className="timeline-row">
        <span className="timeline-time muted">{timeLabel(item.at)}</span>
        <span className="timeline-icon" aria-hidden="true">
          📖
        </span>
        <div className="timeline-main">
          <strong>Guide's lesson: {l.title}</strong>
          <span className="muted">{l.skillName}</span>
        </div>
      </div>
    )
  }
  return null
}

function ExpeditionRow({
  token,
  childId,
  item,
  details,
  onDetail,
}: {
  token: string
  childId: string
  item: ActivityItem
} & DetailCacheProps) {
  const exp = item.expedition!
  const [open, setOpen] = useState(false)
  const [detailError, setDetailError] = useState<string | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const cacheKey = `${childId}:${exp.sessionId}`
  const detail = details[cacheKey] ?? null

  // Lazy-fetch the per-question detail on first expand; the page-level
  // cache (keyed by child + session) means a re-expand never refetches.
  function toggle() {
    const next = !open
    setOpen(next)
    if (!next || detail || detailLoading) return
    setDetailLoading(true)
    setDetailError(null)
    api
      .activitySession(token, childId, exp.sessionId)
      .then((d) => onDetail(cacheKey, d))
      .catch((err) => setDetailError(err instanceof Error ? err.message : String(err)))
      .finally(() => setDetailLoading(false))
  }

  const what = exp.quest
    ? `${exp.quest.emoji || '⭐'} ${exp.quest.name}`
    : exp.skills.map((s) => s.name).join(' · ') || 'Expedition'

  return (
    <div className={`timeline-expedition${open ? ' open' : ''}`}>
      <button className="timeline-row timeline-row-btn" onClick={toggle} aria-expanded={open}>
        <span className="timeline-time muted">{timeLabel(item.at)}</span>
        <span className="timeline-icon" aria-hidden="true">
          ⛵
        </span>
        <div className="timeline-main">
          <strong>
            {what}
            {exp.quest && (
              <span className="muted timeline-quest-by"> quest by {exp.quest.createdBy}</span>
            )}
          </strong>
          <span className="muted">
            {exp.correct}/{exp.questions} correct · {durationLabel(exp.durationSecs)} · 💎
            {exp.gems}
          </span>
        </div>
        <span className="timeline-caret" aria-hidden="true">
          {open ? '▾' : '▸'}
        </span>
      </button>

      {open && (
        <div className="timeline-detail">
          {detailLoading && (
            <div aria-hidden="true">
              {[0, 1].map((i) => (
                <div key={i} className="timeline-answer">
                  <Skeleton width="80%" height="0.9rem" />
                  <Skeleton width="40%" height="0.75rem" />
                </div>
              ))}
            </div>
          )}
          {detailError && <p className="form-error">{detailError}</p>}
          {detail && (
            <>
              <ul className="timeline-answers">
                {detail.answers.map((a) => (
                  <li key={a.seq} className="timeline-answer">
                    <span className="question-text">{a.questionText}</span>
                    <span className={a.correct ? 'answer-good' : 'answer-bad'}>
                      {a.correct ? '✓' : '✗'} {a.learnerAnswer || '—'}
                      {!a.correct && (
                        <span className="muted"> · correct: {a.correctAnswer}</span>
                      )}
                      <span className="muted"> · {Math.round(a.timeMs / 1000)}s</span>
                    </span>
                    <span className="muted timeline-answer-skill">{a.skillName}</span>
                  </li>
                ))}
              </ul>
              {detail.hintCount > 0 && (
                <p className="muted timeline-hints">
                  💡 {detail.hintCount} {detail.hintCount === 1 ? 'hint' : 'hints'} used
                </p>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
