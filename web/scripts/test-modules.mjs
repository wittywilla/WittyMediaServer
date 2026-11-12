// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Walks the module pages (books, comics, PDF, photos, statistics, readers, games) in every look
// and theme, in a headless browser on a running test server with generated media, and fails:
// - text that is under 4.5:1 against what is behind it (3:1 for large text);
// - a control smaller than 44 px (66 px in big screen mode) at phone size, where a finger is the pointer;
// - a page that scrolls sideways;
// - motion that carries on when the person asked for less (prefers-reduced-motion);
// - a page with no heading, or a picture with no text.
// A picture of every page in every look goes to the output folder, to compare with the canvas
// boards in docs/design-2/images/modules.
//
//   WMS_SERVER=http://localhost:19805 WMS_TOKEN=... node scripts/test-modules.mjs out-folder
//
// WMS_PAGES=books,photos limits the pages; WMS_LOOKS=lumen,aero the looks; WMS_THEMES=none skips the
// theme sweep. The pages are found by WMS_MAP (JSON: page name to path), else the defaults below, which
// suit a server whose libraries were added in the order Photos, Books, Comics, Docs.
// Needs the look to be settable: the token's account must be an administrator.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(process.env.WITTYVISUALIZER ? new URL(process.env.WITTYVISUALIZER + '/tests/cdp.mjs', import.meta.url).href : './cdp.mjs')

const server = process.env.WMS_SERVER || 'http://localhost:19805'
const token = process.env.WMS_TOKEN || ''
const [out = '.'] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))

const PAGES = {
  books: '/library/2', book: '/item/13', reader: '/read/13',
  comics: '/library/3', comicReader: '/read/14',
  pdf: '/library/4', pdfReader: '/read/15',
  photos: '/library/1', stats: '/m/wittystats/stats',
  cd: '/cd/3', dvd: '/item/2', bluray: '/item/1', games: '/library/3', game: '/item/7',
  ...(process.env.WMS_MAP ? JSON.parse(process.env.WMS_MAP) : {}),
}
const pick = (v, all) => (v ? v.split(',') : all)
const pages = pick(process.env.WMS_PAGES, Object.keys(PAGES))
const LOOKS = ['lumen', 'rail', 'stage', 'aero', 'classic', 'classicCable', 'classicSatellite', 'compact', 'paper', 'marquee', 'crossbar', 'bigpicture', 'dashboard', 'retro']
const looks = pick(process.env.WMS_LOOKS, LOOKS)
const THEMES = ['default', 'aeroaurora', 'aeroday', 'aeromeadow', 'aerosunset', 'cable', 'cinema', 'evergreen', 'contrast', 'light', 'midnight', 'paper', 'satellite', 'teletext', 'oled']
const themes = process.env.WMS_THEMES === 'none' ? [] : pick(process.env.WMS_THEMES, THEMES)
// A reader draws its own full-screen frame: no menu bar, so it has no look of the shell's to wear.
const bare = (p) => /reader/i.test(p)

