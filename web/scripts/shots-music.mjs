// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Pictures of the music pages at desktop, phone and TV size, and checks of how moving
// between them behaves (back keeps your place, no blank flash), on a running test server.
// Headless: no window opens.
//   WMS_SERVER=http://music.localhost:19509 WMS_NAME=... WMS_PASS=... node scripts/shots-music.mjs out-folder [desktop|phone|tv ...]
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { launch } from './cdp.mjs'

const server = process.env.WMS_SERVER || 'http://127.0.0.1:19509'
const [out = '.', ...only] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
// Node can't look up a *.localhost name the way a browser does, so its own calls go to 127.0.0.1.
const api = server.replace(/\/\/[^/:]+\.localhost/, '//127.0.0.1')
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const login = await (await fetch(api + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: process.env.WMS_NAME, password: process.env.WMS_PASS }) })).json()
const libs = await (await fetch(api + '/api/libraries', { headers: { Authorization: 'Bearer ' + login.token } })).json()
// WMS_LIB picks the library by id; otherwise the first music library.
const lib = libs.find((l) => (process.env.WMS_LIB ? String(l.id) === process.env.WMS_LIB : l.type === 'music'))
const albums = await (await fetch(`${api}/api/music/albums?library=${lib.id}&sort=title`, { headers: { Authorization: 'Bearer ' + login.token } })).json()
const real = albums.find((a) => /21/.test(a.title) && /Adele/i.test(a.artist)) || albums[0]
const long = [...albums].sort((a, b) => b.title.length - a.title.length)[0]
const bare = albums.find((a) => !a.coverItem) || albums[2]
const albumUrl = (a) => `/album/${lib.id}?artist=${encodeURIComponent(a.artist)}&album=${encodeURIComponent(a.title)}`
const artistUrl = (n) => `/artist/${lib.id}?name=${encodeURIComponent(n)}`

const SIZES = {
  desktop: { width: 1440, height: 900, mobile: false, tv: false },
  phone: { width: 390, height: 844, mobile: true, tv: false },
  tv: { width: 1920, height: 1080, mobile: false, tv: true },
}
const report = []
const b = await launch({ width: 1440, height: 900 })
for (const [name, size] of Object.entries(SIZES)) {
  if (only.length && !only.includes(name)) continue
  const page = await b.open()
  await page.send('Emulation.setDeviceMetricsOverride', { width: size.width, height: size.height, deviceScaleFactor: 1, mobile: size.mobile })
  if (size.mobile) await page.send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 })
  await page.go(server + '/')
  await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)}); localStorage.setItem('wms.tvmode', '${size.tv ? 1 : 0}')`)
  const shot = async (label) => {
    // Pictures on screen loaded (or given up on), so the picture isn't of grey squares.
    try { await page.until(`[...document.images].filter((i) => { const r = i.getBoundingClientRect(); return r.bottom > 0 && r.top < innerHeight && i.loading !== 'lazy' || (r.bottom > 0 && r.top < innerHeight) }).every((i) => i.complete)`, 8000) } catch { /* draw what's there */ }
    await wait(700); writeFileSync(join(out, `${name}-${label}.png`), await page.shot()) }
  const visit = async (path, ready = 'document.querySelector("main, .tvm-main") && !document.querySelector(".spinner, .skeleton, .sk-grid, .mus-shape")') => {
    await page.go(server + path)
    try { await page.until(ready, 20000) } catch (e) { report.push(`${name} ${path}: ${e.message}`) }
  }
  // Client-side moves, as a press on a link makes them.
  const push = (path) => page.eval(`(() => { history.pushState({ usr: null, key: Math.random().toString(36).slice(2), idx: (history.state?.idx ?? 0) + 1 }, '', ${JSON.stringify(path)}); dispatchEvent(new PopStateEvent('popstate', { state: history.state })) })()`)
  await visit(`/library/${lib.id}`)
  await shot('home')
  await visit(`/library/${lib.id}?tab=albums`)
  await shot('albums')
  report.push(`${name}: albums page shows ${await page.eval(`document.querySelectorAll('a[href^="/album/"]').length`)} album links on the page`)
  // Back keeps your place: scroll down the albums, open one, come back.
  await page.eval('window.scrollTo(0, 2400)')
  await wait(400)
  const before = await page.eval('Math.round(window.scrollY)')
  const link = await page.eval(`(document.querySelectorAll('a[href^="/album/"]')[12] || document.querySelector('a[href^="/album/"]'))?.getAttribute('href') || ''`)
  if (link) {
    await page.eval(`document.querySelector('a[href=${JSON.stringify(JSON.stringify(link)).slice(1, -1)}]')?.click()`)
    await wait(1500)
    // How many frames showed nothing but a spinner while coming back.
    await page.eval(`window.__blank = 0; (function f(n) { if (document.querySelector('.spinner, .skeleton, .sk-grid')) window.__blank++; if (n) requestAnimationFrame(() => f(n - 1)) })(60)`)
    await page.eval('history.back()')
    await wait(1800)
    const after = await page.eval('Math.round(window.scrollY)')
    const blank = await page.eval('window.__blank')
    report.push(`${name}: albums scrolled to ${before}, back from an album lands at ${after}; ${blank} of 60 frames showed a loading state`)
  }
  await visit(`/library/${lib.id}?tab=artists`)
  await shot('artists')
  await visit(`/library/${lib.id}?tab=songs`)
  const rows = await page.eval(`document.querySelectorAll('.track-list li, .mus-tr:not(.head)').length`)
  report.push(`${name}: the songs list has ${rows} rows on the page`)
  await shot('songs')
  await visit(albumUrl(real))
  await shot('album-real')
  await visit(albumUrl(long))
  await shot('album-long-name')
  await visit(albumUrl(bare))
  await shot('album-no-art')
  await visit(artistUrl(real.artist))
  await shot('artist')
  // Something playing: the bar, then the full player with its queue.
  await page.eval(`document.querySelector('[data-tv-first], .mus-play, .btn.primary')?.click()`)
  await wait(1500)
  await shot('artist-playing')
  await page.eval(`document.querySelector('.ab-info, .mus-bar-info')?.click()`)
  await wait(1200)
  await shot('full-player')
  await page.key('Escape')
  await wait(600)
  // Music's own search where there is one (after), the app's search otherwise (before).
  await visit(process.env.MUSIC_SEARCH ? `/library/${lib.id}?tab=search&q=${encodeURIComponent(process.env.WMS_QUERY || 'Velvet')}` : `/search?q=${encodeURIComponent('Velvet')}`)
  await shot('search')
  // Keyboard: where Tab goes first on the album page.
  await visit(albumUrl(real))
  const tabs = []
  for (let i = 0; i < 6; i++) { await page.key('Tab'); tabs.push(await page.eval(`(() => { const e = document.activeElement; return (e?.getAttribute('aria-label') || e?.textContent || e?.tagName || '').trim().slice(0, 40) })()`)) }
  report.push(`${name}: Tab order on an album: ${tabs.join(' → ')}`)
  page.close()
}
await b.close()
writeFileSync(join(out, 'report.txt'), report.join('\n') + '\n')
console.log(report.join('\n'))
