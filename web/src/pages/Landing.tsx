import { Link } from 'react-router-dom'

// The front door: Supabase-free (kids never pay the auth boot cost), just
// two clearly-signed paths — parents to sign-in, kids to the join code.
export default function Landing() {
  return (
    <div className="landing">
      <Backdrop />

      <section className="landing-hero">
        <header className="landing-brand brand">
          <span className="brand-mark">∑</span>
          <h1>Mathiz</h1>
        </header>

        <span className="beta-pill">Public beta</span>

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
            <span className="door-note">
              Free to start — 30 expeditions on us, no card needed.
            </span>
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
      </section>

      <section className="landing-why">
        <h2 className="landing-why-title">Why Mathiz?</h2>

        <div className="why-tiles">
          <div className="why-tile">
            <span className="why-emoji" aria-hidden>
              ✨
            </span>
            <h3>Questions made for your child</h3>
            <p>
              AI generates every question from her level and mistakes — never a
              worksheet.
            </p>
          </div>

          <div className="why-tile">
            <span className="why-emoji" aria-hidden>
              🗺️
            </span>
            <h3>Math becomes a treasure hunt</h3>
            <p>Islands, gems, streaks, quests.</p>
          </div>

          <div className="why-tile">
            <span className="why-emoji" aria-hidden>
              🧑‍✈️
            </span>
            <h3>Parents steer</h3>
            <p>Progress dashboard, join codes, create your own quests.</p>
          </div>
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
      </section>

      <footer className="landing-footer">
        <Link to="/pricing">Pricing</Link>
        <Link to="/terms">Terms</Link>
        <Link to="/privacy">Privacy</Link>
        <Link to="/contact">Contact</Link>
        <span>© Mathiz</span>
      </footer>
    </div>
  )
}

// Faint nautical scenery behind the content: a dashed sail route meandering
// down the page, a compass rose in the corner, island silhouettes near the
// edges. Pure inline SVG (no external assets), kept at ≤6% contrast against
// --bg so it never competes with the two doors.
function Backdrop() {
  return (
    <div className="landing-backdrop" aria-hidden="true">
      <svg className="bd-route" viewBox="0 0 1440 2400" preserveAspectRatio="none">
        <path
          d="M -80 160
             C 320 280, 940 40, 1150 330
             C 1310 550, 860 640, 560 760
             C 260 880, 320 1060, 640 1160
             C 1000 1270, 1290 1330, 1230 1560
             C 1170 1780, 660 1760, 420 1930
             C 220 2070, 460 2230, 860 2300
             C 1120 2345, 1360 2330, 1520 2280"
          fill="none"
          stroke="currentColor"
          strokeWidth="3"
          strokeLinecap="round"
          strokeDasharray="2 22"
          vectorEffect="non-scaling-stroke"
        />
      </svg>

      <svg className="bd-compass" viewBox="0 0 200 200">
        <g fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="100" cy="100" r="88" />
          <circle cx="100" cy="100" r="64" strokeDasharray="3 8" />
        </g>
        <g fill="currentColor">
          <path d="M100 10 L110 100 L100 190 L90 100 Z" />
          <path d="M10 100 L100 90 L190 100 L100 110 Z" />
          <g transform="rotate(45 100 100)" opacity="0.55">
            <path d="M100 24 L108 100 L100 176 L92 100 Z" />
            <path d="M24 100 L100 92 L176 100 L100 108 Z" />
          </g>
        </g>
      </svg>

      <svg className="bd-island bd-island-left" viewBox="0 0 300 120">
        <path
          d="M0 120 C 28 72 68 60 96 80 C 108 48 152 42 178 68 C 204 44 252 54 270 86 C 286 102 300 112 300 120 Z"
          fill="currentColor"
        />
      </svg>

      <svg className="bd-island bd-island-right" viewBox="0 0 300 120">
        <path
          d="M0 120 C 20 96 50 84 80 94 C 96 58 146 50 176 76 C 196 40 250 46 272 82 C 288 100 300 112 300 120 Z"
          fill="currentColor"
        />
      </svg>

      <svg className="bd-island bd-island-far" viewBox="0 0 300 120">
        <path
          d="M0 120 C 40 84 84 74 118 90 C 140 60 190 56 220 80 C 252 66 284 86 300 108 L 300 120 Z"
          fill="currentColor"
        />
      </svg>
    </div>
  )
}