const api = async (method, path, body, headers = {}) => {
  const r = await fetch(server + '/api' + path, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', ...headers }, body: body ? JSON.stringify(body) : undefined })
  return r.json().catch(() => ({}))
}
async function setLook(experience, theme) {
  const got = await api('GET', '/sdui/config/experience')
  const ff = ['phone', 'tablet', 'desktop', 'web', 'tv', 'headset']
  await api('PUT', '/sdui/config/experience', { default: Object.fromEntries(ff.map((f) => [f, experience])) }, { 'If-Match': got.etag })
  if (theme) await api('PUT', '/settings', { theme })
}

// Runs in the page. Colours are read through a canvas, so any CSS colour syntax the browser keeps comes out as rgba.
const MEASURE = `(() => {
  const cv = document.createElement('canvas'); cv.width = cv.height = 1
  const cx = cv.getContext('2d', { willReadFrequently: true })
  const rgba = (c) => { cx.clearRect(0, 0, 1, 1); cx.fillStyle = '#000'; cx.fillStyle = c; cx.fillRect(0, 0, 1, 1); const d = cx.getImageData(0, 0, 1, 1).data; return [d[0], d[1], d[2], d[3] / 255] }
  const over = (f, b) => [0, 1, 2].map((i) => f[i] * f[3] + b[i] * (1 - f[3])).concat(1)
  const lum = (c) => { const l = c.slice(0, 3).map((v) => { v /= 255; return v <= .03928 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4 }); return .2126 * l[0] + .7152 * l[1] + .0722 * l[2] }
  const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + .05) / (Math.min(x, y) + .05) }
  const visible = (e) => { const r = e.getBoundingClientRect(); if (r.width < 1 || r.height < 1) return false; const s = getComputedStyle(e); return s.visibility !== 'hidden' && s.display !== 'none' && +s.opacity > .05 }
  // What is behind an element: its ancestors' backgrounds laid over each other. A picture or gradient behind it can't be read, so that text is counted apart.
  const behind = (e) => {
    const layers = []
    for (let n = e; n; n = n.parentElement) {
      const s = getComputedStyle(n)
      if (s.backgroundImage !== 'none') return null
      const c = rgba(s.backgroundColor)
      if (c[3] > 0) { layers.push(c); if (c[3] >= .999) break }
    }
    let b = [255, 255, 255, 1]
    if (!layers.length || layers[layers.length - 1][3] < .999) b = rgba(getComputedStyle(document.documentElement).backgroundColor); if (b[3] < 1) b = over(b, [255, 255, 255, 1])
    for (let i = layers.length - 1; i >= 0; i--) b = over(layers[i], b)
    return b
  }
  const bad = [], unread = []
  const seen = new Set()
  // The menu bar, the player bar and the footer belong to the shell, which has its own checks; a reader has no shell.
  const root = document.querySelector('main, [role=main]') || document.body
  const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  for (let t = w.nextNode(); t; t = w.nextNode()) {
    if (!/[\p{L}\p{N}]/u.test(t.nodeValue)) continue   // a lone dot or arrow between words isn't text to read
    const e = t.parentElement
    if (!e || seen.has(e) || /^(SCRIPT|STYLE)$/.test(e.tagName) || !visible(e)) continue
    seen.add(e)
    const s = getComputedStyle(e)
    const b = behind(e)
    const label = e.tagName.toLowerCase() + (e.className && typeof e.className === 'string' ? '.' + e.className.trim().split(/\\s+/).slice(0, 2).join('.') : '') + ' "' + t.nodeValue.trim().slice(0, 28) + '"'
    if (!b) { unread.push(label); continue }
    let op = 1; for (let n = e; n; n = n.parentElement) op *= +getComputedStyle(n).opacity
    const own = rgba(s.color)
    if (own[3] * op < .05) continue   // invisible by design: the text layer over a PDF page, for finding words
    const fg = over([own[0], own[1], own[2], own[3] * op], b)
    const px = parseFloat(s.fontSize), big = px >= 24 || (px >= 18.66 && +s.fontWeight >= 700)
    const r = ratio(fg, b)
    if (r < (big ? 3 : 4.5)) bad.push(label + ' ' + r.toFixed(2) + ':1')
  }
  // Controls a finger has to hit.
  const small = []
  const need = document.documentElement.classList.contains('tv-mode') ? 66 : 44
  for (const e of root.querySelectorAll('a[href], button, input:not([type=hidden]), select, textarea, [role=button], [role=tab], [role=menuitem], [tabindex]:not([tabindex="-1"])')) {
    if (!visible(e) || e.closest('[aria-hidden=true], [inert]')) continue
    const r = e.getBoundingClientRect()
    // A link inside a sentence is as big as its words; the rule is for controls that stand alone.
    if (e.tagName === 'A' && e.closest('p, li, .muted, .legal')) continue
    // A control can be a few pixels short and still sit in a row or a padded label that is the real target.
    const host = e.closest('label') || e
    const h = host.getBoundingClientRect()
    if (Math.max(r.width, h.width) < need - .5 || Math.max(r.height, h.height) < need - .5) small.push((e.getAttribute('aria-label') || e.textContent || e.tagName).trim().slice(0, 24) + ' ' + Math.round(r.width) + 'x' + Math.round(r.height))
  }
  // Motion: what still moves with prefers-reduced-motion on.
  const moving = []
  for (const e of document.querySelectorAll('body *')) {
    const s = getComputedStyle(e)
    const d = (v) => v.split(',').some((x) => parseFloat(x) > .02 * (x.includes('ms') ? 1000 : 1))
    if (s.animationName !== 'none' && d(s.animationDuration) && s.animationIterationCount !== '1' && visible(e)) moving.push(e.tagName.toLowerCase() + '.' + String(e.className).slice(0, 30) + ' ' + s.animationName)
  }
  return {
    bad: bad.slice(0, 12), nBad: bad.length, unread: unread.length, small: small.slice(0, 12), nSmall: small.length,
    sideways: document.documentElement.scrollWidth > innerWidth + 1, moving: moving.slice(0, 6),
    h1: !!document.querySelector('h1, [role=heading][aria-level="1"], main h2'), main: !!document.querySelector('main, [role=main]'),
    noAlt: [...root.querySelectorAll('img:not([alt])')].length,
    look: document.querySelector('[data-experience]')?.dataset.experience || document.documentElement.dataset.experience || '', theme: document.documentElement.dataset.theme || '',
  }
})()`

const browser = await launch({ software: true, width: 1280, height: 800 })
// A second browser whose pointer is a finger, for the target sizes: media queries on the pointer only follow the real device.
const touch = await launch({ software: true, width: 390, height: 844, args: ['--blink-settings=primaryPointerType=2,availablePointerTypes=2,primaryHoverType=1,availableHoverTypes=1', '--touch-events=enabled'] })
const pages_ = {}
for (const [k, b] of [['desk', browser], ['touch', touch]]) {
  pages_[k] = await b.open(server + '/help')
  await pages_[k].send('Emulation.setFocusEmulationEnabled', { enabled: true })
  await pages_[k].eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)})`)
}
let page = pages_.desk
const faults = []
const lines = []
const say = (s) => { lines.push(s); console.log(s) }

async function visit(name, path, label, { phone = false, shot = true } = {}) {
  const w = phone ? 390 : 1280, h = phone ? 844 : 800
  page = phone ? pages_.touch : pages_.desk
  await page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false })
  await page.send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-reduced-motion', value: 'reduce' }] })
  await page.go(server + path)
  await page.until(`document.body.innerText.trim().length > 20 && !document.querySelector('.spinner, .skeleton')`, 20000, path).catch(() => {})
  await page.eval(`new Promise((d) => { const t = Date.now(); const k = () => ([...document.images].every((i) => i.complete) || Date.now() - t > 3500) ? d() : setTimeout(k, 200); k() })`)
  await wait(900)
  const r = await page.eval(MEASURE)
  const where = `${name} @ ${label}${phone ? ' (phone)' : ''}`
  const f = []
  if (r.nBad) f.push(`low contrast x${r.nBad}: ${r.bad.slice(0, 4).join('; ')}`)
  // Targets only count at phone size, where the finger is the pointer; and a reader's own frame is its module's.
  if (phone && r.nSmall) f.push(`targets under 44: x${r.nSmall}: ${r.small.slice(0, 5).join('; ')}`)
  if (r.sideways) f.push('scrolls sideways')
  if (r.moving.length) f.push('still moving with reduced motion: ' + r.moving.join('; '))
  if (!r.h1 && !bare(name)) f.push('no main heading')
  if (r.noAlt) f.push(`${r.noAlt} picture(s) with no alt`)
  if (f.length) { faults.push(`${where}: ${f.join(' | ')}`); say('FAIL ' + where + ': ' + f.join(' | ')) } else say('ok   ' + where + (r.unread ? ` (${r.unread} text on pictures not measured)` : ''))
  if (shot) writeFileSync(join(out, `${name}-${label.replace(/[^\w-]/g, '_')}${phone ? '-phone' : ''}.png`), await page.shot())
}

try {
  // Every look at its own theme, on every page: desktop, and the phone for the library and statistics pages.
  for (const look of looks) {
    await setLook(look)
    const exp = (await api('GET', '/sdui/experiences')).experiences?.find((e) => e.id === look)
    if (exp?.theme) await api('PUT', '/settings', { theme: exp.theme })
    for (const p of pages) {
      await visit(p, PAGES[p], look)
      if (['books', 'photos', 'stats', 'cd', 'dvd', 'bluray', 'games', 'game'].includes(p)) await visit(p, PAGES[p], look, { phone: true, shot: false })
    }
  }
  // Every theme under the first look.
  if (themes.length) {
    await setLook('lumen')
    for (const t of themes) {
      await api('PUT', '/settings', { theme: t })
      for (const p of pages) await visit(p, PAGES[p], 'theme-' + t, { shot: false })
    }
  }
} finally {
  await browser.close(); await touch.close()
  writeFileSync(join(out, 'modules.txt'), lines.join('\n') + '\n')
}
console.log(faults.length ? `\n${faults.length} page(s) with faults; see ${join(out, 'modules.txt')}` : '\nall module pages pass')
process.exit(faults.length ? 1 : 0)
