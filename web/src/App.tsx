import { useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import type { Session } from '@supabase/supabase-js'
import { getSupabase } from './supa'
import Landing from './pages/Landing'
import Login from './pages/Login'
import Dashboard from './pages/dashboard/Layout'
import Join from './pages/Join'
import Play from './pages/Play'
import TerminalPage from './pages/Terminal'
import { Contact, Privacy, Terms } from './pages/Legal'
import BusyBar from './components/BusyBar'

export default function App() {
  return (
    <>
      {/* One global activity bar for every route — api.request() feeds it. */}
      <BusyBar />
      <Routes>
        {/* Kid routes are Supabase-free: a join code is all a child needs. */}
        <Route path="/join" element={<Join />} />
        <Route path="/play" element={<Play />} />
        <Route path="/terminal" element={<TerminalPage />} />

        {/* The front door: static, Supabase-free, routes each persona. */}
        <Route path="/" element={<Landing />} />

        {/* Legal pages: static, Supabase-free. */}
        <Route path="/terms" element={<Terms />} />
        <Route path="/privacy" element={<Privacy />} />
        <Route path="/contact" element={<Contact />} />

        <Route path="/login" element={<ParentArea page="login" />} />
        <Route path="/dashboard/*" element={<ParentArea page="dashboard" />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  )
}

// ParentArea owns the Supabase session; only parent pages pay its boot cost.
function ParentArea({ page }: { page: 'login' | 'dashboard' }) {
  const [session, setSession] = useState<Session | null>(null)
  const [booted, setBooted] = useState(false)
  const [bootError, setBootError] = useState<string | null>(null)

  useEffect(() => {
    let unsub = () => {}
    getSupabase()
      .then((supa) => {
        supa.auth.getSession().then(({ data }) => {
          setSession(data.session)
          setBooted(true)
        })
        const { data } = supa.auth.onAuthStateChange((_event, s) => setSession(s))
        unsub = () => data.subscription.unsubscribe()
      })
      .catch((err) => {
        setBootError(err instanceof Error ? err.message : String(err))
        setBooted(true)
      })
    return () => unsub()
  }, [])

  if (!booted) return <div className="boot">Loading Mathiz…</div>
  if (bootError) {
    return (
      <div className="boot boot-error">
        <div>
          <h1>Mathiz</h1>
          <p>{bootError}</p>
          <p className="muted">
            Kids with a join code can still <a href="/join">start playing</a>.
          </p>
        </div>
      </div>
    )
  }

  if (page === 'login') {
    return session ? <Navigate to="/dashboard" replace /> : <Login />
  }
  return session ? <Dashboard session={session} /> : <Navigate to="/login" replace />
}
