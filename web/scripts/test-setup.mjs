// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Walks first-time setup in a headless browser at TV size, with arrow keys and OK alone, on a test
// server with a fresh data folder. No window opens, and Choose folder is never pressed. On each
// page it maps where every arrow goes from every control, and fails anything the arrows can't
// reach, any control with no visible focus, and anything to press that's under 48px. Then it checks
// that what was chosen took effect.
//
// The server has to have no accounts yet, and modules to install: scripts/make-test-catalog.sh, and
// a server built with -tags updatetest, as docs/MODULES.md says. WMS_MEDIA is a folder of test media
// with a Movies folder in it.
//
//   WMS_SERVER=http://setup.localhost:9811 WMS_MEDIA=/tmp/media node scripts/test-setup.mjs out-folder
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(process.env.WITTYVISUALIZER ? new URL(process.env.WITTYVISUALIZER + '/tests/cdp.mjs', import.meta.url).href : './cdp.mjs')

const server = process.env.WMS_SERVER || 'http://setup.localhost:9811'
const media = process.env.WMS_MEDIA || '/tmp/media'
const [out = '.'] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const KEYS = { ArrowUp: 38, ArrowDown: 40, ArrowLeft: 37, ArrowRight: 39, Enter: 13 }
const browser = await launch({ software: true, width: 1920, height: 1080 })
const page = await browser.open(server + '/?tv=1')
await page.send('Emulation.setDeviceMetricsOverride', { width: 1920, height: 1080, deviceScaleFactor: 1, mobile: false })
const problems = new Set()
const results = []
const expect = (what, ok, detail = '') => { results.push(`${ok ? 'ok  ' : 'FAIL'} ${what}${ok ? '' : ' ' + JSON.stringify(detail)}`); if (!ok) process.exitCode = 1 }

const press = async (key) => {
  const windowsVirtualKeyCode = KEYS[key]
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key, code: key, windowsVirtualKeyCode, text: key === 'Enter' ? '\r' : undefined })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key, code: key, windowsVirtualKeyCode })
}
const type = (text) => page.send('Input.insertText', { text })
const shot = async (name) => writeFileSync(join(out, `setup-${name}.png`), await page.shot())
const call = (method, path, body) => page.eval(`(async () => { const t = localStorage.getItem('wms.token'); const r = await fetch('/api${path}', { method: '${method}', headers: { 'Content-Type': 'application/json', ...(t ? { Authorization: 'Bearer ' + t } : {}) }, body: ${body ? `JSON.stringify(${JSON.stringify(body)})` : 'undefined'} }); return r.json().catch(() => null) })()`)
const atStep = (id) => page.until(`location.pathname === '/setup/${id}' && document.querySelector('main h1')`, 30000, `step ${id}`)

const tagAll = () => page.eval(`(() => {
  const FOCUSABLE = 'a[href], button:not([disabled]), summary, input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
  const seen = []
  let n = window.__n || 0
  for (const el of document.querySelectorAll(FOCUSABLE)) {
    const r = el.getBoundingClientRect()
    if (r.width < 2 || r.height < 2 || (el.closest('details:not([open])') && !el.closest('summary'))) continue
    if (!el.dataset.t) el.dataset.t = String(++n)
    seen.push(el.dataset.t)
  }
  window.__n = n
  return seen
})()`)
const nameOf = (id) => page.eval(`(() => { const e = document.querySelector('[data-t="${id}"]'); return e ? (e.getAttribute('aria-label') || e.textContent || e.placeholder || e.tagName).trim().slice(0, 60) : '?' })()`)
const active = () => page.eval(`document.activeElement?.dataset?.t || ''`)

