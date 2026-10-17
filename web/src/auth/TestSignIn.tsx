import { useState, type FormEvent } from 'react'
import { track } from '../analytics'

// The paste-a-token form of the test adapter (see ./test.ts).
export default function TestSignIn({ onToken }: { onToken: (token: string) => void }) {
  const [token, setToken] = useState('')
  const [error, setError] = useState<string | null>(null)

  function submit(e: FormEvent) {
    e.preventDefault()
    const t = token.trim()
    if (t.split('.').length !== 3) {
      setError('That does not look like a JWT (three dot-separated parts).')
      return
    }
    track.signinCompleted()
    onToken(t)
  }

  return (
    <form onSubmit={submit}>
      <p className="form-notice">
        Test sign-in: this server accepts self-minted parent tokens. Mint one with{' '}
        <code>node .claude/skills/saas-e2e/assets/mint-jwt.mjs</code>.
      </p>
      <label>
        Parent token
        <textarea
          className="test-token-input"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="eyJhbGciOiJIUzI1NiIs…"
          required
          rows={4}
        />
      </label>
      {error && <p className="form-error">{error}</p>}
      <button className="btn btn-primary">Sign in with token</button>
    </form>
  )
}
