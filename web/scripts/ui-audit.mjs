// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The interface audit: opens every page of a test server at each size, theme and look, takes a
// picture, and runs the same detectors on each so the obvious faults are found by a machine before a
// person has to spot them: sideways scroll, text under the top bar, hard-to-read text, tiny targets,
// clipped words, broken pictures, missing focus. Also the visual regression harness: pictures are
// named <page>--<size>--<theme>.png so two runs can be compared. No window opens.
//
//   WMS_SERVER=http://audit.localhost:19516 WMS_USER=owner WMS_PASSWORD=... node scripts/ui-audit.mjs out-folder
//   WMS_SIZES=phone,desktop WMS_THEMES=default,light WMS_ROUTES=/,/settings node scripts/ui-audit.mjs out
//   WMS_NO_SHOTS=1 skips the pictures (the detectors only).
// Output: out/findings.json (every finding) and out/summary.md.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const [out = 'audit'] = process.argv.slice(2)
const SIZES = {
  phone: [390, 844, true], phone360: [360, 740, true], tablet: [768, 1024, true], desktop: [1280, 800, false], wide: [1920, 1080, false],
  tv: [1920, 1080, false, true],
}
const sizes = (process.env.WMS_SIZES || 'phone,tablet,desktop,tv').split(',')
const themes = (process.env.WMS_THEMES || 'default,light,aerosunset').split(',')
const noShots = !!process.env.WMS_NO_SHOTS

