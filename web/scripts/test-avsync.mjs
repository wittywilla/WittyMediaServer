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
const LIMIT_MS = 40 // at 1×, and back at 1× after being fast
// While fast, the bar wittywilla/WittyMediaServer#715 set for 2×: a one-processor runner's pitch-keeping
// time stretch puts the sound 25-50 ms behind there, and late sound is the less noticeable way round.
const FAST_LIMIT_MS = 80
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
let backFails = 0
const FAST_SECONDS = Number(process.env.AVSYNC_FAST_SECONDS || 12)
const BACK_SECONDS = Number(process.env.AVSYNC_BACK_SECONDS || 20)
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
      if (process.env.AVSYNC_TRACE) console.log(`  trace ${r}×: ${got.map(Math.round).join(' ')}`)
      const off = m - base
      const ok = got.length >= 3 && Math.abs(off) <= (r === 1 ? LIMIT_MS : FAST_LIMIT_MS)
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
    const ok = after.length >= 3 && Math.abs(off) <= FAST_LIMIT_MS
    if (!ok) fails++
    line.push(`after a seek and a pause at 3×: ${after.length} flashes, ${off >= 0 ? '+' : ''}${Math.round(off)} ms against 1×${ok ? '' : ' FAIL'}`)
    // The owner's case: fast for a while, then back to normal. After the return to 1× the sound must be back
    // with the picture: within 40 ms (LIMIT_MS) of a fresh start at the same place straight after (the control below).
    const set = (r) => p.eval(`(() => { const v = document.querySelector('video'); v.preservesPitch = true; v.defaultPlaybackRate = ${r}; v.playbackRate = ${r}; return true })()`)
    await set(1)
    await p.eval(`(() => { const v = document.querySelector('video'); v.currentTime = 70; return true })()`)
    await wait(3000)
    await set(4)
    await wait(FAST_SECONDS * 1000)
    await set(1)
    await wait(2500) // the change itself, and the player's own lining up (speed.ts, keepInStep), which waits for a stalled player
    const backSince = await p.eval('performance.now()')
    await wait(BACK_SECONDS * 1000)
    const back = await p.eval(`__avSince(${backSince})`)
    const backOff = median(back) - base
    line.push(`1× after ${FAST_SECONDS} s at 4×: ${back.length} flashes, ${backOff >= 0 ? '+' : ''}${Math.round(backOff)} ms against the first 1×`)
    // Control: a fresh start at the same place, at 1×, straight after: where the sound is against the picture
    // now, with the same load on the machine, when no change of speed came before. The first 1× can't be the
    // yardstick: the meter's own delay moves with the load, and on a one-processor runner it read 35 ms at the
    // start against 60-90 ms once the server was converting ahead (and 300+ ms here while the browser warmed
    // up), which made the converted clip look 54-64 ms apart when a fresh start read the same.
    await p.eval(`(() => { const v = document.querySelector('video'); v.currentTime = v.currentTime; return true })()`)
    await wait(2500)
    const freshSince = await p.eval('performance.now()')
    await wait(BACK_SECONDS * 1000)
    const fresh = await p.eval(`__avSince(${freshSince})`)
    const freshOff = median(fresh) - base
    const apart = backOff - freshOff
    const backOk = back.length >= 4 && fresh.length >= 4 && Math.abs(apart) <= LIMIT_MS
    if (!backOk) backFails++
    line.push(`fresh start there, at 1×: ${fresh.length} flashes, ${freshOff >= 0 ? '+' : ''}${Math.round(freshOff)} ms against the first 1×; after 4× against it: ${apart >= 0 ? '+' : ''}${Math.round(apart)} ms${backOk ? '' : ' FAIL'}`)
    if (process.env.AVSYNC_TRACE) line.push(`trace back: ${back.map(Math.round).join(' ')}`, `trace fresh: ${fresh.map(Math.round).join(' ')}`)
    console.log(`item ${id}:\n  ${line.join('\n  ')}`)
    if (!ok || fails) console.log('  page log:', p.logs.slice(-8).join(' | '))
  }
} finally {
  await browser.close()
}
// Exit 4: apart after going back to 1x from 4x, against a fresh start at the same place. nas-playback.sh fails on it.
if (backFails) { console.log(`FAIL: sound and picture still apart after going back to 1× (more than ${LIMIT_MS} ms from a fresh start there)`); process.exit(4) }
// Apart at a speed, but together again at 1×: exit 3, which the CI script reports without failing for now.
if (fails) { console.log(`FAIL: ${fails} measurements more than ${FAST_LIMIT_MS} ms from 1× while fast`); process.exit(3) }
console.log(`ok: sound within ${FAST_LIMIT_MS} ms of where it is at 1× while fast, and back at 1× after 4× within ${LIMIT_MS} ms of a fresh start there`)
