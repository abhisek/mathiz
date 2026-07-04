import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, deviceToken, type ChildProfile } from '../api'

type Step = 'code' | 'pick' | 'pin'

export default function Join() {
  const navigate = useNavigate()
  const [step, setStep] = useState<Step>('code')
  const [code, setCode] = useState('')
  const [familyName, setFamilyName] = useState('')
  const [children, setChildren] = useState<ChildProfile[]>([])
  const [picked, setPicked] = useState<ChildProfile | null>(null)
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submitCode(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const prev = await api.joinPreview(code)
      setFamilyName(prev.familyName)
      setChildren(prev.children)
      setStep('pick')
    } catch {
      setError("Hmm, that code didn't work. Check it and try again!")
    } finally {
      setBusy(false)
    }
  }

  async function pick(child: ChildProfile) {
    setPicked(child)
    setError(null)
    if (child.hasPin) {
      setStep('pin')
      return
    }
    await redeem(child, '')
  }

  async function submitPin(e: FormEvent) {
    e.preventDefault()
    if (picked) await redeem(picked, pin)
  }

  async function redeem(child: ChildProfile, pinValue: string) {
    setBusy(true)
    setError(null)
    try {
      const res = await api.joinRedeem(code, child.id, pinValue, deviceLabel())
      deviceToken.set(res.token)
      navigate('/play')
    } catch {
      setError(child.hasPin ? "That PIN isn't right. Try again!" : 'Something went wrong. Try again!')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="join-page">
      <div className="join-card">
        <div className="brand">
          <span className="brand-mark">∑</span>
          <h1>Mathiz</h1>
        </div>

        {step === 'code' && (
          <>
            <h2>Got a join code?</h2>
            <p className="muted">Ask your parent for the family code, then type it here.</p>
            <form onSubmit={submitCode}>
              <input
                className="code-input"
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="TIGER-4207"
                autoFocus
                required
              />
              {error && <p className="form-error">{error}</p>}
              <button className="btn btn-kid btn-block" disabled={busy}>
                {busy ? 'Checking…' : "Let's go! →"}
              </button>
            </form>
          </>
        )}

        {step === 'pick' && (
          <>
            <h2>Welcome to {familyName}!</h2>
            <p className="muted">Who are you?</p>
            {error && <p className="form-error">{error}</p>}
            <div className="profile-grid">
              {children.map((c) => (
                <button
                  key={c.id}
                  className="profile-tile"
                  disabled={busy}
                  onClick={() => void pick(c)}
                >
                  <span className="avatar avatar-big">{c.name.charAt(0).toUpperCase()}</span>
                  <span>{c.name}</span>
                  <span className="muted">Grade {c.grade}</span>
                </button>
              ))}
            </div>
          </>
        )}

        {step === 'pin' && picked && (
          <>
            <h2>Hi {picked.name}! 👋</h2>
            <p className="muted">Type your secret PIN.</p>
            <form onSubmit={submitPin}>
              <input
                className="code-input"
                type="password"
                inputMode="numeric"
                value={pin}
                onChange={(e) => setPin(e.target.value)}
                placeholder="••••"
                autoFocus
                required
              />
              {error && <p className="form-error">{error}</p>}
              <button className="btn btn-kid btn-block" disabled={busy}>
                {busy ? 'Checking…' : 'Start playing!'}
              </button>
              <button
                type="button"
                className="linklike"
                onClick={() => {
                  setStep('pick')
                  setPin('')
                  setError(null)
                }}
              >
                ← Not {picked.name}?
              </button>
            </form>
          </>
        )}
      </div>
    </div>
  )
}

function deviceLabel(): string {
  const ua = navigator.userAgent
  if (/iPad|iPhone/.test(ua)) return 'iPad / iPhone'
  if (/Android/.test(ua)) return 'Android device'
  if (/Mac/.test(ua)) return 'Mac'
  if (/Windows/.test(ua)) return 'Windows PC'
  return 'Browser'
}
