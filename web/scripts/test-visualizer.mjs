// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The music visualizer listens to the song however the full player was reached: a song started in the mini
// player and opened later, the full player opened straight away, closed and opened again, the next song, and
// pause then play. Each time the analyser must hear something (not all zeros) and the picture must be drawing.
// Bars (the player's own), and a WittyVisualizer effect when the server has the module.
//
// A real browser's autoplay rules, not the headless default of no rules, and real clicks, so a sound engine
// that waits for a press is caught. No window opens. Needs a music library with at least two songs.
//
//   WMS_SERVER=http://127.0.0.1:9811 WMS_NAME=... WMS_PASS=... node scripts/test-visualizer.mjs [out-folder]
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { launch } from './cdp.mjs'

const server = process.env.WMS_SERVER || 'http://127.0.0.1:9811'
const out = process.argv[2] || 'visualizer-out'
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const call = async (method, path, body, token) => {
  const r = await fetch(server + '/api' + path, { method, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) }, body: body ? JSON.stringify(body) : undefined })
  return r.json().catch(() => ({}))
}
const { token } = await call('POST', '/login', { name: process.env.WMS_NAME, password: process.env.WMS_PASS })
if (!token) throw new Error('could not sign in')
const lib = (await call('GET', '/libraries', null, token)).find((l) => l.type === 'music')
if (!lib) throw new Error('no music library')
const prefs = await call('GET', '/prefs', null, token)
const styles = ['bars', ...(prefs.visualizer ? ['molten'] : [])]

// What the page can't show from outside: what every analyser read, and how often the Bars canvas was drawn.
const HOOKS = `
  window.__heard = { freq: 0, wave: 0, reads: 0 }
  const freq = AnalyserNode.prototype.getByteFrequencyData
  AnalyserNode.prototype.getByteFrequencyData = function (a) { const r = freq.call(this, a); window.__heard.reads++; for (const v of a) if (v > window.__heard.freq) window.__heard.freq = v; return r }
  const wave = AnalyserNode.prototype.getFloatTimeDomainData
  AnalyserNode.prototype.getFloatTimeDomainData = function (a) { const r = wave.call(this, a); window.__heard.reads++; for (const v of a) if (Math.abs(v) > window.__heard.wave) window.__heard.wave = Math.abs(v); return r }
  window.__draws = 0
  const clear = CanvasRenderingContext2D.prototype.clearRect
  CanvasRenderingContext2D.prototype.clearRect = function (...a) { if (this.canvas.classList.contains('np-canvas')) window.__draws++; return clear.apply(this, a) }
`
const AUDIO = `[...document.querySelectorAll('audio')].find((a) => a.src && !a.paused)`
// The browser's own default (a press first), not the test launcher's "anything may play".
const b = await launch({ width: 1400, height: 900, args: ['--autoplay-policy=document-user-activation-required'] })
const results = []
let failed = 0

const click = async (p, sel) => {
  const at = await p.eval(`(() => { const e = document.querySelector(${JSON.stringify(sel)}); if (!e) return null; e.scrollIntoView({ block: 'center' }); const r = e.getBoundingClientRect(); return { x: r.x + r.width / 2, y: r.y + r.height / 2 } })()`)
  if (!at) throw new Error('nothing to press at ' + sel)
  for (const type of ['mousePressed', 'mouseReleased']) await p.send('Input.dispatchMouseEvent', { type, x: at.x, y: at.y, button: 'left', clickCount: 1 })
}

// Listening and drawing, over a few seconds: the effect may first blend in from the last one it showed.
const check = async (p, style, label) => {
  const shader = style !== 'bars'
  let got = null
  const end = Date.now() + 9000
  while (Date.now() < end) {
    await p.eval(`window.__heard = { freq: 0, wave: 0, reads: 0 }; window.__draws = 0`)
    await wait(1500)
    got = await p.eval(`({ heard: window.__heard, draws: window.__draws, playing: !!${AUDIO},
      fps: +(document.querySelector('.np-shader .wv-stage')?.dataset.fps || 0), tap: document.querySelector('.np-shader .wv-stage')?.dataset.tap || '',
      shown: !!document.querySelector('.np-shader.shown'), canvas: !!document.querySelector('.np-canvas') })`)
    got.ok = got.playing && (shader ? got.shown && got.fps > 0 && got.tap === 'live' && got.heard.wave > 0.01 : got.canvas && got.draws > 10 && got.heard.freq > 0)
    if (got.ok) break
  }
  if (!got.ok) { failed++; writeFileSync(join(out, `${style}-${label.replace(/\W+/g, '-')}.png`), await p.shot()) }
  results.push({ style, label, ...got })
  console.log(got.ok ? 'ok  ' : 'FAIL', style, label, JSON.stringify(got))
}

const scenario = async (style, name, fn) => {
  const p = await b.open()
  try {
    await p.send('Page.addScriptToEvaluateOnNewDocument', { source: HOOKS })
    await p.go(server + '/')
    await p.eval(`fetch('/api/login', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: ${JSON.stringify(process.env.WMS_NAME)}, password: ${JSON.stringify(process.env.WMS_PASS)} }) }).then((r) => r.json()).then((d) => localStorage.setItem('wms.token', d.token))`)
    await call('PUT', '/me/ui', { musicPlayer: { viz: true, style }, visualizerPlayer: { on: true, notice: true } }, token)
    await p.go(`${server}/library/${lib.id}?tab=songs`)
    await p.until(`document.querySelector('.mus-tr-play')`, 20000, 'the songs')
    await fn(p)
  } catch (e) {
    failed++
    results.push({ style, label: name, failed: String(e) })
    console.log('FAIL', style, name, String(e))
    writeFileSync(join(out, `${style}-${name}-failed.png`), await p.shot())
  }
  p.close()
}
const play = async (p) => { await click(p, '.mus-tr-play'); await p.until(`${AUDIO}?.currentTime > 0.3`, 20000, 'the music') }
const openFull = async (p) => { await click(p, '.ab-expand'); await p.until(`document.querySelector('.np')`, 5000, 'the full player') }

for (const style of styles) {
  await scenario(style, 'mini first', async (p) => {
    await play(p)
    await wait(4000) // playing in the mini player for a while
    await openFull(p)
    await check(p, style, 'mini player, then the full player')
    await click(p, '.np-close'); await wait(1500)
    await click(p, '.audio-bar [aria-label^="Next"]'); await wait(2500)
    await openFull(p)
    await check(p, style, 'next song in the mini player, then the full player')
    await click(p, '.np [aria-label="Pause"]'); await wait(1500)
    await click(p, '.np [aria-label="Play"]')
    await check(p, style, 'paused and played again')
  })
  await scenario(style, 'full first', async (p) => {
    await play(p)
    await openFull(p)
    await check(p, style, 'the full player straight away')
    await click(p, '.np [aria-label^="Next"]'); await wait(1500)
    await check(p, style, 'next song in the full player')
  })
}
writeFileSync(join(out, 'visualizer.json'), JSON.stringify(results, null, 1))
await b.close()
console.log(failed ? `${failed} failed` : 'all passed')
process.exit(failed ? 1 : 0)
