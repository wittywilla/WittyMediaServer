// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Helpers for the scripts that take the pictures in the docs. A headless browser of its own, no
// window, always 1920 x 1080 unless a script says otherwise. See shots-setup.mjs and shots.mjs.
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(process.env.WITTYVISUALIZER ? new URL(process.env.WITTYVISUALIZER + '/tests/cdp.mjs', import.meta.url).href : './cdp.mjs')

export const wait = (ms) => new Promise((d) => setTimeout(d, ms))

// Runs in the page before every picture, and says what's wrong with it, if anything. Each kind can be allowed by a
// picture that means to show it:
//   setup    a "Finish setting up" notice left from setting up the test server
//   crash    the "stopped unexpectedly" notice an earlier test server left
//   loading  a spinner or a page still coming in
//   error    an error page or a section that couldn't load
//   overlay  a layer over most of the screen that isn't a dialog, a player or a picture, with the page showing through
//   overlap  words drawn over other words, with nothing solid between them (one page's styles leaking onto another)
//   scroll   a page longer than the screen that can't be scrolled
//   fading   a dialog still fading in
//   broken   a picture that didn't load
export function problems(allow) {
  const W = innerWidth, H = innerHeight
  // Drawn on screen now: not folded away (a closed <details> can still report where its words were), hidden or
  // see-through, and inside the window.
  const seen = (e) => {
    if (e.checkVisibility && !e.checkVisibility({ contentVisibilityAuto: true, visibilityProperty: true, opacityProperty: true })) return false
    const r = e.getBoundingClientRect(); const s = getComputedStyle(e)
    return r.width > 1 && r.height > 1 && r.bottom > 0 && r.top < H && r.right > 0 && r.left < W && s.visibility !== 'hidden' && +s.opacity > 0.05
  }
  const own = (e) => [...e.childNodes].some((n) => n.nodeType === 3 && n.nodeValue.trim().length > 1)
  // Not cut off by a box that scrolls or clips (a guide's rows past its bottom edge aren't drawn at all).
  const unclipped = (e) => {
    const r = e.getBoundingClientRect(); const x = r.left + r.width / 2, y = r.top + r.height / 2
    for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) {
      const s = getComputedStyle(p)
      if (s.overflowX === 'visible' && s.overflowY === 'visible') continue
      const q = p.getBoundingClientRect()
      if (x < q.left || x > q.right || y < q.top || y > q.bottom) return false
    }
    return true
  }
  const leaves = (rx) => [...document.querySelectorAll('body *')].filter((e) => e.children.length === 0 && rx.test(e.textContent.trim()) && seen(e) && !e.closest('article, .help-article, [data-help]'))
  const modal = '[role=dialog], [role=alertdialog], [aria-modal=true], dialog[open]'
  // Covers what's under it: a background that's nearly opaque, or a picture. A blur alone doesn't count, since a
  // browser without one draws the layer see-through; nor does a layer that's itself faded.
  const solid = (e) => {
    const s = getComputedStyle(e)
    if (s.backgroundImage !== 'none' && !/gradient/.test(s.backgroundImage)) return true
    const m = s.backgroundColor.match(/[\d.]+/g)
    return !!m && (m.length < 4 || +m[3] >= 0.9) && +s.opacity >= 0.9
  }
  // How see-through something is, with everything around it: a panel faded out is still there, but not drawn.
  const faint = (e) => { let o = 1; for (let p = e; p; p = p.parentElement) o *= +getComputedStyle(p).opacity; return o }
  const found = []
  const want = (k) => !allow.includes(k)
  if (want('setup') && leaves(/^Finish setting up\b/).length) found.push('a "Finish setting up" notice')
  if (want('crash') && leaves(/stopped unexpectedly/).length) found.push('the "stopped unexpectedly" notice')
  if (want('loading')) {
    const busy = [...document.querySelectorAll('.spinner, .sdui-loading, [aria-busy="true"]')].find((e) => seen(e) && unclipped(e))
    if (busy) found.push(`a page still loading (${busy.className || busy.tagName}${busy.textContent.trim() ? ` "${busy.textContent.trim().slice(0, 30)}"` : ''})`)
  }
  if (want('error') && (leaves(/^(This page ran into a problem|Couldn[’']t load this)$/).length || [...document.querySelectorAll('.sdui-error, [role=alert].error')].some(seen))) found.push('an error message')
  if (want('overlay')) {
    for (const e of document.querySelectorAll('body *')) {
      const s = getComputedStyle(e)
      if (s.position !== 'fixed' || !seen(e)) continue
      const r = e.getBoundingClientRect()
      if (r.width * r.height < 0.8 * W * H || e.closest(modal) || e.querySelector(modal)) continue
      if ([...e.querySelectorAll('video, canvas, img, iframe, svg')].some((m) => { const q = m.getBoundingClientRect(); return q.width * q.height > 0.2 * W * H })) continue
      // The page under it still shows: a word outside it, at a point the layer covers, with nothing solid in the layer.
      const under = [...document.querySelectorAll('body *')].find((t) => !e.contains(t) && !t.contains(e) && own(t) && seen(t) && unclipped(t) && (() => {
        const q = t.getBoundingClientRect(); const x = q.left + q.width / 2, y = q.top + q.height / 2
        if (x < r.left || x > r.right || y < r.top || y > r.bottom) return false
        const top = document.elementFromPoint(x, y)
        if (!top || !e.contains(top)) return false
        for (let p = top; p && p !== e.parentElement; p = p.parentElement) if (solid(p)) return false
        return true
      })())
      if (under) { found.push(`a layer over the whole page (${e.className || e.tagName}) with "${under.textContent.trim().slice(0, 30)}" showing through`); break }
    }
  }
  if (want('overlap')) {
    const clashes = []
    for (const a of document.querySelectorAll('body *')) {
      if (!own(a) || !seen(a) || a.closest('[aria-hidden=true]') || !unclipped(a)) continue
      const q = a.getBoundingClientRect(); const x = q.left + q.width / 2, y = q.top + q.height / 2
      if (x < 0 || y < 0 || x >= W || y >= H) continue
      const top = document.elementFromPoint(x, y)
      if (!top || top === a || a.contains(top) || top.contains(a)) continue
      // Another element's words are on top, and nothing between them is solid: both can be read, one over the other.
      let p = top, hidden = false
      for (; p && !p.contains(a); p = p.parentElement) if (solid(p)) { hidden = true; break }
      if (hidden || !own(top) || !seen(top) || faint(top) < 0.3 || faint(a) < 0.3 || getComputedStyle(top).visibility !== 'visible') continue
      // Where the words are actually drawn: a link stretched over a whole card is on top everywhere, but its own
      // words sit apart from the card's other words.
      const drawn = (e) => [...e.childNodes].filter((n) => n.nodeType === 3 && n.nodeValue.trim()).flatMap((n) => { const r = document.createRange(); r.selectNodeContents(n); return [...r.getClientRects()] })
      const mine = drawn(a)
      const over = drawn(top).some((t) => mine.some((m) => { const w = Math.min(t.right, m.right) - Math.max(t.left, m.left), h = Math.min(t.bottom, m.bottom) - Math.max(t.top, m.top); return w > 0 && h > 0 && w * h > 0.3 * Math.min(t.width * t.height, m.width * m.height) }))
      if (!over) continue
      clashes.push(`"${a.textContent.trim().slice(0, 20)}" under "${(top.textContent || top.value || '').trim().slice(0, 20)}"`)
    }
    if (clashes.length >= 3) found.push(`words drawn over other words (${clashes.slice(0, 3).join(', ')})`)
  }
  if (want('broken')) {
    // A picture that didn't load: the browser draws its alt text, or nothing, in its place.
    const img = [...document.querySelectorAll('img')].find((i) => seen(i) && unclipped(i) && i.complete && i.naturalWidth === 0 && i.getAttribute('src'))
    if (img) found.push(`a picture that didn't load (${(img.alt || img.getAttribute('src')).slice(0, 40)})`)
  }
  if (want('overlay')) {
    // The page showing through an open dialog: its words readable where the dialog is, with nothing opaque between.
    const dialogs = [...document.querySelectorAll('.modal, ' + modal)].filter((d) => seen(d) && !d.parentElement?.closest('.modal, ' + modal))
    for (const d of dialogs) {
      const r = d.getBoundingClientRect()
      const through = [...document.querySelectorAll('body *')].filter((t) => !d.contains(t) && !t.contains(d) && own(t) && seen(t) && unclipped(t) && !t.closest('.modal-backdrop, ' + modal)).find((t) => {
        const q = t.getBoundingClientRect(); const x = q.left + q.width / 2, y = q.top + q.height / 2
        if (x < r.left || x > r.right || y < r.top || y > r.bottom) return false
        for (let p = d; p && !p.contains(t); p = p.parentElement) if (solid(p)) return false
        return true
      })
      if (through) { found.push(`the page showing through a dialog ("${through.textContent.trim().slice(0, 30)}")`); break }
    }
  }
  if (want('fading')) {
    // A dialog or panel still fading or sliding in: the page under it shows through.
    const d = [...document.querySelectorAll(modal)].find((e) => seen(e) && faint(e) < 0.95)
    if (d) found.push(`a dialog still fading in ("${(d.getAttribute('aria-label') || d.textContent).trim().slice(0, 30)}")`)
  }
  if (want('scroll')) {
    const de = document.documentElement, b = document.body, stuck = (x) => /hidden|clip/.test(getComputedStyle(x).overflowY)
    if ((stuck(de) || stuck(b)) && Math.max(de.scrollHeight, b.scrollHeight) > H + 40 && ![...document.querySelectorAll(modal)].some(seen)) found.push("a page that can't be scrolled")
  }
  return found
}

// checks: true (the docs' picture scripts) waits out animations and refuses a picture with a problem in it (see
// problems above). The test scripts that use these helpers leave it off: their pages may rightly show a prompt.
export async function open(server, out, { width = 1920, height = 1080, path = '/', checks = false } = {}) {
  mkdirSync(out, { recursive: true })
  const browser = await launch({ software: true, width, height })
  const page = await browser.open(server + path)
  const size = (w, h, mobile = false, scale = 1) => page.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: scale, mobile })
  await size(width, height)
  // What must never be in a picture. A picture that means to show one names it in allow.
  const leftovers = (allow = []) => checks ? page.eval(`(${problems})(${JSON.stringify(allow)})`) : []
  // Before a picture: the fonts are in and every animation that ends (a dialog opening, a panel sliding in) has
  // ended, so nothing is caught half-way. Endless ones (a spinner, a visualizer) don't count.
  const settle = () => !checks ? page.eval(`document.fonts?.ready`) : page.eval(`(async () => {
    await document.fonts?.ready
    for (let i = 0; i < 40; i++) {
      const going = document.getAnimations().filter((a) => a.playState === 'running' && Number.isFinite(a.effect?.getComputedTiming?.().endTime))
      if (!going.length) return
      await new Promise((d) => setTimeout(d, 100))
    }
  })()`)
  const shot = async (name, { allow } = {}) => {
    await settle()
    const found = await leftovers(allow)
    if (found.length) throw new Error(`${name}: not taken, ${found.join(' and ')} is in the picture`)
    writeFileSync(join(out, `${name}.png`), await page.shot())
    console.log('shot', name)
  }
  // A picture of one element at twice the size, however long it is, for guides read at phone width.
  const card = async (name, selector, pad = 16, { allow } = {}) => {
    await settle()
    const box = await page.eval(`(() => { const r = document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect(); return { x: r.x + scrollX, y: r.y + scrollY, width: r.width, height: r.height, fits: r.top >= 0 && r.left >= 0 && r.bottom <= innerHeight && r.right <= innerWidth } })()`)
    const found = await leftovers(allow)
    if (found.length) throw new Error(`${name}: not taken, ${found.join(' and ')} is on the page`)
    const clip = { x: Math.max(0, box.x - pad), y: Math.max(0, box.y - pad), width: box.width + pad * 2, height: box.height + pad * 2, scale: 2 }
    // Beyond the window only when the element is: that mode lays the page out again for the picture, and draws a
    // fixed dialog with the page under it showing through.
    const { data } = await page.send('Page.captureScreenshot', { format: 'png', clip, captureBeyondViewport: !box.fits })
    writeFileSync(join(out, `${name}.png`), Buffer.from(data, 'base64'))
    console.log('card', name, Math.round(clip.width * 2), Math.round(clip.height * 2))
  }
  const call = (method, path, body) => page.eval(`(async () => { const t = localStorage.getItem('wms.token'); const r = await fetch('/api${path}', { method: '${method}', headers: { 'Content-Type': 'application/json', ...(t ? { Authorization: 'Bearer ' + t } : {}) }, body: ${body ? `JSON.stringify(${JSON.stringify(body)})` : 'undefined'} }); return r.json().catch(() => null) })()`)
  // Presses the first visible control whose words match, as a person would pick it out.
  const press = (words, within = 'body') => page.eval(`(() => {
    const rx = ${words instanceof RegExp ? words.toString() : `new RegExp(${JSON.stringify(words.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))})`}
    const all = [...document.querySelectorAll(${JSON.stringify(within)} + ' button, ' + ${JSON.stringify(within)} + ' a, ' + ${JSON.stringify(within)} + ' [role=button], ' + ${JSON.stringify(within)} + ' [role=radio], ' + ${JSON.stringify(within)} + ' [role=checkbox], ' + ${JSON.stringify(within)} + ' [role=tab], ' + ${JSON.stringify(within)} + ' label, ' + ${JSON.stringify(within)} + ' summary')]
    const e = all.find((b) => { const r = b.getBoundingClientRect(); return r.width > 1 && r.height > 1 && rx.test((b.getAttribute('aria-label') || '') + ' ' + (b.title || '') + ' ' + b.textContent) })
    if (!e) return false
    e.scrollIntoView({ block: 'center' }); e.click(); return (e.textContent || '').trim().slice(0, 50)
  })()`)
  const typeIn = async (selector, text) => {
    await page.eval(`document.querySelector(${JSON.stringify(selector)})?.focus()`)
    await page.send('Input.insertText', { text })
  }
  // This machine's addresses stay out of the pictures: the example address the guides use, instead.
  const maskAddresses = () => page.eval(`(() => {
    const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    const seen = []
    for (let n = w.nextNode(); n; n = w.nextNode()) if (/\\b(\\d{1,3}\\.){3}\\d{1,3}\\b/.test(n.nodeValue)) seen.push(n)
    seen.forEach((n, i) => {
      if (i > 0 && /^\\s*https?:\\/\\/[\\d.]+(:\\d+)?\\s*$/.test(n.nodeValue)) { n.parentElement.style.display = 'none'; return }
      n.nodeValue = n.nodeValue.replace(/\\b(\\d{1,3}\\.){3}\\d{1,3}(:\\d+)?\\b/g, '192.168.1.20:9797')
    })
  })()`)
  const until = (expression, ms = 30000, what) => page.until(expression, ms, what)
  // A busy server (just after a scan) can take a while to send a page: wait out its loading skeleton too.
  const go = async (path, settle = 1400) => {
    await page.go(server + path)
    await wait(settle)
    await page.until(`!document.querySelector('.sdui-loading, [aria-busy="true"].skeleton-grid')`, 45000, `${path} to load`).catch((e) => console.log(e.message))
    await wait(400)
  }
  const words = () => page.eval(`document.body.innerText.slice(0, 1500)`)
  return { browser, page, size, shot, card, call, press, typeIn, until, go, words, maskAddresses, leftovers }
}
