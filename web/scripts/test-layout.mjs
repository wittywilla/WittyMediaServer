// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The layout check for Settings: You, the settings menu, every page it opens, every other core and administrator
// settings page, and the settings pages modules add, on a computer, a laptop, a tablet, a phone and in TV mode, in any
// of the layouts (each draws its own top bar), against a real server, in a headless browser (no window opens). The
// rest of the app has its own check (test-layout-pages.mjs). It fails when:
//   under-bar    something on the page is drawn under the top bar when it opens, or a part that stays put as the page
//                scrolls (a sticky column, a strip of links) stops under the bar instead of below it
//   anchor       a link to a section of a page (#licenses) scrolls the section under the bar or the strip of links
//   align        a Settings page's title and lead don't start where its content starts (docs/DESIGN.md, Centring)
//   off-centre   centred words in a box that isn't in the middle of what holds it, so they only look centred to it
//   title-case   a Settings page's title in Title Case; the menu and every title are in sentence case
//   repeated     two headings on one page that say the same thing
//   no-menu      a page the settings menu opens without the menu: on a computer, laptop or tablet the menu is beside
//                every Settings page; on a phone the page has Back to it. A row that leaves Settings fails here (#763).
// Pictures of each page go to the out folder (<page>--<size>[--<layout>].png) for before-and-after comparisons.
//
//   WMS_SERVER=http://layout.localhost:19706 WMS_USER=owner WMS_PASSWORD=... node scripts/test-layout.mjs out-folder
//   WMS_TOKEN_FILE=<file with a sign-in token> instead of a name and password. LAYOUT_SIZES=desktop,phone and
//   LAYOUT_ROUTES=/you,/settings/remote narrow it; LAYOUT_NO_SHOTS=1 skips the pictures. LAYOUT_LOOKS=aero,classic runs
//   each page in each of those layouts (chosen for this profile on every form factor, then put back to the server's);
//   LAYOUT_LOOKS=all is every layout the server offers. LAYOUT_LOOK_ROUTES narrows the pages for the extra layouts.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { open, wait } from './shots-lib.mjs'

// One browser per size and layout, each adding its own exit handlers.
process.setMaxListeners(0)
const server = (process.env.WMS_SERVER || '').replace(/\/$/, '')
const [out = 'layout'] = process.argv.slice(2)
if (!server) { console.error('Set WMS_SERVER.'); process.exit(2) }
mkdirSync(out, { recursive: true })
const noShots = !!process.env.LAYOUT_NO_SHOTS

// Every page the settings menu opens, as the web's menu links it (found as the test runs: the rows of settingsmenu.json,
// drawn inside Settings, and the pages modules add), and every settings page of the server's own (SettingsPage.tsx).
const menu = JSON.parse(readFileSync(new URL('../../pkg/sdui/data/settingsmenu.json', import.meta.url), 'utf8'))
const CORE = ['libraries', 'pages', 'metadata', 'playback', 'quality', 'channels', 'transcoding', 'library-transcoding', 'appearance', 'users',
  'sharing', 'logs', 'network', 'data-folder', 'remote', 'security', 'activity', 'notifications', 'diagnoser', 'repair', 'health', 'backup',
  'reset', 'about', 'legal', 'module-settings', 'modules'].map((p) => '/settings/' + p)
const fixedRoutes = process.env.LAYOUT_ROUTES ? process.env.LAYOUT_ROUTES.split(',') : ['/you', '/settings', ...CORE]
// Pages a link opens at one of its sections: the section must land below the bar.
const ANCHORS = [...new Set(menu.groups.flatMap((g) => g.rows.map((r) => r.path)).filter((p) => p && p.includes('#')))]
const SIZES = {
  desktop: { width: 1920, height: 1080 },
  laptop: { width: 1280, height: 800 },
  tablet: { width: 768, height: 1024, mobile: true },
  phone: { width: 390, height: 844, mobile: true },
  tv: { width: 1920, height: 1080, tv: true },
}
const sizes = (process.env.LAYOUT_SIZES || 'desktop,laptop,tablet,phone,tv').split(',')
// TV mode opens every settings page too: a TV can change some, and the rest say what is set and where to change it.
// LAYOUT_TV_ROUTES narrows them.
const TV_ROUTES = process.env.LAYOUT_TV_ROUTES ? process.env.LAYOUT_TV_ROUTES.split(',') : null

