// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// TV mode never draws a computer's keys (the owner's rule): no keycaps, no "Ctrl+[" or "Esc" on any TV page, before
// and after a key has been pressed, at 1920x1080 and 1280x720. The keys are in the Keys dialog, opened by the one Keys
// button; it opens, shows the remote's keys, and Back closes it. Headless: no window opens.
//   WMS_SERVER=http://127.0.0.1:19583 WMS_TOKEN_FILE=<a file holding a sign-in token> node scripts/test-tv-keys.mjs [out folder]
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { launch } from './cdp.mjs'

const server = process.env.WMS_SERVER
const token = readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim()
const out = process.argv[2] || ''
if (out) mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const H = { Authorization: 'Bearer ' + token }
const libs = await (await fetch(server + '/api/libraries', { headers: H })).json()

// Every kind of TV page there is on this server: Home, each library, a title or an album and an artist, search,
// You, settings, Live TV.
const pages = [['home', '/'], ['search', '/search'], ['you', '/you'], ['settings', '/settings'], ['live-tv', '/tv']]
for (const l of libs) {
  pages.push(['library-' + l.id, '/library/' + l.id])
  if (l.type === 'music') {
    const albums = await (await fetch(`${server}/api/music/albums?library=${l.id}&sort=title`, { headers: H })).json()
    const a = albums[0]
    if (a) {
      pages.push(['album', `/album/${l.id}?artist=${encodeURIComponent(a.artist)}&album=${encodeURIComponent(a.title)}`])
      pages.push(['artist', `/artist/${l.id}?name=${encodeURIComponent(a.artist)}`])
    }
  } else {
    const items = await (await fetch(`${server}/api/items?library=${l.id}&limit=1`, { headers: H })).json().catch(() => null)
    const first = Array.isArray(items) ? items[0] : items?.items?.[0]
    if (first) pages.push(['detail-' + l.id, '/item/' + first.id])
  }
}

// What a computer's key looks like on the page: a keycap that is drawn, or a shortcut written out.
const KEYS = /\b(Ctrl|Shift|Esc|Enter|F6|Backspace|Page Up|Page Down)\b|Ctrl\+/
const look = `(() => {
  const shown = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none' }
  const caps = [...document.querySelectorAll('kbd')].filter(shown).map((e) => e.textContent.trim())
  const words = [...document.querySelectorAll('.key-legend, .np-hints, .kar-hints, .dock')].filter(shown).map((e) => e.textContent.trim()).join(' | ')
  return { caps, words, info: !!document.querySelector('.key-info-btn') }
})()`

let failed = 0
const b = await launch({ width: 1920, height: 1080 })
for (const [w, h] of [[1920, 1080], [1280, 720]]) {
  const page = await b.open()
  await page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false })
  await page.go(server + '/')
  await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.setItem('wms.tvmode', '1')`)
  for (const [name, path] of pages) {
    await page.go(server + path)
    await wait(2500)
    for (const when of ['first', 'after a key']) {
      if (when === 'after a key') { await page.key('ArrowDown'); await wait(600) }
      const r = await page.eval(look)
      const bad = r.caps.length > 0 || KEYS.test(r.words)
      if (bad) failed++
      console.log(`${bad ? 'FAIL' : 'ok  '} ${w}x${h} ${name} (${when}): ${r.caps.length} keycaps${r.caps.length ? ' [' + r.caps.slice(0, 8).join(' ') + ']' : ''}${KEYS.test(r.words) ? ', shortcut words: ' + r.words.slice(0, 120) : ''}`)
    }
    if (out) writeFileSync(join(out, `${w}x${h}-${name}.png`), await page.shot())
  }
  // Now playing and karaoke, where a computer shows its keys along the foot.
  const album = pages.find(([n]) => n === 'album')
  if (album) {
    await page.go(server + album[1])
    await wait(2500)
    await page.eval(`document.querySelector('[data-tv-first], .mus-play, .btn.primary')?.click()`)
    await wait(2000)
    await page.eval(`document.querySelector('.audio-bar [data-open-full], .audio-bar .ab-open, .ab-expand, .ab-info')?.click()`)
    await wait(1500)
    for (const [name, open] of [['now playing', ''], ['karaoke', 'k']]) {
      if (open) { await page.key(open); await wait(2000) }
      const r = await page.eval(look)
      const bad = r.caps.length > 0 || KEYS.test(r.words)
      if (bad) failed++
      console.log(`${bad ? 'FAIL' : 'ok  '} ${w}x${h} ${name}: ${r.caps.length} keycaps${r.caps.length ? ' [' + r.caps.slice(0, 8).join(' ') + ']' : ''}`)
      if (out) writeFileSync(join(out, `${w}x${h}-${name.replace(' ', '-')}.png`), await page.shot())
    }
    await page.key('Escape'); await page.key('Escape'); await wait(500)
  }
  // The Keys dialog: the one button opens it with the remote's keys (no computer's), and Back closes it.
  await page.go(server + '/')
  await wait(2500)
  const opened = await page.eval(`(() => { const b = document.querySelector('.key-info-btn'); if (!b) return 'no Keys button'; b.click(); return 'clicked' })()`)
  await wait(500)
  const dialog = await page.eval(`(() => { const d = document.querySelector('.key-help'); return d ? d.textContent : '' })()`)
  if (out) writeFileSync(join(out, `${w}x${h}-keys-dialog.png`), await page.shot())
  await page.key('Escape')
  await wait(400)
  const closed = await page.eval(`!document.querySelector('.key-help')`)
  const ok = opened === 'clicked' && dialog && !/Ctrl|Shift|F6/.test(dialog) && closed
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${w}x${h} Keys dialog: ${opened}; ${dialog ? 'shows ' + dialog.slice(0, 80) : 'did not open'}; ${closed ? 'Back closes it' : 'still open after Back'}`)
  page.close()
}
await b.close()
console.log(failed ? `${failed} problem(s)` : 'TV mode draws no keyboard keys')
process.exit(failed ? 1 : 0)
