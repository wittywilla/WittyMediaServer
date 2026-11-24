// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The layout check for every page outside Settings (Settings has its own: test-layout.mjs): home, the libraries and a
// title of each, search, collections, music, games, photos, books, Live TV, You, Help, the review pages, a page that
// isn't there, and the signed-out pages. Each at a computer's sizes, a tablet, a phone and in TV mode, against a real
// server, in a headless browser (no window opens). It fails when:
//   under-bar     words, a picture or a control is drawn under the top bar as the page opens, or a part that stays
//                 put as the page scrolls stops under the bar instead of below it
//   anchor        a section brought into view (a link to #it, or the page scrolling to it) lands under the bar
//   overflow-x    the page scrolls sideways
//   mixed-align   the page's title and what follows it don't agree: one centred and the other along the edge, or a
//                 title and its lead in different alignments (docs/DESIGN.md, Centring)
//   align         a title along the edge that doesn't start where the content under it starts
//   off-centre    centred words in a box that isn't in the middle of what holds it, so they only look centred to it
//   repeated      the same heading twice on a page, or a heading right above a label that says the same thing
//   focus         a control reached with Tab shows no focus ring
//   focus-scrolled  Tab scrolled a box that has overflow hidden, moving what it holds out of place
//   tv-target     in TV mode, something to press smaller than 48 by 48
// Pictures of each page go to the out folder (<page>--<size>.png) for before-and-after comparisons.
//
//   WMS_SERVER=http://pages.localhost:19731 WMS_TOKEN_FILE=token.txt node scripts/test-layout-pages.mjs out-folder
//   or WMS_USER and WMS_PASSWORD instead of a token. LAYOUT_SIZES=desktop,phone and LAYOUT_ROUTES=/,/you narrow it;
//   LAYOUT_NO_SHOTS=1 skips the pictures; LAYOUT_REPORT=1 lists every finding but exits 0 (for an audit).
//   LAYOUT_NAV=floatingGlassBar,ornamentTabs also checks each page under those top-bar styles (data-nav-style), at
//   the computer sizes: a bar that floats, or is taller, is where a guess at its height shows.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { open, wait } from './shots-lib.mjs'

const server = (process.env.WMS_SERVER || '').replace(/\/$/, '')
const [out = 'layout-pages'] = process.argv.slice(2)
if (!server) { console.error('Set WMS_SERVER.'); process.exit(2) }
mkdirSync(out, { recursive: true })
const noShots = !!process.env.LAYOUT_NO_SHOTS

const SIZES = {
  wide: { width: 1920, height: 1080 },
  desktop: { width: 1280, height: 720 },
  tablet: { width: 768, height: 1024, mobile: true },
  phone: { width: 375, height: 812, mobile: true },
  tv: { width: 1920, height: 1080, tv: true },
}
const sizes = (process.env.LAYOUT_SIZES || 'wide,desktop,tablet,phone,tv').split(',')

// Pages every server has (You is Settings' own check, test-layout.mjs). The libraries, and one title from each (a film, a show, an album, a book...), are found on
// the server as it runs, so the check covers whatever kinds of library it holds.
const FIXED = ['/', '/libraries', '/search?q=a', '/collections', '/music', '/games', '/photos', '/books', '/tv', '/tv/recordings', '/help', '/review', '/fix-matches', '/no-such-page']
// TV mode draws its own pages for most of these; these are the ones it shares.
const TV_SKIP = new Set(['/help', '/review', '/fix-matches'])
// Signed out: the standalone pages (docs/DESIGN.md, standalone pages).
const SIGNED_OUT = ['/', '/pair', '/no-such-page']

