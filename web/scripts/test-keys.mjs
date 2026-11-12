// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Walks every page of the app with the keyboard alone, in a headless browser on a running test
// server with libraries. No window opens, and nothing is pressed but Tab, Shift+Tab and the
// arrows, so nothing is chosen, saved or deleted, except that an arrow on a slider or a choice
// moves it (which is what a remote does too).
//
// It finds the pages by following the app's own links from Home, the side menu and Settings, and
// takes one page of each kind (one film, one show, one album and so on). On each page, at desktop
// size, it presses Tab from the top to the end and fails:
// - a control Tab never reaches, or a place Tab can't leave;
// - a control with no name for a screen reader (from the browser's own accessibility tree);
// - focus that doesn't show, or lands off screen or under something else, such as the top bar;
// - a page with no main heading or no main landmark, or a picture with no alt text.
// Then in big screen mode at TV size it maps where every arrow goes from every control, and fails
// anything the arrows can't reach from where a remote starts.
//
//   WMS_SERVER=http://keys.localhost:9811 WMS_USER=owner WMS_PASSWORD=... node scripts/test-keys.mjs out-folder
//
// WMS_PAGES=/settings/users,/fix-matches walks only those. WMS_ONLY=tab or WMS_ONLY=arrows runs half.
// A report is written to out-folder/keys.txt, with a picture of each page that has a fault.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(process.env.WITTYVISUALIZER ? new URL(process.env.WITTYVISUALIZER + '/tests/cdp.mjs', import.meta.url).href : './cdp.mjs')

const server = process.env.WMS_SERVER || 'http://keys.localhost:9811'
const [out = '.'] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const only = process.env.WMS_ONLY || ''
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
// Pages that leave the app's frame, start something or need a link of their own.
const SKIP = /^\/(s|read|game|couch\/[^/]+\/play|couch-rig|tv\/watch|watch|setup|big|logout|sign-?out)(\/|$)|\/download(\/|$)/
const MAX_PAGES = Number(process.env.WMS_MAX_PAGES || 120)
const MAX_ARROW_NODES = 160

