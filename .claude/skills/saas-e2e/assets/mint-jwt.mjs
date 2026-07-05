// Mint a Supabase-style HS256 parent JWT for local E2E.
// Secret must match MATHIZ_SUPABASE_JWT_SECRET; issuer must match
// MATHIZ_SUPABASE_URL + /auth/v1.
// Usage: node mint-jwt.mjs [sub] [email]
import crypto from 'node:crypto'

const secret = process.env.MATHIZ_SUPABASE_JWT_SECRET ?? 'e2e-test-secret-with-plenty-of-length!!'
const supabaseUrl = process.env.MATHIZ_SUPABASE_URL ?? 'https://dummy.supabase.co'
const sub = process.argv[2] ?? 'e2e-parent-1'
const email = process.argv[3] ?? 'parent@e2e.test'

const b64 = (o) => Buffer.from(JSON.stringify(o)).toString('base64url')
const header = b64({ alg: 'HS256', typ: 'JWT' })
const payload = b64({
  sub,
  aud: 'authenticated',
  email,
  iss: `${supabaseUrl}/auth/v1`,
  exp: Math.floor(Date.now() / 1000) + 3600,
  iat: Math.floor(Date.now() / 1000),
  user_metadata: { full_name: 'E2E Parent' },
})
const sig = crypto.createHmac('sha256', secret).update(`${header}.${payload}`).digest('base64url')
console.log(`${header}.${payload}.${sig}`)