// Words in a title that keep their capital: names of things, not Title Case.
const NAMES = /^(Witty\w*|TV|TVs|HTTPS?|VPN|IPTV|DVR|DVD|DVDs|Blu-ray|QR|PIN|ID|IDs|Wi-Fi|Android|Apple|Linux|Windows|macOS|FreeBSD|Docker|FFmpeg|GPU|CPU|HDR|EPG|M3U|XMLTV|API|URL|OK|I)$/

// Runs in the page. `phase` is 'open' (as the page first appears) or 'scrolled'. `menu` is what a settings page must
// have to get back to the menu: 'side' (the menu beside it), 'back' (a phone's Back to it) or null (not a settings page).
const DETECT = (phase, menu = null) => `(() => {
  const out = []
  const vw = innerWidth
  const vis = (e) => { const r = e.getBoundingClientRect(); const s = getComputedStyle(e); return r.width > 1 && r.height > 1 && s.visibility !== 'hidden' && s.display !== 'none' && +s.opacity > 0.05 }
  const name = (e) => (e.tagName.toLowerCase() + (e.id ? '#' + e.id : '') + (typeof e.className === 'string' && e.className ? '.' + e.className.trim().split(/\\s+/).slice(0, 3).join('.') : '')).slice(0, 80)
  const txt = (e) => (e.getAttribute('aria-label') || e.textContent || '').trim().replace(/\\s+/g, ' ').slice(0, 50)
  const add = (kind, e, detail) => out.push({ kind, where: e ? name(e) : '', text: e ? txt(e) : '', detail })
  const pinned = (e) => { for (let p = e; p && p !== document.body; p = p.parentElement) { const s = getComputedStyle(p).position; if (s === 'fixed' || s === 'sticky') return p } return null }
  const main = document.querySelector('main') || document.body
  // The top bar: whatever the layout pins across the top of the window outside the page (the shell's bar, a floating
  // glass bar and its margin, tabs, a cable box's bar). Its bottom is where the page may start.
  const bars = [...document.querySelectorAll('body *')].filter((e) => {
    if (main.contains(e) || !vis(e)) return false
    const p = getComputedStyle(e).position; if (p !== 'fixed' && p !== 'sticky') return false
    const r = e.getBoundingClientRect(); return r.top <= 24 && r.bottom > 0 && r.width > vw * 0.4 && r.height < 220
  })
  const bar = bars[0]
  const barBottom = bars.reduce((m, b) => Math.max(m, b.getBoundingClientRect().bottom), 0)
  // Pages drawn over a backdrop run under the bar on purpose (docs/DESIGN.md): their backdrop, not their words.
  const behind = (e) => e.closest('.detail-hero, .hero, .hero-art, .backdrop, [data-under-bar]')
  // What a person reads or presses: words, pictures, controls.
  const content = [...main.querySelectorAll('h1, h2, h3, p, img, button, a, input, select, label, .avatar-tile, [role=button]')].filter((e) => vis(e) && !bars.some((b) => b.contains(e)))
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
      if (r.top < barBottom - 1 && r.bottom > barBottom && r.top > -r.height) add('under-bar', e, 'stays at ' + Math.round(r.top) + ', the bar (' + name(bar) + ') ends at ' + Math.round(barBottom))
    }
  }
  if (${JSON.stringify(menu)} && ${JSON.stringify(phase)} === 'open') {
    const shell = document.querySelector('.set-shell'), nav = shell && shell.querySelector('.set-nav')
    // A phone's Back to the menu: the page's own, or the top bar's, which takes its place there (shell/TopBar.tsx TopBack).
    const back = shell && [...document.querySelectorAll('.set-back a, .set-back button, .top-back')].find(vis)
    if (!shell) add('no-menu', null, 'not drawn inside Settings: no settings menu, no way back to it')
    else if (${JSON.stringify(menu)} === 'side' && !(nav && vis(nav))) add('no-menu', nav, 'the settings menu is hidden beside this page')
    else if (${JSON.stringify(menu)} === 'back' && !(back && vis(back))) add('no-menu', null, 'no Back to the settings menu')
  }
  if (${JSON.stringify(phase)} === 'open') {
    // A Settings page: its title, lead and content start at the same edge. A page that draws its own title (a module's
    // section) is held to the same: its title's box is its head.
    const ownH1 = [...document.querySelectorAll('.set-main h1')].find(vis)
    const head = document.querySelector('.set-main .set-head') || (ownH1 && (ownH1.closest('header') || (ownH1.parentElement !== document.querySelector('.set-main') && ownH1.parentElement) || ownH1))
    if (head && vis(head)) {
      const h1 = head.matches('h1') ? head : head.querySelector('h1')
      const textLeft = (e) => { const rg = document.createRange(); rg.selectNodeContents(e); const rs = [...rg.getClientRects()].filter((r) => r.width > 0); return rs.length ? Math.min(...rs.map((r) => r.left)) : e.getBoundingClientRect().left }
      const textRight = (e) => { const rg = document.createRange(); rg.selectNodeContents(e); const rs = [...rg.getClientRects()].filter((r) => r.width > 0); return rs.length ? Math.max(...rs.map((r) => r.right)) : e.getBoundingClientRect().right }
      const rtl = getComputedStyle(head).direction === 'rtl'
      const edge = (e) => rtl ? textRight(e) : textLeft(e)
      const blocks = [...head.parentElement.children].filter((c) => c !== head && vis(c) && !c.matches('.set-help, .set-back, .set-toc, .set-notice, .set-notice-row, script, style') && c.getBoundingClientRect().top > head.getBoundingClientRect().bottom - 1)
      const first = blocks[0]
      if (h1 && first) {
        // A bar that runs past the column on both sides (negative margins) starts its words at its padding.
        const fs = getComputedStyle(first), fr = first.getBoundingClientRect()
        const bleed = parseFloat(rtl ? fs.marginRight : fs.marginLeft) < 0
        const want = rtl ? fr.right - (bleed ? parseFloat(fs.paddingRight) : 0) : fr.left + (bleed ? parseFloat(fs.paddingLeft) : 0)
        // A head that starts with a picture before its title (the module store's icon) starts at the picture.
        const pic = head.firstElementChild && !head.firstElementChild.contains(h1) && vis(head.firstElementChild) && head.firstElementChild.getBoundingClientRect().right <= h1.getBoundingClientRect().left + 1 ? head.firstElementChild : null
        for (const e of pic ? [pic] : [h1, head.matches('h1') ? null : head.querySelector('p')].filter(Boolean)) {
          const d = Math.round((e === pic ? (rtl ? e.getBoundingClientRect().right : e.getBoundingClientRect().left) : edge(e)) - want)
          if (Math.abs(d) > 2) add('align', e, 'starts ' + d + 'px from the content under it (' + name(first) + ')')
        }
      }
      if (h1 && getComputedStyle(h1).textAlign === 'center' && !document.documentElement.classList.contains('tv-mode')) add('align', h1, 'a Settings page title in the middle, not at the edge its content starts at')
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
    // The page's own, not the menu's group names beside it.
    const hs = [...main.querySelectorAll('h1, h2, h3')].filter((h) => vis(h) && !h.closest('nav')).map((h) => h.textContent.trim().toLowerCase().replace(/\\s+/g, ' ')).filter(Boolean)
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
  for (const e of document.querySelectorAll('body *')) {
    if (!vis(e)) continue
    const s = getComputedStyle(e).position
    if (s !== 'fixed' && s !== 'sticky') continue
    const r = e.getBoundingClientRect()
    if (r.top <= 24 + cover && r.bottom > cover && r.width > innerWidth * 0.3 && r.height < innerHeight / 3 && !e.contains(t)) { cover = r.bottom; by = e.className }
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

const api = async (method, path, body) => {
  const r = await fetch(server.replace('://localhost', '://127.0.0.1').replace(/:\/\/[a-z0-9-]+\.localhost/, '://127.0.0.1') + '/api' + path, { method, headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: body ? JSON.stringify(body) : undefined })
  return { status: r.status, body: await r.json().catch(() => null) }
}

// The pages the settings menu opens: every link it draws, on the Settings page and beside a settings page (a module's
// own tab, its page, a row whose page has an address elsewhere). Each must open inside Settings (no-menu).
let ROUTES = fixedRoutes
if (!process.env.LAYOUT_ROUTES) {
  const s = await open(server, out, { width: 1920, height: 1080, path: '/legal' })
  try {
    await s.page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.tvclient')`)
    const links = []
    for (const [path, sel] of [['/settings', 'main a.sm-row[href]'], ['/settings/about', '.set-nav a[href]'], ['/settings/modules', '.set-main a[href^="/settings/modules/store/"]']]) {
      await s.go(path, 400)
      await s.page.until(`document.querySelectorAll(${JSON.stringify(sel)}).length > (${JSON.stringify(path)} === '/settings/modules' ? 0 : 10)`, 20000, 'the settings menu').catch(() => {})
      await wait(800)
      links.push(...await s.page.eval(`[...document.querySelectorAll(${JSON.stringify(sel)})].map((a) => new URL(a.href).pathname)`))
    }
    const extra = [...new Set(links)].filter((p) => p !== '/' && !fixedRoutes.includes(p))
    console.log('the menu opens', extra.join(' '))
    ROUTES = [...fixedRoutes, ...extra]
  } finally { await s.browser.close() }
}