const DETECT = (phase, tv) => `(() => {
  const out = []
  const vw = innerWidth
  const vis = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 1 && r.height > 1 && s.visibility !== 'hidden' && s.display !== 'none' && +s.opacity > 0.05 && !e.closest('.sr-only, [aria-hidden=true], [inert]') }
  const name = (e) => (e.tagName.toLowerCase() + (e.id ? '#' + e.id : '') + (typeof e.className === 'string' && e.className ? '.' + e.className.trim().split(/\\s+/).slice(0, 3).join('.') : '')).slice(0, 80)
  const txt = (e) => (e.getAttribute('aria-label') || e.textContent || '').trim().replace(/\\s+/g, ' ').slice(0, 50)
  const add = (kind, e, detail) => out.push({ kind, where: e ? name(e) : '', text: e ? txt(e) : '', detail })
  const pinned = (e) => { for (let p = e; p && p !== document.body; p = p.parentElement) { const s = getComputedStyle(p).position; if (s === 'fixed' || s === 'sticky') return p } return null }
  const layer = (e) => e.closest('dialog, [role=dialog], .modal, .sheet, .popover, .menu, [role=menu], [role=listbox], .toast, .player, .audio-bar, .dock, .tabbar')
  const bar = [...document.querySelectorAll('header.topbar')].find((b) => vis(b) && getComputedStyle(b).position !== 'static' && b.getBoundingClientRect().top <= 16)
  const barBottom = bar ? bar.getBoundingClientRect().bottom : 0
  const main = document.querySelector('main') || document.body
  // Pictures that open a page run under the bar on purpose (docs/DESIGN.md): their picture, not their words.
  const behind = (e) => e.closest('.detail-hero, .hero, .hero-art, .backdrop, [data-under-bar]')
  // The part of it that can be seen: a box that scrolls inside the page (the guide, a list) clips what it has
  // scrolled past at its own edge, which is not under the bar.
  const shown = (e) => { const r = e.getBoundingClientRect(); let top = r.top, bottom = r.bottom; for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) { const o = getComputedStyle(p); if (/(auto|scroll|hidden|clip)/.test(o.overflowY) && p.scrollHeight > p.clientHeight + 1) { const pr = p.getBoundingClientRect(); top = Math.max(top, pr.top); bottom = Math.min(bottom, pr.bottom) } } return { top, bottom } }
  const content = [...main.querySelectorAll('h1, h2, h3, p, img, button, a, input, select, label, [role=button]')].filter((e) => vis(e) && !e.closest('header.topbar') && !layer(e))
  if (${JSON.stringify(phase)} === 'open') {
    if (bar) for (const e of content) {
      if (pinned(e) && pinned(e) !== e) continue
      if (behind(e) && !/^H[1-3]$/.test(e.tagName)) continue
      const r = shown(e)
      if (r.bottom - r.top < 1) continue
      if (r.top < barBottom - 1 && r.bottom > 1) { add('under-bar', e, 'top ' + Math.round(r.top) + ' under the bar to ' + Math.round(barBottom)); if (out.length > 6) break }
    }
    // Sideways scroll.
    const de = document.documentElement
    if (de.scrollWidth > vw + 1) {
      const wide = [...document.body.querySelectorAll('*')].filter((e) => vis(e) && e.getBoundingClientRect().right > vw + 1 && !pinned(e)).filter((e) => { for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) { if (/(auto|scroll|hidden|clip)/.test(getComputedStyle(p).overflowX)) return false } return true })
      add('overflow-x', wide[0] || null, 'the page is ' + de.scrollWidth + ' wide in a ' + vw + ' window')
    }
    // The page's own title, its lead and the sections under it agree on one alignment.
    const loose = (e) => !e.closest('.card, [class*="-card"], li, dialog, [role=dialog], .hero, .detail-hero, .row, .grid, .empty, .tile, fieldset, form, table, nav, aside, .set-shell, .standalone, .sa-card') && !layer(e)
    const centred = (e) => { const s = getComputedStyle(e); if (s.textAlign === 'center') return true; const p = e.parentElement, ps = p && getComputedStyle(p); return !!ps && ps.display.includes('flex') && ps.flexDirection.startsWith('column') && ps.alignItems === 'center' }
    const h1 = [...main.querySelectorAll('h1')].find((h) => vis(h) && loose(h))
    if (h1) {
      const lead = h1.nextElementSibling && h1.nextElementSibling.matches('p') && vis(h1.nextElementSibling) ? h1.nextElementSibling : (h1.parentElement && [...h1.parentElement.children].find((c) => c !== h1 && c.matches('p') && vis(c)))
      if (lead && centred(lead) !== centred(h1)) add('mixed-align', lead, 'the title is ' + (centred(h1) ? 'centred' : 'along the edge') + ', its lead ' + (centred(lead) ? 'centred' : 'along the edge'))
      const below = h1.getBoundingClientRect().bottom
      const sections = [...main.querySelectorAll('h2, h3')].filter((h) => vis(h) && loose(h) && h.getBoundingClientRect().top > below)
      const off = sections.filter((h) => centred(h) !== centred(h1))
      if (sections.length && off.length) add('mixed-align', off[0], 'the title is ' + (centred(h1) ? 'centred' : 'along the edge') + ', but ' + off.length + ' of ' + sections.length + ' section headings are ' + (centred(h1) ? 'along the edge' : 'centred'))
      if (!centred(h1)) {
        // Along the edge: the title starts where the content under it starts.
        const textLeft = (e) => { const rg = document.createRange(); rg.selectNodeContents(e); const rs = [...rg.getClientRects()].filter((r) => r.width > 0); return rs.length ? Math.min(...rs.map((r) => r.left)) : e.getBoundingClientRect().left }
        let head = h1; while (head.parentElement && head.parentElement !== main && head.parentElement.children.length === 1) head = head.parentElement
        // The page's content under it: a block at least half as wide as the column (not a row of switches at the end).
        const colW = head.parentElement ? head.parentElement.getBoundingClientRect().width : vw
        const first = head.parentElement && [...head.parentElement.children].find((c) => c !== head && vis(c) && !c.matches('p, .sr-only, script, style') && c.getBoundingClientRect().top >= head.getBoundingClientRect().bottom - 1 && getComputedStyle(c).position === 'static' && c.getBoundingClientRect().width >= colW / 2)
        if (first && getComputedStyle(head).direction !== 'rtl') {
          const fr = first.getBoundingClientRect(), d = Math.round(textLeft(h1) - fr.left)
          const fullBleed = fr.width >= vw - 2
          if (!fullBleed && Math.abs(d) > 2) add('align', h1, 'starts ' + d + 'px from the content under it (' + name(first) + ')')
        }
      }
    }
    // Centred words in a box that is off to one side of what holds it.
    for (const e of main.querySelectorAll('h1, h2, h3, p')) {
      if (!vis(e) || getComputedStyle(e).textAlign !== 'center' || behind(e) || layer(e)) continue
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
    // The same heading twice, or a heading right above a label that says the same.
    const norm = (s) => s.trim().toLowerCase().replace(/\\s+/g, ' ')
    const hs = [...main.querySelectorAll('h1, h2, h3')].filter((h) => vis(h) && !layer(h)).map((h) => norm(h.textContent)).filter(Boolean)
    const dup = hs.filter((h, i) => hs.indexOf(h) !== i)
    if (dup.length) add('repeated', null, 'headings said twice: ' + [...new Set(dup)].join(', '))
    for (const h of main.querySelectorAll('h1, h2, h3, legend')) {
      if (!vis(h) || layer(h)) continue
      const n = h.nextElementSibling
      const label = n && (n.matches('label, legend, h2, h3, h4, .label') ? n : n.querySelector(':scope > label:first-child, :scope > legend:first-child'))
      if (label && vis(label) && norm(label.textContent) && norm(label.textContent) === norm(h.textContent)) add('repeated', label, 'says the same as the heading above it')
    }
    // In TV mode: everything a remote can land on is at least 48 by 48.
    if (${JSON.stringify(!!tv)}) {
      let n = 0
      for (const e of document.querySelectorAll('button, a[href], [role=button], [role=tab], [role=option], input:not([type=hidden]), select, [tabindex="0"]')) {
        if (!vis(e) || e.closest('p') || e.disabled) continue
        const r = e.getBoundingClientRect()
        if (r.width < 47.5 || r.height < 47.5) { add('tv-target', e, Math.round(r.width) + ' x ' + Math.round(r.height)); if (++n >= 5) break }
      }
    }
  }
  if (bar && ${JSON.stringify(phase)} === 'scrolled') {
    for (const e of main.querySelectorAll('*')) {
      if (!vis(e) || getComputedStyle(e).position !== 'sticky' || layer(e)) continue
      const r = e.getBoundingClientRect()
      // Only while it is stuck (at its own top): once what holds it scrolls away it goes with it, as sticky parts do.
      const stuck = Math.abs(r.top - (parseFloat(getComputedStyle(e).top) || 0)) < 2
      if (stuck && r.top < barBottom - 1 && r.bottom > barBottom && r.top > -r.height) add('under-bar', e, 'stays at ' + Math.round(r.top) + ', the bar ends at ' + Math.round(barBottom))
    }
  }
  return out
})()`

