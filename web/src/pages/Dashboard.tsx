import { useCallback, useEffect, useState, type FormEvent } from 'react'
import type { Session } from '@supabase/supabase-js'
import {
  api,
  type BillingInfo,
  type ChildProfile,
  type ChildStats,
  type ChildWithSummary,
  type Device,
  type FamilySpace,
  type Invite,
} from '../api'
import { getSupabase } from '../supa'

interface Props {
  session: Session
}

export default function Dashboard({ session }: Props) {
  const token = session.access_token
  const [family, setFamily] = useState<FamilySpace | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const me = await api.me(token)
      setFamily(me.family)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoaded(true)
    }
  }, [token])

  useEffect(() => {
    void load()
  }, [load])

  async function signOut() {
    const supa = await getSupabase()
    await supa.auth.signOut()
  }

  if (!loaded) return <div className="boot">Loading…</div>

  return (
    <div className="page">
      <header className="topbar">
        <div className="brand brand-small">
          <span className="brand-mark">∑</span>
          <span>Mathiz</span>
        </div>
        <div className="topbar-right">
          <span className="muted">{session.user.email}</span>
          <button className="btn btn-ghost" onClick={signOut}>
            Sign out
          </button>
        </div>
      </header>

      {error && <p className="form-error">{error}</p>}

      {family ? (
        <FamilyView token={token} family={family} />
      ) : (
        <CreateFamily token={token} onCreated={load} />
      )}
    </div>
  )
}

function CreateFamily({ token, onCreated }: { token: string; onCreated: () => Promise<void> }) {
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.createFamily(token, name)
      await onCreated()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="center-card">
      <h2>Create your Family Space</h2>
      <p className="muted">
        A Family Space holds your children's profiles and their learning progress.
      </p>
      <form onSubmit={submit}>
        <label>
          Family name
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. The Sharmas"
            required
          />
        </label>
        {error && <p className="form-error">{error}</p>}
        <button className="btn btn-primary" disabled={busy}>
          {busy ? 'Creating…' : 'Create Family Space'}
        </button>
      </form>
    </div>
  )
}

