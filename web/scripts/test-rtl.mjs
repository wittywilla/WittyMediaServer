// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Right-to-left languages: Arabic and Hebrew pages, in a headless browser, with screenshots. No window opens.
//
// A title, file name, artist or "2 hr 10 min" is written left to right. Shown in a right-to-left line without being
// isolated, the browser's bidi rules move its digits and dashes ("00-aurora-…-2019" shows as "…-2019-00", and
// "2 hr 10 min" as "hr 10 min 2"), and a long one is cut at the wrong end. This walks the app's pages in each
// language and, for every piece of text that starts left to right, checks on screen that its first letter comes
// before its last and that a cut one still shows its start. Each page's screenshot goes in the out folder.
//
// With WMS_BIN, the script starts that server on its own: a new data folder, an account, a small library of files with
// Latin, Hebrew and mixed names that ffmpeg makes, then each language in turn. Nothing outside its own temporary
// folders is touched. With WMS_SERVER (and WMS_NAME, WMS_PASS) it uses a test server that's already running, and
// changes that server's language: never point it at a server someone uses.
//
//   WMS_BIN=../wittymediaserver node scripts/test-rtl.mjs out-folder
//   WMS_SERVER=http://rtl.localhost:9822 WMS_NAME=admin WMS_PASS=... node scripts/test-rtl.mjs out-folder
import { spawn, spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
const { launch } = await import(process.env.WITTYVISUALIZER ? new URL(process.env.WITTYVISUALIZER + '/tests/cdp.mjs', import.meta.url).href : './cdp.mjs')

const [out = 'rtl-shots'] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const LANGS = (process.env.WMS_LANGS || 'ar,he').split(',')
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const problems = []
let child = null
let tmp = ''

const freePort = () => new Promise((ok) => { const s = createServer(); s.listen(0, '127.0.0.1', () => { const p = s.address().port; s.close(() => ok(p)) }) })

// A few seconds of silence or colour bars, named as a library is: the names are what's being tested.
function makeMedia(root) {
  const ff = process.env.FFMPEG || 'ffmpeg'
  const make = (file, args) => {
    mkdirSync(resolve(file, '..'), { recursive: true })
    const r = spawnSync(ff, ['-hide_banner', '-loglevel', 'error', '-y', ...args, file])
    if (r.status !== 0) throw new Error(`ffmpeg couldn't make ${file}: ${r.stderr}`)
  }
  const audio = (file, tags = {}) => make(file, ['-f', 'lavfi', '-i', 'anullsrc=r=44100:cl=stereo', '-t', '2', ...Object.entries(tags).flatMap(([k, v]) => ['-metadata', `${k}=${v}`])])
  const video = (file) => make(file, ['-f', 'lavfi', '-i', 'testsrc=size=320x180:rate=10', '-t', '2', '-pix_fmt', 'yuv420p'])
  // The bug report's own album: no tags, so the file name is the title.
  audio(join(root, 'Music', 'Aurora', 'A Different Kind of Human (Step 2)', '00-aurora-a_different_kind_of_human__step_2-24bit-48khz-web-flac-2019.flac'))
  audio(join(root, 'Music', 'Aurora', 'A Different Kind of Human (Step 2)', '01-aurora-the_river-24bit-48khz-web-flac-2019.flac'))
  audio(join(root, 'Music', 'Various', '2019 Hits', '02 - Track 2 (Remix).flac'), { title: '2 Become 1 (Remix 2019)', artist: 'Spice 2', album: '2019 Hits', track: '2' })
  audio(join(root, 'Music', 'Ofra', 'שירים', '01 - שיר אחד.flac'), { title: 'שיר אחד - Song 1', artist: 'עפרה Ofra', album: 'שירים', track: '1' })
  // A film with a year and a running time, from an NFO beside it: "2018 · 2 hr 10 min" on its page.
  video(join(root, 'Movies', '2001 A Space Test (2018)', '2001 A Space Test (2018).mkv'))
  writeFileSync(join(root, 'Movies', '2001 A Space Test (2018)', '2001 A Space Test (2018).nfo'),
    '<?xml version="1.0" encoding="UTF-8"?>\n<movie><title>2001: A Space Test</title><year>2018</year><runtime>130</runtime><plot>A 4K-HDR10 test, 24-bit 48kHz.</plot><genre>Science Fiction</genre></movie>\n')
  video(join(root, 'Movies', 'ספר הסרטים (2020)', 'ספר הסרטים (2020).mkv'))
}

async function startServer() {
  tmp = mkdtempSync(join(tmpdir(), 'wms-rtl-'))
  const media = join(tmp, 'media')
  makeMedia(media)
  const port = await freePort()
  child = spawn(resolve(process.env.WMS_BIN), ['-data', join(tmp, 'data'), '-listen', `127.0.0.1:${port}`], { stdio: ['ignore', 'ignore', 'pipe'] })
  let log = ''
  child.stderr.on('data', (d) => { log = (log + d).slice(-4000) })
  child.on('exit', (code) => { if (code) console.error(`the server stopped (${code}):\n${log}`) })
  const server = `http://127.0.0.1:${port}`
  for (let i = 0; i < 120; i++) {
    try { if ((await fetch(server + '/api/health')).ok) break } catch { /* starting */ }
    await wait(500)
  }
  const name = 'rtl', password = 'rtl-test-' + Math.random().toString(36).slice(2)
  const res = await fetch(server + '/api/setup', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name, password }) })
  if (!res.ok) throw new Error(`setup: ${res.status} ${await res.text()}`)
  const { token } = await res.json()
  const call = async (method, path, body) => {
    const r = await fetch(server + '/api' + path, { method, headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: body ? JSON.stringify(body) : undefined })
    if (!r.ok) throw new Error(`${method} ${path}: ${r.status} ${await r.text()}`)
    return r.json().catch(() => null)
  }
  await call('POST', '/setup/complete')
  await call('PUT', '/settings', { saveMetadata: '0', saveWittyinfo: '0' })
  for (const [n, type, dir] of [['Music', 'music', 'Music'], ['Movies', 'movies', 'Movies']]) {
    const lib = await call('POST', '/libraries', { name: n, type, paths: [join(media, dir)] })
    await call('POST', `/libraries/${lib.id}/scan`)
  }
  // The scans are small: wait until every library has its files.
  for (let i = 0; i < 120; i++) {
    const libs = await call('GET', '/libraries')
    const list = Array.isArray(libs) ? libs : libs?.libraries || []
    const busy = await Promise.all(list.map((l) => call('GET', `/libraries/${l.id}/scan`).then((s) => !!s?.scanning).catch(() => false)))
    if (!busy.some(Boolean)) break
    await wait(500)
  }
  return { server, token, call }
}

