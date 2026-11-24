// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The module store, end to end, in a headless browser, against a test server and a small local
// catalogue (scripts/store-e2e/catalog.sh makes it). It serves the catalogue itself, in each of its
// states in turn, and fails if:
// - opening the store covers the rest of the app (#694), or the old Browse address doesn't land on Browse;
// - search doesn't find one module, or a withdrawn one isn't shown as withdrawn with what to do;
// - Install, Update (from Updates, after a newer version is listed) or Remove doesn't happen;
// - a malformed catalogue, or a file that isn't one, isn't explained plainly, or breaks Installed,
//   or any other page (Home, Libraries);
// - a store answer of the wrong shape breaks the store, or anything throws on the way.
//
//   WMS_SERVER=http://127.0.0.1:19613 WMS_TOKEN=... WMS_CATALOG_DIR=<catalog.sh's output> WMS_CATALOG_PORT=8150 \
//     node scripts/test-module-store.mjs [out-folder]
//
// The server must be a -tags updatetest build started with WMS_UPDATE_KEY=<a/test-key.pub> and
// WMS_MODULE_CATALOG=http://127.0.0.1:<WMS_CATALOG_PORT>/catalog.json, with WittyStats not installed.
// Pictures of each step go to the output folder.
import { createServer } from 'node:http'
import { mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { join, normalize } from 'node:path'
const { launch } = await import('./cdp.mjs')

const server = process.env.WMS_SERVER || 'http://127.0.0.1:19613'
const token = process.env.WMS_TOKEN || (process.env.WMS_TOKEN_FILE ? readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim() : '')
const dir = process.env.WMS_CATALOG_DIR
const port = +(process.env.WMS_CATALOG_PORT || 8150)
const [out = 'store-e2e-out'] = process.argv.slice(2)
if (!dir || !token) { console.error('set WMS_CATALOG_DIR and WMS_TOKEN (or WMS_TOKEN_FILE)'); process.exit(2) }
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))

// The catalogue, in the state the test is at: a, b, c, or 'garbled' (a file that isn't one).
let state = 'a'
const http = createServer((req, res) => {
  const path = decodeURIComponent(new URL(req.url, 'http://x').pathname)
  if (state === 'garbled' && path.startsWith('/catalog.json')) { res.writeHead(200, { 'Content-Type': 'application/json' }); return res.end('{"modules": [ this is not json') }
  const file = normalize(join(dir, state === 'garbled' ? 'b' : state, path))
  if (!file.startsWith(normalize(dir))) { res.writeHead(403); return res.end() }
  try { statSync(file); res.writeHead(200); res.end(readFileSync(file)) } catch { res.writeHead(404); res.end() }
})
await new Promise((d) => http.listen(port, '127.0.0.1', d))

const api = async (method, path, body) => {
  const r = await fetch(server + '/api' + path, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined })
  return r.json().catch(() => ({}))
}
const stats = async () => (await api('GET', '/modules')).modules?.find((m) => m.id === 'wittystats')

const failures = []
const check = (ok, what) => { console.log((ok ? 'ok   ' : 'FAIL ') + what); if (!ok) failures.push(what) }

