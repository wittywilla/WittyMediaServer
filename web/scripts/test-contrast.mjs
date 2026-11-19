// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// How readable the words are, measured from what is drawn: every bundled theme in every look, in a
// headless browser on a running test server. No window opens.
//
// For each page it takes a picture with every word made see-through, so the picture shows exactly
// what each line of text sits on (the scene, glass, gradients, a picture), then compares each line's
// colour with the pixels under it. Body text needs 4.5:1 and large titles (24px, or 18.7px bold)
// 3:1 (WCAG 2.2 AA). The 5th-worst percent of the pixels under a line counts, so a stray border or
// the edge of an icon doesn't fail a line, but a light patch of a picture does.
//
// A picture can be anything, so the words over the Home billboard and a title's header are measured
// over the worst pictures there are: white, black and a fine black-and-white checker in place of the
// backdrop. That's the guarantee they read over any picture. Other words over a picture (a card's
// title on its poster) are listed as notes, not failures, since they depend on the picture.
//
// Sign in details come from the environment. It changes that account's look and this browser's
// theme, so use a test server with at least one film or show.
//
//   WMS_SERVER=http://contrast.localhost:19513 WMS_USER=tester WMS_PASSWORD=... node scripts/test-contrast.mjs out-folder
//
// WMS_THEMES=aerosunset,midnight and WMS_LOOKS=aero,classic run only those. WMS_PAGES=home,detail,settings,tv
// picks the pages (all by default; settings and TV mode in each theme's first look only, as they
// don't change much between looks). WMS_DETAIL=/item/123 picks the title page (else the billboard's first). WMS_SHOTS=1 also saves a picture of each page. It writes
// contrast.json and contrast.md (the report) to the folder given, and exits 1 if anything fails.
// node scripts/test-contrast.mjs --report out part1 part2 makes one report of runs made in parts.
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { launch } from './cdp.mjs'

const server = process.env.WMS_SERVER || 'http://contrast.localhost:19513'
const [out = 'contrast-report', ...parts] = process.argv.slice(2).filter((a) => a !== '--report')
mkdirSync(out, { recursive: true })
// --report out part1 part2...: one report from runs made in parts (WMS_THEMES split over several browsers at once).
if (process.argv.includes('--report')) {
  const runs = parts.map((p) => JSON.parse(readFileSync(join(p, 'contrast.json'), 'utf8')))
  const all = runs.flatMap((r) => r.results)
  writeFileSync(join(out, 'contrast.json'), JSON.stringify({ parts, results: all }, null, 1))
  process.exit(report(all, runs.flatMap((r) => r.themes), runs[0].looks, runs[0].pages))
}
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const list = (v) => (v ? v.split(',').map((s) => s.trim()).filter(Boolean) : null)
const PAGES = list(process.env.WMS_PAGES) || ['home', 'detail', 'settings', 'tv']
const SHOTS = process.env.WMS_SHOTS === '1'

