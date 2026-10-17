import { useEffect, useState } from 'react'
import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom'
import { getAuth, type AuthAdapter, type AuthUser } from './auth'
import { ensureAnalyticsBooted, track } from './analytics'
import Landing from './pages/Landing'
import Login from './pages/Login'
import Dashboard from './pages/dashboard/Layout'
import Join from './pages/Join'
import Play from './pages/Play'
import { Contact, Privacy, Terms } from './pages/Legal'
import Pricing from './pages/Pricing'
import HowItWorks from './pages/HowItWorks'
import Demo from './pages/Demo'
import BusyBar from './components/BusyBar'
import SiteFooter from './components/SiteFooter'

export default function App() {
  return (
    <>
      {/* One global activity bar for every route — api.request() feeds it. */}
      <BusyBar />
      <Routes>
        {/* Kid routes load no auth adapter: a join code is all a child needs. */}
        <Route path="/join" element={<Join />} />
        <Route path="/play" element={<Play />} />

        {/* Public pages: static, auth-free, and the only routes that get
            the site footer. Adding a public page here gives it the same
            footer for free; nothing outside this layout can grow one, which
            is what keeps pricing links off the kid surfaces above. */}
        <Route element={<PublicLayout />}>
          {/* The front door: routes each persona. */}
          <Route path="/" element={<Landing />} />
          <Route path="/pricing" element={<Pricing />} />
          <Route path="/how-it-works" element={<HowItWorks />} />
          <Route path="/terms" element={<Terms />} />
          <Route path="/privacy" element={<Privacy />} />
          <Route path="/contact" element={<Contact />} />
        </Route>

        {/* /demo is deliberately single-exit (see Demo.tsx): it gets the
            legal links so a shared demo link is still compliant, but no
            second pitch competing with its one CTA. */}
        <Route element={<PublicLayout variant="minimal" />}>
          <Route path="/demo" element={<Demo />} />
        </Route>

        <Route path="/login" element={<ParentArea page="login" />} />
        <Route path="/dashboard/*" element={<ParentArea page="dashboard" />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  )
}

// PublicLayout is the shell for signed-out pages: the page fills the
// viewport, the footer sits under it. It owns the "fill the screen"
// responsibility that each public page used to carry itself, so the footer
// lands at the bottom of the fold instead of a scroll below it.
function PublicLayout({ variant }: { variant?: 'full' | 'minimal' }) {
  return (
    <div className="public-shell">
      <Outlet />
      <SiteFooter variant={variant} />
    </div>
  )
}

// ParentArea owns the auth adapter; only parent pages pay its boot cost.
function ParentArea({ page }: { page: 'login' | 'dashboard' }) {
  const [adapter, setAdapter] = useState<AuthAdapter | null>(null)
  const [user, setUser] = useState<AuthUser | null>(null)
  const [booted, setBooted] = useState(false)
  const [bootError, setBootError] = useState<string | null>(null)
  const location = useLocation()

  // Route pageviews for the parent surfaces (/login + /dashboard/*) —
  // manual SPA tracking, gated on the analytics boot so none get dropped.
  useEffect(() => {
    void ensureAnalyticsBooted('parent').then(() => track.pageview(location.pathname))
  }, [location.pathname])

  useEffect(() => {
    let unsub = () => {}
    let cancelled = false
    getAuth()
      .then((a) => {
        if (cancelled) return
        setAdapter(a)
        setUser(a.user())
        // Token refreshes fire onChange too: keep the same object unless
        // the parent actually changed, so the dashboard doesn't re-render.
        unsub = a.onChange(() => {
          const next = a.user()
          setUser((prev) => (prev?.id === next?.id && prev?.email === next?.email ? prev : next))
        })
        setBooted(true)
      })
      .catch((err) => {
        if (cancelled) return
        setBootError(err instanceof Error ? err.message : String(err))
        setBooted(true)
      })
    return () => {
      cancelled = true
      unsub()
    }
  }, [])

  if (!booted) return <div className="boot">Loading Mathiz…</div>
  if (bootError || !adapter) {
    return (
      <div className="boot boot-error">
        <div>
          <h1>Mathiz</h1>
          <p>{bootError ?? 'Sign-in is unavailable.'}</p>
          <p className="muted">
            Kids with a join code can still <a href="/join">start playing</a>.
          </p>
        </div>
      </div>
    )
  }

  if (page === 'login') {
    return user ? <Navigate to="/dashboard" replace /> : <Login SignIn={adapter.SignIn} />
  }
  // Keyed on the parent: a different parent signing in (another tab) gets
  // a fresh dashboard, never the previous parent's cached family.
  return user ? (
    <Dashboard key={user.id} adapter={adapter} email={user.email} />
  ) : (
    <Navigate to="/login" replace />
  )
}
