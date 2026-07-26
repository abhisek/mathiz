import { Link } from 'react-router-dom'
import ShareMathiz from './ShareMathiz'

// The footer for public, signed-out pages. Rendered by PublicLayout in
// App.tsx rather than by each page, so a new public route gets it by being
// routed — and so it can never appear on a kid or parent surface, which are
// routed outside that layout entirely.
//
// Kid surfaces must never link to pricing (see the frontend skill); routing
// is what enforces that here, not discipline at each call site.
export default function SiteFooter({
  variant = 'full',
}: {
  // 'minimal' is legal links only — for pages whose conversion design wants
  // a single primary exit (currently /demo, the shareable one).
  variant?: 'full' | 'minimal'
}) {
  return (
    <footer className="site-footer">
      {variant === 'full' && (
        <>
          <Link to="/">Home</Link>
          <Link to="/demo">Demo</Link>
          <Link to="/how-it-works">How it works</Link>
          <Link to="/pricing">Pricing</Link>
        </>
      )}
      <Link to="/terms">Terms</Link>
      <Link to="/privacy">Privacy</Link>
      <Link to="/contact">Contact</Link>
      {variant === 'full' && <ShareMathiz variant="link" />}
      <span>© Mathiz</span>
    </footer>
  )
}
