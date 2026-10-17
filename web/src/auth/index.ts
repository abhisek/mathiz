import type { ComponentType } from 'react'
import { api, setParentUnauthorizedHandler, type AuthBootConfig } from '../api'
import { resetAnalytics } from '../analytics'

// The auth adapter is the ONLY place that knows the deployment's auth
// provider (specs/17-auth-providers.md). Everything else sees this
// interface. Adding a provider = one file here + one case in loadAdapter.
export interface AuthAdapter {
  // Fresh per call; the adapter caches and refreshes. fresh=true forces a
  // refresh (the server answered 401).
  getToken(fresh?: boolean): Promise<string | null>
  // The signed-in parent, or null when signed out. id is the provider's
  // user ID: it changes when a different parent signs in, email may not.
  user(): AuthUser | null
  // Fires on sign-in and sign-out. Returns the unsubscribe function.
  onChange(cb: () => void): () => void
  signOut(): Promise<void>
  // The provider's sign-in form, rendered inside /login.
  SignIn: ComponentType
}

export interface AuthUser {
  id: string
  email: string
}

let loading: Promise<AuthAdapter> | null = null
let signingOut: Promise<void> | null = null
let sessionEnded = false

// signOut is the one way out, from the button and from a dead session:
// analytics must forget the parent either way, and concurrent 401s must
// not fire one provider sign-out each.
export function signOut(reason?: 'session-ended'): Promise<void> {
  if (reason) sessionEnded = true
  if (!signingOut) {
    resetAnalytics()
    signingOut = getAuth()
      .then((a) => a.signOut())
      .catch((err) => console.warn('sign-out failed', err))
      .finally(() => {
        signingOut = null
      })
  }
  return signingOut
}

// sessionEndedByServer reports that the last sign-out was forced by the
// server rejecting the session, so /login can say why the parent is there.
// The next sign-in clears it.
export function sessionEndedByServer(): boolean {
  return sessionEnded
}

// getAuth boots the adapter once per page. Kid surfaces and public pages
// never call it, so they never download an auth SDK.
export function getAuth(): Promise<AuthAdapter> {
  if (!loading) {
    loading = loadAdapter().catch((err) => {
      loading = null // let a later visit retry a failed boot
      throw err
    })
  }
  return loading
}

async function loadAdapter(): Promise<AuthAdapter> {
  const cfg = await api.bootConfig()
  const adapter = await createAdapter(cfg.auth)
  setParentUnauthorizedHandler(() => void signOut('session-ended'))
  adapter.onChange(() => {
    if (adapter.user()) sessionEnded = false
  })
  return adapter
}

// One static import() per provider: Vite splits each into its own chunk,
// so a deployment downloads only its own provider's SDK. A computed
// import path would defeat that.
async function createAdapter(cfg: AuthBootConfig | undefined): Promise<AuthAdapter> {
  switch (cfg?.provider) {
    case 'supabase':
      return (await import('./supabase')).createSupabaseAdapter(cfg)
    case 'test':
      return (await import('./test')).createTestAdapter()
    default:
      throw new Error(`This server's sign-in provider (${cfg?.provider ?? 'none'}) isn't supported by this app version`)
  }
}
