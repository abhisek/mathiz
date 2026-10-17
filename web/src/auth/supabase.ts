import { createElement } from 'react'
import { createClient, type Session } from '@supabase/supabase-js'
import type { AuthBootConfig } from '../api'
import type { AuthAdapter } from '.'
import SupabaseSignIn from './SupabaseSignIn'

// Supabase Auth adapter: the only file in the SPA that imports
// supabase-js. Mathiz uses Supabase for parent sign-in only; no app data
// lives there.
export async function createSupabaseAdapter(cfg: AuthBootConfig): Promise<AuthAdapter> {
  if (!cfg.url || !cfg.publicKey) {
    throw new Error('The server is missing its sign-in configuration')
  }
  const client = createClient(cfg.url, cfg.publicKey)
  let session: Session | null = (await client.auth.getSession()).data.session
  const listeners = new Set<() => void>()
  client.auth.onAuthStateChange((_event, s) => {
    session = s
    for (const cb of listeners) cb()
  })

  return {
    async getToken(fresh) {
      const { data } = fresh ? await client.auth.refreshSession() : await client.auth.getSession()
      return data.session?.access_token ?? null
    },
    user: () => (session ? { id: session.user.id, email: session.user.email ?? '' } : null),
    onChange(cb) {
      listeners.add(cb)
      return () => {
        listeners.delete(cb)
      }
    },
    async signOut() {
      await client.auth.signOut()
    },
    SignIn: () => createElement(SupabaseSignIn, { client }),
  }
}