const b = await launch({ software: true, width: 1280, height: 900 })
const p = await b.open(server + '/')
await p.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)})`)
let shot = 0
const snap = async (name) => writeFileSync(join(out, `${String(++shot).padStart(2, '0')}-${name}.png`), await p.shot())
const go = async (path, ready = '.st-tabs') => { await p.go(server + path); await p.until(`document.querySelector(${JSON.stringify(ready)})`, 30000, ready) }
// Presses the first button or link whose name (its label, else its text) matches.
const press = (re, scope = 'document') => p.eval(`(() => {
  const re = new RegExp(${JSON.stringify(re)})
  const el = [...${scope}.querySelectorAll('button, a, [role=tab]')].find((e) => re.test(e.getAttribute('aria-label') || e.textContent.trim()) && e.offsetParent !== null)
  if (!el) return false
  el.scrollIntoView({ block: 'center' }); el.click(); return true
})()`)
const text = () => p.eval('document.querySelector("main")?.innerText || document.body.innerText')
const appBroken = () => p.eval(`/This page ran into a problem/.test(document.body.innerText)`)
const settle = async (cond, ms = 60000, what = cond) => p.until(cond, ms, what)
// The server looks at most once a minute, however often Check now is pressed: a step that needs a
// new look waits for that minute to pass.
let lastCheck = 0
const minuteGone = async () => { const left = lastCheck + 62000 - Date.now(); if (left > 0) await wait(left); lastCheck = Date.now() }
const pollApi = async (fn, ms = 90000, what = 'the server') => { const end = Date.now() + ms; for (;;) { if (await fn()) return true; if (Date.now() > end) throw new Error('timed out waiting for ' + what); await wait(500) } }

try {
  // A. The store opens in place, and the old address lands on Browse.
  state = 'a'
  await minuteGone(); await api('POST', '/modulestore/check')
  await go('/settings/modules/store')
  check((await p.eval('location.search')).includes('tab=browse'), 'the old Browse address lands on the Browse tab')
  await settle(`document.querySelector('.st-card')`, 30000, 'store cards')
  const cover = await p.eval(`(() => { const s = getComputedStyle(document.querySelector('.st-page')); const top = document.elementFromPoint(innerWidth / 2, 20); return { pos: s.position, topbar: !!top && !top.closest('.st-page') } })()`)
  check(cover.pos !== 'fixed' && cover.topbar, 'the store sits in the page, and the top bar is still on top (#694)')
  await snap('browse')

  // B. Search finds one module.
  await p.eval(`(() => { const i = document.querySelector('.st-browse input[placeholder="Search modules"]'); i.focus(); const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set; set.call(i, 'stats'); i.dispatchEvent(new Event('input', { bubbles: true })) })()`)
  await settle(`document.querySelectorAll('.st-card').length === 1`, 10000, 'one result for "stats"')
  check(/WittyStats/.test(await p.eval(`document.querySelector('.st-card').innerText`)), 'searching "stats" finds WittyStats alone')
  await wait(400)
  check((await p.eval('location.search')).includes('q=stats'), 'the search is kept in the address')
  await snap('search')

  // C. A withdrawn module says so, with what to do, and offers no Install.
  await go('/settings/modules?tab=browse&q=visualizer', '.st-card')
  const vis = await p.eval(`document.querySelector('.st-card').innerText`)
  check(/Withdrawn/.test(vis) && /Check again later/.test(vis), 'a withdrawn module is shown as withdrawn, with what to do')
  check(!(await p.eval(`!!document.querySelector('.st-card .st-card-act')`)), 'a withdrawn module has no Install button')

  // D. Install.
  await go('/settings/modules?tab=browse&q=stats', '.st-card')
  check(/Works on this server/.test(await p.eval(`document.querySelector('.st-card').innerText`)), 'a module that fits says it works on this server')
  check(await press('^Install WittyStats'), 'Install is offered for WittyStats')
  await pollApi(async () => (await stats())?.installed, 90000, 'WittyStats installed')
  await settle(`/Installed/.test(document.querySelector('.st-card')?.innerText || '')`, 30000, 'the card says Installed')
  check(true, 'WittyStats installs from Browse')
  await snap('installed-card')
  await go('/settings/modules', '.ms-mod')
  check(await p.eval(`!!document.getElementById('mod-wittystats')`), 'Installed lists WittyStats')
  await snap('installed-tab')

  // E. Update, from the Updates tab, once a newer version is listed.
  state = 'b'
  const from = (await stats()).version
  await go('/settings/modules?tab=updates', '.st-tabs')
  await settle(`[...document.querySelectorAll('.spec-page button, .spec-chip')].some((b) => /Check now/.test(b.textContent))`, 30000, 'Check now')
  await minuteGone(); await press('^Check now$')
  await settle(`/Updates · 1/.test(document.querySelector('.st-tabs').innerText)`, 60000, 'the Updates tab counting one')
  check(/What the updates bring/.test(await text()) && /Store test: a newer version/.test(await text()), 'Updates shows what the new version brings')
  await snap('updates')
  // Pressed again within the minute, the server doesn't look: the store says when it last did.
  await press('^Check now$')
  await settle(`/already looked at/.test(document.body.innerText)`, 15000, 'the "already looked" words')
  check(true, 'a second Check now within the minute says when the server last looked, not "up to date"')
  check(await press('^Update All Modules'), 'Update All is offered')
  await pollApi(async () => { const s = await stats(); return s && s.version !== from && !s.latest }, 90000, 'WittyStats updated')
  check(true, `WittyStats updates from ${from}`)

  // F. Remove.
  await go('/settings/modules', '#mod-wittystats')
  await p.eval(`document.querySelector('#mod-wittystats .ms-head').click()`)
  await wait(300)
  check(await press('^Remove$', `document.getElementById('mod-wittystats')`), 'Remove is offered in the module’s row')
  await settle(`!!document.querySelector('.set-modal')`, 5000, 'the Remove question')
  await press('^Remove$', `document.querySelector('.set-modal')`)
  await pollApi(async () => !(await stats())?.installed, 60000, 'WittyStats removed')
  check(true, 'WittyStats is removed')

  // G. A malformed catalogue, then a file that isn't one: said plainly, and nothing else breaks.
  for (const bad of ['c', 'garbled']) {
    state = bad
    await minuteGone(); await api('POST', '/modulestore/check')
    await go('/settings/modules?tab=browse', '.st-tabs')
    await wait(1500)
    const words = await text()
    check(!(await appBroken()) && /couldn|didn|wrong|problem|can.t|refused|ignored|check out/i.test(words), `${bad}: Browse says the list couldn’t be read, in words`)
    await snap('browse-' + bad)
    await go('/settings/modules', '.ms-mod')
    check(!(await appBroken()), `${bad}: Installed still shows`)
    for (const path of ['/', '/settings/libraries']) {
      await p.go(server + path); await wait(2500)
      check(!(await appBroken()), `${bad}: ${path} still works`)
    }
  }

  // I. Modules that need a choice (the list post-update setup reuses): one with an update for this
  //    server, one withdrawn for security with nothing to switch to. Each says so, with its buttons.
  const fake = (id, name, status, fit) => ({ key: 'official/' + id, store: 'official', storeName: 'Test', storeHost: 'x', official: true, kind: 'official', id, name, fit, status: { id, name, installed: true, phase: 'idle', state: 'installed', version: '0.8.1', ...status } })
  const choiceBody = { server: '0.9.0-alpha', platform: 'linux-amd64', policy: '', repairRoute: '/settings/repair', stores: [], updates: 1, items: [
    fake('wittyold', 'WittyOld', { olderServer: true, madeFor: '0.8.x', latest: { version: '0.9.0' } }, { ok: true, kind: 'ok' }),
    fake('wittygone', 'WittyGone', { withdrawn: { version: '0.8.1', severity: 'security', reason: 'x' } }, { ok: false, kind: 'withdrawn' }),
  ] }
  await p.send('Fetch.enable', { patterns: [{ urlPattern: '*/api/modulestore', requestStage: 'Response' }] })
  p.on('Fetch.requestPaused', (e) => p.send('Fetch.fulfillRequest', { requestId: e.requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body: Buffer.from(JSON.stringify(choiceBody)).toString('base64') }))
  await go('/settings/modules', '.st-tabs')
  await settle(`!!document.querySelector('.st-choices')`, 15000, 'the choices list')
  const choices = await p.eval(`document.querySelector('.st-choices').innerText`)
  check(/2 modules need your choice/.test(choices), 'Installed lists the two modules that need a choice')
  check(/Needs an update/.test(choices) && !!(await p.eval(`!!document.querySelector('.st-choices [aria-label="Update WittyOld to 0.9.0"]')`)), 'an older module: Needs an update, with Update')
  check(/security problem/.test(choices) && !!(await p.eval(`!!document.querySelector('.st-choices [aria-label="Remove WittyGone"]')`)), 'withdrawn for security: says so, with Remove')
  await press('^Remove WittyGone$', `document.querySelector('.st-choices')`)
  await settle(`!!document.querySelector('[role=dialog]')`, 5000, 'the Remove question')
  check(/saved data are kept/.test(await p.eval(`document.querySelector('[role=dialog]').innerText`)), 'Remove says the module’s data is kept')
  await snap('choices')
  await p.send('Fetch.disable')

  // H. A store answer of the wrong shape (a server or catalogue this page doesn't expect): the
  //    store copes, or at worst its own fence catches it; the app around it carries on.
  await p.send('Fetch.enable', { patterns: [{ urlPattern: '*/api/modulestore', requestStage: 'Response' }] })
  p.on('Fetch.requestPaused', (e) => p.send('Fetch.fulfillRequest', { requestId: e.requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }],
    body: Buffer.from(JSON.stringify({ items: [{ id: 'odd', name: 7, fit: 'yes' }, null, 'x', { id: 'odd2', name: 'Odd', fit: { ok: true, kind: 'ok' }, status: { installed: true, latest: 5 } }], stores: 'none', updates: 'many' })).toString('base64') }))
  await go('/settings/modules?tab=browse', '.st-tabs')
  await wait(2000)
  check(!(await appBroken()) && await p.eval(`!!document.querySelector('.st-tabs')`), 'an answer of the wrong shape doesn’t take the store or the app down')
  await snap('odd-answer')
  await p.send('Fetch.disable')

  const errors = p.logs.filter((l) => /^exception/.test(l))
  check(errors.length === 0, 'nothing threw' + (errors.length ? ': ' + errors.slice(0, 3).join(' | ') : ''))
} catch (e) {
  failures.push(String(e?.message || e))
  console.log('FAIL ' + (e?.message || e))
  await snap('failed').catch(() => {})
} finally {
  await b.close()
  http.close()
}
console.log(failures.length ? `\n${failures.length} failed` : '\nall passed')
process.exit(failures.length ? 1 : 0)