async function map(label) {
  const all = await tagAll()
  const graph = new Map()
  const start = await active()
  for (const id of all) {
    const edges = {}
    for (const k of ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight']) {
      await page.eval(`document.querySelector('[data-t="${id}"]')?.focus({ preventScroll: true })`)
      await press(k)
      await wait(30)
      edges[k] = await active()
    }
    graph.set(id, edges)
  }
  if (start) await page.eval(`document.querySelector('[data-t="${start}"]')?.focus({ preventScroll: true })`)
  const reached = new Set([all[0]]); const q = [all[0]]
  while (q.length) for (const t of Object.values(graph.get(q.shift()) || {})) if (t && !reached.has(t) && graph.has(t)) { reached.add(t); q.push(t) }
  for (const a of all) if (!reached.has(a)) problems.add(`${label}: the arrows can't reach "${await nameOf(a)}"`)
  const small = await page.eval(`[...document.querySelectorAll('main button, main select, main textarea, main input:not([type=hidden])')].filter((e) => { const r = e.getBoundingClientRect(); return r.width > 2 && r.height > 2 && r.height < 48 }).map((e) => (e.getAttribute('aria-label') || e.textContent || e.tagName).trim().slice(0, 40))`)
  for (const s of small) problems.add(`${label}: under 48px: "${s}"`)
  if (await page.eval(`document.documentElement.scrollWidth > document.documentElement.clientWidth + 1`)) problems.add(`${label}: sideways scroll`)
  return { all, graph }
}

async function arrowTo(label, wanted) {
  const { all, graph } = await map(label)
  let target = ''
  for (const id of all) if (wanted(await nameOf(id))) { target = id; break }
  if (!target) throw new Error(`${label}: nothing there to reach`)
  let cur = await active()
  if (!graph.has(cur)) { await press('ArrowDown'); await wait(60); await tagAll(); cur = await active() }
  if (!graph.has(cur)) throw new Error(`${label}: an arrow lands nowhere`)
  // Looks before each press, as someone would: coming back into a group lands where it was left,
  // so where an arrow goes can differ from the map.
  for (let tries = 0; cur !== target; tries++) {
    if (tries > 40) throw new Error(`${label}: no route to "${await nameOf(target)}"`)
    const prev = new Map([[cur, null]]); const q = [cur]
    while (q.length && !prev.has(target)) {
      const c = q.shift()
      for (const [k, t] of Object.entries(graph.get(c) || {})) if (t && !prev.has(t)) { prev.set(t, [c, k]); q.push(t) }
    }
    if (!prev.has(target)) throw new Error(`${label}: no route to "${await nameOf(target)}"`)
    let step = target
    while (prev.get(step)[0] !== cur) step = prev.get(step)[0]
    await press(prev.get(step)[1]); await wait(60)
    cur = await active()
    const shows = await page.eval(`(() => { const e = document.activeElement; const s = getComputedStyle(e); return e !== document.body && ((s.outlineStyle !== 'none' && parseFloat(s.outlineWidth) > 0) || s.boxShadow !== 'none') })()`)
    if (!shows) problems.add(`${label}: no visible focus on "${await nameOf(await active())}"`)
  }
}
const ok = async (label, wanted) => { await arrowTo(label, wanted); await press('Enter'); await wait(250) }
const says = (t) => (n) => n.includes(t)
const named = (t) => (n) => n === t

