// Renders the marketing stills into ../public/:
//   card.html        → og.png           (1200×630 OpenGraph share card)
//   demo-poster.html → demo-poster.png  (1280×720 demo video poster)
// Run from the repo root (playwright can come from a global install):
//   NODE_PATH=$(npm root -g) node web/og/render.cjs
// Chromium path defaults to the CI/sandbox install; override with OG_CHROMIUM.
const path = require('node:path')
const { chromium } = require('playwright')

const exe = process.env.OG_CHROMIUM ?? '/opt/pw-browsers/chromium'

const RENDERS = [
  { src: 'card.html', out: 'og.png', width: 1200, height: 630 },
  { src: 'demo-poster.html', out: 'demo-poster.png', width: 1280, height: 720 },
]

;(async () => {
  const browser = await chromium.launch({ executablePath: exe }).catch(() => chromium.launch())
  for (const r of RENDERS) {
    const page = await browser.newPage({
      viewport: { width: r.width, height: r.height },
      deviceScaleFactor: 1,
    })
    await page.goto('file://' + path.join(__dirname, r.src))
    await page.waitForTimeout(200)
    await page.screenshot({ path: path.join(__dirname, '..', 'public', r.out) })
    await page.close()
    console.log(`wrote web/public/${r.out}`)
  }
  await browser.close()
})()