const browser = await launch({ software: true, width: 1440, height: 900 })
const page = await browser.open(server + '/')
const size = (w, h) => page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false })
// Headless has no window manager, and focus styles need a focused page.
await page.send('Emulation.setFocusEmulationEnabled', { enabled: true })
await page.send('Accessibility.enable')
const call = (method, path, body) => page.eval(`(async () => { const t = localStorage.getItem('wms.token'); const r = await fetch('/api${path}', { method: '${method}', headers: { 'Content-Type': 'application/json', ...(t ? { Authorization: 'Bearer ' + t } : {}) }, body: ${body ? `JSON.stringify(${JSON.stringify(body)})` : 'undefined'} }); return r.json().catch(() => null) })()`)
const login = await call('POST', '/login', { name: process.env.WMS_USER, password: process.env.WMS_PASSWORD })
if (!login?.token) throw new Error('sign-in failed: ' + JSON.stringify(login))
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)}); localStorage.setItem('wms.tenfoot', '0')`)
await call('POST', '/legal/acknowledge')

const KEYS = { Tab: 9, ArrowUp: 38, ArrowDown: 40, ArrowLeft: 37, ArrowRight: 39, Escape: 27 }
const press = async (key, shift = false) => {
  const p = { key, code: key, windowsVirtualKeyCode: KEYS[key], modifiers: shift ? 8 : 0 }
  await page.send('Input.dispatchKeyEvent', { type: 'rawKeyDown', ...p })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', ...p })
}

const faults = new Map() // page -> Set of lines
const fault = (where, what) => { if (!faults.has(where)) faults.set(where, new Set()); faults.get(where).add(what) }

// Waits for a page to stop changing: no spinner, and the same number of controls twice running.
async function settle() {
  let last = -1
  for (let i = 0; i < 40; i++) {
    await wait(250)
    const n = await page.eval(`document.querySelector('.spinner, [aria-busy=true]') ? -2 : document.querySelectorAll('a, button, input').length`)
    if (n >= 0 && n === last) return
    last = n
  }
}

// A link that saves a file never finishes loading, so a page gets a time limit.
const go = async (path) => {
  await Promise.race([page.go(server + path), wait(20000).then(() => { throw new Error('did not load: ' + path) })])
  await page.until(`document.querySelector('main, .app, [role=main]')`, 20000, path)
  await settle()
}

// ---------------------------------------- finding the pages
const template = (p) => p.split('#')[0].replace(/^\/settings\/modules\/store\/[^/]+$/, '/settings/modules/store/:id').replace(/\/[0-9a-f]{8,}|\/\d+(?=\/|$)/g, '/:id').replace(/^\/(brand\/[^/]+)\/[^/]+$/, '/$1/:name').replace(/^\/(album|artist)\/[^/]+$/, '/$1/:lib').replace(/^\/help\/[^/]+$/, '/help/:guide')
async function links() {
  return page.eval(`[...document.querySelectorAll('a[href]:not([download])')].map((a) => a.getAttribute('href')).filter((h) => h && h.startsWith('/') && !h.startsWith('//') && !h.startsWith('/api/'))`)
}
const wanted = process.env.WMS_PAGES ? process.env.WMS_PAGES.split(',') : null
const pages = []
if (wanted) pages.push(...wanted)
else {
  const seen = new Set(), queue = ['/', '/settings/libraries', '/settings/about', '/settings/modules/store', '/help', '/search?q=a', '/fix-matches', '/controllers', '/collections', '/pair', '/tv', '/tv/recordings', '/review', '/couch', '/design', '/not-a-page']
  while (queue.length && pages.length < MAX_PAGES) {
    const p = queue.shift()
    const key = template(p.split('?')[0])
    if (p.includes('#')) continue // a part of a page already on the list
    if (seen.has(key) || SKIP.test(p)) continue
    seen.add(key)
    pages.push(p)
    console.log(`found ${p}`)
    try { await go(p) } catch { continue }
    for (const h of await links()) if (!seen.has(template(h.split('?')[0])) && !SKIP.test(h)) queue.push(h)
  }
}
console.log(`${pages.length} pages`)

// ---------------------------------------- what's on a page
// Tags every control a person could reach, and what it looks like without focus.
const tagAll = () => page.eval(`(() => {
  const FOCUSABLE = 'a[href], button:not([disabled]), summary, input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable=true]'
  const look = (e) => { if (!e) return ''; const s = getComputedStyle(e); return [s.outlineStyle, s.outlineWidth, s.outlineColor, s.boxShadow, s.borderColor, s.backgroundColor, s.color, s.transform, s.textDecorationLine, s.filter].join('|') }
  window.__looks = window.__looks || new Map()
  let n = window.__n || 0
  const out = []
  for (const el of document.querySelectorAll(FOCUSABLE)) {
    const r = el.getBoundingClientRect()
    if (r.width < 2 || r.height < 2 || el.closest('[inert], [aria-hidden=true]') || (el.closest('details:not([open])') && !el.closest('summary'))) continue
    const s = getComputedStyle(el)
    if (s.visibility === 'hidden') continue
    if (!el.dataset.k) el.dataset.k = String(++n)
    if (document.activeElement !== el) window.__looks.set(el.dataset.k, [el, el.firstElementChild, el.parentElement].map(look).join('#'))
    out.push(el.dataset.k)
  }
  window.__n = n
  return out
})()`)
const nameOf = (k) => page.eval(`(() => { const e = document.querySelector('[data-k="${k}"]'); if (!e) return '?'; const t = (e.getAttribute('aria-label') || e.innerText || e.getAttribute('title') || e.placeholder || e.value || '').trim().replace(/\\s+/g, ' ').slice(0, 50); return e.tagName.toLowerCase() + (e.className && typeof e.className === 'string' ? '.' + e.className.trim().split(/\\s+/).slice(0, 2).join('.') : '') + (t ? ' "' + t + '"' : '') })()`)

// What the focused control is like: whether its focus shows, where it is, and what covers it.
const focusNow = () => page.eval(`(() => {
  const e = document.activeElement
  if (!e || e === document.body || e === document.documentElement) return { k: '' }
  const look = (x) => { if (!x) return ''; const s = getComputedStyle(x); return [s.outlineStyle, s.outlineWidth, s.outlineColor, s.boxShadow, s.borderColor, s.backgroundColor, s.color, s.transform, s.textDecorationLine, s.filter].join('|') }
  const now = [e, e.firstElementChild, e.parentElement].map(look).join('#')
  const before = window.__looks?.get(e.dataset.k)
  const r = e.getBoundingClientRect()
  const vw = innerWidth, vh = innerHeight
  const onScreen = r.bottom > 0 && r.right > 0 && r.top < vh && r.left < vw
  let coveredBy = ''
  if (onScreen) {
    // Covered means the middle and both ends of it are under something else: a corner under the top bar is fine.
    const over = [[.5, .5], [.15, .5], [.85, .5]].map(([fx, fy]) => {
      const x = Math.min(Math.max(r.left + r.width * fx, 1), vw - 1), y = Math.min(Math.max(r.top + r.height * fy, 1), vh - 1)
      const hit = document.elementFromPoint(x, y)
      return hit && hit !== e && !e.contains(hit) && !hit.contains(e) && !hit.closest('label')?.contains(e) ? hit : null
    })
    if (over.every(Boolean)) coveredBy = (over[0].closest('header, nav, aside, [class]') || over[0]).className?.toString().split(' ')[0] || over[0].tagName
  }
  return { k: e.dataset.k || '?', shows: before === undefined ? null : before !== now, onScreen, coveredBy, tag: e.tagName }
})()`)

// The accessibility tree: every control or picture with no name.
const ROLES = new Set(['button', 'link', 'checkbox', 'radio', 'switch', 'textbox', 'searchbox', 'combobox', 'slider', 'spinbutton', 'tab', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'option', 'listbox', 'img', 'image', 'treeitem', 'progressbar', 'meter'])
async function nameless() {
  const { nodes } = await page.send('Accessibility.getFullAXTree')
  const out = []
  for (const n of nodes) {
    if (n.ignored || !ROLES.has(n.role?.value) || (n.name?.value || '').trim()) continue
    if (!n.backendDOMNodeId) continue
    const { object } = await page.send('DOM.resolveNode', { backendNodeId: n.backendDOMNodeId }).catch(() => ({}))
    if (!object) continue
    const r = await page.send('Runtime.callFunctionOn', { objectId: object.objectId, returnByValue: true, functionDeclaration: `function () { const r = this.getBoundingClientRect(); if (r.width < 1 && r.height < 1) return ''; const h = this.outerHTML; return h.slice(0, h.indexOf('>') + 1).slice(0, 140) }` })
    if (r.result.value) out.push(`${n.role.value}: ${r.result.value}`)
  }
  return out
}

async function pageChecks(where) {
  const p = await page.eval(`({
    h1: document.querySelectorAll('h1').length,
    main: !!document.querySelector('main, [role=main]'),
    title: document.title,
    lang: document.documentElement.lang,
    noAlt: [...document.querySelectorAll('img:not([alt])')].filter((i) => i.getBoundingClientRect().width > 1).map((i) => i.className || i.src.slice(-40)).slice(0, 5),
    badRefs: [...document.querySelectorAll('[aria-labelledby], [aria-describedby], [aria-controls]')].flatMap((e) => ['aria-labelledby', 'aria-describedby', 'aria-controls'].flatMap((a) => (e.getAttribute(a) || '').split(/\\s+/).filter((id) => id && !document.getElementById(id)).map((id) => a + '=' + id))).slice(0, 5),
    dupIds: (() => { const s = new Map(); for (const e of document.querySelectorAll('[id]')) s.set(e.id, (s.get(e.id) || 0) + 1); return [...s].filter(([, n]) => n > 1).map(([id]) => id).slice(0, 5) })(),
    sideways: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  })`)
  if (p.h1 === 0) fault(where, 'no main heading (h1)')
  if (!p.main) fault(where, 'no main landmark')
  if (!p.lang) fault(where, 'no lang on the page')
  for (const i of p.noAlt) fault(where, `picture with no alt: ${i}`)
  for (const r of p.badRefs) fault(where, `points at a missing id: ${r}`)
  for (const d of p.dupIds) fault(where, `id used twice: ${d}`)
  if (p.sideways) fault(where, 'scrolls sideways')
  for (const n of await nameless()) fault(where, `no name: ${n}`)
  return p.title
}

// ---------------------------------------- Tab from the top to the end
async function tabWalk(where) {
  const all = await tagAll()
  await page.eval(`document.activeElement?.blur(); window.scrollTo(0, 0)`)
  const order = []
  const limit = all.length * 2 + 40
  // A page can put focus on its own content as it opens, so Tab goes on past the end of the page and
  // round from the top, until it's back where it started.
  let first = '', wrapped = false
  for (let i = 0; i < limit; i++) {
    await press('Tab')
    await wait(40)
    const f = await focusNow()
    if (!f.k) continue
    if (f.k === '?') { await tagAll(); continue }
    if (f.k === first) { wrapped = true; break }
    if (!first) first = f.k
    // Three times running on the same control means Tab is held there.
    if (order.length >= 2 && order.at(-1) === f.k && order.at(-2) === f.k) { fault(where, `Tab is stuck on ${await nameOf(f.k)}`); break }
    order.push(f.k)
    if (f.shows === false) fault(where, `focus doesn't show on ${await nameOf(f.k)}`)
    if (!f.onScreen) fault(where, `focus goes off screen on ${await nameOf(f.k)}`)
    else if (f.coveredBy) fault(where, `focus is under "${f.coveredBy}" on ${await nameOf(f.k)}`)
    if (order.length % 40 === 0) await tagAll() // things that appear on the way
  }
  if (!wrapped) {
    const tail = await Promise.all(order.slice(-6).map(nameOf))
    fault(where, `Tab never reaches the end of the page; it goes round: ${tail.join(' → ')}`)
  }
  const now = new Set(await tagAll())
  const visited = new Set(order)
  // A group that keeps only its chosen member in the Tab order (tabindex -1 on the rest, or radio
  // buttons with one chosen) is reached
  // by the arrows from there, as it should be.
  const outOfOrder = new Set(await page.eval(`[...document.querySelectorAll('[data-k][tabindex="-1"], input[type=radio][data-k]:not(:checked)')].filter((e) => e.type !== 'radio' || document.querySelector('input[type=radio][name="' + e.name + '"]:checked')).map((e) => e.dataset.k)`))
  for (const k of all) if (now.has(k) && !visited.has(k) && !outOfOrder.has(k)) fault(where, `Tab never reaches ${await nameOf(k)}`)
  // Shift+Tab goes back the way it came.
  if (order.length > 3) {
    await page.eval(`document.querySelector('[data-k="${order[3]}"]')?.focus()`)
    await press('Tab', true); await wait(25)
    const back = (await focusNow()).k
    if (back !== order[2]) fault(where, `Shift+Tab from ${await nameOf(order[3])} goes to ${back ? await nameOf(back) : 'nothing'}, not back to ${await nameOf(order[2])}`)
  }
  return order.length
}

