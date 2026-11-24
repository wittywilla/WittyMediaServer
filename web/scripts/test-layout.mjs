// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The layout check: every signed-in page, on a computer, a laptop, a phone and in TV mode, against a real server, in
// a headless browser (no window opens). It fails when:
//   under-bar    something on the page is drawn under the top bar when it opens, or a part that stays put as the page
//                scrolls (a sticky column, a strip of links) stops under the bar instead of below it
//   anchor       a link to a section of a page (#licenses) scrolls the section under the bar or the strip of links
//   align        a Settings page's title and lead don't start where its content starts (docs/DESIGN.md, Centring)
//   off-centre   centred words in a box that isn't in the middle of what holds it, so they only look centred to it
//   title-case   a Settings page's title in Title Case; the menu and every title are in sentence case
//   repeated     two headings on one page that say the same thing
// Pictures of each page go to the out folder (<page>--<size>.png) for before-and-after comparisons.
//
//   WMS_SERVER=http://layout.localhost:19706 WMS_USER=owner WMS_PASSWORD=... node scripts/test-layout.mjs out-folder
//   WMS_TOKEN_FILE=<file with a sign-in token> instead of a name and password. LAYOUT_SIZES=desktop,phone and
//   LAYOUT_ROUTES=/you,/settings/remote narrow it; LAYOUT_NO_SHOTS=1 skips the pictures.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { open, wait } from './shots-lib.mjs'

const server = (process.env.WMS_SERVER || '').replace(/\/$/, '')
const [out = 'layout'] = process.argv.slice(2)
if (!server) { console.error('Set WMS_SERVER.'); process.exit(2) }
mkdirSync(out, { recursive: true })
const noShots = !!process.env.LAYOUT_NO_SHOTS

// Every page the settings menu opens, and the app's own pages around them.
const menu = JSON.parse(readFileSync(new URL('../../pkg/sdui/data/settingsmenu.json', import.meta.url), 'utf8'))
const menuPaths = [...new Set(menu.groups.flatMap((g) => g.rows.map((r) => r.path)).filter((p) => p && p.startsWith('/')).map((p) => p.split('#')[0]))]
const ROUTES = process.env.LAYOUT_ROUTES ? process.env.LAYOUT_ROUTES.split(',') : ['/', '/you', '/settings', ...menuPaths.filter((p) => p !== '/you'), '/search', '/libraries', '/collections', '/music', '/tv']
// Pages a link opens at one of its sections: the section must land below the bar.
const ANCHORS = [...new Set(menu.groups.flatMap((g) => g.rows.map((r) => r.path)).filter((p) => p && p.includes('#')))]
const SIZES = {
  desktop: { width: 1920, height: 1080 },
  laptop: { width: 1280, height: 800 },
  phone: { width: 390, height: 844, mobile: true },
  tv: { width: 1920, height: 1080, tv: true },
}
const sizes = (process.env.LAYOUT_SIZES || 'desktop,laptop,phone,tv').split(',')
// TV mode draws its own pages; these are the ones it shares with the rest of the app.
const TV_ROUTES = ['/', '/you', '/settings/about', '/settings/quality', '/settings/appearance']

// Words in a title that keep their capital: names of things, not Title Case.
const NAMES = /^(Witty\w*|TV|TVs|HTTPS?|VPN|IPTV|DVR|DVD|DVDs|Blu-ray|QR|PIN|ID|IDs|Wi-Fi|Android|Apple|Linux|Windows|macOS|FreeBSD|Docker|FFmpeg|GPU|CPU|HDR|EPG|M3U|XMLTV|API|URL|OK|I)$/

