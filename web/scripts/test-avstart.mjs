// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A film starts with its picture, and with its sound in step, wherever it starts: from the beginning,
// part way through (resume), and after a seek. In the web player, against a real server, on the
// flash-and-beep clips (scripts/test/nas-playback.sh makes them, in the shapes real libraries have:
// open-GOP H.264, HEVC 10-bit, sound that starts after the picture).
//
// For each start: how long the picture is black, or stands still, while the clock runs (more than
// 0.2 s fails), and where the sound is against the flashes over the next 15 s (more than 40 ms from
// the same clip started at the beginning fails; the offset at the beginning itself is the measuring
// path's own, and is checked against the sound-first clip's in the summary).
//
//   WMS_URL=… WMS_TOKEN=… node scripts/test-avstart.mjs <item id>...
import { launch } from './cdp.mjs'

const url = process.env.WMS_URL
const token = process.env.WMS_TOKEN
const items = process.argv.slice(2)
if (!url || !token || !items.length) { console.error('usage: WMS_URL=… WMS_TOKEN=… node scripts/test-avstart.mjs <item id>...'); process.exit(2) }
const BLACK_LIMIT_MS = 200
const AV_LIMIT_MS = 40
const MEASURE_SECONDS = Number(process.env.AVSTART_SECONDS || 15)
const STARTS = (process.env.AVSTART_STARTS || '0,37.3,seek:80').split(',')

// The meter: flash and beep onsets on the page's clock (as test-avsync.mjs), and every 4 ms whether
// the picture is black, or the same as before while the clock moved on.
const METER = `(() => {
  const v = document.querySelector('video')
  v.muted = false; v.volume = 1
  const ctx = new AudioContext(); ctx.resume()
  const src = ctx.createMediaElementSource(v)
  const sp = ctx.createScriptProcessor(1024, 2, 2)
  const g = ctx.createGain(); g.gain.value = 0
  src.connect(sp); sp.connect(g); g.connect(ctx.destination)
  const av = window.__av = { beeps: [], flashes: [], black: 0, held: 0, firstFrame: null, armedAt: performance.now() }
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
  const c = document.createElement('canvas'); c.width = 32; c.height = 18
  const cx = c.getContext('2d', { willReadFrequently: true })
  let lit = false, last = '', lastT = -1, lastAt = performance.now(), sameSince = performance.now()
  setInterval(() => {
    const now = performance.now(), dt = now - lastAt; lastAt = now
    if (v.paused || v.readyState < 2) return
    cx.drawImage(v, 0, 0, 32, 18)
    const d = cx.getImageData(0, 0, 32, 18).data
    let all = 0, mid = 0, n = 0, sig = ''
    for (let y = 0; y < 18; y++) for (let x = 0; x < 32; x++) {
      const i = (y * 32 + x) * 4, l = (d[i] + d[i + 1] + d[i + 2]) / 3
      all += l
      if (x >= 12 && x < 20 && y >= 6 && y < 12) { mid += d[i]; n++ }
      sig += (l >> 3).toString(32) // the test picture moves all over, every frame
    }
    all /= 32 * 18; mid /= n
    const moving = v.currentTime !== lastT; lastT = v.currentTime
    if (moving) {
      if (av.firstFrame == null) av.firstFrame = now
      if (all < 16) av.black += dt
      // Held: the same picture for clearly longer than a frame (24 fps shows each for 42 ms) while the clock runs.
      else if (sig === last && now - sameSince > 120) av.held += dt
    }
    if (sig !== last) sameSince = now
    last = sig
    if (mid > 180 && !lit) { lit = true; av.flashes.push(now) } else if (mid < 60) lit = false
  }, 4)
  window.__avOffsets = () => av.flashes.map((f) => {
    let best = null
    for (const b of av.beeps) { const d = b - f; if (Math.abs(d) < 600 && (best == null || Math.abs(d) < Math.abs(best))) best = d }
    return best
  }).filter((d) => d != null)
  return true
})()`

const median = (a) => { const s = [...a].sort((x, y) => x - y); return s.length ? s[Math.floor(s.length / 2)] : NaN }
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const browser = await launch({ software: true, args: ['--autoplay-policy=no-user-gesture-required'] })
const p = await browser.open('about:blank')
let fails = 0
try {
  for (const id of items) {
    const lines = []
    let base = NaN
    for (const start of STARTS) {
      try {
      const seekTo = start.startsWith('seek:') ? Number(start.slice(5)) : null
      // One tab, navigated from film to film: leaving a page stops its film, and its stream on the server.
      await p.go(url + '/')
      await p.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}), true`)
      await p.go(`${url}/play/${id}${seekTo == null && Number(start) > 0 ? `?start=${start}` : ''}`)
      await p.until(`document.querySelector('video') && document.querySelector('video').readyState >= 1`, 60000, 'the video to open').catch(async (e) => { throw new Error(e.message + '; the page says: ' + (await p.eval('document.body.innerText.slice(0, 160)'))) })
      await p.eval(METER)
      await p.eval(`(() => { const v = document.querySelector('video'); v.play(); return true })()`)
      if (seekTo != null) {
        await wait(4000)
        await p.eval(`(() => { const v = document.querySelector('video'); window.__av.flashes = []; window.__av.beeps = []; window.__av.black = 0; window.__av.held = 0; window.__av.firstFrame = null; window.__av.armedAt = performance.now(); return true })()`)
        // As a viewer does: the player's own +10 s key, so a rewrapped film restarts its stream as it should.
        for (let k = 0; k < Math.round(seekTo / 10); k++) { await p.key('l', 'KeyL'); await wait(150) }
      }
      await wait(MEASURE_SECONDS * 1000)
      if (process.env.AVSTART_TRACE) console.log('  trace:', JSON.stringify(await p.eval(`__av.flashes.map((f, i) => [Math.round((f - __av.armedAt) / 1000), __avOffsets()[i] == null ? null : Math.round(__avOffsets()[i])])`)))
      const r = await p.eval(`(() => { const v = document.querySelector('video'); return { offs: __avOffsets(), black: Math.round(__av.black), held: Math.round(__av.held), first: __av.firstFrame == null ? null : Math.round(__av.firstFrame - __av.armedAt), t: v.currentTime } })()`)
      const m = median(r.offs)
      if (Number.isNaN(base)) base = m
      const off = m - base
      const okPicture = r.black <= BLACK_LIMIT_MS && r.held <= BLACK_LIMIT_MS && r.first != null
      const okSound = r.offs.length >= 3 && Math.abs(off) <= AV_LIMIT_MS
      if (!okPicture || !okSound) fails++
      lines.push(`${seekTo != null ? 'skip +' + seekTo + ' s' : start === '0' ? 'from the start' : 'resume at ' + start}: picture after ${r.first} ms, black ${r.black} ms, held ${r.held} ms${okPicture ? '' : ' FAIL'}; sound median ${Math.round(m)} ms (${off >= 0 ? '+' : ''}${Math.round(off)} against the start)${okSound ? '' : ' FAIL'}; at ${r.t.toFixed(1)} s`)
      } catch (e) { fails++; lines.push(`${start}: FAIL, ${String(e.message || e).slice(0, 120)}`) }
    }
    console.log(`item ${id}:\n  ${lines.join('\n  ')}`)
  }
} finally {
  await browser.close()
}
if (fails) { console.log(`FAIL: ${fails} starts with black or held picture over ${BLACK_LIMIT_MS} ms, or sound more than ${AV_LIMIT_MS} ms out`); process.exit(1) }
console.log('ok: every start has its picture at once and its sound in step')
