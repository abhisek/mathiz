// Renders card.html → ../public/og.png at exactly 1200×630.
// Run from the repo root (playwright can come from a global install):
//   NODE_PATH=$(npm root -g) node web/og/render.cjs
// Chromium path defaults to the CI/sandbox install; override with OG_CHROMIUM.
const path = require('node:path')
const { chromium } = require('playwright')

const exe = process.env.OG_CHROMIUM ?? '/opt/pw-browsers/chromium'

;(async () => {
  const browser = await chromium.launch({ executablePath: exe }).catch(() => chromium.launch())
  const page = await browser.newPage({ viewport: { width: 1200, height: 630 }, deviceScaleFactor: 1 })
  await page.goto('file://' + path.join(__dirname, 'card.html'))
  await page.waitForTimeout(200)
  await page.screenshot({ path: path.join(__dirname, '..', 'public', 'og.png') })
  await browser.close()
  console.log('wrote web/public/og.png')
})()
