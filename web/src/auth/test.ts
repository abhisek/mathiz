import { createElement } from 'react'
import type { AuthAdapter, AuthUser } from '.'
import TestSignIn from './TestSignIn'

// Test adapter: the parent pastes a token minted for the server's `test`
// preset (.claude/skills/saas-e2e/assets/mint-jwt.mjs). The server offers
// this provider only with MATHIZ_AUTH_ALLOW_TEST=1, so it exists for local
// development and browser E2E, never production.
const storageKey = 'mathiz.testAuthToken'

function readToken(): string | null {
  try {
    return sessionStorage.getItem(storageKey)
  } catch {
    return null
  }
}

// userFromToken reads sub and email for display only. The server verifies
// the signature; the browser never trusts these claims.
function userFromToken(token: string): AuthUser {
  try {
    const b64 = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
    const claims = JSON.parse(new TextDecoder().decode(bytes)) as { sub?: unknown; email?: unknown }
    return {
      id: typeof claims.sub === 'string' ? claims.sub : token,
      email: typeof claims.email === 'string' ? claims.email : '',
    }
  } catch {
    return { id: token, email: '' }
  }
}

export function createTestAdapter(): AuthAdapter {
  const listeners = new Set<() => void>()
  let current = readToken()
  const setToken = (token: string | null) => {
    try {
      if (token) sessionStorage.setItem(storageKey, token)
      else sessionStorage.removeItem(storageKey)
    } catch {
      // Storage blocked: the token lives for this page view only.
    }
    current = token
    for (const cb of listeners) cb()
  }

  return {
    getToken: async () => current,
    user: () => (current ? userFromToken(current) : null),
    onChange(cb) {
      listeners.add(cb)
      return () => {
        listeners.delete(cb)
      }
    },
    async signOut() {
      setToken(null)
    },
    SignIn: () => createElement(TestSignIn, { onToken: setToken }),
  }
}