// The detectors run inside the page. Each returns { kind, where, detail }.
const DETECT = `(() => {
  const out = []
  const vw = innerWidth, vh = innerHeight
  const vis = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 1 && r.height > 1 && s.visibility !== 'hidden' && s.display !== 'none' && +s.opacity > 0.05 }
  const name = (e) => (e.tagName.toLowerCase() + (e.id ? '#' + e.id : '') + (typeof e.className === 'string' && e.className ? '.' + e.className.trim().split(/\\s+/).slice(0, 3).join('.') : '')).slice(0, 80)
  const txt = (e) => (e.getAttribute('aria-label') || e.textContent || '').trim().replace(/\\s+/g, ' ').slice(0, 40)
  const add = (kind, e, detail) => out.push({ kind, where: name(e), text: txt(e), detail })
  // 1 sideways scroll
  const de = document.documentElement
  if (de.scrollWidth > vw + 1) out.push({ kind: 'overflow-x', where: 'document', detail: de.scrollWidth + ' > ' + vw })
  const all = [...document.body.querySelectorAll('*')].filter(vis)
  // 2 elements running off the right edge (not inside a scroller)
  const scrollers = (e) => { for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) { const o = getComputedStyle(p); if (/(auto|scroll|hidden|clip)/.test(o.overflowX) && p.scrollWidth >= p.clientWidth) return true } return false }
  for (const e of all) { const r = e.getBoundingClientRect(); if (r.right > vw + 2 && !scrollers(e) && getComputedStyle(e).position !== 'fixed' && r.left < vw) { add('off-edge', e, Math.round(r.right) + ' > ' + vw); if (out.length > 400) break } }
  // 3 the page's first heading under a fixed bar
  const fixedTop = all.filter((e) => { const s = getComputedStyle(e); const r = e.getBoundingClientRect(); return (s.position === 'fixed' || s.position === 'sticky') && r.top <= 40 && r.height < 140 && r.width > vw * 0.4 })
  const h = [...document.querySelectorAll('main h1, h1, .page-title, .hero h2, [role=heading]')].find((e) => vis(e) && !e.closest('header, nav'))
  if (h) { const hr = h.getBoundingClientRect(); for (const b of fixedTop) { const br = b.getBoundingClientRect(); if (!b.contains(h) && hr.top < br.bottom - 1 && hr.bottom > br.top + 1) add('under-bar', h, 'heading ' + Math.round(hr.top) + '-' + Math.round(hr.bottom) + ' vs bar ' + name(b) + ' to ' + Math.round(br.bottom)) } }
  // 4 contrast of text against the nearest solid background
  const parse = (c) => { const m = c.match(/rgba?\\(([^)]+)\\)/); if (!m) return null; const p = m[1].split(/[ ,\\/]+/).filter(Boolean).map(parseFloat); return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1] }
  const over = (f, b) => [f[0] * f[3] + b[0] * (1 - f[3]), f[1] * f[3] + b[1] * (1 - f[3]), f[2] * f[3] + b[2] * (1 - f[3]), 1]
  const lum = (c) => { const v = c.slice(0, 3).map((x) => { x /= 255; return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4) }); return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2] }
  const ratio = (a, b) => { const x = lum(a), y = lum(b); return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05) }
  const bgOf = (e) => { let img = false; let acc = null; for (let p = e; p; p = p.parentElement) { const s = getComputedStyle(p); if (s.backgroundImage && s.backgroundImage !== 'none') img = true; const c = parse(s.backgroundColor); if (c && c[3] > 0) { acc = acc ? over(acc, c) : c; if (c[3] >= 0.99) return { c: acc, img } } } return { c: acc ? over(acc, parse(getComputedStyle(document.body).backgroundColor) || [0, 0, 0, 1]) : parse(getComputedStyle(document.body).backgroundColor) || [0, 0, 0, 1], img } }
  const seen = new Set()
  for (const e of all) {
    if (![...e.childNodes].some((n) => n.nodeType === 3 && n.nodeValue.trim())) continue
    const s = getComputedStyle(e); const f = parse(s.color); if (!f) continue
    const bg = bgOf(e); const fg = over(f, bg.c)
    const size = parseFloat(s.fontSize), bold = +s.fontWeight >= 700
    const need = size >= 24 || (size >= 18.6 && bold) ? 3 : 4.5
    const r = ratio(fg, bg.c)
    if (r < need) { const key = name(e) + Math.round(r * 10); if (seen.has(key)) continue; seen.add(key); add(bg.img ? 'contrast-over-image' : 'contrast', e, r.toFixed(2) + ':1 needs ' + need + (bg.img ? ' (picture behind: check by eye)' : '')) }
  }
  // 5 small targets
  const touch = matchMedia('(pointer: coarse)').matches || vw < 900
  for (const e of all.filter((x) => x.matches('button, a[href], [role=button], [role=tab], input:not([type=hidden]), select, [role=menuitem]'))) { const r = e.getBoundingClientRect(); const min = touch ? 40 : 24; const inline = e.matches('a') && getComputedStyle(e).display === 'inline'; if ((r.width < min || r.height < min) && !e.closest('.sr-only') && !inline) add('small-target', e, Math.round(r.width) + 'x' + Math.round(r.height)) }
  // 6 clipped or ellipsised words
  for (const e of all) { const s = getComputedStyle(e); if (e.scrollWidth > e.clientWidth + 2 && s.textOverflow === 'ellipsis' && e.textContent.trim().length > 0) add('truncated', e, e.scrollWidth + ' > ' + e.clientWidth); else if (e.scrollWidth > e.clientWidth + 4 && /(hidden|clip)/.test(s.overflowX) && e.children.length === 0 && e.textContent.trim().length > 2 && s.textOverflow !== 'ellipsis') add('clipped-text', e, e.scrollWidth + ' > ' + e.clientWidth) }
  // 7 broken pictures
  for (const i of document.querySelectorAll('img')) if (vis(i) && i.complete && i.naturalWidth === 0) add('broken-image', i, i.currentSrc.slice(-60))
  // 8 buttons that touch or overlap their neighbour
  const btn = all.filter((x) => x.matches('button, .btn, a.btn')).slice(0, 200)
  for (let a = 0; a < btn.length; a++) for (let b = a + 1; b < btn.length; b++) { if (btn[a].contains(btn[b]) || btn[b].contains(btn[a])) continue; const x = btn[a].getBoundingClientRect(), y = btn[b].getBoundingClientRect(); const w = Math.min(x.right, y.right) - Math.max(x.left, y.left), hh = Math.min(x.bottom, y.bottom) - Math.max(x.top, y.top); if (w > 2 && hh > 2 && getComputedStyle(btn[a]).position !== 'fixed') add('overlap', btn[a], 'with ' + name(btn[b])) }
  // 9 raw error words
  const body = document.body.innerText
  if (/\\b(undefined|NaN|\\[object Object\\]|null)\\b/.test(body)) out.push({ kind: 'raw-value', where: 'text', detail: (body.match(/.{0,30}\\b(undefined|NaN|\\[object Object\\])\\b.{0,30}/) || [''])[0] })
  if (document.body.innerText.trim().length < 12) out.push({ kind: 'empty-page', where: 'main', detail: 'nothing drawn' })
  return out.slice(0, 120)
})()`

