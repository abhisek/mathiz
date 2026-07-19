import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { api, type Invite, type ParentInvite, type ParentMember } from '../../api'
import { track } from '../../analytics'
import { useAction } from '../../hooks'
import Skeleton from '../../components/Skeleton'
import { useDashboard } from './context'

// Family — co-parents roster + join codes.
export default function Family() {
  const { token, family, role } = useDashboard()
  return (
    <div className="family">
      <ParentsSection token={token} familyId={family.id} isOwner={role === 'owner'} />
      <InvitesSection token={token} familyId={family.id} />
    </div>
  )
}

// ---- Co-parents (specs/12-saas.md, "Co-parents") ----
// Any member sees the roster; inviting, revoking, and removing are owner-only
// (the server enforces via authz — the UI just hides the controls).

function ParentsSection({
  token,
  familyId,
  isOwner,
}: {
  token: string
  familyId: string
  isOwner: boolean
}) {
  const [members, setMembers] = useState<ParentMember[]>([])
  const [pending, setPending] = useState<ParentInvite[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const res = await api.listParents(token, familyId)
      setMembers(res.parents ?? [])
      setPending(res.invites ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [token, familyId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteError, setInviteError] = useState<string | null>(null)
  const [inviting, setInviting] = useState(false)

  async function invite(e: FormEvent) {
    e.preventDefault()
    if (inviting) return
    setInviting(true)
    setInviteError(null)
    try {
      await api.inviteParent(token, familyId, inviteEmail.trim())
      track.coparentInvited()
      setInviteEmail('')
      await refresh()
    } catch (err) {
      // 409s carry a human-readable reason (already a member / already
      // invited) — surface it inline next to the form.
      setInviteError(err instanceof Error ? err.message : String(err))
    } finally {
      setInviting(false)
    }
  }

  const [revokingId, setRevokingId] = useState<string | null>(null)

  const [revoke, revoking] = useAction(async (id: string) => {
    setRevokingId(id)
    try {
      await api.revokeParentInvite(token, id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setRevokingId(null)
    }
  })

  const [removingId, setRemovingId] = useState<string | null>(null)

  const [remove, removing] = useAction(async (m: ParentMember) => {
    const who = m.displayName || m.email
    if (!window.confirm(`Remove ${who} from the family? They lose access immediately.`)) return
    setRemovingId(m.accountId)
    try {
      await api.removeParent(token, familyId, m.accountId)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setRemovingId(null)
    }
  })

  return (
    <section className="parents">
      <div className="section-head">
        <h3>Parents</h3>
      </div>
      <p className="muted">
        Co-parents can do everything except billing and managing parents.
      </p>
      {error && <p className="form-error">{error}</p>}
      {loading ? (
        <ul className="parent-list" aria-hidden="true">
          {[0, 1].map((i) => (
            <li key={i}>
              <Skeleton width="11rem" height="0.9rem" />
              <Skeleton width="4rem" height="1.4rem" />
            </li>
          ))}
        </ul>
      ) : (
        <ul className="parent-list">
          {members.map((m) => (
            <li key={m.accountId}>
              <span className="parent-who">
                <strong>{m.displayName || m.email}</strong>
                {m.displayName && <span className="muted"> {m.email}</span>}
              </span>
              <span className={`quest-status ${m.role === 'owner' ? 'quest-status-active' : 'quest-status-archived'}`}>
                {m.role === 'owner' ? 'Owner' : 'Parent'}
              </span>
              {isOwner && m.role !== 'owner' && (
                <button
                  className="btn btn-ghost btn-danger"
                  disabled={removing}
                  onClick={() => void remove(m)}
                >
                  {removing && removingId === m.accountId ? 'Removing…' : 'Remove'}
                </button>
              )}
            </li>
          ))}
          {pending.map((inv) => (
            <li key={inv.id}>
              <span className="parent-who">
                <span className="muted">{inv.email}</span>
              </span>
              <span className="quest-status quest-status-draft">invited</span>
              {isOwner && (
                <button
                  className="btn btn-ghost btn-danger"
                  disabled={revoking}
                  onClick={() => void revoke(inv.id)}
                >
                  {revoking && revokingId === inv.id ? 'Revoking…' : 'Revoke'}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {isOwner && (
        <form className="parent-invite-form" onSubmit={invite}>
          <input
            type="email"
            value={inviteEmail}
            onChange={(e) => setInviteEmail(e.target.value)}
            placeholder="co-parent@example.com"
            required
          />
          <button className="btn btn-secondary" disabled={inviting || !inviteEmail.trim()}>
            {inviting ? 'Inviting…' : 'Invite parent'}
          </button>
        </form>
      )}
      {inviteError && <p className="form-error">{inviteError}</p>}
      {isOwner && (
        <p className="form-hint">
          No email is sent — ask them to sign in with this address and they'll
          see the invite on their dashboard.
        </p>
      )}
    </section>
  )
}

// Join codes — minting, listing, revoking (kids redeem them at /join).
function InvitesSection({ token, familyId }: { token: string; familyId: string }) {
  const [invites, setInvites] = useState<Invite[]>([])
  // True until the FIRST fetch resolves — drives the skeletons.
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const invs = await api.listInvites(token, familyId)
      setInvites(invs.invites ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [token, familyId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const [inviteDays, setInviteDays] = useState(7)
  const [minting, setMinting] = useState(false)

  async function mintInvite() {
    if (minting) return
    setMinting(true)
    try {
      await api.createInvite(token, familyId, inviteDays * 24)
      track.joinCodeCreated()
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setMinting(false)
    }
  }

  const [revokingInviteId, setRevokingInviteId] = useState<string | null>(null)

  const [revokeInvite, revokingInvite] = useAction(async (id: string) => {
    setRevokingInviteId(id)
    try {
      await api.revokeInvite(token, id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setRevokingInviteId(null)
    }
  })

  return (
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
          <button className="btn btn-secondary" onClick={mintInvite} disabled={minting}>
            {minting ? 'Minting…' : 'New join code'}
          </button>
        </div>
      </div>
      <p className="muted">
        Share a code with your child. They open <code>{window.location.origin}/join</code>,
        type the code, pick their name, and start learning — no email needed.
      </p>
      {error && <p className="form-error">{error}</p>}
      {loading ? (
        <ul className="invite-list" aria-hidden="true">
          {[0, 1].map((i) => (
            <li key={i}>
              <Skeleton width="6.5rem" height="1.8rem" />
              <Skeleton width="9rem" height="0.8rem" />
            </li>
          ))}
        </ul>
      ) : invites.length === 0 ? (
        <p className="muted">No active codes.</p>
      ) : (
        <ul className="invite-list">
          {invites.map((inv) => (
            <li key={inv.id}>
              <code className="join-code">{inv.code}</code>
              <span className="muted">
                expires {new Date(inv.expiresAt).toLocaleDateString()}
              </span>
              <button
                className="btn btn-ghost btn-danger"
                disabled={revokingInvite}
                onClick={() => void revokeInvite(inv.id)}
              >
                {revokingInvite && revokingInviteId === inv.id ? 'Revoking…' : 'Revoke'}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
