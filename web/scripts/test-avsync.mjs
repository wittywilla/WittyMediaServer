// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Sound and picture stay together at every playback speed, in the web player, against a real server.
//
// The clips (scripts/test/nas-playback.sh makes them) flash the middle of the picture white and beep
// at the same instant every 2 seconds. In the player's page the flash's onset is read from the
// picture every 4 ms and the beep's from the element's sound through Web Audio, both on the page's
// clock. The offset at 1× is the measuring path's own (sound buffers, polling), so each speed is
// held to within 40 ms of it, by the median of its flashes: a player that let the sound drift or
// jump at a speed change shows at once.
//
//   WMS_URL=http://127.0.0.1:19811 WMS_TOKEN=… node scripts/test-avsync.mjs <item id>...
import { launch } from './cdp.mjs'

const url = process.env.WMS_URL
const token = process.env.WMS_TOKEN
const items = process.argv.slice(2)
if (!url || !token || !items.length) { console.error('usage: WMS_URL=… WMS_TOKEN=… node scripts/test-avsync.mjs <item id>...'); process.exit(2) }
const RATES = [1, 1.5, 2, 3]
const LIMIT_MS = 40
const SECONDS_EACH = 9

// Runs in the page: the meter (see the top of this file).
const METER = `(() => {
  const v = document.querySelector('video')
  v.muted = false; v.volume = 1
  const ctx = new AudioContext(); ctx.resume()
  const src = ctx.createMediaElementSource(v)
  const sp = ctx.createScriptProcessor(1024, 2, 2)
  const g = ctx.createGain(); g.gain.value = 0
  src.connect(sp); sp.connect(g); g.connect(ctx.destination)
  const av = window.__av = { beeps: [], flashes: [] }
  let on = false, quiet = 0
  sp.onaudioprocess = (e) => {
    const ch = e.inputBuffer.getChannelData(0)
    for (let k = 0; k < ch.length; k++) {
      const a = Math.abs(ch[k])
      if (!on && a > 0.2) {
        on = true; quiet = 0
        const ts = ctx.getOutputTimestamp()
        av.beeps.push(ts.performanceTime + (e.playbackTime + k / ctx.sampleRate - ts.contextTime) * 1000)
      } else if (on) { if (a < 0.05) { if (++quiet > 4800) { on = false; quiet = 0 } } else quiet = 0 }
    }
  }
  const c = document.createElement('canvas'); c.width = 16; c.height = 9
  const cx = c.getContext('2d', { willReadFrequently: true })
  let lit = false
  setInterval(() => {
    if (v.paused || v.readyState < 2) return
    cx.drawImage(v, 0, 0, 16, 9)
    const d = cx.getImageData(6, 3, 4, 3).data
    let s = 0; for (let i = 0; i < d.length; i += 4) s += d[i]; s /= d.length / 4
    if (s > 180 && !lit) { lit = true; av.flashes.push({ at: performance.now(), rate: v.playbackRate }) } else if (s < 60) lit = false
  }, 4)
  window.__avSince = (since) => av.flashes.filter((f) => f.at >= since).map((f) => {
    let best = null
    for (const b of av.beeps) { const d = b - f.at; if (Math.abs(d) < 400 && (best == null || Math.abs(d) < Math.abs(best))) best = d }
    return best
  }).filter((d) => d != null)
  return true
})()`

const median = (a) => { const s = [...a].sort((x, y) => x - y); return s.length ? s[Math.floor(s.length / 2)] : NaN }
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const browser = await launch({ software: true, args: ['--autoplay-policy=no-user-gesture-required'] })
let fails = 0
try {
  for (const id of items) {
    const p = await browser.open(url + '/')
    await p.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}), true`)
    await p.go(`${url}/play/${id}`)
    await p.until(`document.querySelector('video') && document.querySelector('video').readyState >= 2`, 60000, 'the video to start')
    await p.eval(METER)
    await p.eval(`(() => { const v = document.querySelector('video'); v.currentTime = 2; v.play(); return true })()`)
    await wait(1500)
    const line = []
    let base = NaN
    for (const r of RATES) {
      // As the player's own speed control sets it (web/src/player/speed.ts).
      await p.eval(`(() => { const v = document.querySelector('video'); v.preservesPitch = true; v.defaultPlaybackRate = ${r}; v.playbackRate = ${r}; return true })()`)
      await wait(600) // the change itself, then measure
      const since = await p.eval('performance.now()')
      await wait(SECONDS_EACH * 1000)
      const got = await p.eval(`__avSince(${since})`)
      const m = median(got)
      if (r === 1) base = m
      const off = m - base
      const ok = got.length >= 3 && Math.abs(off) <= LIMIT_MS
      if (!ok) fails++
      line.push(`${r}×: ${got.length} flashes, median ${Math.round(m)} ms (${off >= 0 ? '+' : ''}${Math.round(off)} against 1×)${ok ? '' : ' FAIL'}`)
    }
    // A seek, then a pause, at 2×: the sound mustn't come back out of step.
    await p.eval(`(() => { const v = document.querySelector('video'); v.currentTime = 30; return true })()`)
    await wait(3000)
    await p.eval(`document.querySelector('video').pause(), true`)
    await wait(1500)
    await p.eval(`document.querySelector('video').play(), true`)
    await wait(800)
    const since = await p.eval('performance.now()')
    await wait(SECONDS_EACH * 1000)
    const after = await p.eval(`__avSince(${since})`)
    const off = median(after) - base
    const ok = after.length >= 3 && Math.abs(off) <= LIMIT_MS
    if (!ok) fails++
    line.push(`after a seek and a pause at 3×: ${after.length} flashes, ${off >= 0 ? '+' : ''}${Math.round(off)} ms against 1×${ok ? '' : ' FAIL'}`)
    console.log(`item ${id}:\n  ${line.join('\n  ')}`)
    if (!ok || fails) console.log('  page log:', p.logs.slice(-8).join(' | '))
  }
} finally {
  await browser.close()
}
if (fails) { console.log(`FAIL: ${fails} measurements more than ${LIMIT_MS} ms from 1×`); process.exit(1) }
console.log(`ok: sound within ${LIMIT_MS} ms of the picture at ${RATES.join('×, ')}×, after a seek and a pause`)