// ---------------------------------------- the arrows, as a remote
async function arrowWalk(where) {
  await page.eval(`document.activeElement?.blur(); window.scrollTo(0, 0)`)
  // A remote's first press, with nothing focused.
  await press('ArrowDown'); await wait(80)
  let start = (await focusNow()).k
  if (!start || start === '?') { await tagAll(); await press('ArrowDown'); await wait(80); start = (await focusNow()).k }
  const all = await tagAll()
  if (!start || start === '?') { if (all.length) fault(where, 'the arrows focus nothing from the top of the page'); return 0 }
  const reached = new Set([start]), queue = [start]
  const ends = Date.now() + 60000
  while (queue.length && reached.size < MAX_ARROW_NODES && Date.now() < ends) {
    const k = queue.shift()
    for (const key of ['ArrowDown', 'ArrowRight', 'ArrowUp', 'ArrowLeft']) {
      await page.eval(`(() => { const e = document.querySelector('[data-k="${k}"]'); e?.focus({ preventScroll: false }) })()`)
      if ((await focusNow()).k !== k) break
      await press(key); await wait(35)
      // Down on a menu button opens its menu, as it should. What's in a menu or a dialog isn't part
      // of the page, so the walk notes it opened and closes it again, as a person would.
      if (await page.eval(`(() => { const a = document.activeElement, from = document.querySelector('[data-k="${k}"]'); const o = a?.closest('[role=menu], [role=dialog], [role=listbox]'); return !!o && !o.contains(from) })()`)) {
        if (process.env.WMS_DEBUG) console.log('   ', await nameOf(k), key, '-> opens a menu or dialog')
        await press('Escape'); await wait(80)
        continue
      }
      const f = await focusNow()
      if (process.env.WMS_DEBUG) console.log('   ', await nameOf(k), key, '->', f.k ? await nameOf(f.k) : '(nothing)')
      if (f.k === '?') { await tagAll(); continue }
      if (f.k && !reached.has(f.k)) {
        reached.add(f.k); queue.push(f.k)
        if (f.shows === false) fault(where, `big screen: focus doesn't show on ${await nameOf(f.k)}`)
        if (f.coveredBy) fault(where, `big screen: focus is under "${f.coveredBy}" on ${await nameOf(f.k)}`)
      }
      if (!(await page.eval(`location.pathname + location.search`)).startsWith(where.split('?')[0])) { await go(where); await tagAll(); break }
    }
  }
  if (reached.size < MAX_ARROW_NODES && Date.now() < ends) {
    const now = new Set(await tagAll())
    // The skip link is for Tab: across the room the arrows have no need of it. Nor what the app
    // marks for the arrows to pass over (data-nav-skip): TV mode's Back button is the remote's
    // own Back key, a spotlight's dots are its left and right.
    const tabOnly = new Set(await page.eval(`[...document.querySelectorAll('.skip-link[data-k], [data-nav-skip][data-k]')].map((e) => e.dataset.k)`))
    const missed = all.filter((k) => now.has(k) && !reached.has(k) && !tabOnly.has(k))
    for (const k of missed.slice(0, 12)) fault(where, `big screen: the arrows can't reach ${await nameOf(k)}`)
    if (missed.length > 12) fault(where, `big screen: and ${missed.length - 12} more the arrows can't reach`)
  }
  return reached.size
}

