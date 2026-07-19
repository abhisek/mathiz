import { useEffect, useState } from 'react'
import { Link, Navigate, useSearchParams } from 'react-router-dom'
import { api, type BillingInfo } from '../../api'
import Skeleton from '../../components/Skeleton'
import { useDashboard } from './context'

// Billing — owner-only. A co-parent landing here (deep link, stale tab) is
// bounced to the kids page; the server enforces the same rule with 404s.
export default function Billing() {
  const { token, family, role } = useDashboard()
  const [searchParams, setSearchParams] = useSearchParams()
  // Checkout success lands on this route (?billing=success). Read the flag
  // once, then scrub it from the URL so a refresh doesn't re-celebrate.
  const [justPaid, setJustPaid] = useState(false)

  useEffect(() => {
    if (searchParams.get('billing') !== 'success') return
    setJustPaid(true)
    const next = new URLSearchParams(searchParams)
    next.delete('billing')
    setSearchParams(next, { replace: true })
  }, [searchParams, setSearchParams])

  if (role !== 'owner') {
    return <Navigate to="/dashboard" replace />
  }

  return (
    <div className="family">
      {justPaid && (
        <div className="tip-card">
          ✅ Payment received — the expeditions are being loaded aboard. The
          balance below updates as soon as the payment provider confirms.
        </div>
      )}
      <BillingCard token={token} familyId={family.id} />
    </div>
  )
}

// BillingCard shows the expedition wallet. When the server runs without
// billing (public beta / self-hosted free mode → the endpoint 404s) it shows
// a friendly beta card instead; the plans guard also covers any malformed
// response so a billing hiccup can never blank the whole dashboard.
function BillingCard({ token, familyId }: { token: string; familyId: string }) {
  const [info, setInfo] = useState<BillingInfo | null>(null)
  const [hidden, setHidden] = useState(false)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .billing(token, familyId)
      .then(setInfo)
      .catch(() => setHidden(true))
      .finally(() => setLoading(false))
  }, [token, familyId])

  if (hidden) {
    return (
      <section className="billing">
        <div className="beta-card">
          <h3>You're in the public beta 🎉</h3>
          <p className="muted">
            Mathiz is free while we polish — your family has unlimited
            expeditions during the beta.
          </p>
          <Link to="/pricing">See what pricing will look like →</Link>
        </div>
      </section>
    )
  }

  // First load: anonymous skeleton shaped like the wallet + plan tiles (no
  // heading — self-hosted free mode hides this section entirely).
  if (loading) {
    return (
      <section className="billing" aria-hidden="true">
        <div className="section-head">
          <Skeleton width="9rem" height="1.2rem" />
        </div>
        <div className="wallet">
          <Skeleton width="4.5rem" height="2.1rem" />
          <Skeleton width="11rem" height="0.8rem" />
        </div>
        <div className="plan-grid">
          {[0, 1, 2].map((i) => (
            <div key={i} className="plan-card">
              <Skeleton width="5rem" height="0.95rem" />
              <Skeleton width="3.5rem" height="1.4rem" />
              <Skeleton width="100%" height="0.7rem" />
              <Skeleton width="100%" height="2.2rem" />
            </div>
          ))}
        </div>
      </section>
    )
  }

  if (!info || !Array.isArray(info.plans)) return null

  const currentPlan = info.plans.find((p) => p.id === info.plan)

  async function buy(planId: string) {
    setBusy(planId)
    setError(null)
    try {
      const { url } = await api.billingCheckout(token, familyId, planId)
      window.location.href = url
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(null)
    }
  }

  async function portal() {
    try {
      const { url } = await api.billingPortal(token, familyId)
      window.location.href = url
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <section className="billing">
      <div className="section-head">
        <h3>Expedition wallet</h3>
        {info.status === 'active' && (
          <button className="btn btn-ghost" onClick={() => void portal()}>
            Manage billing
          </button>
        )}
      </div>
      <div className="wallet">
        <span className="wallet-balance">⛵ {info.balance}</span>
        <span className="muted">
          expeditions left
          {info.status === 'active' && currentPlan
            ? ` · ${currentPlan.name} plan${info.periodEnd ? `, renews ${new Date(info.periodEnd).toLocaleDateString()}` : ''}`
            : ' · no plan yet'}
        </span>
      </div>
      {info.balance <= 10 && (
        <p className="wallet-low">
          Running low — pick a plan so the crew can keep exploring.
        </p>
      )}
      {error && <p className="form-error">{error}</p>}
      <div className="plan-grid">
        {info.plans.map((p) => (
          <div key={p.id} className={`plan-card${p.id === info.plan ? ' plan-current' : ''}`}>
            <strong>{p.name}</strong>
            <span className="plan-price">
              ${(p.priceUsdCents / 100).toFixed(0)}
              {p.monthlyCredits ? '/mo' : ''}
            </span>
            <span className="muted plan-blurb">{p.blurb}</span>
            <button
              className={p.monthlyCredits ? 'btn btn-primary' : 'btn btn-secondary'}
              disabled={busy !== null || p.id === info.plan}
              onClick={() => void buy(p.id)}
            >
              {p.id === info.plan ? 'Current plan' : busy === p.id ? 'Opening…' : p.monthlyCredits ? 'Subscribe' : 'Buy pack'}
            </button>
          </div>
        ))}
      </div>
    </section>
  )
}