const browser = await launch({ software: true, width: 1440, height: 900 })
const page = await browser.open(server + '/')
await page.send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 900, deviceScaleFactor: 1, mobile: false })
const call = (method, path, body) => page.eval(`(async () => { const r = await fetch('/api${path}', { method: '${method}', headers: { 'Content-Type': 'application/json', ...(localStorage.getItem('wms.token') ? { Authorization: 'Bearer ' + localStorage.getItem('wms.token') } : {}) }, body: ${body ? `JSON.stringify(${JSON.stringify(body)})` : 'undefined'} }); return r.json().catch(() => null) })()`)
const login = await call('POST', '/login', { name: process.env.WMS_USER, password: process.env.WMS_PASSWORD })
if (!login?.token) throw new Error('could not sign in: ' + JSON.stringify(login))
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)}); localStorage.setItem('wms.tvmode', '0')`)
await call('POST', '/legal/acknowledge')

const themes = (await call('GET', '/themes'))
const themeIds = (Array.isArray(themes) ? themes : themes?.themes || []).filter((t) => t.builtin !== false).map((t) => t.id)
const THEMES = list(process.env.WMS_THEMES) || themeIds
const looks = (await call('GET', '/sdui/experiences'))?.experiences?.map((x) => x.id) || []
const LOOKS = list(process.env.WMS_LOOKS) || looks
const device = () => page.eval(`localStorage.getItem('wms.sdui.device') || ''`)

// The measuring, run in the page. mode 'hide' makes every word see-through and stills the page;
// 'measure' gets the lines of text, their colours and the pixels under them from the picture.
const PAGE_FN = `(${function (mode, png, backdrop) {
  // Constructed sheets: the content security policy turns away a <style> added to the page, not these.
  const sheets = (window.__contrastSheets ||= {})
  const sheet = (name, css) => { if (!sheets[name]) { sheets[name] = new CSSStyleSheet(); sheets[name].replaceSync(css) } return sheets[name] }
  const on = (name, css) => { const s = sheet(name, css); if (!document.adoptedStyleSheets.includes(s)) document.adoptedStyleSheets = [...document.adoptedStyleSheets, s] }
  const off = (name) => { const s = sheets[name]; document.adoptedStyleSheets = document.adoptedStyleSheets.filter((x) => x !== s) }
  if (mode === 'hide') { on('hide', '*, *::before, *::after { color: transparent !important; -webkit-text-fill-color: transparent !important; text-shadow: none !important; caret-color: transparent !important; text-decoration-color: transparent !important; } ::placeholder { color: transparent !important; }'); return true }
  if (mode === 'show') { off('hide'); return true }
  if (mode === 'still') {
    // Nothing moves while it's measured: no transitions, and the billboard's arrows and pause hidden.
    // An entrance (a page fading in) is played to its end, not stopped part way; what loops (a scene's drift) is held.
    on('still', '*, *::before, *::after { transition: none !important; caret-color: transparent !important; } .hero-pause, .sdui-hero-arrow { visibility: hidden !important; }')
    for (const a of document.getAnimations()) { try { const t = a.effect?.getComputedTiming?.(); if (t && t.endTime !== Infinity) a.finish(); else a.pause() } catch { /* a scroll-driven one */ } }
    document.activeElement?.blur?.()
    return true
  }
  if (mode === 'backdrop') {
    // The worst pictures there are, in place of the billboard's and a title header's backdrop.
    const fill = { light: 'linear-gradient(#fff, #fff)', dark: 'linear-gradient(#000, #000)', busy: 'repeating-conic-gradient(#000 0 25%, #fff 0 50%) 0 0 / 6px 6px' }[backdrop]
    for (const el of document.querySelectorAll('.hero .bg, .detail-hero .bg')) {
      if (!el.dataset.realBg) el.dataset.realBg = el.style.background || ' '
      el.style.background = fill || (el.dataset.realBg === ' ' ? '' : el.dataset.realBg)
      if (!fill) delete el.dataset.realBg
    }
    for (const el of document.querySelectorAll('.detail-hero .canvas-loop, .detail-hero video, .detail-hero canvas')) el.style.visibility = fill ? 'hidden' : ''
    return true
  }
  // measure
  return (async () => {
    const img = new Image(); img.src = 'data:image/png;base64,' + png; await img.decode()
    const cv = document.createElement('canvas'); cv.width = img.width; cv.height = img.height
    const cx = cv.getContext('2d', { willReadFrequently: true }); cx.drawImage(img, 0, 0)
    const px = cx.getImageData(0, 0, cv.width, cv.height).data
    const scale = img.width / innerWidth
    const parse = (c) => {
      let m = /rgba?\(([\d.]+)[ ,]+([\d.]+)[ ,]+([\d.]+)(?:[ ,/]+([\d.]+%?))?/.exec(c)
      if (m) return [+m[1], +m[2], +m[3], m[4] === undefined ? 1 : m[4].endsWith('%') ? parseFloat(m[4]) / 100 : +m[4]]
      m = /color\(srgb ([\d.e-]+) ([\d.e-]+) ([\d.e-]+)(?: \/ ([\d.]+))?/.exec(c)
      if (m) return [m[1] * 255, m[2] * 255, m[3] * 255, m[4] === undefined ? 1 : +m[4]]
      return null
    }
    const lin = (v) => { const s = v / 255; return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4 }
    const lum = (r, g, b) => 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
    const hidden = (e) => { for (let x = e; x; x = x.parentElement) { const c = getComputedStyle(x); if (c.display === 'none' || c.visibility === 'hidden' || +c.opacity === 0) return true } return !!e.closest('[aria-hidden=true], .sr-only, script, style, noscript, option, select, [inert]') }
    const opacityOf = (e) => { let o = 1; for (let x = e; x; x = x.parentElement) o *= +getComputedStyle(x).opacity; return o }
    const path = (e) => { const out = []; for (let x = e; x && x !== document.body && out.length < 4; x = x.parentElement) { const cls = typeof x.className === 'string' ? x.className.trim().split(/\s+/).filter((c) => c && !/^(active|on|open|sdui-clamp)$/.test(c)).slice(0, 2).join('.') : ''; out.unshift(x.tagName.toLowerCase() + (cls ? '.' + cls : '')) } return out.join(' > ') }
    const pictureUnder = (r) => {
      const x = Math.min(innerWidth - 1, Math.max(0, r.left + r.width / 2)), y = Math.min(innerHeight - 1, Math.max(0, r.top + r.height / 2))
      for (const e of document.elementsFromPoint(x, y)) {
        if (e.matches('img, video, canvas, picture')) return 'image'
        if (e.matches('.hero .bg, .detail-hero .bg')) return 'backdrop'
        const bi = getComputedStyle(e).backgroundImage
        if (bi && bi.includes('url(')) return e.closest('.hero, .detail-hero') ? 'backdrop' : 'image'
      }
      return ''
    }
    const lines = []
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    const seen = new Map()
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      const text = n.textContent.replace(/\s+/g, ' ').trim()
      if (!text || !/[\p{L}\p{N}]/u.test(text)) continue
      const el = n.parentElement
      if (!el || hidden(el)) continue
      const c = getComputedStyle(el)
      const fg = parse(c.webkitTextFillColor && c.webkitTextFillColor !== c.color && !/rgba?\(0, 0, 0, 0\)/.test(c.webkitTextFillColor) ? c.webkitTextFillColor : c.color)
      if (!fg) continue
      // Text drawn as a gradient (a brand word) has no one colour.
      if (c.backgroundClip === 'text' || c.webkitBackgroundClip === 'text') continue
      fg[3] *= opacityOf(el)
      if (fg[3] <= 0.05) continue
      const size = parseFloat(c.fontSize), bold = parseInt(c.fontWeight) >= 700
      const need = size >= 24 || (size >= 18.66 && bold) ? 3 : 4.5
      // A disabled control is exempt (WCAG 1.4.3).
      if (el.closest('button:disabled, [aria-disabled=true], fieldset:disabled, input:disabled')) continue
      const range = document.createRange(); range.selectNodeContents(n)
      for (const r of range.getClientRects()) {
        if (r.width < 2 || r.height < 4 || r.bottom <= 0 || r.top >= innerHeight || r.right <= 0 || r.left >= innerWidth) continue
        // What's under the line's middle band (the glyphs' x-height), on a grid.
        const x0 = Math.max(0, r.left + 1), x1 = Math.min(innerWidth - 1, r.right - 1)
        const y0 = Math.max(0, r.top + r.height * 0.25), y1 = Math.min(innerHeight - 1, r.bottom - r.height * 0.25)
        if (x1 <= x0 || y1 <= y0) continue
        // Covered by something else (scrolled under the sticky bar, behind a menu): not what anyone reads there.
        const top = document.elementFromPoint((x0 + x1) / 2, (y0 + y1) / 2)
        if (top && !el.contains(top) && !top.contains(el)) continue
        const step = Math.max(1, Math.sqrt(((x1 - x0) * (y1 - y0)) / 600))
        const ratios = []
        let sum = [0, 0, 0], count = 0
        for (let y = y0; y <= y1; y += step) for (let x = x0; x <= x1; x += step) {
          const i = (Math.floor(y * scale) * cv.width + Math.floor(x * scale)) * 4
          const b = [px[i], px[i + 1], px[i + 2]]
          const t = [0, 1, 2].map((k) => fg[k] * fg[3] + b[k] * (1 - fg[3]))
          const lt = lum(...t), lb = lum(...b)
          ratios.push((Math.max(lt, lb) + 0.05) / (Math.min(lt, lb) + 0.05))
          sum = sum.map((s, k) => s + b[k]); count++
        }
        if (!ratios.length) continue
        ratios.sort((a, b) => a - b)
        const worst = ratios[Math.floor(ratios.length * 0.05)]
        const key = path(el) + '|' + text.slice(0, 40)
        const prev = seen.get(key)
        const row = { where: path(el), text: text.slice(0, 48), color: c.color, alpha: +fg[3].toFixed(2), size: Math.round(size), need, ratio: +worst.toFixed(2), median: +ratios[Math.floor(ratios.length / 2)].toFixed(2), under: 'rgb(' + sum.map((s) => Math.round(s / count)).join(',') + ')', picture: pictureUnder(r), y: Math.round(r.top) }
        if (!prev || row.ratio < prev.ratio) { if (prev) lines.splice(lines.indexOf(prev), 1, row); else lines.push(row); seen.set(key, row) }
      }
    }
    return lines
  })()
}})`

const fn = (mode, png = '', backdrop = '') => page.eval(`${PAGE_FN}(${JSON.stringify(mode)}, ${JSON.stringify(png)}, ${JSON.stringify(backdrop)})`)
async function measureScreen(label) {
  await fn('still')
  await wait(250)
  if (SHOTS && label) writeFileSync(join(out, label.replace(/[^\w.-]+/g, '-') + '.png'), await page.shot())
  await fn('hide')
  await wait(120)
  const png = (await page.send('Page.captureScreenshot', { format: 'png', optimizeForSpeed: true })).data
  if (process.env.WMS_DEBUG && label) writeFileSync(join(out, label.replace(/[^\w.-]+/g, '-') + '-hidden.png'), Buffer.from(png, 'base64'))
  await fn('show')
  return fn('measure', png)
}
// A page, a screen at a time down to SCREENS screens, each line kept at its worst. Pictures only for the first.
const SCREENS = +(process.env.WMS_SCREENS || 3)
async function measure(label, screens = SCREENS) {
  const all = new Map()
  for (let k = 0; k < screens; k++) {
    if (k) {
      const moved = await page.eval(`(() => { const y = scrollY; scrollTo(0, y + Math.round(innerHeight * 0.75)); return scrollY !== y })()`)
      if (!moved) break
      await wait(350)
    }
    for (const l of await measureScreen(k ? '' : label)) {
      const key = l.where + '|' + l.text
      if (!all.has(key) || l.ratio < all.get(key).ratio) all.set(key, l)
    }
  }
  if (screens > 1) { await page.eval('scrollTo(0, 0)'); await wait(200) }
  return [...all.values()]
}

// Settles a freshly loaded page: the theme's sheet in, the words drawn, pictures loaded.
async function settle(selector, what) {
  // Drawn and showing words, not a skeleton waiting for the server.
  await page.until(`[...document.querySelectorAll(${JSON.stringify(selector)})].some((e) => e.getClientRects().length && e.textContent.trim() && !e.closest('.skel, [aria-busy=true]'))`, 90000, what)
  await page.until(`!document.querySelector('link[data-wms-theme][data-state=loading]')`, 20000, 'the theme')
  await page.eval(`document.fonts.ready.then(() => true)`)
  // The billboard's and the header's picture.
  await page.eval(`Promise.race([new Promise((d) => setTimeout(d, 6000)), Promise.all([...document.querySelectorAll('.hero .bg, .detail-hero .bg')].map((e) => { const m = /url\\("?([^")]+)/.exec(e.style.backgroundImage); if (!m) return true; const i = new Image(); i.src = m[1]; return i.decode().catch(() => true) }))])`)
  await wait(700)
}

const results = [] // { theme, look, page, backdrop, lines }
let detailUrl = process.env.WMS_DETAIL ? server + process.env.WMS_DETAIL : ''
const started = Date.now()
for (const theme of THEMES) {
  for (const [li, look] of LOOKS.entries()) {
    await page.eval(`localStorage.setItem('wms.theme', ${JSON.stringify(theme)}); localStorage.setItem('wms.tvmode', '0')`)
    await call('PUT', '/sdui/experience?device=' + encodeURIComponent(await device()), { experience: look })
    const pages = PAGES.filter((p) => li === 0 || p === 'home' || p === 'detail')
    for (const name of pages) {
      const tv = name === 'tv'
      await page.eval(`localStorage.setItem('wms.tvmode', ${tv ? "'1'" : "'0'"})`)
      // A loaded machine can be slow to draw a page: one more try before it counts as not loading.
      let failed = null
      for (let attempt = 0; attempt < 2; attempt++) {
        try {
          if (name === 'home' || tv) {
            await page.go(server + '/')
            await settle('.hero .info h1, .hub-head h2, .sdui-head-text h2, .sdui h2, .sdui .card .title', 'Home')
            if (!detailUrl) detailUrl = await page.eval(`(document.querySelector('.hero .actions a[href*="/item/"], a[href*="/item/"]') || {}).href || ''`)
          } else if (name === 'detail') {
            if (!detailUrl) { await page.go(server + '/'); await settle('a[href*="/item/"]', 'a title on Home'); detailUrl = await page.eval(`document.querySelector('.hero .actions a[href*="/item/"], a[href*="/item/"]').href`) }
            await page.go(detailUrl)
            await settle('.detail h1, .sdui-detail h1', 'a title page')
          } else if (name === 'settings') {
            await page.go(server + '/settings')
            await settle('main h1, main h2, .settings h1', 'Settings')
          }
          failed = null
          break
        } catch (e) { failed = e }
      }
      if (failed) {
        results.push({ theme, look, page: name, backdrop: '', error: String(failed.message || failed), lines: [] })
        console.log(`${theme} ${look} ${name}: ${failed.message}`)
        continue
      }
      const hasBackdrop = await page.eval(`!!document.querySelector('.hero .bg, .detail-hero:not(.bare) .bg')`)
      const label = `${theme}-${look}-${name}`
      results.push({ theme, look, page: name, backdrop: 'real', lines: await measure(label) })
      if (hasBackdrop) {
        for (const backdrop of ['light', 'dark', 'busy']) {
          await fn('backdrop', '', backdrop)
          await wait(80)
          const lines = (await measure(`${label}-${backdrop}`, 1)).filter((l) => l.picture === 'backdrop' || /hero/.test(l.where))
          results.push({ theme, look, page: name, backdrop, lines })
        }
        await fn('backdrop', '', '')
      }
      const r = results.filter((x) => x.theme === theme && x.look === look && x.page === name)
      const fails = r.flatMap((x) => x.lines.filter((l) => l.ratio < l.need - 0.02 && (x.backdrop !== 'real' || !l.picture)))
      console.log(`${theme} ${look} ${name}: ${fails.length ? fails.length + ' lines short' : 'ok'}`)
    }
  }
}
await page.eval(`localStorage.setItem('wms.tvmode', '0')`)
await browser.close()
writeFileSync(join(out, 'contrast.json'), JSON.stringify({ server, when: new Date().toISOString(), seconds: Math.round((Date.now() - started) / 1000), themes: THEMES, looks: LOOKS, pages: PAGES, results }, null, 1))
process.exitCode = report(results, THEMES, LOOKS, PAGES)

// The report: failures are words on flat surfaces, and words over the billboard and header backdrops
// measured over the worst pictures. Words over other pictures are notes.
function report(results, THEMES, LOOKS, PAGES) {
  const failing = (x, l) => l.ratio < l.need - 0.02 && (x.backdrop !== 'real' || !l.picture)
  const rows = []
  for (const x of results) for (const l of x.lines) if (failing(x, l)) rows.push({ theme: x.theme, look: x.look, page: x.page, backdrop: x.backdrop, ...l })
  const notes = []
  for (const x of results) for (const l of x.lines) if (x.backdrop === 'real' && l.picture && l.ratio < l.need - 0.02) notes.push({ theme: x.theme, look: x.look, page: x.page, ...l })

  const md = [`# Text contrast, measured from the drawn page`, '', `${THEMES.length} themes × ${LOOKS.length} looks, pages ${PAGES.join(', ')}; A line fails under 4.5:1 (3:1 for large text), measured over the 5th-worst percent of the pixels under it. The billboard and title header are measured over a white, a black and a checkered picture in place of their backdrop.`, '']
  md.push('## By theme', '', '| Theme | Lines measured | Failing (flat) | Failing (over worst-case backdrop) | Worst |', '|---|---|---|---|---|')
  for (const theme of THEMES) {
    const mine = results.filter((x) => x.theme === theme)
    const n = mine.reduce((s, x) => s + x.lines.length, 0)
    const flat = rows.filter((r) => r.theme === theme && r.backdrop === 'real').length
    const over = rows.filter((r) => r.theme === theme && r.backdrop !== 'real').length
    const worst = rows.filter((r) => r.theme === theme).sort((a, b) => a.ratio / a.need - b.ratio / b.need)[0]
    md.push(`| ${theme} | ${n} | ${flat} | ${over} | ${worst ? `${worst.ratio}:1 "${worst.text.slice(0, 30)}" (${worst.look}, ${worst.page}${worst.backdrop !== 'real' ? ', ' + worst.backdrop + ' backdrop' : ''})` : '—'} |`)
  }
  const errors = results.filter((x) => x.error)
  if (errors.length) { md.push('', '## Pages that did not load', ''); for (const e of errors) md.push(`- ${e.theme} ${e.look} ${e.page}: ${e.error}`) }
  md.push('', '## Failing lines (worst first, one per theme, place and text)', '', '| Theme | Look | Page | Backdrop | Where | Text | Colour | Size | Ratio | Needs |', '|---|---|---|---|---|---|---|---|---|---|')
  const once = new Set()
  for (const r of rows.sort((a, b) => a.ratio / a.need - b.ratio / b.need)) {
    const k = r.theme + r.page + r.where + r.text + r.backdrop
    if (once.has(k)) continue
    once.add(k)
    md.push(`| ${r.theme} | ${r.look} | ${r.page} | ${r.backdrop} | \`${r.where}\` | ${r.text.replace(/\|/g, '/').slice(0, 30)} | ${r.color}${r.alpha < 1 ? ' @' + r.alpha : ''} on ${r.under} | ${r.size} | ${r.ratio} | ${r.need} |`)
  }
  if (notes.length) md.push('', `## Notes: ${notes.length} lines over other pictures (posters, thumbnails) read under the bar on today's pictures. They depend on the picture, so they don't fail the check.`)
  writeFileSync(join(out, 'contrast.md'), md.join('\n') + '\n')
  console.log(`\n${rows.length} failing lines (${new Set(rows.map((r) => r.theme)).size} themes); report in ${join(out, 'contrast.md')}`)
  return rows.length || errors.length ? 1 : 0
}
