import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { track } from '../analytics'
import { getSupabase } from '../supa'

// OTP-first parent sign-in: email → emailed code (6–10 digits depending on
// the Supabase project's OTP length setting; or the magic link in the
// same email). Password auth stays available as an explicit fallback.
type Mode = 'otp' | 'password'
type PasswordMode = 'signin' | 'signup'

export default function Login() {
  const [mode, setMode] = useState<Mode>('otp')
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  // OTP state
  const [codeSent, setCodeSent] = useState(false)
  const [code, setCode] = useState('')

  // Password-fallback state
  const [passwordMode, setPasswordMode] = useState<PasswordMode>('signin')
  const [password, setPassword] = useState('')

  function reset(nextMode: Mode) {
    setMode(nextMode)
    setError(null)
    setNotice(null)
    setCodeSent(false)
    setCode('')
  }

  async function sendCode(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const supa = await getSupabase()
      const { error } = await supa.auth.signInWithOtp({
        email,
        options: {
          shouldCreateUser: true,
          // Magic-link clicks must land where the Supabase client boots.
          emailRedirectTo: `${window.location.origin}/login`,
        },
      })
      if (error) throw error
      setCodeSent(true)
      setNotice(
        `Check ${email} — enter the code from the email below, or just click the link in it. Both sign you in.`,
      )
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function verifyCode(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const supa = await getSupabase()
      const { error } = await supa.auth.verifyOtp({
        email,
        token: code.trim(),
        type: 'email',
      })
      if (error) throw error
      track.signinCompleted()
      // Session lands via onAuthStateChange; ParentArea redirects.
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function submitPassword(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    setNotice(null)
    try {
      const supa = await getSupabase()
      if (passwordMode === 'signup') {
        const { data, error } = await supa.auth.signUp({
          email,
          password,
          options: { emailRedirectTo: `${window.location.origin}/login` },
        })
        if (error) throw error
        if (!data.session) {
          setNotice('Check your inbox to confirm your email, then sign in.')
        } else {
          track.signinCompleted()
        }
      } else {
        const { error } = await supa.auth.signInWithPassword({ email, password })
        if (error) throw error
        track.signinCompleted()
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
        <Link to="/" className="brand brand-link" title="Back to the front page">
          <span className="brand-mark">∑</span>
          <h1>Mathiz</h1>
        </Link>
        <p className="tagline">
          An AI math playground for kids. Adaptive practice, questions generated
          for <em>your</em> child — never from a worksheet.
        </p>

        {mode === 'otp' && !codeSent && (
          <form onSubmit={sendCode}>
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
            {error && <p className="form-error">{error}</p>}
            <button className="btn btn-primary" disabled={busy}>
              {busy ? 'One moment…' : 'Email me a sign-in code'}
            </button>
            <p className="form-hint">
              New here? Same button — your account is created on first sign-in.
            </p>
          </form>
        )}

        {mode === 'otp' && codeSent && (
          <form onSubmit={verifyCode}>
            {notice && <p className="form-notice">{notice}</p>}
            <label>
              Sign-in code
              <input
                className="otp-input"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder="12345678"
                required
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={10}
                autoFocus
              />
            </label>
            {error && <p className="form-error">{error}</p>}
            <button className="btn btn-primary" disabled={busy || code.trim().length < 6}>
              {busy ? 'Checking…' : 'Sign in'}
            </button>
            <p className="auth-switch">
              <button className="linklike" type="button" onClick={(e) => sendCode(e)}>
                Resend code
              </button>{' '}
              ·{' '}
              <button
                className="linklike"
                type="button"
                onClick={() => {
                  setCodeSent(false)
                  setCode('')
                  setNotice(null)
                  setError(null)
                }}
              >
                Use a different email
              </button>
            </p>
          </form>
        )}

        {mode === 'password' && (
          <form onSubmit={submitPassword}>
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
                autoComplete={passwordMode === 'signup' ? 'new-password' : 'current-password'}
              />
            </label>
            {error && <p className="form-error">{error}</p>}
            {notice && <p className="form-notice">{notice}</p>}
            <button className="btn btn-primary" disabled={busy}>
              {busy
                ? 'One moment…'
                : passwordMode === 'signup'
                  ? 'Create parent account'
                  : 'Sign in'}
            </button>
            <p className="auth-switch">
              {passwordMode === 'signin' ? (
                <>
                  New to Mathiz?{' '}
                  <button
                    className="linklike"
                    type="button"
                    onClick={() => setPasswordMode('signup')}
                  >
                    Create an account
                  </button>
                </>
              ) : (
                <>
                  Already have an account?{' '}
                  <button
                    className="linklike"
                    type="button"
                    onClick={() => setPasswordMode('signin')}
                  >
                    Sign in
                  </button>
                </>
              )}
            </p>
          </form>
        )}

        <p className="auth-switch">
          {mode === 'otp' ? (
            <button className="linklike" onClick={() => reset('password')}>
              Prefer a password? Sign in with one
            </button>
          ) : (
            <button className="linklike" onClick={() => reset('otp')}>
              ← Back to sign-in with email code
            </button>
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