async function useServer() {
  const server = process.env.WMS_SERVER
  const r = await fetch(server + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: process.env.WMS_NAME, password: process.env.WMS_PASS }) })
  if (!r.ok) throw new Error(`sign-in: ${r.status}`)
  const { token } = await r.json()
  const call = async (method, path, body) => {
    const res = await fetch(server + '/api' + path, { method, headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: body ? JSON.stringify(body) : undefined })
    if (!res.ok) throw new Error(`${method} ${path}: ${res.status}`)
    return res.json().catch(() => null)
  }
  return { server, token, call }
}

// In the page: every piece of visible text whose first strong letter is left to right, checked on screen.
const CHECK = `(() => {
  const L = /[A-Za-z\\u00C0-\\u024F\\u0370-\\u03FF\\u0400-\\u04FF]/
  const R = /[\\u0590-\\u08FF\\uFB1D-\\uFDFF\\uFE70-\\uFEFF]/
  const firstStrong = (s) => { for (const c of s) { if (L.test(c)) return 'L'; if (R.test(c)) return 'R' } return '' }
  const out = []
  const where = (el) => { const parts = []; for (let e = el; e && e !== document.body && parts.length < 3; e = e.parentElement) parts.unshift(e.tagName.toLowerCase() + (typeof e.className === 'string' && e.className ? '.' + e.className.trim().split(' ').filter(Boolean).join('.') : '')); return parts.join(' > ') }
  const walk = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  const range = document.createRange()
  const box = (node, i, n) => { range.setStart(node, i); range.setEnd(node, i + n); return range.getClientRects()[0] }
  for (let node = walk.nextNode(); node; node = walk.nextNode()) {
    const text = node.data
    const trimmed = text.trim()
    // Text with right-to-left letters in it is the app's own words, translated (partly, until a translation is done).
    if (trimmed.length < 2 || firstStrong(trimmed) !== 'L' || R.test(trimmed)) continue
    const el = node.parentElement
    if (!el || el.closest('script, style, input, textarea, svg, code, pre, [aria-hidden="true"], [hidden]')) continue
    const cs = getComputedStyle(el)
    if (cs.visibility === 'hidden' || cs.display === 'none') continue
    const whole = el.getBoundingClientRect()
    if (whole.width < 2 || whole.height < 2 || whole.bottom < 0 || whole.top > innerHeight * 3) continue
    // The first and last characters that show: not spaces, nor the invisible marks that isolate text.
    const shown = (c) => !/[\\s\\u200e\\u200f\\u202a-\\u202e\\u2066-\\u2069]/.test(c)
    let start = 0
    while (start < text.length && !shown(text[start])) start++
    let end = text.length - 1
    while (end > start && !shown(text[end])) end--
    // Only text that has digits or marks that the rules could move: plain words read the same either way.
    if (!/[\\d\\-_.()[\\]:/·]/.test(trimmed)) continue
    const a = box(node, start, 1), b = box(node, end, 1)
    if (!a || !b) continue
    const oneLine = Math.abs(a.top - b.top) < a.height / 2
    if (oneLine && a.left > b.right + 0.5) out.push({ what: 'reordered', text: trimmed.slice(0, 120), where: where(el) })
    // Cut with an ellipsis: its start has to be the part that shows.
    let cut = el
    while (cut && cut !== document.body && getComputedStyle(cut).textOverflow !== 'ellipsis') cut = cut.parentElement
    if (cut && cut !== document.body && cut.scrollWidth > cut.clientWidth + 1) {
      const r = cut.getBoundingClientRect()
      if (a.left < r.left - 1 || a.right > r.right + 1) out.push({ what: 'cut at its start', text: trimmed.slice(0, 120), where: where(cut) })
    }
  }
  return out
})()`