function FamilyView({ token, family }: { token: string; family: FamilySpace }) {
  const [children, setChildren] = useState<ChildWithSummary[]>([])
  const [invites, setInvites] = useState<Invite[]>([])
  const [showAddChild, setShowAddChild] = useState(false)
  const [openChild, setOpenChild] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [kids, invs] = await Promise.all([
        api.listChildren(token, family.id),
        api.listInvites(token, family.id),
      ])
      setChildren(kids.children ?? [])
      setInvites(invs.invites ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [token, family.id])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const [inviteDays, setInviteDays] = useState(7)

  async function mintInvite() {
    try {
      await api.createInvite(token, family.id, inviteDays * 24)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  async function revokeInvite(id: string) {
    try {
      await api.revokeInvite(token, id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="family">
      <div className="family-header">
        <h2>{family.name}</h2>
        <button className="btn btn-primary" onClick={() => setShowAddChild(true)}>
          + Add child
        </button>
      </div>
      {error && <p className="form-error">{error}</p>}

      {(() => {
        const active = children.filter((c) => !c.profile.archived)
        const noPin = active.filter((c) => !c.profile.hasPin)
        if (active.length < 2 || noPin.length === 0) return null
        return (
          <div className="tip-card">
            🔒 With more than one explorer aboard, a PIN on each profile keeps
            siblings from mixing up each other's maps. Missing a PIN:{' '}
            <strong>{noPin.map((c) => c.profile.name).join(', ')}</strong> —
            open their card below to set one.
          </div>
        )
      })()}

      <section className="cards">
        {children.length === 0 && (
          <div className="empty">
            <p>No children yet. Add your first learner to get started!</p>
          </div>
        )}
        {children.map((c) => (
          <ChildCard
            key={c.profile.id}
            token={token}
            child={c}
            open={openChild === c.profile.id}
            onToggle={() =>
              setOpenChild(openChild === c.profile.id ? null : c.profile.id)
            }
            onChanged={refresh}
          />
        ))}
      </section>

      <BillingCard token={token} familyId={family.id} />

      <section className="invites">
        <div className="section-head">
          <h3>Join codes</h3>
          <div className="invite-mint">
            <label className="invite-ttl">
              Expires in{' '}
              <select
                value={inviteDays}
                onChange={(e) => setInviteDays(Number(e.target.value))}
              >
                <option value={7}>7 days</option>
                <option value={30}>30 days</option>
                <option value={90}>90 days</option>
              </select>
            </label>
            <button className="btn btn-secondary" onClick={mintInvite}>
              New join code
            </button>
          </div>
        </div>
        <p className="muted">
          Share a code with your child. They open <code>{window.location.origin}/join</code>,
          type the code, pick their name, and start learning — no email needed.
        </p>
        {invites.length === 0 ? (
          <p className="muted">No active codes.</p>
        ) : (
          <ul className="invite-list">
            {invites.map((inv) => (
              <li key={inv.id}>
                <code className="join-code">{inv.code}</code>
                <span className="muted">
                  expires {new Date(inv.expiresAt).toLocaleDateString()}
                </span>
                <button className="btn btn-ghost btn-danger" onClick={() => revokeInvite(inv.id)}>
                  Revoke
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {showAddChild && (
        <AddChildModal
          token={token}
          familyId={family.id}
          onClose={() => setShowAddChild(false)}
          onAdded={async () => {
            setShowAddChild(false)
            await refresh()
          }}
        />
      )}
    </div>
  )
}

// BillingCard shows the expedition wallet. Hidden entirely when the server
// runs without billing (self-hosted free mode → the endpoint 404s; the
// plans guard also covers any malformed response so a billing hiccup can
// never blank the whole dashboard).
function BillingCard({ token, familyId }: { token: string; familyId: string }) {
  const [info, setInfo] = useState<BillingInfo | null>(null)
  const [hidden, setHidden] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .billing(token, familyId)
      .then(setInfo)
      .catch(() => setHidden(true))
  }, [token, familyId])

  if (hidden || !info || !Array.isArray(info.plans)) return null

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

function AddChildModal({
  token,
  familyId,
  onClose,
  onAdded,
}: {
  token: string
  familyId: string
  onClose: () => void
  onAdded: () => Promise<void>
}) {
  const [name, setName] = useState('')
  const [grade, setGrade] = useState(3)
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.addChild(token, familyId, name, grade, pin)
      await onAdded()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h3>Add a child</h3>
        <form onSubmit={submit}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Grade
            <select value={grade} onChange={(e) => setGrade(Number(e.target.value))}>
              {[2, 3, 4, 5].map((g) => (
                <option key={g} value={g}>
                  Grade {g}
                </option>
              ))}
            </select>
          </label>
          <label>
            PIN <span className="muted">(optional, 4–6 digits — stops siblings swapping profiles)</span>
            <input
              value={pin}
              onChange={(e) => setPin(e.target.value)}
              inputMode="numeric"
              pattern="\d{4,6}"
              placeholder="e.g. 4321"
            />
          </label>
          {error && <p className="form-error">{error}</p>}
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>
              Cancel
            </button>
            <button className="btn btn-primary" disabled={busy}>
              {busy ? 'Adding…' : 'Add child'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ChildCard({
  token,
  child,
  open,
  onToggle,
  onChanged,
}: {
  token: string
  child: ChildWithSummary
  open: boolean
  onToggle: () => void
  onChanged: () => Promise<void>
}) {
  const { profile, summary } = child
  const [stats, setStats] = useState<ChildStats | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    void api.childStats(token, profile.id).then(setStats).catch(() => {})
    void api
      .listDevices(token, profile.id)
      .then((d) => setDevices(d.devices ?? []))
      .catch(() => {})
  }, [open, token, profile.id])

  async function revokeDevice(id: string) {
    try {
      await api.revokeDevice(token, id)
      const d = await api.listDevices(token, profile.id)
      setDevices(d.devices ?? [])
      setActionError(null)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }

  async function archive() {
    if (!window.confirm(`Archive ${profile.name}? Their devices will be signed out.`)) return
    try {
      await api.updateChild(token, profile.id, { archived: true })
      await onChanged()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  }

  const pct =
    summary.totalSkills > 0 ? Math.round((summary.masteredSkills / summary.totalSkills) * 100) : 0

  return (
    <div className={`child-card${open ? ' open' : ''}`}>
      <button className="child-card-head" onClick={onToggle}>
        <div className="avatar">{profile.name.charAt(0).toUpperCase()}</div>
        <div className="child-meta">
          <strong>{profile.name}</strong>
          <span className="muted">
            Grade {profile.grade}
            {!profile.hasPin && <span className="pin-chip">no PIN</span>}
          </span>
        </div>
        <div className="child-numbers">
          <span title="Skills mastered">🏆 {summary.masteredSkills}</span>
          <span title="Gems earned">💎 {summary.gems}</span>
        </div>
        <div className="progress">
          <div className="progress-bar" style={{ width: `${pct}%` }} />
        </div>
      </button>

      {open && (
        <div className="child-detail">
          {actionError && <p className="form-error">{actionError}</p>}
          {summary.lastSessionAt ? (
            <p className="muted">
              Last practice: {new Date(summary.lastSessionAt).toLocaleString()}
            </p>
          ) : (
            <p className="muted">No practice sessions yet.</p>
          )}

          {stats?.mastery.strands && (
            <div className="islands-progress">
              <h4>Island progress</h4>
              {stats.mastery.strands.map((s) => (
                <div key={s.id} className="island-row">
                  <span className="island-label">🏝️ {s.name}</span>
                  <div className="progress island-bar">
                    <div
                      className="progress-bar"
                      style={{ width: `${s.total > 0 ? (s.mastered / s.total) * 100 : 0}%` }}
                    />
                  </div>
                  <span className="muted">
                    {s.mastered}/{s.total}
                  </span>
                </div>
              ))}
            </div>
          )}

          {stats?.learnerProfile && (
            <div className="learner-profile">
              <h4>What the AI tutor has learned about {profile.name}</h4>
              <p>{stats.learnerProfile.summary}</p>
              {stats.learnerProfile.strengths?.length > 0 && (
                <p>
                  <strong>Strengths:</strong> {stats.learnerProfile.strengths.join(' · ')}
                </p>
              )}
              {stats.learnerProfile.weaknesses?.length > 0 && (
                <p>
                  <strong>Working on:</strong> {stats.learnerProfile.weaknesses.join(' · ')}
                </p>
              )}
            </div>
          )}

          {stats && stats.recentSessions.length > 0 && (
            <>
              <h4>Recent sessions</h4>
              <table className="sessions">
                <thead>
                  <tr>
                    <th>When</th>
                    <th>Questions</th>
                    <th>Correct</th>
                    <th>Gems</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.recentSessions.map((s, i) => (
                    <tr key={i}>
                      <td>{new Date(s.at).toLocaleString()}</td>
                      <td>{s.questions}</td>
                      <td>{s.correct}</td>
                      <td>{s.gems}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}

          <PinControl token={token} profile={profile} onChanged={onChanged} />

          <h4>Devices</h4>
          {devices.length === 0 ? (
            <p className="muted">No devices connected yet — share a join code.</p>
          ) : (
            <ul className="device-list">
              {devices.map((d) => (
                <li key={d.id}>
                  <span>{d.label || 'Unnamed device'}</span>
                  <span className="muted">
                    {d.lastUsedAt
                      ? `last used ${new Date(d.lastUsedAt).toLocaleDateString()}`
                      : 'never used'}
                  </span>
                  <button className="btn btn-ghost btn-danger" onClick={() => revokeDevice(d.id)}>
                    Sign out
                  </button>
                </li>
              ))}
            </ul>
          )}

          <div className="danger-zone">
            <button className="btn btn-ghost btn-danger" onClick={archive}>
              Archive {profile.name}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// Set / change a child's PIN after creation. Encouraged (nudge + chip),
// never forced.
function PinControl({
  token,
  profile,
  onChanged,
}: {
  token: string
  profile: ChildProfile
  onChanged: () => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function save(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api.updateChild(token, profile.id, { pin })
      await onChanged()
      setEditing(false)
      setPin('')
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="pin-control">
      <h4>Profile PIN</h4>
      {!editing ? (
        <p className="muted pin-status">
          {profile.hasPin ? (
            <>PIN is set — only {profile.name} can open this profile. </>
          ) : (
            <>
              No PIN — anyone with the join code can open {profile.name}'s
              profile.{' '}
            </>
          )}
          <button className="linklike" onClick={() => setEditing(true)}>
            {profile.hasPin ? 'Change PIN' : 'Set a PIN'}
          </button>
        </p>
      ) : (
        <form className="pin-form" onSubmit={save}>
          <input
            value={pin}
            onChange={(e) => setPin(e.target.value)}
            placeholder="4–6 digits"
            inputMode="numeric"
            pattern="[0-9]{4,6}"
            minLength={4}
            maxLength={6}
            required
            autoFocus
          />
          <button className="btn btn-primary" disabled={busy}>
            {busy ? 'Saving…' : 'Save PIN'}
          </button>
          <button
            className="btn btn-ghost"
            type="button"
            onClick={() => {
              setEditing(false)
              setPin('')
              setError(null)
            }}
          >
            Cancel
          </button>
          {error && <p className="form-error">{error}</p>}
        </form>
      )}
    </div>
  )
}
