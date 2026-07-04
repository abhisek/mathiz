import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { getSupabase } from '../supa'

type Mode = 'signin' | 'signup'

export default function Login() {
  const [mode, setMode] = useState<Mode>('signin')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const supa = await getSupabase()
      if (mode === 'signup') {
        const { data, error } = await supa.auth.signUp({ email, password })
        if (error) throw error
        if (!data.session) {
          setNotice('Check your inbox to confirm your email, then sign in.')
        }
      } else {
        const { error } = await supa.auth.signInWithPassword({ email, password })
        if (error) throw error
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="brand">
          <span className="brand-mark">∑</span>
          <h1>Mathiz</h1>
        </div>
        <p className="tagline">
          An AI math playground for kids. Adaptive practice, questions generated
          for <em>your</em> child — never from a worksheet.
        </p>

        <form onSubmit={submit}>
          <label>
            Email
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              required
              autoComplete="email"
            />
          </label>
          <label>
            Password
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              required
              minLength={8}
              autoComplete={mode === 'signup' ? 'new-password' : 'current-password'}
            />
          </label>
          {error && <p className="form-error">{error}</p>}
          {notice && <p className="form-notice">{notice}</p>}
          <button className="btn btn-primary" disabled={busy}>
            {busy ? 'One moment…' : mode === 'signup' ? 'Create parent account' : 'Sign in'}
          </button>
        </form>

        <p className="auth-switch">
          {mode === 'signin' ? (
            <>
              New to Mathiz?{' '}
              <button className="linklike" onClick={() => setMode('signup')}>
                Create an account
              </button>
            </>
          ) : (
            <>
              Already have an account?{' '}
              <button className="linklike" onClick={() => setMode('signin')}>
                Sign in
              </button>
            </>
          )}
        </p>

        <div className="kid-door">
          <span>Are you a kid with a join code?</span>
          <Link className="btn btn-kid" to="/join">
            Start playing →
          </Link>
        </div>
      </div>
    </div>
  )
}