const settingsBefore = JSON.stringify(await call('GET', '/settings'))
const titles = new Map()
// The report so far, written after every page so a long walk can be read as it goes.
function writeReport(done, note = '') {
  const lines = []
  for (const [p, set] of faults) { lines.push(`\n${p}`); for (const f of set) lines.push(`  ${f}`) }
  const total = [...faults.values()].reduce((a, s) => a + s.size, 0)
  const head = `${done} of ${pages.length} pages, ${total} faults on ${faults.size} of them`
  writeFileSync(join(out, 'keys.txt'), [head, ...(note ? [note] : []), ...lines].join('\n') + '\n')
  return head
}
let done = 0
try {
  for (const p of pages) {
    const line = [p]
    try {
      if (only !== 'arrows') {
        await size(1440, 900)
        await page.eval(`localStorage.setItem('wms.tenfoot', '0')`)
        await go(p)
        const title = await pageChecks(p)
        // An address that redirects (Settings to its first page) is the page it lands on, not another page with the same title.
        const landed = await page.eval(`location.pathname + location.search`)
        if (titles.has(title) && title && titles.get(title).landed !== landed) fault(p, `same page title as ${titles.get(title).p}: "${title}"`)
        else titles.set(title, { p, landed })
        line.push(`tab ${await tabWalk(p)}`)
      }
      if (only !== 'tab') {
        await size(1920, 1080)
        await page.eval(`localStorage.setItem('wms.tenfoot', '1')`)
        await go(p)
        line.push(`arrows ${await arrowWalk(p)}`)
        await page.eval(`localStorage.setItem('wms.tenfoot', '0')`)
      }
    } catch (e) {
      fault(p, 'the walk stopped: ' + e.message.split('\n')[0])
    }
    const n = faults.get(p)?.size || 0
    line.push(n ? `${n} faults` : 'ok')
    console.log(line.join('  '))
    if (n) writeFileSync(join(out, `keys-${p.replace(/[^a-z0-9]+/gi, '_').replace(/^_|_$/g, '') || 'home'}.png`), await page.shot())
    writeReport(++done)
  }
} finally {
  const settingsAfter = JSON.stringify(await call('GET', '/settings').catch(() => null))
  console.log(writeReport(done, settingsAfter !== settingsBefore ? 'NOTE the arrows changed a server setting on the way (a slider or a choice)' : ''))
  await browser.close()
}
process.exit(faults.size ? 1 : 0)