// Tabs through the page and reports controls that take focus without showing it.
const FOCUS = `(async () => {
  const bad = [], seen = new Set(); let n = 0
  const style = (e) => { const s = getComputedStyle(e); return s.outlineStyle + s.outlineWidth + s.outlineColor + s.boxShadow + s.borderColor + s.backgroundColor + s.transform }
  return { stops: 0 }
})()`

const s = await open(server, out, { width: 1280, height: 800, path: '/help' })
const { page, call, go, size, shot } = s
const login = await call('POST', '/login', { name: process.env.WMS_USER || 'owner', password: process.env.WMS_PASSWORD })
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)
await call('POST', '/legal/acknowledge')

const lr = await call('GET', '/libraries'); const libs = lr.libraries || lr
const first = async (path) => { const r = await call('GET', path); return r }
const routes = ['/', '/search', '/you', '/libraries', '/music', '/photos', '/books', '/games', '/tv', '/collections', '/help', '/design',
  ...libs.map((l) => `/library/${l.id}`), '/settings', '/settings/libraries', '/settings/appearance', '/settings/users', '/settings/modules', '/settings/playback', '/settings/transcoding', '/settings/remote', '/settings/security', '/settings/logs', '/settings/about', '/settings/backup', '/settings/channels', '/settings/sharing', '/settings/metadata', '/settings/activity', '/settings/legal', '/settings/diagnoser', '/settings/repair']
// Detail pages from whatever the libraries hold.
for (const l of libs) {
  try {
    if (l.type === 'shows') { const r = await call('GET', `/shows?library=${l.id}&limit=1`); const sh = (r.shows || r.items || [])[0]; if (sh) routes.push(`/show/${sh.id}`) }
    else { const r = await call('GET', `/items?library=${l.id}&limit=1`); const it = (r.items || r)[0]; if (it) routes.push(`/item/${it.id}`) }
  } catch { /* a library with nothing */ }
}
const only = process.env.WMS_ROUTES ? process.env.WMS_ROUTES.split(',') : null
const list = [...new Set(only || routes)]
const findings = []
mkdirSync(out, { recursive: true })
const slug = (p) => (p === '/' ? 'home' : p.replace(/^\//, '').replace(/[^a-z0-9]+/gi, '-'))

for (const theme of themes) {
  await call('PUT', '/settings', { theme })
  for (const sz of sizes) {
    const [w, h, mobile, tv] = SIZES[sz]
    await size(w, h, mobile)
    for (const path of list) {
      const url = server + path + (tv ? (path.includes('?') ? '&' : '?') + 'tv=1' : (path.includes('?') ? '&' : '?') + 'tv=0')
      try {
        await page.go(url)
        await wait(1600)
        await page.until(`document.fonts?.status !== 'loading'`, 8000).catch(() => {})
        const found = await page.eval(DETECT)
        for (const f of found) findings.push({ path, size: sz, theme, ...f })
        if (!noShots) writeFileSync(join(out, `${slug(path)}--${sz}--${theme}.png`), await page.shot())
        console.log(theme, sz, path, found.length)
      } catch (e) { findings.push({ path, size: sz, theme, kind: 'harness', detail: String(e.message).slice(0, 200) }); console.log('FAIL', path, e.message) }
    }
  }
}
const logs = page.logs.filter((l) => /^(error|exception)/.test(l))
writeFileSync(join(out, 'findings.json'), JSON.stringify({ findings, logs }, null, 1))
// A summary grouped by what is wrong and where.
const by = {}
for (const f of findings) { const k = `${f.kind} | ${f.where}`; (by[k] ||= { n: 0, pages: new Set(), ex: f }).n++; by[k].pages.add(f.path) }
const rows = Object.entries(by).sort((a, b) => b[1].n - a[1].n).map(([k, v]) => `| ${k} | ${v.n} | ${[...v.pages].slice(0, 5).join(' ')} | ${v.ex.detail} |`)
writeFileSync(join(out, 'summary.md'), '| kind and element | count | pages | example |\n|---|---|---|---|\n' + rows.join('\n') + '\n\nConsole errors:\n' + [...new Set(logs)].join('\n') + '\n')
console.log('done', findings.length, 'findings')
s.browser.close?.()
process.exit(0)
