// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyTV: the channel stays at the same moment from the guide's little TV to full screen and back.
//
// The guide's preview and the full-screen player share one <video> and one stream. Going back to the
// guide used to highlight the first row for a moment, before the channel just left: the preview took
// that, dropped the channel playing, and tuned it again a second later, somewhere else in it. Here the
// channel is picked in the guide, watched full screen, left for the guide and watched again, and the
// video must keep one media source, never leave or tune the channel again, and run on without a jump.
//
//   WMS_URL=http://127.0.0.1:19690 WMS_TOKEN=… node scripts/test-tv-handover.mjs <channel name> <channel key>
import { launch } from './cdp.mjs'

const url = process.env.WMS_URL
const token = process.env.WMS_TOKEN
const [name, key] = process.argv.slice(2)
if (!url || !token || !name || !key) { console.error('usage: WMS_URL=… WMS_TOKEN=… node scripts/test-tv-handover.mjs <channel name> <channel key>'); process.exit(2) }
const wait = (ms) => new Promise((r) => setTimeout(r, ms))

// Before the page: a note of every tune and leave, and of the video's media source.
const SPY = `(() => {
  window.__net = []
  const f0 = window.fetch
  window.fetch = function (u, o) { const s = String(u && u.url || u); if (s.includes('/tune') || s.includes('/leave')) window.__net.push((o && o.method || 'GET') + ' ' + s.split('?')[0].replace(location.origin, '')); return f0.apply(this, arguments) }
  const b = navigator.sendBeacon && navigator.sendBeacon.bind(navigator)
  if (b) navigator.sendBeacon = (u, d) => { window.__net.push('BEACON ' + String(u).split('?')[0].replace(location.origin, '')); return b(u, d) }
})()`
const browser = await launch({ software: true, width: 1280, height: 720 })
let failed = false
const fail = (m) => { failed = true; console.log('FAIL', m) }
try {
  const page = await browser.open(url + '/legal')
  await page.send('Page.addScriptToEvaluateOnNewDocument', { source: SPY })
  await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.setItem('wms.tvmode', '0')`)
  await page.go(url + '/tv')
  await wait(3000)
  const press = (rx) => page.eval(`(() => { const all = [...document.querySelectorAll('button, a, [role=button], [role=gridcell], [role=rowheader], [role=row], [role=option], [tabindex]')].filter((b) => b.getBoundingClientRect().width > 1 && ${rx}.test((b.getAttribute('aria-label') || '') + ' ' + b.textContent)); const e = all.sort((a, b) => a.textContent.length - b.textContent.length)[0]; if (!e) return false; e.click(); return true })()`)
  if (!(await press(`new RegExp(${JSON.stringify(name)})`))) fail(`no channel "${name}" in the guide`)
  await wait(6000)
  if (!(await press('/Watch full screen/'))) fail('no Watch full screen')
  await wait(8000)
  const state = () => page.eval(`(() => { const v = document.querySelector('video'); return { path: location.pathname, t: v ? v.currentTime : -1, src: v ? v.src : '', net: (window.__net || []).splice(0) } })()`)
  const full = await state()
  if (!full.path.endsWith('/tv/watch/' + key)) fail(`full screen is ${full.path}`)
  const samples = [full]
  await page.eval('history.back()')
  for (let i = 0; i < 10; i++) { await wait(500); samples.push(await state()) }
  await page.eval('history.forward()')
  for (let i = 0; i < 6; i++) { await wait(500); samples.push(await state()) }
  for (let i = 1; i < samples.length; i++) {
    const a = samples[i - 1], b = samples[i]
    if (b.src !== full.src) fail(`a new media source at ${b.path} (${i * 0.5} s): the stream was started again`)
    if (b.t < a.t - 0.05 || b.t > a.t + 2) fail(`the time jumped at ${b.path}: ${a.t.toFixed(2)} to ${b.t.toFixed(2)}`)
    for (const n of b.net) fail(`${n} while going between the guide and full screen`)
  }
  console.log(failed ? 'FAILED' : `ok: one stream, ${samples[0].t.toFixed(1)} s to ${samples.at(-1).t.toFixed(1)} s with no jump, no leave and no tune`)
} catch (e) {
  fail(e.message)
} finally {
  await browser.close()
}
process.exit(failed ? 1 : 0)
