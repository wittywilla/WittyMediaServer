// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The first click after using a controller must land on what is under the pointer. Puts the page in controller
// styling (html.using-controller), presses the mouse on a card without moving first, and checks that the thing
// clicked is the thing that was under the pointer and that nothing moved between the press and the release.
//   WMS_SERVER=http://audit.localhost:19516 WMS_PASSWORD=... node scripts/test-controller-click.mjs
//   (the server needs libraries of movies or shows; WMS_OLD=1 restores the old press handler, which must FAIL)
import { open, wait } from './shots-lib.mjs'
const s = await open(process.env.WMS_SERVER, '.', { width: 1280, height: 800, path: '/help' })
const { page } = s
const login = await s.call('POST', '/login', { name: process.env.WMS_USER || 'owner', password: process.env.WMS_PASSWORD })
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)
const libs = await s.call('GET', '/libraries'); const lib = (libs.libraries || libs).find((l) => l.type === 'movies')
let bad = 0
// Pages whose first screen holds still (Home's carousel moves by itself, which would read as a layout change).
const pages = (libs.libraries || libs).filter((l) => ['movies', 'shows'].includes(l.type)).map((l) => `/library/${l.id}`)
for (const path of pages) {
  await page.go(process.env.WMS_SERVER + path + '?tv=0'); await wait(3500)
  // WMS_OLD=1 puts back the handler that used to remove the class on press, to show this test catches it.
  if (process.env.WMS_OLD) await page.eval(`window.addEventListener('mousedown', () => document.documentElement.classList.remove('using-controller'), { passive: true })`)
  if (!process.env.WMS_NO_CLASS) await page.eval(`document.documentElement.classList.add('using-controller')`)
  await wait(1500) // the page settles (pictures, carousels) before the press
  const r = await page.eval(`(() => { const c = [...document.querySelectorAll('a.card, a[href^="/item/"], a[href^="/show/"]')].find((e) => { const b = e.getBoundingClientRect(); return b.width > 40 && b.top > 150 && b.bottom < innerHeight }); if (!c) return null; const b = c.getBoundingClientRect(); window.__hits = []; window.addEventListener('click', (e) => { e.preventDefault(); window.__hits.push(e.target.closest('a')?.getAttribute('href') || e.target.tagName) }, true); return { x: Math.round(b.left + b.width / 2), y: Math.round(b.top + b.height / 2), href: c.getAttribute('href'), rect: [b.left, b.top, b.width, b.height] } })()`)
  if (!r) { console.log(path, 'no card to press'); bad++; continue }
  const under = await page.eval(`document.elementFromPoint(${r.x}, ${r.y})?.closest('a')?.getAttribute('href')`)
  // Press and release at that point, with no mouse move first (the case that re-laid the page out).
  await page.send('Input.dispatchMouseEvent', { type: 'mousePressed', x: r.x, y: r.y, button: 'left', clickCount: 1 })
  await wait(350) // a person holds the button for a moment: long enough for a layout change to show
  const during = await page.eval(`[...document.querySelector('a[href="${r.href}"]').getBoundingClientRect().toJSON ? [] : []]; (() => { const b = document.querySelector('a[href="${r.href}"]').getBoundingClientRect(); return [b.left, b.top, b.width, b.height] })()`)
  await page.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: r.x, y: r.y, button: 'left', clickCount: 1 })
  await wait(300)
  const hits = await page.eval('window.__hits')
  const moved = during.some((v, i) => Math.abs(v - r.rect[i]) > 0.5)
  const ok = hits.length === 1 && hits[0] === under && !moved
  console.log(ok ? 'ok  ' : 'FAIL', path, 'under:', under, 'clicked:', JSON.stringify(hits), moved ? 'LAYOUT MOVED between press and release' : 'layout steady')
  if (!ok) bad++
}
process.exit(bad ? 1 : 0)