// Runs in the page. `phase` is 'open' (as the page first appears) or 'scrolled'.
const DETECT = (phase) => `(() => {
  const out = []
  const vw = innerWidth
  const vis = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 1 && r.height > 1 && s.visibility !== 'hidden' && s.display !== 'none' && +s.opacity > 0.05 }
  const name = (e) => (e.tagName.toLowerCase() + (e.id ? '#' + e.id : '') + (typeof e.className === 'string' && e.className ? '.' + e.className.trim().split(/\\s+/).slice(0, 3).join('.') : '')).slice(0, 80)
  const txt = (e) => (e.getAttribute('aria-label') || e.textContent || '').trim().replace(/\\s+/g, ' ').slice(0, 50)
  const add = (kind, e, detail) => out.push({ kind, where: e ? name(e) : '', text: e ? txt(e) : '', detail })
  const pinned = (e) => { for (let p = e; p && p !== document.body; p = p.parentElement) { const s = getComputedStyle(p).position; if (s === 'fixed' || s === 'sticky') return p } return null }
  // The top bar: the shell's own, when it is drawn and stays at the top.
  const bar = [...document.querySelectorAll('header.topbar')].find((b) => vis(b) && getComputedStyle(b).position !== 'static' && b.getBoundingClientRect().top <= 1)
  const barBottom = bar ? bar.getBoundingClientRect().bottom : 0
  const main = document.querySelector('main') || document.body
  // Pages drawn over a backdrop run under the bar on purpose (docs/DESIGN.md): their backdrop, not their words.
  const behind = (e) => e.closest('.detail-hero, .hero, .hero-art, .backdrop, [data-under-bar]')
  // What a person reads or presses: words, pictures, controls.
  const content = [...main.querySelectorAll('h1, h2, h3, p, img, button, a, input, select, label, .avatar-tile, [role=button]')].filter((e) => vis(e) && !e.closest('header.topbar'))
  if (bar && ${JSON.stringify(phase)} === 'open') {
    for (const e of content) {
      if (pinned(e) && pinned(e) !== e) continue
      if (behind(e) && !/^H[1-3]$/.test(e.tagName)) continue
      const r = e.getBoundingClientRect()
      if (r.top < barBottom - 1 && r.bottom > 1) { add('under-bar', e, 'top ' + Math.round(r.top) + ' under the bar to ' + Math.round(barBottom)); if (out.length > 6) break }
    }
  }
  if (bar && ${JSON.stringify(phase)} === 'scrolled') {
    // A part that stays put while the page scrolls must stay below the bar.
    for (const e of main.querySelectorAll('*')) {
      if (!vis(e) || getComputedStyle(e).position !== 'sticky') continue
      const r = e.getBoundingClientRect()
      if (r.top < barBottom - 1 && r.bottom > barBottom && r.top > -r.height) add('under-bar', e, 'stays at ' + Math.round(r.top) + ', the bar ends at ' + Math.round(barBottom))
    }
  }
  if (${JSON.stringify(phase)} === 'open') {
    // A Settings page: its title, lead and content start at the same edge.
    const head = document.querySelector('.set-main .set-head')
    if (head && vis(head)) {
      const h1 = head.querySelector('h1')
      const textLeft = (e) => { const rg = document.createRange(); rg.selectNodeContents(e); const rs = [...rg.getClientRects()].filter((r) => r.width > 0); return rs.length ? Math.min(...rs.map((r) => r.left)) : e.getBoundingClientRect().left }
      const textRight = (e) => { const rg = document.createRange(); rg.selectNodeContents(e); const rs = [...rg.getClientRects()].filter((r) => r.width > 0); return rs.length ? Math.max(...rs.map((r) => r.right)) : e.getBoundingClientRect().right }
      const rtl = getComputedStyle(head).direction === 'rtl'
      const edge = (e) => rtl ? textRight(e) : textLeft(e)
      const blocks = [...head.parentElement.children].filter((c) => c !== head && vis(c) && !c.matches('.set-help, .set-back, .set-toc, .set-notice, .set-notice-row, script, style') && c.getBoundingClientRect().top > head.getBoundingClientRect().bottom - 1)
      const first = blocks[0]
      if (h1 && first) {
        const want = rtl ? first.getBoundingClientRect().right : first.getBoundingClientRect().left
        for (const e of [h1, head.querySelector('p')].filter(Boolean)) {
          const d = Math.round(edge(e) - want)
          if (Math.abs(d) > 2) add('align', e, 'starts ' + d + 'px from the content under it (' + name(first) + ')')
        }
      }
      if (h1) {
        const words = h1.textContent.trim().split(/\\s+/).slice(1)
        const caps = words.filter((w) => /^[A-Z][a-z]+$/.test(w.replace(/[^A-Za-z-]/g, '')) && !${NAMES}.test(w.replace(/[^A-Za-z-]/g, '')))
        if (caps.length) add('title-case', h1, 'capitals on ' + caps.join(', '))
      }
    }
    // Centred words in a box that is off to one side of what holds it.
    for (const e of main.querySelectorAll('h1, h2, h3, p, .set-head, .sm-person')) {
      if (!vis(e) || getComputedStyle(e).textAlign !== 'center' || behind(e)) continue
      const p = e.parentElement; if (!p || !vis(p)) continue
      const ps = getComputedStyle(p)
      if ((ps.display.includes('flex') || ps.display.includes('grid')) && [...p.children].filter(vis).length > 1 && !(ps.display.includes('grid') && ps.gridTemplateColumns.split(' ').length === 1) && !(ps.display.includes('flex') && ps.flexDirection.startsWith('column'))) continue
      const r = e.getBoundingClientRect(), pr = p.getBoundingClientRect()
      const padL = parseFloat(ps.paddingLeft) + parseFloat(ps.borderLeftWidth), padR = parseFloat(ps.paddingRight) + parseFloat(ps.borderRightWidth)
      const inner = pr.width - padL - padR
      if (r.width >= inner - 2) continue
      const d = Math.round((r.left + r.width / 2) - (pr.left + padL + inner / 2))
      if (Math.abs(d) > 3) add('off-centre', e, 'centred words, but their box is ' + d + 'px off the middle of ' + name(p))
    }
    // The same heading twice.
    const hs = [...main.querySelectorAll('h1, h2, h3')].filter(vis).map((h) => h.textContent.trim().toLowerCase().replace(/\\s+/g, ' ')).filter(Boolean)
    const dup = hs.filter((h, i) => hs.indexOf(h) !== i)
    if (dup.length) add('repeated', null, 'headings said twice: ' + [...new Set(dup)].join(', '))
  }
  return out
})()`

