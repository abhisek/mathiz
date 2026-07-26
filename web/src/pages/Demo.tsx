import { useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { ensureAnalyticsBooted, track } from '../analytics'

// /demo — the shareable "see it before you sign up" page: one headline,
// the launch video, one exit (start free). Deliberately no nav, feature
// grids, or second pitch — a demo page with one exit converts better than
// a second homepage. Supabase-free like every public page.
//
// The video lives on our public CDN bucket; the dated filename is
// immutable and cache-friendly — re-recording the demo means uploading a
// new dated file and changing this one constant.
const DEMO_VIDEO_URL = 'https://cdn.mathiz.app/demo/mathiz-launch-demo-20260726.mp4'

export default function Demo() {
  useEffect(() => {
    void ensureAnalyticsBooted('public')
    track.demoViewed()
  }, [])

  // First play only — pause/resume must not double count.
  const played = useRef(false)

  return (
    <div className="demo-page">
      <Link to="/" className="brand brand-small demo-brand">
        <span className="brand-mark">∑</span>
        <span>Mathiz</span>
      </Link>

      <h1>See Mathiz in action</h1>
      <p className="demo-tag">
        One minute: a real treasure-map expedition, every question AI-made.
      </p>

      {/* preload="metadata" keeps the page featherweight until play. */}
      <video
        className="demo-video"
        controls
        preload="metadata"
        poster="/demo-poster.png"
        onPlay={() => {
          if (!played.current) {
            played.current = true
            track.demoPlayClicked()
          }
        }}
      >
        <source src={DEMO_VIDEO_URL} type="video/mp4" />
        Your browser can't play this video —{' '}
        <a href={DEMO_VIDEO_URL}>download it instead</a>.
      </video>

      <div className="demo-cta">
        <p className="muted">
          Mathiz is in public beta — <strong>free right now</strong>, no card needed.
        </p>
        <Link to="/login" className="btn btn-primary">
          Start free →
        </Link>
        <Link to="/" className="muted demo-home-link">
          or explore mathiz.app
        </Link>
      </div>
    </div>
  )
}