// The layouts to run in. Each is chosen for this profile on every form factor, as Settings, Appearance does, and the
// server's choice is put back at the end.
const FORMS = ['web', 'desktop', 'tablet', 'phone', 'tv']
let looks = (process.env.LAYOUT_LOOKS || '').split(',').filter(Boolean)
if (looks[0] === 'all') looks = ((await api('GET', '/sdui/experience')).body?.choice?.options || []).map((o) => o.value)
const lookRoutes = process.env.LAYOUT_LOOK_ROUTES ? process.env.LAYOUT_LOOK_ROUTES.split(',') : null
const choose = async (look) => {
  for (const ff of FORMS) {
    const q = `?platform=web&formFactor=${ff}`
    const r = look ? await api('PUT', '/sdui/experience' + q, { experience: look }) : await api('DELETE', '/sdui/experience' + q)
    if (r.status >= 300) console.log('could not choose', look || 'the server\'s layout', 'for', ff, r.status, JSON.stringify(r.body))
  }
}

const findings = []
const slug = (p) => (p === '/' ? 'home' : p.replace(/^\//, '').replace(/[/?=&#]+/g, '-'))
try {
  for (const look of [null, ...looks]) {
    if (look) await choose(look)
    const tag = look ? `--${look}` : ''
    const lookName = look || 'default'
    for (const sizeName of sizes) {
      const size = SIZES[sizeName]
      if (!size) { console.error('Unknown size', sizeName); continue }
      const s = await open(server, out, { width: size.width, height: size.height, path: '/legal' })
      try {
        if (size.mobile) await s.size(size.width, size.height, true, 2)
        await s.page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)}); localStorage.removeItem('wms.tvclient')`)
        const all = look && lookRoutes ? lookRoutes : ROUTES
        const routes = size.tv && TV_ROUTES ? all.filter((r) => TV_ROUTES.includes(r)) : all
        const seen = new Set()
        // Where each settings page's title starts: every one at the same place, so moving between them nothing jumps.
        const edges = []
        for (const route of routes) {
          const path = size.tv ? route + (route.includes('?') ? '&' : '?') + 'tv=1&client=tv' : route
          await s.go(path, 400)
          // Ready: the page has its title (or the settings groups), and a moment for pictures and late parts.
          await s.page.until(`!!document.querySelector('main h1, main [data-settings-group], main .row, main .grid, .tv-root')`, 20000, 'the page').catch(() => {})
          await wait(900)
          const here = await s.page.eval(`location.pathname`)
          // A settings page has the menu beside it, or on a phone Back to it; one that sends you out of Settings fails.
          const menuWant = size.tv || !route.startsWith('/settings/') ? null : sizeName === 'phone' ? 'back' : 'side'
          if (menuWant && here !== '/settings' && !here.startsWith('/settings/')) {
            findings.push({ route, size: sizeName, look: lookName, kind: 'no-menu', where: '', text: '', detail: 'opens ' + here + ', outside Settings, without the settings menu' })
            if (!noShots) await s.shot(`${slug(route)}--${sizeName}${tag}`)
            console.log('FAIL', route, sizeName, lookName, 'opens', here, 'outside Settings')
            continue
          }
          // A page that sends you on (a module's settings tab opening its own settings pages) is checked where it lands,
          // once; one that lands on a page checked anyway is skipped.
          if (!here.startsWith(route.split('?')[0]) && (seen.has(here) || all.includes(here))) { console.log('SKIP', route, sizeName, lookName, 'went to', here); continue }
          seen.add(here)
          const found = await s.page.eval(DETECT('open', here === '/settings' ? null : menuWant))
          if (menuWant && here !== '/settings') {
            // The page editor is a wide workspace of its own (pages-editor.css .pe-wide): the whole frame widens for it.
            const left = await s.page.eval(`(() => { if (document.querySelector('.pe-wide')) return null; const h = [...document.querySelectorAll('.set-main h1')].find((e) => e.getBoundingClientRect().width > 1); if (!h) return null; const hd = h.closest('header'); const f = hd && hd.firstElementChild; const pic = f && !f.contains(h) && f.getBoundingClientRect().width > 1 && f.getBoundingClientRect().right <= h.getBoundingClientRect().left + 1; return Math.round((pic ? f : h).getBoundingClientRect().left) })()`)
            if (left != null) edges.push({ route, left })
          }
          if (!noShots) await s.shot(`${slug(route)}--${sizeName}${tag}`)
          await s.page.eval(`scrollTo(0, Math.min(700, document.documentElement.scrollHeight - innerHeight))`)
          await wait(300)
          found.push(...await s.page.eval(DETECT('scrolled')))
          if (!noShots && found.some((f) => f.kind === 'under-bar')) await s.shot(`${slug(route)}--${sizeName}${tag}--scrolled`)
          for (const f of found) findings.push({ route, size: sizeName, look: lookName, ...f })
          console.log(found.length ? 'FAIL' : 'PASS', route, sizeName, lookName, found.length ? JSON.stringify(found) : '')
        }
        if (edges.length > 2) {
          const count = new Map(); for (const e of edges) count.set(e.left, (count.get(e.left) || 0) + 1)
          const usual = [...count.entries()].sort((a, b) => b[1] - a[1])[0][0]
          for (const e of edges) if (Math.abs(e.left - usual) > 2) {
            findings.push({ route: e.route, size: sizeName, look: lookName, kind: 'align', where: 'h1', text: '', detail: `the title starts at ${e.left}, other settings pages' at ${usual}: it moves going from one to the other` })
            console.log('FAIL', e.route, sizeName, lookName, 'title at', e.left, 'not', usual)
          }
        }
        if (!size.tv && !(look && lookRoutes)) {
          for (const link of ANCHORS) {
            const [route, id] = link.split('#')
            await s.go(route, 400)
            await s.page.until(`!!document.getElementById(${JSON.stringify(id)})`, 15000, id).catch(() => {})
            await wait(500)
            await s.page.eval(`location.hash = ${JSON.stringify('#' + id)}`)
            // The page scrolls to the section once it's drawn and holds it there while the page settles (settings/jump.ts).
            // Judged once it has stayed put for a second near the top of the window, or for three seconds anywhere (a long
            // page still drawing, Appearance on a phone, can start its jump after a second's stillness), for up to fifteen.
            await s.page.eval(`new Promise((done) => { let last = NaN, still = 0, n = 0; const t = setInterval(() => { const e = document.getElementById(${JSON.stringify(id)}); const y = e ? Math.round(e.getBoundingClientRect().top) : NaN; still = y === last ? still + 1 : 0; last = y; const near = y >= 0 && y < innerHeight / 3; if ((near && still >= 5) || still >= 15 || ++n > 75) { clearInterval(t); done() } }, 200) })`)
            const a = await s.page.eval(ANCHOR_CHECK(id))
            if (a.missing) { console.log('SKIP', link, sizeName, lookName, 'no such section here'); continue }
            // Below the bar, and in view: a section left far down the page means the link didn't take you there.
            const ok = a.top >= a.cover - 1 && a.top < a.height
            if (!ok) { findings.push({ route: link, size: sizeName, look: lookName, kind: 'anchor', where: '#' + id, detail: `section at ${a.top}, covered to ${a.cover} by ${a.by}` }); if (!noShots) await s.shot(`${slug(link)}--${sizeName}${tag}`) }
            console.log(ok ? 'PASS' : 'FAIL', link, sizeName, lookName, JSON.stringify(a))
          }
        }
      } finally { await s.browser.close() }
    }
  }
} finally {
  if (looks.length) await choose(null)
}

writeFileSync(join(out, 'layout-findings.json'), JSON.stringify(findings, null, 1))
const by = {}
for (const f of findings) by[f.kind] = (by[f.kind] || 0) + 1
console.log(findings.length ? `FAILED: ${findings.length} findings ${JSON.stringify(by)}` : 'All layout checks passed')
process.exit(findings.length ? 1 : 0)