// A link to a section: the section's top must be below the bar and anything stuck under it.
const ANCHOR_CHECK = (id) => `(() => {
  const t = document.getElementById(${JSON.stringify(id)})
  if (!t) return { missing: true }
  const vis = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 1 && r.height > 1 && s.visibility !== 'hidden' && s.display !== 'none' }
  let cover = 0, by = ''
  for (const e of document.querySelectorAll('header.topbar, main *')) {
    if (!vis(e)) continue
    const s = getComputedStyle(e).position
    if (s !== 'fixed' && s !== 'sticky') continue
    const r = e.getBoundingClientRect()
    if (r.top <= 2 + cover && r.bottom > cover && r.width > innerWidth * 0.3 && r.height < innerHeight / 3 && !e.contains(t)) { cover = r.bottom; by = e.className }
  }
  // A section near the page's end can't scroll to the top: it only has to be on screen, below the bar.
  const room = document.documentElement.scrollHeight - scrollY - innerHeight < 2 ? innerHeight : innerHeight / 2
  return { top: Math.round(t.getBoundingClientRect().top), cover: Math.round(cover), by: String(by).slice(0, 40), height: Math.round(room) }
})()`

let token = process.env.WMS_TOKEN_FILE ? readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim() : ''
if (!token) {
  const r = await fetch(server.replace('://localhost', '://127.0.0.1') + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: process.env.WMS_USER || 'owner', password: process.env.WMS_PASSWORD }) }).catch((e) => ({ json: () => ({ error: String(e) }) }))
  const j = await r.json()
  token = j.token
  if (!token) { console.error('FAIL sign-in', j); process.exit(1) }
}