// Sections of the page: each, brought into view as a link to it would, must land below the bar and anything stuck
// under it. Returns the findings.
const ANCHORS = `(async () => {
  const out = []
  const vis = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 1 && r.height > 1 && s.visibility !== 'hidden' && s.display !== 'none' }
  const main = document.querySelector('main'); if (!main) return out
  const targets = [...main.querySelectorAll('[id]')].filter((e) => vis(e) && /^(H[1-4]|SECTION|DIV|DETAILS|FIELDSET|ARTICLE)$/.test(e.tagName) && e.getBoundingClientRect().top + scrollY > innerHeight * 0.6).slice(0, 3)
  for (const t of targets) {
    location.hash = '#' + t.id
    await new Promise((d) => setTimeout(d, 350))
    let cover = 0, by = ''
    for (const e of document.querySelectorAll('header.topbar, main *')) {
      if (!vis(e)) continue
      const s = getComputedStyle(e).position
      if (s !== 'fixed' && s !== 'sticky') continue
      const r = e.getBoundingClientRect()
      if (r.top <= 2 + cover && r.bottom > cover && r.width > innerWidth * 0.3 && r.height < innerHeight / 3 && !e.contains(t)) { cover = r.bottom; by = String(e.className).slice(0, 40) }
    }
    const top = Math.round(t.getBoundingClientRect().top)
    const room = document.documentElement.scrollHeight - innerHeight - scrollY
    if (top < cover - 1 && room > 1) out.push({ kind: 'anchor', where: '#' + t.id, text: (t.textContent || '').trim().slice(0, 40), detail: 'section at ' + top + ', covered to ' + Math.round(cover) + ' by ' + by })
  }
  history.replaceState(null, '', location.pathname + location.search)
  return out
})()`

