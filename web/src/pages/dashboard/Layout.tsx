import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { Navigate, NavLink, Outlet, Route, Routes } from 'react-router-dom'
import type { Session } from '@supabase/supabase-js'
import {
  api,
  type ChildWithSummary,
  type FamilySpace,
  type PendingParentInvite,
} from '../../api'
import { useAction } from '../../hooks'
import { getSupabase } from '../../supa'
import { type DashboardContext } from './context'
import Kids from './Kids'
import Activity from './Activity'
import Quests from './Quests'
import QuestEditor from './QuestEditor'
import Family from './Family'
import Billing from './Billing'

interface Props {
  session: Session
}

// Dashboard mounts the nested route tree under /dashboard/*.
export default function Dashboard({ session }: Props) {
  return (
    <Routes>
      <Route element={<DashboardLayout session={session} />}>
        <Route index element={<Kids />} />
        <Route path="activity" element={<Activity />} />
        <Route path="quests" element={<Quests />} />
        <Route path="quests/:id" element={<QuestEditor />} />
        <Route path="family" element={<Family />} />
        <Route path="billing" element={<Billing />} />
        <Route path="*" element={<Navigate to="/dashboard" replace />} />
      </Route>
    </Routes>
  )
}

// DashboardLayout owns the /me fetch (account, family, role, pendingInvite)
// and the children list; sub-pages get both through outlet context so a
// route change never refetches the whole world.
function DashboardLayout({ session }: Props) {
  const token = session.access_token
  const [family, setFamily] = useState<FamilySpace | null>(null)
  // 'owner' | 'parent' — only meaningful alongside a family.
  const [role, setRole] = useState<string | null>(null)
  const [pendingInvite, setPendingInvite] = useState<PendingParentInvite | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refreshMe = useCallback(async () => {
    try {
      const me = await api.me(token)
      setFamily(me.family)
      setRole(me.role ?? null)
      setPendingInvite(me.pendingInvite ?? null)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoaded(true)
    }
  }, [token])

  useEffect(() => {
    void refreshMe()
  }, [refreshMe])

  const [children, setChildren] = useState<ChildWithSummary[]>([])
  // True until the FIRST refresh resolves — drives the skeletons. Later
  // refreshes (after mutations) never flip it back.
  const [childrenLoading, setChildrenLoading] = useState(true)
  const familyId = family?.id ?? null

  const refreshChildren = useCallback(async () => {
    if (!familyId) return
    try {
      const kids = await api.listChildren(token, familyId)
      setChildren(kids.children ?? [])
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setChildrenLoading(false)
    }
  }, [token, familyId])

  useEffect(() => {
    void refreshChildren()
  }, [refreshChildren])

  async function signOut() {
    const supa = await getSupabase()
    await supa.auth.signOut()
  }

  if (!loaded) return <div className="boot">Loading…</div>

  const isOwner = (role ?? 'parent') === 'owner'

  return (
    <div className="dash-shell">
      <aside className="dash-nav">
        <div className="brand brand-small dash-brand">
          <span className="brand-mark">∑</span>
          <span>Mathiz</span>
        </div>
        <nav className="dash-nav-items" aria-label="Dashboard">
          <DashNavLink to="/dashboard" end label="Kids" emoji="🧒" />
          <DashNavLink to="/dashboard/activity" label="Activity" emoji="🗓️" />
          <DashNavLink to="/dashboard/quests" label="Quests" emoji="⭐" />
          <DashNavLink to="/dashboard/family" label="Family" emoji="👪" />
          {isOwner && <DashNavLink to="/dashboard/billing" label="Billing" emoji="⛵" />}
        </nav>
        <div className="dash-nav-foot">
          <span className="muted dash-email">{session.user.email}</span>
          <button className="btn btn-ghost" onClick={signOut}>
            Sign out
          </button>
        </div>
      </aside>

      <main className="dash-main">
        {error && <p className="form-error">{error}</p>}
        {family ? (
          <Outlet
            context={
              {
                token,
                family,
                role: role ?? 'parent',
                children,
                childrenLoading,
                refreshChildren,
                refreshMe,
              } satisfies DashboardContext
            }
          />
        ) : (
          // No family yet: the create flow renders whatever the sub-route is.
          <>
            {pendingInvite && (
              <AcceptInviteBanner token={token} invite={pendingInvite} onAccepted={refreshMe} />
            )}
            {pendingInvite && (
              <p className="muted invite-or">…or create your own family instead:</p>
            )}
            <CreateFamily token={token} onCreated={refreshMe} />
          </>
        )}
      </main>
    </div>
  )
}

function DashNavLink({
  to,
  label,
  emoji,
  end = false,
}: {
  to: string
  label: string
  emoji: string
  end?: boolean
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) => `dash-nav-link${isActive ? ' dash-nav-active' : ''}`}
    >
      <span className="dash-nav-emoji" aria-hidden="true">
        {emoji}
      </span>
      {label}
    </NavLink>
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

// AcceptInviteBanner: the signed-in account has no family, but a pending
// co-parent invite matches its email (specs/12-saas.md, "Co-parents").
// Accepting joins the family; the /me reload then swaps in the family view.
function AcceptInviteBanner({
  token,
  invite,
  onAccepted,
}: {
  token: string
  invite: PendingParentInvite
  onAccepted: () => Promise<void>
}) {
  const [error, setError] = useState<string | null>(null)

  const [accept, accepting] = useAction(async () => {
    try {
      await api.acceptParentInvite(token, invite.id)
      await onAccepted()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  })

  return (
    <div className="center-card accept-banner">
      <h2>You're invited! 🎉</h2>
      <p>
        <strong>{invite.invitedBy || 'A parent'}</strong> invited you to join{' '}
        <strong>{invite.familyName || 'their family'}</strong> as a co-parent.
      </p>
      <p className="muted">
        Co-parents can do everything except billing and managing parents.
      </p>
      {error && <p className="form-error">{error}</p>}
      <button className="btn btn-primary" disabled={accepting} onClick={() => void accept()}>
        {accepting ? 'Joining…' : `Join ${invite.familyName || 'the family'}`}
      </button>
    </div>
  )
}
