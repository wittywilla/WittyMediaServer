// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The settings menu (#612, pkg/sdui/data/settingsmenu.json) on the web: the same groups and rows, in the same order,
// as the Android phone app's You page, on a computer, a phone and in TV mode.
//   desktop 1920x1080  /settings: the person on the left, every group on the right; a settings page has the same
//                      rows beside it, the open one marked
//   phone 412x915      /you: one column, the person, the groups, This server and Sign out
//   TV device          /you?tv=1&client=tv: the same rows as tiles; a row a TV can't change says what is set and opens
//                      the panel with that setting's own address and a QR code; no keyboard keys drawn
//   computer, TV mode  the same panel says Leave TV mode
//   WMS_SERVER=http://sp.localhost:19704 WMS_TOKEN_FILE=<file with a sign-in token> node scripts/test-settings-menu.mjs <out folder>
// The account must be an administrator. It changes nothing on the server.
import { readFileSync, writeFileSync } from 'node:fs'
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const token = readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim()
const [out] = process.argv.slice(2)
const menu = JSON.parse(readFileSync(new URL('../../pkg/sdui/data/settingsmenu.json', import.meta.url), 'utf8'))
const results = []
const check = (name, ok, detail) => { results.push({ name, ok: !!ok, detail }); console.log(ok ? 'PASS' : 'FAIL', name, detail === undefined ? '' : JSON.stringify(detail)) }

// The groups and rows drawn, in order.
const READ = `[...document.querySelectorAll('[data-settings-group]')].map((g) => ({ id: g.dataset.settingsGroup, rows: [...g.querySelectorAll('[data-row]')].map((r) => r.dataset.row) }))`
const expected = (ids) => menu.groups.map((g) => ({ id: g.id, rows: g.rows.map((r) => r.id).filter((id) => ids.includes(id)) })).filter((g) => g.rows.length)
// Every row in the order the menu has it: what is drawn must be the menu's order, with nothing it doesn't name.
const inOrder = (drawn) => {
  const want = expected(drawn.flatMap((g) => g.rows))
  return JSON.stringify(drawn.filter((g) => g.id !== 'fromModules')) === JSON.stringify(want)
}

async function page(name, { width = 1920, height = 1080, mobile = false, path }) {
  const s = await open(server, out, { width, height, path: '/legal' })
  if (mobile) await s.size(width, height, true, 2)
  await s.page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.tvclient')`)
  await s.go(path, 2000)
  await s.page.until(`document.querySelectorAll('[data-settings-group]').length > 4`, 60000, 'the settings groups').catch(() => {})
  await wait(2500)
  return s
}

async function desktop() {
  const s = await page('desktop', { path: '/settings' })
  try {
    const drawn = await s.page.eval(READ)
    check('desktop: groups in the Android order', JSON.stringify(drawn.map((g) => g.id)) === JSON.stringify(['account', 'household', 'yours', 'modules', 'device', 'server', 'maintenance', 'help']), drawn.map((g) => g.id))
    check('desktop: rows in the menu order', inOrder(drawn))
    const ids = drawn.flatMap((g) => g.rows)
    for (const id of ['names', 'quality', 'network', 'dataFolder', 'remote', 'pages', 'licences']) check(`desktop: row ${id}`, ids.includes(id))
    check('desktop: no app-only rows on the web', !ids.some((id) => ['topMenu', 'servers', 'away', 'appLog'].includes(id)))
    const sides = await s.page.eval(`(() => { const l = document.querySelector('.sm-left').getBoundingClientRect(), r = document.querySelector('.sm-right').getBoundingClientRect(); return [Math.round(l.left), Math.round(l.right), Math.round(r.left)] })()`)
    check('desktop: the person on the left, the groups on the right', sides[1] <= sides[2], sides)
    check('desktop: values on their rows', /\d/.test(await s.page.eval(`document.querySelector('[data-row="devices"] .sm-value')?.textContent || ''`)))
    await s.maskAddresses()
    await s.shot('web-desktop-settings')
    // A settings page: the same rows beside it, the open one marked.
    await s.go('/settings/remote', 2500)
    await s.page.until(`!!document.querySelector('.sm-nav [data-row="remote"]')`, 30000, 'the menu beside the page').catch(() => {})
    const nav = await s.page.eval(READ)
    check('desktop page: the same rows beside it', inOrder(nav) && nav.flatMap((g) => g.rows).length === ids.length, nav.flatMap((g) => g.rows).length)
    check('desktop page: Remote access is marked', await s.page.eval(`document.querySelector('.sm-nav [data-row="remote"]')?.getAttribute('aria-current') === 'page'`))
    await s.shot('web-desktop-settings-remote')
  } catch (e) { check('desktop: ran', false, String(e?.message || e)) } finally { await s.browser.close() }
}

