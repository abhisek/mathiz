import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { cachedCurriculum, type CurriculumIsland } from '../api'
import { ensureAnalyticsBooted, track } from '../analytics'
import Skeleton from '../components/Skeleton'

// Public "How Mathiz teaches" page (/how-it-works). Static-feeling and
// Supabase-free like the legal/pricing pages. Reading experience is
// progressive disclosure: a 4-step TL;DR strip, then per-section one-line
// gists with the fuller story folded behind native <details>. Skimmers get
// the strip; readers get the depth.
//
// Staleness rule: NOTHING here may hardcode curriculum facts (grade ranges,
// island names or counts, skill counts) — the outline and every number
// derive from GET /api/v1/curriculum, so seed changes update this page
// automatically.
export default function HowItWorks() {
  useEffect(() => {
    void ensureAnalyticsBooted('public').then(() => track.howItWorksViewed())
  }, [])

  return (
    <div className="hiw-page">
      <Link to="/" className="brand legal-brand">
        <span className="brand-mark">∑</span>
        <span className="legal-brand-name">Mathiz</span>
      </Link>

      <h1 className="hiw-title">How Mathiz teaches</h1>

      {/* The 10-second version. */}
      <div className="hiw-steps">
        <div className="hiw-step">
          <span className="hiw-step-icon" aria-hidden>
            ✨
          </span>
          Every question generated fresh for your child
        </div>
        <div className="hiw-step">
          <span className="hiw-step-icon" aria-hidden>
            🏝️
          </span>
          Skills mastered by doing, on a treasure map
        </div>
        <div className="hiw-step">
          <span className="hiw-step-icon" aria-hidden>
            🔄
          </span>
          Re-checked days later so it sticks
        </div>
        <div className="hiw-step">
          <span className="hiw-step-icon" aria-hidden>
            🧭
          </span>
          You can steer with quests
        </div>
      </div>

      <section className="hiw-section">
        <h2>🎲 Not another worksheet</h2>
        <p>
          There is no question bank — every single question is generated fresh
          for <em>your</em> child, from their level and their recent mistakes.
        </p>
        <details className="hiw-more">
          <summary>The longer story</summary>
          <p>
            Two kids on the same skill get different questions; the same kid
            never sees a rerun. Each question is built from what the tutor
            knows right now — the skills in progress, the errors made this
            week, the pace that fits. It's the difference between a stack of
            photocopies and a tutor who knows where your child got stuck
            yesterday.
          </p>
        </details>
      </section>

      <section className="hiw-section">
        <h2>🧭 The mastery journey</h2>
        <p>
          Skills move from <strong>learning</strong> to <strong>mastered</strong>{' '}
          as your child proves them — and mastered skills get quietly
          re-checked later, like a good tutor circling back.
        </p>
        <details className="hiw-more">
          <summary>The longer story</summary>
          <p>
            Proving a skill happens in two stages: first with hints allowed,
            then a short round without them. Mastered doesn't mean forgotten
            about — Mathiz re-checks mastered skills on a growing rhythm
            (1, 3, 7, 14, 30, then 60 days). If one has faded it's marked{' '}
            <strong>rusty</strong> and comes back for review before moving on.
            That rhythm is what makes practice stick instead of wash out.
          </p>
        </details>
      </section>

      <section className="hiw-section">
        <h2>⛵ What an expedition is</h2>
        <p>
          Practice happens in short expeditions — 5 questions on one skill,
          launched from a spot on the map. Wrong answers earn hints, not
          buzzers.
        </p>
        <details className="hiw-more">
          <summary>The longer story</summary>
          <p>
            Two misses and the guide steps in with a micro-lesson: a plain
            explanation, a worked example, and a practice question — then the
            expedition continues. Master the skill and the chest opens, the
            fog lifts, and new islands come into reach. Gems and streaks keep
            it feeling like a game; the engine underneath keeps it honest.
          </p>
        </details>
      </section>

      <section className="hiw-section">
        <h2>🧑‍✈️ You can steer</h2>
        <p>
          The engine picks what's next, but you hold the wheel when you want
          it: create a <strong>quest</strong> and it lands on your child's map
          as a special spot.
        </p>
        <details className="hiw-more">
          <summary>The longer story</summary>
          <p>
            Want a specific topic covered this week? From your dashboard,
            write the questions yourself or let the AI draft them for your
            review — nothing reaches your child unapproved. To them it's just
            more treasure. <Link to="/login">Sign in</Link> to try it.
          </p>
        </details>
      </section>

      <section className="hiw-section">
        <h2>🗺️ What Mathiz covers</h2>
        <p>
          The treasure map is a real curriculum, organized into islands that
          mirror how math is taught at school — each skill unlocking once its
          prerequisites are mastered. Everything it covers today:
        </p>
        <CurriculumOutline />
      </section>

      <Link to="/login" className="btn btn-primary hiw-cta">
        Start free →
      </Link>

      <p className="legal-back muted">
        <Link to="/">← Back to Mathiz</Link> · <Link to="/pricing">Pricing</Link>
      </p>
    </div>
  )
}

// CurriculumOutline renders the islands as accordions: name + skill count +
// grade range visible collapsed, skills by grade band on expand. Everything
// (names, counts, grade ranges) derives from the API — no hardcoded
// curriculum facts anywhere on this page. Skeleton on first load; a generic
// (fact-free) note if the fetch fails so the page never shows an error wall
// or stale hardcoded claims.
function CurriculumOutline() {
  const [islands, setIslands] = useState<CurriculumIsland[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    cachedCurriculum()
      .then((c) => {
        if (!cancelled) setIslands(c.islands ?? [])
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (failed) {
    return (
      <p className="muted">
        The curriculum outline couldn't load just now — refresh to see every
        island and skill Mathiz teaches.
      </p>
    )
  }

  if (islands === null) {
    return (
      <div className="hiw-islands" aria-hidden="true">
        {[0, 1, 2].map((i) => (
          <div key={i} className="hiw-island">
            <Skeleton width="12rem" height="1rem" />
            <Skeleton width="100%" height="0.75rem" />
          </div>
        ))}
      </div>
    )
  }

  return (
    <div className="hiw-islands">
      {islands.map((island, idx) => {
        // Group the island's skills (already grade-ordered) by grade band.
        const grades: { grade: number; names: string[] }[] = []
        for (const s of island.skills) {
          const last = grades[grades.length - 1]
          if (last && last.grade === s.grade) last.names.push(s.name)
          else grades.push({ grade: s.grade, names: [s.name] })
        }
        const gradeSpan =
          grades.length > 1
            ? `grades ${grades[0].grade}–${grades[grades.length - 1].grade}`
            : `grade ${grades[0]?.grade ?? ''}`
        return (
          // First island open by way of invitation; the rest are a click away.
          <details key={island.id} className="hiw-island" open={idx === 0}>
            <summary>
              <h3>{island.name}</h3>
              <span className="muted">
                {island.skills.length} skills · {gradeSpan}
              </span>
            </summary>
            {grades.map((g) => (
              <p key={g.grade} className="hiw-grade-row">
                <span className="hiw-grade">Grade {g.grade}</span>
                {g.names.join(' · ')}
              </p>
            ))}
          </details>
        )
      })}
    </div>
  )
}