// Tab through the first stops: each must draw a focus ring (an outline, a ring of shadow, or a change of border).
const FOCUS = `(() => {
  const e = document.activeElement
  if (!e || e === document.body) return null
  const s = getComputedStyle(e), a = getComputedStyle(e, '::after'), b = getComputedStyle(e, '::before')
  const ring = (st) => (st.outlineStyle !== 'none' && parseFloat(st.outlineWidth) > 0) || (st.boxShadow && st.boxShadow !== 'none')
  // A card draws its ring round its picture, not round itself (styles.css, .card:focus-visible .poster).
  const inner = [...e.querySelectorAll(':scope > *, :scope > * > *')].slice(0, 12).some((c) => { const cs = getComputedStyle(c); return ring(cs) || (cs.transform !== 'none' && c.getBoundingClientRect().width > 8) })
  const ok = ring(s) || ring(a) || ring(b) || s.transform !== 'none' || inner
  const r = e.getBoundingClientRect()
  return { ok, where: (e.tagName.toLowerCase() + (typeof e.className === 'string' && e.className ? '.' + e.className.trim().split(/\\s+/).slice(0, 2).join('.') : '')).slice(0, 60), text: (e.getAttribute('aria-label') || e.textContent || '').trim().slice(0, 40), seen: r.width > 0 && r.height > 0 }
})()`

