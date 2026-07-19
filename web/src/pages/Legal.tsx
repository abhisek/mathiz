import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

// Minimal legal placeholder pages (/terms, /privacy, /contact). Static and
// Supabase-free, honest v1 documents — enough for payment-provider review,
// clearly marked as placeholders until counsel-reviewed versions land.

const LAST_UPDATED = 'July 19, 2026'

function LegalPage({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="legal-page">
      <Link to="/" className="brand legal-brand">
        <span className="brand-mark">∑</span>
        <span className="legal-brand-name">Mathiz</span>
      </Link>

      <article className="legal-card">
        <h1>{title}</h1>
        <p className="legal-meta">
          Version 1 (initial placeholder document) · Last updated: {LAST_UPDATED}
        </p>
        {children}
      </article>

      <p className="legal-back muted">
        <Link to="/">← Back to Mathiz</Link>
      </p>
    </div>
  )
}

export function Terms() {
  return (
    <LegalPage title="Terms of Service">
      <p>
        Welcome to Mathiz, a math-practice game for kids managed by their
        parents. By creating an account or using Mathiz you agree to these
        terms.
      </p>
      <h2>The service</h2>
      <p>
        Mathiz generates personalized math questions and progress tracking for
        children whose parent or guardian set up their profile. Accounts are
        created and managed by an adult; children access Mathiz only through a
        join code their grown-up shares with them.
      </p>
      <h2>Payments</h2>
      <p>
        Mathiz is free to start. Paid plans and top-ups add practice-session
        credits to your family's balance; payments are processed by our payment
        provider and are refundable where required by law. We do not store your
        card details.
      </p>
      <h2>Fair use</h2>
      <p>
        Don't abuse the service: no attempting to access other families' data,
        no automated scraping, no reselling access. We may suspend accounts
        that break these rules.
      </p>
      <h2>Warranty and liability</h2>
      <p>
        Mathiz is provided "as is" without warranties of any kind, to the
        extent permitted by law. Our liability is limited to the amount you
        paid us in the twelve months before a claim.
      </p>
      <h2>Changes</h2>
      <p>
        We may update these terms; material changes will be announced to the
        account email before they take effect.
      </p>
    </LegalPage>
  )
}

export function Privacy() {
  return (
    <LegalPage title="Privacy Policy">
      <p>
        Mathiz collects only what it needs to teach math, and nothing more.
      </p>
      <h2>What we store</h2>
      <ul>
        <li>
          <strong>Parent email</strong> — used for sign-in, handled via
          Supabase authentication.
        </li>
        <li>
          <strong>Child first name and learning progress</strong> — skill
          mastery, answers, and session history, used solely to personalize
          questions and show parents progress. No child email, password, or
          contact details are ever collected.
        </li>
        <li>
          <strong>Billing records</strong> — handled by our payment provider;
          we never see or store card numbers.
        </li>
      </ul>
      <h2>What we don't do</h2>
      <ul>
        <li>No advertising, and no ad trackers.</li>
        <li>We never sell or share your data with third parties.</li>
      </ul>
      <h2>Deletion</h2>
      <p>
        Contact us any time to delete your family's account and all associated
        data — see the <Link to="/contact">contact page</Link>.
      </p>
    </LegalPage>
  )
}

export function Contact() {
  return (
    <LegalPage title="Contact">
      <p>
        Questions, billing issues, or a data-deletion request? Email us and a
        human will get back to you:
      </p>
      <p>
        <a className="legal-mail" href="mailto:support@mathiz.app">
          support@mathiz.app
        </a>
      </p>
      <p className="muted">
        (Placeholder support address — final support contact to be confirmed.)
      </p>
    </LegalPage>
  )
}
