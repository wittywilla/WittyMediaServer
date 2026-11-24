// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Standalone pages (sign-in, Who's watching, sign in with your phone, can't reach the server, the apps' choice of
// server, From away), in a headless browser at the sizes people use, against the built web client (web/dist) and a
// stand-in for the server's API in this script. Nothing else is needed: no Go, no data folder. No window opens.
//
// Each page must be one card, centred both ways, with the server's logo and name on it, and fit without scrolling
// on any screen 720 pixels tall or more (docs/DESIGN.md, "Standalone pages"). Pictures of each go to the out folder.
//
//   npm run build && node scripts/test-standalone.mjs [out-folder]
import { createServer } from 'node:http'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'
const { launch } = await import('./cdp.mjs')

const dist = fileURLToPath(new URL('../dist/', import.meta.url))
const [out = ''] = process.argv.slice(2)
if (out) mkdirSync(out, { recursive: true })
if (!existsSync(join(dist, 'index.html'))) { console.error('Build the web client first: npm run build'); process.exit(2) }

const SERVER_NAME = 'Riverside Media'
const SIZES = [[1280, 720], [1366, 768], [1440, 900], [1920, 1080], [360, 640], [390, 844]]
const TYPES = { '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2', '.json': 'application/json', '.ico': 'image/x-icon', '.webmanifest': 'application/manifest+json' }

// What the stand-in answers, per page under test.
let scene = {}
const profiles = [
  { id: 1, name: 'willow', displayName: 'Willow', kind: 'adult', hasPassword: true },
  { id: 2, name: 'sam', displayName: 'Sam', kind: 'adult', hasPassword: true },
  { id: 3, name: 'kids', displayName: 'Kids', kind: 'kids', hasPassword: false },
  { id: 4, name: 'guest', displayName: 'Guest', kind: 'guest', hasPassword: false },
]
const status = () => ({ name: 'WittyMediaServer', version: '0.8.3-alpha', setupRequired: false, onboarding: false, pickerLocal: true, user: null, brandName: SERVER_NAME, legalAck: true, plainConnection: false })
function pairStart() {
  const now = Math.floor(Date.now() / 1000)
  return { sessionId: 's1', deviceSecret: 'x', pin: '481902', pinExpiresAt: now + 30, period: 30, qr: 'wms-pair:s1', code: 'QDLJ-SKDB', approveUrl: scene.approve, expiresAt: now + 300, expiresInSec: 300, intervalSec: 60 }
}
function api(path, method, res) {
  const json = (code, body) => { res.writeHead(code, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(body)) }
  if (path === '/api/status') return scene.down ? json(503, { error: 'down' }) : json(200, status())
  if (path === '/api/profiles') return json(200, scene.profiles ? { style: 'profiles', profiles } : { style: 'classic' })
  if (path === '/api/auth/pair' && method === 'POST') return scene.pairFails ? json(500, { error: 'The server is busy.' }) : json(200, pairStart())
  if (path.startsWith('/api/auth/pair/')) return json(200, path.endsWith('/pin') ? { pin: '481902', pinExpiresAt: Math.floor(Date.now() / 1000) + 30 } : { status: 'waiting' })
  if (path === '/api/login' && method === 'POST') return json(401, { error: 'Type the code from your authenticator app.', twoFactor: true })
  return json(404, { error: 'not found' })
}
const server = createServer((req, res) => {
  const url = new URL(req.url, 'http://x')
  if (url.pathname.startsWith('/api/')) { req.resume(); return req.on('end', () => api(url.pathname, req.method, res)) }
  let file = normalize(join(dist, decodeURIComponent(url.pathname)))
  if (!file.startsWith(normalize(dist)) || !extname(file)) file = join(dist, 'index.html')
  // The build keeps only a compressed copy of the larger files, which the server sends as they are.
  const gz = !existsSync(file) && existsSync(file + '.gz')
  if (!gz && !existsSync(file)) file = join(dist, 'index.html')
  res.writeHead(200, { 'Content-Type': TYPES[extname(file)] || 'application/octet-stream', ...(gz ? { 'Content-Encoding': 'gzip' } : {}) })
  res.end(readFileSync(gz ? file + '.gz' : file))
})
await new Promise((d) => server.listen(0, '127.0.0.1', d))
const base = `http://127.0.0.1:${server.address().port}`
// SA_SERVE=<scene as JSON>: only serve, to look at by hand.
if (process.env.SA_SERVE) { scene = { approve: '', ...JSON.parse(process.env.SA_SERVE) }; console.log(base); await new Promise(() => {}) }