let browser = null
try {
  const { server, token, call } = process.env.WMS_BIN ? await startServer() : await useServer()
  // A busy machine (a CI runner, or many builds at once) can take a while to start a browser.
  browser = await launch({ software: true, width: 1280, height: 800, startMs: 90000 })
  const page = await browser.open(server + '/')
  await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)})`)
  const settle = async () => {
    await page.until(`document.readyState === 'complete' && !document.querySelector('.skel, .spinner, [aria-busy="true"]')`, 20000, 'the page to load').catch(() => null)
    await wait(600)
  }
  for (const lang of LANGS) {
    await call('PUT', '/settings', { uiLanguage: lang })
    const seen = new Set()
    const queue = ['/', '/libraries', '/music', '/search?q=20', '/settings/backup', '/settings/remote', '/tv/channels', '/you']
    const per = {}
    let n = 0
    while (queue.length && n < 24) {
      const path = queue.shift()
      if (seen.has(path)) continue
      seen.add(path)
      await page.go(server + path)
      await settle()
      const dir = await page.eval('document.documentElement.dir')
      if (['ar', 'he', 'fa', 'ur'].includes(lang) && dir !== 'rtl') problems.push({ lang, path, what: `the page is ${dir || 'not marked'}, not rtl` })
      for (const p of await page.eval(CHECK)) problems.push({ lang, path, ...p })
      const name = `${lang}-${String(++n).padStart(2, '0')}-${path.replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '') || 'home'}.png`
      writeFileSync(join(out, name), await page.shot())
      // Follow a few links to the pages titles are shown on.
      const links = await page.eval(`[...document.querySelectorAll('a[href]')].map((a) => a.getAttribute('href')).filter((h) => /^\\/(library|item|album|artist|show)\\//.test(h))`)
      for (const h of links) {
        const kind = h.split('/')[1]
        if ((per[kind] = (per[kind] || 0) + 1) <= 3 && !seen.has(h)) queue.push(h)
      }
    }
    console.log(`${lang}: ${n} pages`)
  }
} finally {
  await browser?.close().catch(() => null)
  if (child) child.kill()
  if (tmp) { await wait(500); try { rmSync(tmp, { recursive: true, force: true }) } catch { /* the server may still hold a file */ } }
}
for (const p of problems) console.log(`FAIL ${p.lang} ${p.path}: ${p.what}: "${p.text ?? ''}" (${p.where ?? ''})`)
console.log(problems.length ? `${problems.length} problems; screenshots in ${out}` : `right to left is fine; screenshots in ${out}`)
process.exit(problems.length ? 1 : 0)