const findings = []
const slug = (p) => (p === '/' ? 'home' : p.replace(/^\//, '').replace(/[/?=&#]+/g, '-'))
for (const sizeName of sizes) {
  const size = SIZES[sizeName]
  if (!size) { console.error('Unknown size', sizeName); continue }
  const s = await open(server, out, { width: size.width, height: size.height, path: '/legal' })
  try {
    if (size.mobile) await s.size(size.width, size.height, true, 2)
    await s.page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.tvclient')`)
    const routes = size.tv ? ROUTES.filter((r) => TV_ROUTES.includes(r)) : ROUTES
    for (const route of routes) {
      const path = size.tv ? route + (route.includes('?') ? '&' : '?') + 'tv=1&client=tv' : route
      await s.go(path, 400)
      // Ready: the page has its title (or the settings groups), and a moment for pictures and late parts.
      await s.page.until(`!!document.querySelector('main h1, main [data-settings-group], main .row, main .grid, .tv-root')`, 20000, 'the page').catch(() => {})
      await wait(900)
      const here = await s.page.eval(`location.pathname`)
      if (!here.startsWith(route.split('?')[0])) { console.log('SKIP', route, sizeName, 'went to', here); continue }
      const found = await s.page.eval(DETECT('open'))
      if (!noShots) await s.shot(`${slug(route)}--${sizeName}`)
      await s.page.eval(`scrollTo(0, Math.min(700, document.documentElement.scrollHeight - innerHeight))`)
      await wait(300)
      found.push(...await s.page.eval(DETECT('scrolled')))
      if (!noShots && found.some((f) => f.kind === 'under-bar')) await s.shot(`${slug(route)}--${sizeName}--scrolled`)
      for (const f of found) findings.push({ route, size: sizeName, ...f })
      console.log(found.length ? 'FAIL' : 'PASS', route, sizeName, found.length ? JSON.stringify(found) : '')
    }
    if (!size.tv) {
      for (const link of ANCHORS) {
        const [route, id] = link.split('#')
        await s.go(route, 400)
        await s.page.until(`!!document.getElementById(${JSON.stringify(id)})`, 15000, id).catch(() => {})
        await wait(500)
        await s.page.eval(`location.hash = ${JSON.stringify('#' + id)}`)
        // The page scrolls to the section once it's drawn (settings/jump.ts): wait for it to come into view.
        await s.page.until(`(() => { const t = document.getElementById(${JSON.stringify(id)}); return !!t && t.getBoundingClientRect().top < innerHeight / 2 })()`, 8000, '#' + id).catch(() => {})
        await wait(400)
        const a = await s.page.eval(ANCHOR_CHECK(id))
        if (a.missing) { console.log('SKIP', link, sizeName, 'no such section here'); continue }
        // Below the bar, and in view: a section left far down the page means the link didn't take you there.
        const ok = a.top >= a.cover - 1 && a.top < a.height
        if (!ok) { findings.push({ route: link, size: sizeName, kind: 'anchor', where: '#' + id, detail: `section at ${a.top}, covered to ${a.cover} by ${a.by}` }); if (!noShots) await s.shot(`${slug(link)}--${sizeName}`) }
        console.log(ok ? 'PASS' : 'FAIL', link, sizeName, JSON.stringify(a))
      }
    }
  } finally { await s.browser.close() }
}

writeFileSync(join(out, 'layout-findings.json'), JSON.stringify(findings, null, 1))
const by = {}
for (const f of findings) by[f.kind] = (by[f.kind] || 0) + 1
console.log(findings.length ? `FAILED: ${findings.length} findings ${JSON.stringify(by)}` : 'All layout checks passed')
process.exit(findings.length ? 1 : 0)
