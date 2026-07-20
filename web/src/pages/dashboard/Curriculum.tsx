import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, cachedCurriculum, type CurriculumIsland } from '../../api'
import { track } from '../../analytics'
import Skeleton from '../../components/Skeleton'
import { useDashboard } from './context'

// Curriculum browser (/dashboard/curriculum): the full skill map as a list —
// islands as sections, every skill a row with the selected child's state
// (merged from the public curriculum + the child's mastery stats). Each row
// offers a quiet "Create quest →" jump into quest authoring with that skill
// preselected. Parent dashboard only — kids never see this framing.
export default function Curriculum() {
  const { token, children, childrenLoading } = useDashboard()
  const kids = useMemo(
    () => children.filter((c) => !c.profile.archived).map((c) => c.profile),
    [children],
  )

  useEffect(() => {
    track.curriculumViewed()
  }, [])

  // The catalog (shared module-level cache — static per server).
  const [islands, setIslands] = useState<CurriculumIsland[] | null>(null)
  const [curriculumError, setCurriculumError] = useState<string | null>(null)
  useEffect(() => {
    let cancelled = false
    cachedCurriculum()
      .then((c) => {
        if (!cancelled) setIslands(c.islands ?? [])
      })
      .catch((err) => {
        if (!cancelled)
          setCurriculumError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      cancelled = true
    }
  }, [])

  // Child chips — single-select, defaulting to the first child (Activity's
  // pattern).
  const [childId, setChildId] = useState<string | null>(null)
  useEffect(() => {
    if (childId && kids.some((k) => k.id === childId)) return
    setChildId(kids[0]?.id ?? null)
  }, [kids, childId])

  // Per-skill state for the selected child; a skill absent from the stats
  // is "not started". Guarded against the child changing mid-flight.
  const [skillStates, setSkillStates] = useState<Record<string, string>>({})
  const [statsLoading, setStatsLoading] = useState(true)
  const [statsError, setStatsError] = useState<string | null>(null)

  useEffect(() => {
    if (!childId) {
      setSkillStates({})
      setStatsLoading(false)
      setStatsError(null)
      return
    }
    let cancelled = false
    setStatsLoading(true)
    setStatsError(null)
    api
      .childStats(token, childId)
      .then((stats) => {
        if (cancelled) return
        const map: Record<string, string> = {}
        for (const s of stats.mastery.skills ?? []) map[s.id] = s.state
        setSkillStates(map)
      })
      .catch((err) => {
        if (cancelled) return
        setStatsError(err instanceof Error ? err.message : String(err))
        setSkillStates({})
      })
      .finally(() => {
        if (!cancelled) setStatsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token, childId])

  return (
    <section className="curriculum">
      <div className="section-head">
        <h3>Curriculum</h3>
      </div>
      <p className="muted">
        Every skill on the treasure map and where{' '}
        {kids.find((k) => k.id === childId)?.name ?? 'your child'} stands on it.
        Want one covered sooner? Create a quest right from its row.
      </p>

      <div className="activity-chips">
        {childrenLoading &&
          [0, 1].map((i) => <Skeleton key={i} width="6rem" height="2rem" />)}
        {kids.map((k) => (
          <button
            key={k.id}
            type="button"
            className={`chip${childId === k.id ? ' chip-active' : ''}`}
            aria-pressed={childId === k.id}
            onClick={() => setChildId(k.id)}
          >
            {k.name}
          </button>
        ))}
        {!childrenLoading && kids.length === 0 && (
          <p className="muted">
            No explorers aboard yet — <Link to="/dashboard">add a child</Link>{' '}
            to see their progress alongside each skill.
          </p>
        )}
      </div>

      {curriculumError && <p className="form-error">{curriculumError}</p>}
      {statsError && <p className="form-error">{statsError}</p>}

      {islands === null && !curriculumError ? (
        <div aria-hidden="true">
          {[0, 1].map((i) => (
            <div key={i} className="curriculum-island">
              <Skeleton width="14rem" height="1.1rem" />
              {[0, 1, 2].map((j) => (
                <div key={j} className="curriculum-row" style={{ cursor: 'default' }}>
                  <Skeleton width="11rem" height="0.9rem" />
                  <Skeleton width="5rem" height="1.3rem" />
                </div>
              ))}
            </div>
          ))}
        </div>
      ) : (
        islands?.map((island) => (
          <div key={island.id} className="curriculum-island">
            <h4 className="curriculum-island-head">{island.name}</h4>
            <ul className="curriculum-list">
              {island.skills.map((s) => (
                <li key={s.id} className="curriculum-row">
                  <span className="curriculum-skill">
                    {s.name}
                    <span className="muted curriculum-grade">grade {s.grade}</span>
                  </span>
                  {childId &&
                    (statsLoading ? (
                      <Skeleton width="5.5rem" height="1.3rem" />
                    ) : (
                      <StatePill state={skillStates[s.id]} />
                    ))}
                  <Link
                    className="curriculum-quest-link"
                    to={`/dashboard/quests?create=${encodeURIComponent(s.id)}`}
                  >
                    Create quest →
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))
      )}
    </section>
  )
}

// StatePill: the child's standing on one skill, in parent language. Any
// state we don't recognize (including absent) reads as not started.
function StatePill({ state }: { state?: string }) {
  switch (state) {
    case 'mastered':
      return <span className="curriculum-pill curriculum-pill-mastered">🏆 Mastered</span>
    case 'learning':
      return <span className="curriculum-pill curriculum-pill-learning">🌱 Learning</span>
    case 'rusty':
      return <span className="curriculum-pill curriculum-pill-rusty">🌧️ Rusty</span>
    default:
      return <span className="curriculum-pill curriculum-pill-none">Not started</span>
  }
}
