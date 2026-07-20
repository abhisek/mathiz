import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { cachedCurriculum, type CurriculumIsland } from '../api'
import { ensureAnalyticsBooted, track } from '../analytics'
import Skeleton from '../components/Skeleton'

// Public "How Mathiz teaches" page (/how-it-works). Static-feeling and
// Supabase-free like the legal/pricing pages: parent language, warm but
// concrete, ending in the full curriculum from GET /api/v1/curriculum
// (rendered from the shared cache; static fallback copy if the fetch fails
// so the page never shows an error wall).
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

      <section className="hiw-section">
        <h2>🎲 Not another worksheet</h2>
        <p>
          There is no question bank. Every single question is generated fresh
          for <em>your</em> child — from their current level, the skills they're
          working on, and the mistakes they've made recently. Two kids on the
          same skill get different questions; the same kid never sees a rerun.
          It's the difference between a stack of photocopies and a tutor who
          knows where your child got stuck yesterday.
        </p>
      </section>

      <section className="hiw-section">
        <h2>🧭 The mastery journey</h2>
        <p>
          Each skill moves from <strong>learning</strong> to{' '}
          <strong>mastered</strong> as your child proves they can do it — first
          with hints allowed, then in a short timed round without them. And
          mastered doesn't mean forgotten about: Mathiz quietly re-checks
          mastered skills days later, on a 1/3/7/14/30/60-day rhythm. If one
          has faded it's marked <strong>rusty</strong> and comes back for
          review — like a good tutor circling back to make sure it stuck.
        </p>
      </section>

      <section className="hiw-section">
        <h2>⛵ What an expedition is</h2>
        <p>
          Practice happens in short expeditions: 5 questions on one skill,
          launched from a spot on the treasure map. A wrong answer earns a
          hint, not a buzzer. Two misses and the guide steps in with a
          micro-lesson — a plain explanation, a worked example, and a practice
          question — before the expedition continues. Master the skill and the
          chest opens, the fog lifts, and new islands come into reach.
        </p>
      </section>

      <section className="hiw-section">
        <h2>🧑‍✈️ You can steer</h2>
        <p>
          The engine picks what's next, but you hold the wheel when you want
          it. Want HCF covered this week? Create a <strong>quest</strong> from
          your dashboard — write the questions yourself or let the AI draft
          them for your review — and it appears on your child's map as a
          special spot. To your child it's just more treasure;{' '}
          <Link to="/login">sign in</Link> to try it.
        </p>
      </section>

      <section className="hiw-section">
        <h2>🗺️ What Mathiz covers</h2>
        <p>
          The treasure map is a real curriculum: skills for US grades 2–5,
          organized into islands that mirror how math is taught at school —
          each skill unlocking once its prerequisites are mastered.
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

// CurriculumOutline renders the islands as headed groups with skills listed
// by grade band — compact and scannable, answering "does it cover what
// school does?". Skeleton on first load; static summary if the fetch fails.
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
    // Static fallback — matches the shipped skill graph's shape.
    return (
      <p className="muted">
        Five islands — Number &amp; Place Value, Addition &amp; Subtraction,
        Multiplication &amp; Division, Fractions, and Measurement — covering
        the core skills of US grades 2–5, from place value and number bonds
        through long division, equivalent fractions, and decimals.
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
            <Skeleton width="85%" height="0.75rem" />
          </div>
        ))}
      </div>
    )
  }

  return (
    <div className="hiw-islands">
      {islands.map((island) => {
        // Group the island's skills (already grade-ordered) by grade band.
        const grades: { grade: number; names: string[] }[] = []
        for (const s of island.skills) {
          const last = grades[grades.length - 1]
          if (last && last.grade === s.grade) last.names.push(s.name)
          else grades.push({ grade: s.grade, names: [s.name] })
        }
        return (
          <div key={island.id} className="hiw-island">
            <h3>{island.name}</h3>
            {grades.map((g) => (
              <p key={g.grade} className="hiw-grade-row">
                <span className="hiw-grade">Grade {g.grade}</span>
                {g.names.join(' · ')}
              </p>
            ))}
          </div>
        )
      })}
    </div>
  )
}
