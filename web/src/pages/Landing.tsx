import { Link } from 'react-router-dom'

// The front door: Supabase-free (kids never pay the auth boot cost), just
// two clearly-signed paths — parents to sign-in, kids to the join code.
export default function Landing() {
  return (
    <div className="landing">
      <header className="landing-brand brand">
        <span className="brand-mark">∑</span>
        <h1>Mathiz</h1>
      </header>

      <p className="landing-tag">
        A math playground where kids <strong>learn by doing</strong> — every
        question is created just for them, never copied from a worksheet.
      </p>

      <div className="landing-doors">
        <Link to="/login" className="door door-parent">
          <span className="door-emoji" aria-hidden>
            🧭
          </span>
          <h2>I'm a parent</h2>
          <p>
            Set up your family, add your kids, and watch their skills grow
            island by island.
          </p>
          <span className="btn btn-primary btn-block">Parent sign in</span>
        </Link>

        <Link to="/join" className="door door-kid">
          <span className="door-emoji" aria-hidden>
            🏴‍☠️
          </span>
          <h2>I'm a kid</h2>
          <p>
            Got a join code from your grown-up? Your treasure map is waiting,
            explorer!
          </p>
          <span className="btn btn-kid btn-block">Enter my code</span>
        </Link>
      </div>

      <ol className="landing-how">
        <li>
          <strong>Parents</strong> create the family and share a join code.
        </li>
        <li>
          <strong>Kids</strong> enter the code — no email, no password.
        </li>
        <li>
          Solve math, dig treasure, collect gems, unlock the map. 💎
        </li>
      </ol>
    </div>
  )
}
