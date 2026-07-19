import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type BillingPlan, type PricingInfo } from '../api'
import Skeleton from '../components/Skeleton'

// Public pricing page (/pricing). Static-feeling and Supabase-free like the
// legal pages: the catalog comes from GET /api/v1/pricing, which is served
// even when the server runs without a billing provider. billingEnabled=false
// (the public beta) swaps in a "everything is free right now" banner — no
// frontend change needed when billing flips on in prod.

// Fallback for the copy when the catalog fetch fails — matches
// credits.StarterCredits server-side.
const STARTER_FALLBACK = 30

export default function Pricing() {
  const [info, setInfo] = useState<PricingInfo | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api
      .pricing()
      .then(setInfo)
      // Fetch failure: keep the static explainer + trust lines, drop the
      // cards. A marketing page never shows an error wall.
      .catch(() => setInfo(null))
      .finally(() => setLoading(false))
  }, [])

  const starter = info?.starterCredits ?? STARTER_FALLBACK

  return (
    <div className="pricing-page">
      <Link to="/" className="brand legal-brand">
        <span className="brand-mark">∑</span>
        <span className="legal-brand-name">Mathiz</span>
      </Link>

      <h1 className="pricing-title">Simple, honest pricing</h1>

      {info && !info.billingEnabled && (
        <p className="pricing-beta">
          ⛵ Mathiz is in public beta — everything is free right now. These are
          the plans we expect to launch.
        </p>
      )}

      <section className="pricing-explainer">
        <h2>What's an expedition?</h2>
        <p>
          1 credit = 1 expedition: 5 AI-generated questions made just for your
          child, with hints and micro-lessons included. Every family starts
          with {starter} free expeditions.
        </p>
      </section>

      {loading && (
        <div className="pricing-plans" aria-hidden="true">
          <div className="plan-grid">
            {[0, 1, 2].map((i) => (
              <div key={i} className="plan-card">
                <Skeleton width="5rem" height="0.95rem" />
                <Skeleton width="3.5rem" height="1.4rem" />
                <Skeleton width="100%" height="0.7rem" />
                <Skeleton width="100%" height="0.7rem" />
              </div>
            ))}
          </div>
        </div>
      )}

      {info && Array.isArray(info.plans) && <PlanCards plans={info.plans} />}

      <ul className="pricing-trust">
        <li>Cancel anytime</li>
        <li>Unused top-up expeditions never expire</li>
        <li>Kids never see prices or paywalls — ever</li>
      </ul>

      <Link to="/login" className="btn btn-primary pricing-cta">
        Start free →
      </Link>

      <p className="legal-back muted">
        <Link to="/">← Back to Mathiz</Link>
      </p>
    </div>
  )
}

// The catalog: three subscriptions prominent, the top-up pack visually
// secondary underneath.
function PlanCards({ plans }: { plans: BillingPlan[] }) {
  const subs = plans.filter((p) => p.monthlyCredits)
  const topups = plans.filter((p) => p.topupCredits)

  return (
    <div className="pricing-plans">
      <div className="plan-grid">
        {subs.map((p) => (
          <div key={p.id} className="plan-card">
            <strong>{p.name}</strong>
            <span className="plan-price">${(p.priceUsdCents / 100).toFixed(0)}/mo</span>
            <span className="pricing-credits">
              {p.monthlyCredits} expeditions / month
            </span>
            <span className="muted plan-blurb">{p.blurb}</span>
          </div>
        ))}
      </div>
      {topups.map((p) => (
        <div key={p.id} className="pricing-topup">
          <strong>{p.name}</strong>
          <span className="pricing-topup-price">
            ${(p.priceUsdCents / 100).toFixed(0)} · {p.topupCredits} expeditions
          </span>
          <span className="muted">{p.blurb}</span>
        </div>
      ))}
    </div>
  )
}