try {
  await wait(800)
  expect('it starts on the welcome page', await page.eval(`location.pathname`) === '/setup/welcome')
  expect('restoring a backup is on the first page', await page.eval(`[...document.querySelectorAll('button')].some((b) => b.textContent === 'Restore from a backup')`))
  await ok('welcome', says('Set up a new server'))
  await atStep('account'); await wait(500)
  await type('owner')
  await press('ArrowDown'); await wait(60); await type('a long test password')
  await press('ArrowDown'); await wait(60); await type('a long test password')
  await ok('account', says('I understand'))
  expect('OK ticks a box', await page.eval(`document.querySelector('.su-ack button').getAttribute('aria-checked')`) === 'true')
  await ok('account', says('Create account'))
  await atStep('modules'); await wait(1000)
  expect('nothing has gone online yet', !(await call('GET', '/modulestore')).checkedAt)
  await ok('modules', says('WittyPlay'))
  await ok('modules', says('Install them by themselves'))
  await shot('modules')
  await ok('modules', says('Download and install'))
  await page.until(`/ready|didn’t install/i.test(document.querySelector('h1')?.textContent || '')`, 240000, 'modules to install')
  await wait(300)
  const mods = (await call('GET', '/modules')).modules
  expect('WittyPlay came through the store', mods.find((m) => m.id === 'wittyplay')?.installed && (await call('GET', '/modulestore/run')).run?.kind === 'install')
  const set1 = await call('GET', '/settings')
  expect('updates by themselves was saved', set1.moduleAutoInstall === '1' && set1.moduleUpdateCheck === '1')
  await press('Enter')
  await atStep('media'); await wait(600)
  await page.eval(`(document.activeElement || document.body).dispatchEvent(new KeyboardEvent('keydown', { key: 'GoBack', bubbles: true }))`)
  await atStep('modules')
  expect('the remote\'s Back goes back a step', true)
  await page.eval(`history.forward()`)
  await atStep('media'); await wait(600)
  await ok('media', says('Movies'))
  await wait(300)
  await arrowTo('media', named('Folder path'))
  await type(media + '/Movies')
  await wait(100)
  await ok('media', says('Add Movies'))
  await wait(800)
  await ok('media', named('Continue'))
  await atStep('games'); await wait(400)
  expect('the games line is word for word', await page.eval(`document.querySelector('.lead').textContent`) === 'Point it at your games folder and leave it. Anything it can’t prove is listed for you to look at.')
  await ok('games', named('Continue'))
  expect('no discs step without a disc module', await page.eval(`location.pathname`) === '/setup/details' || await atStep('details'))
  await wait(400)
  await ok('details', named('Continue'))
  await atStep('logos'); await wait(400)
  expect('logos are asked about with their rights notice', await page.eval(`!!document.querySelector('[data-legal=logos]')`) && (await call('GET', '/logos/status')).consent === '')
  await ok('logos', says('No thanks, use plain names'))
  await atStep('music'); await wait(400)
  expect('logos were declined', (await call('GET', '/logos/status')).consent === 'no')
  await ok('music', says('Find lyrics online'))
  await ok('music', named('Continue'))
  await atStep('listings'); await wait(400)
  expect('streaming lists are off', !(await call('GET', '/listings/status')).enabled)
  await ok('listings', says('Leave it off and continue'))
  await atStep('look'); await wait(800)
  await arrowTo('look', says('Paper'))
  expect('passing through a choice doesn\'t choose it', await page.eval(`document.activeElement.getAttribute('aria-checked')`) === 'false')
  await press('Enter'); await wait(250)
  await shot('look')
  await ok('look', named('Continue'))
  await atStep('done'); await wait(1000)
  await map('done')
  await shot('done')
  const said = await page.eval(`document.querySelector('main').textContent`)
  for (const w of ['Library transcoding', 'Modules: ', 'Module updates: installed by themselves', 'Logos: plain names', 'Lyrics: looked up online', 'Streaming lists: off']) expect(`the last page says "${w}"`, said.includes(w))
  const set2 = await call('GET', '/settings')
  expect('the answers were saved', set2.theme === 'paper' && set2.lyricsOnline === '1' && set2.musicInfoOnline === '0', [set2.theme, set2.lyricsOnline, set2.musicInfoOnline])
  await arrowTo('done', says('Start watching'))
  await press('Enter')
  await page.until(`!location.pathname.startsWith('/setup')`, 15000, 'leaving setup')
  expect('setup is over', !(await call('GET', '/status')).onboarding)
} catch (e) {
  expect('STOPPED: ' + e.message, false)
  await shot('stopped').catch(() => {})
} finally {
  for (const p of problems) expect(p, false)
  console.log(results.join('\n'))
  page.close()
  await browser.close()
}
