// Mint a parent JWT for the `test` auth preset (local dev and E2E).
// Secret must match MATHIZ_AUTH_HS256_SECRET. The server accepts it only
// with MATHIZ_AUTH_PROVIDER=test and MATHIZ_AUTH_ALLOW_TEST=1.
// Usage: node mint-jwt.mjs [sub] [email]
import crypto from 'node:crypto'

const secret = process.env.MATHIZ_AUTH_HS256_SECRET ?? 'e2e-test-secret-with-plenty-of-length!!'
const sub = process.argv[2] ?? 'e2e-parent-1'
const email = process.argv[3] ?? 'parent@e2e.test'

const b64 = (o) => Buffer.from(JSON.stringify(o)).toString('base64url')
const header = b64({ alg: 'HS256', typ: 'JWT' })
const payload = b64({
  sub,
  aud: 'authenticated',
  email,
  email_verified: true,
  iss: 'mathiz-test',
  exp: Math.floor(Date.now() / 1000) + 3600,
  iat: Math.floor(Date.now() / 1000),
  user_metadata: { full_name: 'E2E Parent' },
})
const sig = crypto.createHmac('sha256', secret).update(`${header}.${payload}`).digest('base64url')
console.log(`${header}.${payload}.${sig}`)