let token = process.env.WMS_TOKEN_FILE ? readFileSync(process.env.WMS_TOKEN_FILE, 'utf8').trim() : (process.env.WMS_TOKEN || '')
if (!token) {
  const r = await fetch(server.replace('://localhost', '://127.0.0.1').replace(/:\/\/[^/:]+\.localhost/, '://127.0.0.1') + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: process.env.WMS_USER || 'owner', password: process.env.WMS_PASSWORD }) }).catch((e) => ({ json: () => ({ error: String(e) }) }))
  const j = await r.json()
  token = j.token
  if (!token) { console.error('FAIL sign-in', j); process.exit(1) }
}

// The libraries, and one title in each, found in the running app.
async function discover(s) {
  await s.go('/libraries', 1500)
  const libs = await s.page.eval(`[...new Set([...document.querySelectorAll('a[href*="/library/"]')].map((a) => new URL(a.href).pathname))]`).catch(() => [])
  const titles = []
  const kinds = new Set()
  for (const lib of libs) {
    await s.go(lib, 1500)
    await s.page.until(`!!document.querySelector('main a[href*="/item/"], main a[href*="/show/"], main a[href*="/album/"], main a[href*="/artist/"], main a[href*="/read/"], main a[href*="/creator/"]')`, 15000, 'a title').catch(() => {})
    const found = await s.page.eval(`[...document.querySelectorAll('main a[href]')].map((a) => new URL(a.href).pathname).filter((p) => /^\\/(item|show|album|artist|creator|person)\\//.test(p))`).catch(() => [])
    for (const p of found) { const k = p.split('/')[1]; if (!kinds.has(k + lib)) { kinds.add(k + lib); titles.push(p); break } }
  }
  return [...libs, ...titles]
}

const NAVS = (process.env.LAYOUT_NAV || '').split(',').filter(Boolean)
const findings = []
const slug = (p) => (p === '/' ? 'home' : p.replace(/^\//, '').replace(/[/?=&#]+/g, '-')).slice(0, 60)
let routes = process.env.LAYOUT_ROUTES ? process.env.LAYOUT_ROUTES.split(',') : null
const tab = (page) => page.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9 }).then(() => page.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9 }))