async function phone() {
  const s = await page('phone', { width: 412, height: 915, mobile: true, path: '/you' })
  try {
    const drawn = await s.page.eval(READ)
    check('phone: rows in the menu order', inOrder(drawn) && drawn.length === 8, drawn.map((g) => g.id))
    check('phone: one column', await s.page.eval(`!document.querySelector('.sm-home.wide') && !!document.querySelector('.sm-list')`))
    check('phone: This server and Sign out at the foot', await s.page.eval(`!!document.querySelector('.sm-server .btn.danger')`))
    await s.maskAddresses()
    await s.shot('web-phone-you')
  } catch (e) { check('phone: ran', false, String(e?.message || e)) } finally { await s.browser.close() }
}

async function tv(client) {
  const s = await page('tv-' + client, { path: `/you?tv=1&client=${client}` })
  const who = client === 'tv' ? 'TV device' : 'computer in TV mode'
  try {
    const drawn = await s.page.eval(READ)
    const ids = drawn.flatMap((g) => g.rows)
    check(`${who}: rows in the menu order`, inOrder(drawn))
    check(`${who}: a TV signs in no TVs`, !ids.includes('signInTv'))
    check(`${who}: rows a TV can't change stay in their place`, ['libraries', 'remote', 'network', 'dataFolder', 'nameAndLogo', 'logs', 'reset'].every((id) => ids.includes(id)))
    const value = await s.page.eval(`document.querySelector('[data-row="remote"] .sm-value')?.textContent || ''`)
    check(`${who}: Remote access says what is set`, value === 'On' || value === 'Off', value)
    check(`${who}: no keyboard keys drawn`, !/\b(Esc|Ctrl\+|Enter)\b/.test(await s.page.eval(`document.querySelector('.sm-home').innerText`)))
    await s.shot(`web-tvmode-${client}-you`)
    await s.page.eval(`document.querySelector('[data-row="remote"]').scrollIntoView({ block: 'center' }); document.querySelector('[data-row="remote"]').click()`)
    await s.page.until(`!!document.querySelector('[role=dialog] .tvl-dialog')`, 15000, 'the panel').catch(() => {})
    await wait(1500)
    const panel = await s.page.eval(`document.querySelector('[role=dialog]')?.innerText || ''`)
    if (client === 'tv') {
      check('TV device: the panel has the setting\'s own address', panel.includes('/settings/remote'), panel)
      check('TV device: and a QR code', await s.page.eval(`!!document.querySelector('[role=dialog] svg, [role=dialog] canvas, [role=dialog] img')`))
      check('TV device: and what is set now', /Set now/i.test(panel), panel)
    } else {
      check('computer in TV mode: the panel says Leave TV mode', panel.includes('Leave TV mode'), panel)
    }
    await s.shot(`web-tvmode-${client}-panel`)
  } catch (e) { check(`${who}: ran`, false, String(e?.message || e)) } finally { await s.browser.close() }
}

await desktop()
await phone()
await tv('tv')
await tv('desktop')
writeFileSync(out + '/settings-menu-report.json', JSON.stringify(results, null, 1))
console.log(results.filter((r) => r.ok).length + ' of ' + results.length + ' passed')
process.exit(results.every((r) => r.ok) ? 0 : 1)
