import { createClient, type SupabaseClient } from '@supabase/supabase-js'
import { api } from './api'

// The Supabase client is created from server-provided boot config, so one
// build works against any environment.
let client: SupabaseClient | null = null

export async function getSupabase(): Promise<SupabaseClient> {
  if (client) return client
  const cfg = await api.bootConfig()
  if (!cfg.supabaseUrl || !cfg.supabaseAnonKey) {
    throw new Error('Server is missing Supabase configuration')
  }
  client = createClient(cfg.supabaseUrl, cfg.supabaseAnonKey)
  return client
}