async function check(s, route, sizeName, size, signedOut) {
  const path = size.tv ? route + (route.includes('?') ? '&' : '?') + 'tv=1&client=tv' : route
  await s.go(path, 1200)
  await s.page.until(`!!document.querySelector('main h1, main .row, main .grid, .tv-root, .standalone, .sa-card, form, .login')`, 20000, 'the page').catch(() => {})
  await wait(1500)
  const tag = (signedOut ? 'out-' : '') + slug(route)
  const found = await s.page.eval(DETECT('open', size.tv))
  if (!noShots) await s.shot(`${tag}--${sizeName}`)
  // Focus rings: the first five stops.
  if (!size.mobile) {
    await s.page.eval(`document.activeElement?.blur(); scrollTo({ top: 0, behavior: 'instant' })`)
    const seen = new Set()
    for (let i = 0; i < 5; i++) {
      await tab(s.page); await wait(120)
      const f = await s.page.eval(FOCUS)
      if (!f || !f.seen || seen.has(f.where + f.text)) continue
      seen.add(f.where + f.text)
      if (!f.ok) found.push({ kind: 'focus', where: f.where, text: f.text, detail: 'no focus ring' })
    }
    // A box with overflow hidden can't be scrolled by a person, but focus scrolls it: what it holds slides out of
    // place, under the bar or off its own edge, with no way back (Live TV's dock did this, #679).
    const shifted = await s.page.eval(`[...document.querySelectorAll('body *')].filter((e) => (e.scrollTop > 0 || e.scrollLeft > 0) && /hidden/.test(getComputedStyle(e).overflowY + getComputedStyle(e).overflowX) && !/(auto|scroll)/.test(getComputedStyle(e).overflowY + getComputedStyle(e).overflowX)).slice(0, 3).map((e) => ({ where: (e.tagName.toLowerCase() + (typeof e.className === 'string' && e.className ? '.' + e.className.trim().split(/\s+/).slice(0, 2).join('.') : '')).slice(0, 60), by: e.scrollTop + ',' + e.scrollLeft }))`)
    for (const x of shifted) found.push({ kind: 'focus-scrolled', where: x.where, text: '', detail: 'Tab scrolled a box with overflow hidden by ' + x.by })
    await s.page.eval(`document.activeElement?.blur()`)
  }
  await s.page.eval(`scrollTo({ top: Math.min(700, document.documentElement.scrollHeight - innerHeight), behavior: 'instant' })`)
  await wait(400)
  found.push(...await s.page.eval(DETECT('scrolled', size.tv)))
  if (!noShots && found.some((f) => f.kind === 'under-bar')) await s.shot(`${tag}--${sizeName}--scrolled`)
  await s.page.eval(`scrollTo({ top: 0, behavior: 'instant' })`)
  found.push(...await s.page.eval(ANCHORS).catch(() => []))
  // The same page under each other top-bar style: the bar measures itself (--bar-space) when its style changes.
  if (!size.mobile && !size.tv && !signedOut) for (const nav of NAVS) {
    const had = await s.page.eval(`document.querySelector('.app')?.dataset.navStyle || ''`)
    await s.page.eval(`scrollTo({ top: 0, behavior: 'instant' }); (() => { const a = document.querySelector('.app'); if (a) a.dataset.navStyle = ${JSON.stringify(nav)} })()`)
    await wait(500)
    const more = [...await s.page.eval(DETECT('open', false)), ...await s.page.eval(`scrollTo({ top: Math.min(700, document.documentElement.scrollHeight - innerHeight), behavior: 'instant' })`).then(() => wait(400)).then(() => s.page.eval(DETECT('scrolled', false)))]
    for (const f of more) found.push({ ...f, nav })
    if (!noShots && more.length) await s.shot(`${tag}--${sizeName}--${nav}`)
    // Back to the style the page had.
    await s.page.eval(`scrollTo({ top: 0, behavior: 'instant' }); (() => { const a = document.querySelector('.app'); if (!a) return; if (${JSON.stringify(had)}) a.dataset.navStyle = ${JSON.stringify(had)}; else delete a.dataset.navStyle })()`)
    await wait(300)
  }
  for (const f of found) findings.push({ route: (signedOut ? '(signed out) ' : '') + route, size: sizeName, ...f })
  console.log(found.length ? 'FAIL' : 'PASS', (signedOut ? '(signed out) ' : '') + route, sizeName, found.length ? JSON.stringify(found) : '')
}

for (const sizeName of sizes) {
  const size = SIZES[sizeName]
  if (!size) { console.error('Unknown size', sizeName); continue }
  const s = await open(server, out, { width: size.width, height: size.height, path: '/legal' })
  try {
    if (size.mobile) await s.size(size.width, size.height, true, 2)
    if (!process.env.LAYOUT_ONLY_SIGNED_OUT) {
      await s.page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.tvclient')`)
      if (!routes) routes = [...FIXED, ...await discover(s)]
      for (const route of routes) {
        if (size.tv && TV_SKIP.has(route)) continue
        await check(s, route, sizeName, size, false)
      }
    }
    if (!process.env.LAYOUT_ROUTES) {
      await s.page.eval(`localStorage.removeItem('wms.token'); sessionStorage.clear()`)
      await s.page.send('Network.clearBrowserCookies').catch(() => {})
      for (const route of SIGNED_OUT) await check(s, route, sizeName, size, true)
    }
  } finally { await s.browser.close() }
}

writeFileSync(join(out, 'layout-pages-findings.json'), JSON.stringify(findings, null, 1))
const by = {}
for (const f of findings) by[f.kind] = (by[f.kind] || 0) + 1
console.log(findings.length ? `FAILED: ${findings.length} findings ${JSON.stringify(by)}` : 'All page layout checks passed')
process.exit(findings.length && !process.env.LAYOUT_REPORT ? 1 : 0)
