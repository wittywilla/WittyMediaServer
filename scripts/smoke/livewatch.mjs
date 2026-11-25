// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A live channel watched in the web app for a while (ten minutes by default), with every jump of its
// clock and every change of its speed logged with the code that made it, every wait for data, and
// every freeze: the clock standing still for 250 ms or more while it's meant to be playing. 0.8.6
// froze for a second or more every few seconds on a slow link, because its live sync jumped ahead
// past what it had loaded (TvHost.tsx, player/sync.ts liveStep). scripts/smoke/livewatch.sh runs it
// against a server held to a small NAS's CPU and memory behind a 5 Mbit/s link.
//
//   WMS_URL=... WMS_TOKEN=... node scripts/smoke/livewatch.mjs <out folder> <path> [seconds] [quality]
//
// It passes with no jumps of over a second and no freezes after the first WARMUP seconds (tune-in).
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { launch } from '../../web/scripts/cdp.mjs'

const server = process.env.WMS_URL
const token = process.env.WMS_TOKEN
const [out = 'livewatch-out', path = '/tv', secs = '600', quality = ''] = process.argv.slice(2)
const WARMUP = 20000
const FREEZE = 250
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))

let b
for (let i = 1; !b; i++) {
  try { b = await launch({ software: true, width: 1280, height: 720, tmp: process.env.SMOKE_TMP, args: ['--disable-dev-shm-usage', '--autoplay-policy=no-user-gesture-required'] }) } catch (e) { if (i === 3) throw e }
}
const p = await b.open()
let code = 1
try {
  // Every seek and change of speed, with where it came from; every wait for data.
  await p.send('Page.addScriptToEvaluateOnNewDocument', { source: `(() => {
    window.__ev = []; const t0 = performance.now()
    const who = () => (new Error().stack || '').split(String.fromCharCode(10)).slice(3, 5).map((l) => l.trim()).join(' < ')
    const note = (kind, extra) => window.__ev.push({ at: Math.round(performance.now() - t0), kind, ...extra })
    const P = HTMLMediaElement.prototype
    const ct = Object.getOwnPropertyDescriptor(P, 'currentTime'), pr = Object.getOwnPropertyDescriptor(P, 'playbackRate')
    Object.defineProperty(P, 'currentTime', { get() { return ct.get.call(this) }, set(v) { note('seek', { from: ct.get.call(this), to: +v, by: who() }); ct.set.call(this, v) }, configurable: true })
    Object.defineProperty(P, 'playbackRate', { get() { return pr.get.call(this) }, set(v) { if (+v !== pr.get.call(this)) note('rate', { to: +v, by: who() }); pr.set.call(this, v) }, configurable: true })
    document.addEventListener('waiting', (e) => { if (e.target instanceof HTMLMediaElement) note('waiting', {}) }, true)
  })()` })
  await p.go(server + '/legal')
  await p.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.setItem('wms.tvmode', '0')` +
    (quality ? `; localStorage.setItem('wms.quality', ${JSON.stringify(JSON.stringify({ home: quality, away: quality }))})` : ''))
  await p.go(server + path)
  await p.until(`(() => { const v = document.querySelector('video'); return v && v.currentTime > 0.5 && !v.paused })()`, 90000, 'the channel playing')
  const samples = []
  const t0 = Date.now()
  while (Date.now() - t0 < Number(secs) * 1000) {
    await wait(100)
    const x = await p.eval(`(() => { const v = document.querySelector('video'); if (!v) return null; const q = v.getVideoPlaybackQuality(); return { t: v.currentTime, f: q.totalVideoFrames, d: q.droppedVideoFrames, p: v.paused, behind: +(v.dataset.behind || 0), ahead: v.buffered.length ? v.buffered.end(v.buffered.length - 1) - v.currentTime : 0 } })()`).catch(() => null)
    if (x) samples.push({ at: Date.now() - t0, ...x })
  }
  const ev = await p.eval('window.__ev')
  // Freezes: the clock standing still while meant to be playing.
  const freezes = []
  let still = null
  for (let i = 1; i < samples.length; i++) {
    const a = samples[i - 1], c = samples[i]
    const stopped = !c.p && Math.abs(c.t - a.t) < 0.02
    if (stopped && !still) still = a
    if (!stopped && still) { if (c.at - still.at >= FREEZE) freezes.push({ at: still.at, ms: c.at - still.at }); still = null }
  }
  if (still && samples.at(-1).at - still.at >= FREEZE) freezes.push({ at: still.at, ms: samples.at(-1).at - still.at })
  const late = freezes.filter((f) => f.at >= WARMUP)
  // Not the start: placing a new stream at its live point is a seek from nothing.
  const jumps = ev.filter((e) => e.kind === 'seek' && e.from > 0.5 && Math.abs(e.to - e.from) > 1)
  const behind = samples.map((x) => x.behind).filter((x) => x)
  const last = samples.at(-1) || { f: 0, d: 0 }
  const summary = {
    path, quality: quality || 'full', minutes: +(Number(secs) / 60).toFixed(1), played: +(samples.at(-1)?.t - samples[0]?.t || 0).toFixed(1),
    freezes: freezes.length, freezesAfterTuneIn: late.length, frozenMs: freezes.reduce((n, f) => n + f.ms, 0), longestFreezeMs: Math.max(0, ...freezes.map((f) => f.ms)),
    jumps: jumps.length, seeks: ev.filter((e) => e.kind === 'seek').length, rateChanges: ev.filter((e) => e.kind === 'rate').length, waiting: ev.filter((e) => e.kind === 'waiting').length,
    behindMin: behind.length ? +Math.min(...behind).toFixed(2) : null, behindMax: behind.length ? +Math.max(...behind).toFixed(2) : null,
    aheadMin: +Math.min(...samples.slice(Math.floor(samples.length / 10)).map((x) => x.ahead)).toFixed(2), frames: last.f, dropped: last.d,
  }
  writeFileSync(join(out, 'livewatch.json'), JSON.stringify({ summary, ev, freezes, samples: samples.filter((_, i) => i % 10 === 0) }, null, 1))
  console.log(JSON.stringify(summary))
  for (const j of jumps.slice(0, 10)) console.log(`  jump at ${(j.at / 1000).toFixed(1)} s from ${j.from.toFixed(2)} to ${j.to.toFixed(2)}: ${j.by}`)
  for (const f of late.slice(0, 10)) console.log(`  freeze at ${(f.at / 1000).toFixed(1)} s for ${f.ms} ms`)
  const why = []
  if (summary.played < 0.9 * Number(secs)) why.push(`played only ${summary.played} s of ${secs}`)
  if (jumps.length) why.push(`${jumps.length} jumps of the clock while playing`)
  if (late.length) why.push(`${late.length} freezes of ${FREEZE} ms or more after tune-in (longest ${Math.max(...late.map((f) => f.ms))} ms)`)
  console.log(why.length ? 'FAIL: ' + why.join('; ') : 'OK')
  code = why.length ? 1 : 0
} catch (e) {
  console.log('FAIL: ' + (e.stack || e))
} finally {
  try { writeFileSync(join(out, 'livewatch.png'), await p.shot()) } catch { /* no picture */ }
  await b.close()
}
process.exit(code)
