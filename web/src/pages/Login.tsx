import type { ComponentType } from 'react'
import { Link } from 'react-router-dom'
import { sessionEndedByServer } from '../auth'

// The /login page shell. The sign-in form itself comes from the
// deployment's auth adapter (src/auth/), so this page names no provider.
export default function Login({ SignIn }: { SignIn: ComponentType }) {
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

        {sessionEndedByServer() && (
          <p className="form-notice">Your session ended. Please sign in again.</p>
        )}
        <SignIn />

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
