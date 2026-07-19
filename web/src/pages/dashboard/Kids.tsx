import { useEffect, useState, type FormEvent } from 'react'
import { Navigate, useSearchParams } from 'react-router-dom'
import {
  api,
  type ChildProfile,
  type ChildStats,
  type ChildWithSummary,
  type Device,
} from '../../api'
import { track } from '../../analytics'
import { useAction } from '../../hooks'
import Skeleton from '../../components/Skeleton'
import { useDashboard } from './context'

// Kids — the dashboard index: child cards, add child, PIN nudge, rename.
export default function Kids() {
  const { token, family, children, childrenLoading, refreshChildren, refreshMe } = useDashboard()
  const [searchParams] = useSearchParams()
  const [showAddChild, setShowAddChild] = useState(false)
  const [openChild, setOpenChild] = useState<string | null>(null)

  // Grace for in-flight checkouts during deploy: the server used to redirect
  // checkout success to /dashboard — forward it to the billing page.
  if (searchParams.get('billing') === 'success') {
    return <Navigate to="/dashboard/billing?billing=success" replace />
  }

  return (
    <div className="family">
      <div className="family-header">
        <FamilyName token={token} familyId={family.id} name={family.name} onRenamed={refreshMe} />
        <button className="btn btn-primary" onClick={() => setShowAddChild(true)}>
          + Add child
        </button>
      </div>

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
        {childrenLoading &&
          [0, 1].map((i) => (
            <div key={i} className="child-card" aria-hidden="true">
              <div className="child-card-head" style={{ cursor: 'default' }}>
                <Skeleton circle width="2.6rem" height="2.6rem" />
                <div className="child-meta" style={{ gap: '0.4rem' }}>
                  <Skeleton width="7rem" height="0.95rem" />
                  <Skeleton width="4.5rem" height="0.7rem" />
                </div>
                <Skeleton width="5.5rem" height="0.9rem" />
                <Skeleton width="100%" height="8px" />
              </div>
            </div>
          ))}
        {!childrenLoading && children.length === 0 && (
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
            onChanged={refreshChildren}
          />
        ))}
      </section>

      {showAddChild && (
        <AddChildModal
          token={token}
          familyId={family.id}
          onClose={() => setShowAddChild(false)}
          onAdded={async () => {
            setShowAddChild(false)
            await refreshChildren()
          }}
        />
      )}
    </div>
  )
}

// FamilyName renders the space name with a quiet inline rename affordance
// (PATCH /api/v1/family/{id} — any member may rename).
function FamilyName({
  token,
  familyId,
  name,
  onRenamed,
}: {
  token: string
  familyId: string
  name: string
  onRenamed: () => Promise<void>
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(name)
  const [error, setError] = useState<string | null>(null)

  const [save, saving] = useAction(async () => {
    try {
      await api.renameFamily(token, familyId, draft.trim())
      await onRenamed()
      setEditing(false)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  })

  if (!editing) {
    return (
      <div className="family-name">
        <h2>{name}</h2>
        <button
          className="linklike"
          onClick={() => {
            setDraft(name)
            setEditing(true)
          }}
        >
          Rename
        </button>
      </div>
    )
  }

  return (
    <form
      className="family-rename-form"
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        required
        autoFocus
      />
      <button className="btn btn-primary" disabled={saving || !draft.trim()}>
        {saving ? 'Saving…' : 'Save'}
      </button>
      <button
        type="button"
        className="btn btn-ghost"
        onClick={() => {
          setEditing(false)
          setError(null)
        }}
      >
        Cancel
      </button>
      {error && <p className="form-error">{error}</p>}
    </form>
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
      track.childAdded(grade)
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
  const [statsLoading, setStatsLoading] = useState(false)
  const [devices, setDevices] = useState<Device[]>([])
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setStatsLoading(true)
    void api
      .childStats(token, profile.id)
      .then(setStats)
      .catch(() => {})
      .finally(() => setStatsLoading(false))
    void api
      .listDevices(token, profile.id)
      .then((d) => setDevices(d.devices ?? []))
      .catch(() => {})
  }, [open, token, profile.id])

  const [revokingDeviceId, setRevokingDeviceId] = useState<string | null>(null)

  const [revokeDevice, revokingDevice] = useAction(async (id: string) => {
    setRevokingDeviceId(id)
    try {
      await api.revokeDevice(token, id)
      // Only the device list changed — targeted refetch, not a full refresh.
      const d = await api.listDevices(token, profile.id)
      setDevices(d.devices ?? [])
      setActionError(null)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    } finally {
      setRevokingDeviceId(null)
    }
  })

  // Full refresh on archive is deliberate: it changes cards, tips, quests.
  const [archive, archiving] = useAction(async () => {
    if (!window.confirm(`Archive ${profile.name}? Their devices will be signed out.`)) return
    try {
      await api.updateChild(token, profile.id, { archived: true })
      await onChanged()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err))
    }
  })

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

          {/* First expansion: the stats sections below arrive async — show
              their shape so the extra content isn't a surprise. Re-opens keep
              the cached stats visible while the refetch runs. */}
          {!stats && statsLoading && (
            <div className="child-detail-loading" aria-hidden="true">
              <Skeleton width="9rem" height="1rem" />
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} width="100%" height="0.85rem" />
              ))}
              <Skeleton width="12rem" height="1rem" />
              <Skeleton width="100%" height="3.4rem" />
            </div>
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
                  <button
                    className="btn btn-ghost btn-danger"
                    disabled={revokingDevice}
                    onClick={() => void revokeDevice(d.id)}
                  >
                    {revokingDevice && revokingDeviceId === d.id ? 'Signing out…' : 'Sign out'}
                  </button>
                </li>
              ))}
            </ul>
          )}

          <div className="danger-zone">
            <button
              className="btn btn-ghost btn-danger"
              disabled={archiving}
              onClick={() => void archive()}
            >
              {archiving ? 'Archiving…' : `Archive ${profile.name}`}
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
