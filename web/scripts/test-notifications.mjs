// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The bell (docs/NOTIFICATIONS.md), in the browser: on a computer, on a phone and in TV mode it is beside the
// activity button with the server's unread count; its list opens and matches the server's; Mark all read clears
// the count on the server too; a kind turned off in Settings is off on the server; and in TV mode the remote's
// arrows reach the bell, OK opens it, Back closes it, and no computer's keys are drawn. Headless: no window opens.
// Works on any server: with no notifications it checks the empty list.
//   WMS_SERVER=http://127.0.0.1:19873 WMS_TOKEN_FILE=<a file holding an administrator's token> node scripts/test-notifications.mjs [out folder]
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { launch } from './cdp.mjs'

const server = process.env.WMS_SERVER
const token = readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim()
const out = process.argv[2] || ''
if (out) mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const H = { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }
const api = async (method, path, body) => {
  const r = await fetch(server + '/api' + path, { method, headers: H, body: body ? JSON.stringify(body) : undefined })
  if (!r.ok) throw new Error(`${method} ${path}: ${r.status}`)
  return r.json()
}

let failed = 0
const check = (ok, what) => { if (!ok) failed++; console.log(`${ok ? 'ok  ' : 'FAIL'} ${what}`) }
// A remote's key, with the code a TV's browser gives it.
const CODES = { ArrowUp: 38, ArrowDown: 40, ArrowLeft: 37, ArrowRight: 39, Enter: 13, Escape: 27 }
const press = async (page, key) => {
  const k = { key, code: key, windowsVirtualKeyCode: CODES[key], nativeVirtualKeyCode: CODES[key] }
  await page.send('Input.dispatchKeyEvent', key === 'Enter' ? { type: 'keyDown', text: String.fromCharCode(13), ...k } : { type: 'rawKeyDown', ...k })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', ...k })
  await wait(150)
}
const shot = async (page, name) => { if (out) writeFileSync(join(out, name + '.png'), await page.shot()) }

const b = await launch({ width: 1280, height: 800 })
const start = await api('GET', '/me/notifications')
const choicesBefore = (await api('GET', '/me/notifications/choices')).choices

for (const [name, w, h, mobile, tv] of [['computer', 1280, 800, false, false], ['phone', 390, 844, true, false], ['tv', 1920, 1080, false, true]]) {
  const page = await b.open()
  await page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile })
  await page.go(server + '/')
  await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.setItem('wms.tvmode', '${tv ? 1 : 0}')`)
  await page.go(server + '/')
  const bell = tv ? '.tvm-bar .tvm-bell' : '.topbar .note-btn'
  await page.until(`document.querySelector('${bell}')`, 15000, 'the bell')
  const unread = (await api('GET', '/me/notifications/count')).unread
  await page.until(`document.querySelector('${bell}').getAttribute('aria-label').includes(${unread ? `'${unread} unread'` : `'Notifications'`})`, 15000, 'the count')
  const label = await page.eval(`document.querySelector('${bell}').getAttribute('aria-label')`)
  check(!!label, `${name}: the bell says ${JSON.stringify(label)}`)
  if (!tv) check(await page.eval(`!!document.querySelector('.topbar .act-btn, .topbar .note-btn') && document.querySelector('.topbar .note-btn').previousElementSibling !== null`), `${name}: the bell sits in the top bar beside the others`)

  if (tv) {
    // Up to the bar and along it to its end, as a remote does.
    await wait(1500)
    await press(page, 'ArrowUp')
    for (let i = 0; i < 12; i++) await press(page, 'ArrowRight')
    check(await page.eval(`document.activeElement === document.querySelector('${bell}')`), `${name}: the remote's arrows reach the bell`)
    await press(page, 'Enter')
  } else {
    await page.eval(`document.querySelector('${bell}').click()`)
  }
  await page.until(`document.querySelector('.note-sheet')`, 10000, 'the panel')
  await page.until(`document.querySelector('.note-sheet .note-list, .note-sheet .ui-empty, .note-sheet [class*=empty]')`, 10000, 'the list')
  const shown = await page.eval(`document.querySelectorAll('.note-sheet .note').length`)
  check(shown === Math.min(start.notes.length, 30), `${name}: the list shows ${shown} of the server's ${start.notes.length}`)
  check(await page.eval(`document.querySelectorAll('.note-sheet .note.unread').length`) === unread || unread > 30, `${name}: unread ones are marked`)
  if (tv) {
    const caps = await page.eval(`[...document.querySelectorAll('.note-sheet kbd')].length`)
    check(caps === 0 && !(await page.eval(`!!document.querySelector('.note-sheet a[href="/settings/notifications"]')`)), `${name}: no computer's keys and no settings link on a TV`)
  }
  await shot(page, name + '-panel')
  if (tv) {
    await press(page, 'Escape')
    await page.until(`!document.querySelector('.note-sheet')`, 5000, 'Back to close the panel')
    check(true, `${name}: Back closes the panel`)
  }
  page.close()
}

// Mark all read, from the panel, and the server agrees.
{
  const page = await b.open()
  await page.go(server + '/')
  await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.setItem('wms.tvmode', '0')`)
  await page.go(server + '/')
  await page.until(`document.querySelector('.topbar .note-btn')`)
  await page.eval(`document.querySelector('.topbar .note-btn').click()`)
  await page.until(`document.querySelector('.note-sheet .note-bar button, .note-sheet [class*=empty]')`)
  if (start.notes.length && start.unread) {
    await page.eval(`[...document.querySelectorAll('.note-sheet .note-bar button')].find((b) => b.textContent.includes('Mark all read')).click()`)
    await page.until(`!document.querySelector('.note-sheet .note.unread')`, 5000, 'every one read')
    check((await api('GET', '/me/notifications/count')).unread === 0, 'Mark all read: the server has none unread')
    await page.until(`!document.querySelector('.topbar .note-count')`, 5000, 'the count to go')
    check(true, 'Mark all read: the bell has no count')
  }

  // A kind turned off in Settings is off on the server; turned on again after.
  await page.go(server + '/settings/notifications')
  await page.until(`document.querySelector('[role=switch]')`, 15000, 'the choices')
  await page.eval(`document.querySelector('[role=switch]').click()`)
  await wait(1000)
  const after = (await api('GET', '/me/notifications/choices')).choices
  check(after[0].on === !choicesBefore[0].on, `Settings: ${after[0].kind} turned ${after[0].on ? 'on' : 'off'} on the server`)
  await shot(page, 'settings')
  await api('PUT', '/me/notifications/choices', { choices: choicesBefore })
  page.close()
}

await b.close()
console.log(failed ? `${failed} failed` : 'all passed')
process.exit(failed ? 1 : 0)