// The pages: where to go, what the stand-in says, and what to do there first.
const PAGES = [
  { id: 'who', path: '/login', scene: { profiles: true }, ready: `document.querySelector('.who-tile')` },
  { id: 'password', path: '/login', scene: {}, ready: `document.querySelector('input[type=password]')` },
  { id: 'two-step', path: '/login', scene: {}, ready: `document.querySelector('input[type=password]')`,
    then: `(() => { const s = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set; for (const [sel, v] of [['input[type=text]', 'willow'], ['input[type=password]', 'pw']]) { const i = document.querySelector(sel); s.call(i, v); i.dispatchEvent(new Event('input', { bubbles: true })) } Array.from(document.querySelectorAll('button')).find((b) => /^Sign in$/.test(b.textContent.trim()))?.click() })()`,
    after: `document.querySelector('input[autocomplete=one-time-code]')` },
  { id: 'phone-here', path: '/login', scene: { approve: 'http://127.0.0.1:19480/pair?c=QDLJ-SKDB' }, ready: `document.querySelector('input[type=password]')`,
    then: `Array.from(document.querySelectorAll('button')).find((b) => b.textContent.trim() === 'Sign in with your phone').click()`, after: `document.querySelector('.spec-code-chars')` },
  { id: 'phone-lan', path: '/login', scene: { approve: 'http://192.168.1.20:8096/pair?c=QDLJ-SKDB' }, ready: `document.querySelector('input[type=password]')`,
    then: `Array.from(document.querySelectorAll('button')).find((b) => b.textContent.trim() === 'Sign in with your phone').click()`, after: `document.querySelector('.qr')` },
  { id: 'phone-ended', path: '/login', scene: { pairFails: true }, ready: `document.querySelector('input[type=password]')`,
    then: `Array.from(document.querySelectorAll('button')).find((b) => b.textContent.trim() === 'Sign in with your phone').click()`, after: `/Get a new code/.test(document.body.innerText)` },
  { id: 'unreachable', path: '/', scene: { down: true }, ready: `/Try again/i.test(document.body.innerText)`, name: false },
  { id: 'connect', path: '/', scene: {}, native: true, ready: `document.querySelector('input')`, name: false },
  { id: 'away', path: '/away', scene: {}, ready: `document.querySelector('.away-steps')` },
]

// The card, measured: where it is, whether the page scrolls, and the logo and name on it.
const MEASURE = `(() => {
  const card = document.querySelector('[data-standalone-card]')
  const root = document.scrollingElement
  const W = window.innerWidth, H = window.innerHeight
  const r = card ? card.getBoundingClientRect() : null
  const brand = card && card.querySelector('.sa-brand')
  const mark = brand && brand.querySelector('.brand-mark, img, svg')
  const mr = mark ? mark.getBoundingClientRect() : null
  return {
    scrolls: root.scrollHeight > H + 1, extra: root.scrollHeight - H, hscroll: root.scrollWidth > W + 1,
    card: r && { left: r.left, right: W - r.right, top: r.top, bottom: H - r.bottom, w: r.width, h: r.height },
    brand: brand ? brand.innerText.trim() : '', mark: !!mr && mr.width > 8 && mr.height > 8 && (mark.tagName !== 'IMG' || (mark.complete && mark.naturalWidth > 0)),
  }
})()`

const results = []
const expect = (what, ok, detail = '') => { results.push(`${ok ? 'ok  ' : 'FAIL'} ${what}${ok ? '' : ' ' + JSON.stringify(detail)}`); if (!ok) process.exitCode = 1 }
// A CI runner's Chrome has no sandbox to start in.
const browser = await launch({ software: true, width: 1920, height: 1080, args: process.env.CI ? ['--no-sandbox'] : [] })
try {
  for (const p of PAGES) {
    for (const tv of [false, true]) {
      const sizes = tv ? [[1920, 1080], [1280, 720]] : SIZES
      for (const [w, h] of sizes) {
        scene = { approve: '', ...p.scene }
        const page = await browser.open()
        await page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: w < 600 })
        if (p.native) await page.send('Page.addScriptToEvaluateOnNewDocument', { source: `window.Capacitor = {}` })
        await page.send('Page.addScriptToEvaluateOnNewDocument', { source: `try { localStorage.setItem('wms.tvmode', '${tv ? 1 : 0}') } catch {}` })
        const at = `${p.id}${tv ? ' TV' : ''} ${w}×${h}`
        try {
          await page.go(base + p.path + (tv ? '?tv=1' : '?tv=0'))
          await page.until(p.ready, 15000, `${at}: the page`)
          if (p.then) { await page.eval(p.then); await page.until(p.after, 15000, `${at}: the next step`) }
          // The logo has loaded (a picture), and the card has come to rest.
          await page.until(`[...document.querySelectorAll('[data-standalone-card] .sa-brand img')].every((i) => i.complete && i.naturalWidth > 0)`, 10000, `${at}: the logo`).catch(() => {})
          // The card slides in: measured where it comes to rest.
          await page.eval(`document.fonts.ready.then(() => { document.getAnimations().forEach((a) => { try { a.finish() } catch {} }); return new Promise((d) => requestAnimationFrame(() => requestAnimationFrame(d))) })`)
          const m = await page.eval(MEASURE)
          if (out) writeFileSync(join(out, `${p.id}${tv ? '-tv' : ''}-${w}x${h}.png`), await page.shot())
          expect(`${at}: one card`, !!m.card, m)
          expect(`${at}: no sideways scrolling`, !m.hscroll)
          if (h >= 720) expect(`${at}: fits without scrolling`, !m.scrolls, m.extra)
          if (m.card) {
            expect(`${at}: centred across`, Math.abs(m.card.left - m.card.right) <= 4, m.card)
            if (!m.scrolls) expect(`${at}: centred down`, Math.abs(m.card.top - m.card.bottom) <= 4, m.card)
          }
          expect(`${at}: the server's logo`, m.mark, m)
          if (p.name !== false) expect(`${at}: the server's name`, m.brand.includes(SERVER_NAME), m.brand)
          else expect(`${at}: a name at the top`, m.brand.length > 0, m.brand)
        } catch (e) {
          expect(at, false, e.message)
          if (out) writeFileSync(join(out, `${p.id}${tv ? '-tv' : ''}-${w}x${h}.png`), await page.shot())
        }
        page.close()
      }
    }
  }
} finally {
  await browser.close()
  server.close()
}
console.log(results.join('\n'))
console.log(`${results.filter((r) => r.startsWith('ok')).length} ok, ${results.filter((r) => r.startsWith('FAIL')).length} failed`)
process.exit(process.exitCode || 0)
