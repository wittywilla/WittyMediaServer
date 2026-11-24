// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The web app's smoke test, in a headless browser on a real server that scripts/smoke/server.sh
// set up: sign in with the form, play the film, play a song, watch a demo tuner channel, start
// the Game Boy game and see its picture, and open settings. Each step leaves a picture and the
// page's console in the run's folder, so a failure says what was on the screen.
//
//   WMS_URL=... WMS_NAME=... WMS_PASS=... WMS_TOKEN=... node scripts/smoke/web.mjs <out folder> [step ...]
import { mkdirSync, writeFileSync, appendFileSync, readdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { launch } from '../../web/scripts/cdp.mjs'

const server = process.env.WMS_URL
const token = process.env.WMS_TOKEN
const [out = 'smoke-out', ...only] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
let n = readdirSync(out).filter((f) => /^\d\d-.*\.log$/.test(f)).length

const api = async (path) => {
  const r = await fetch(server + path, { headers: { Authorization: 'Bearer ' + token } })
  if (!r.ok) throw new Error(`${path}: ${r.status} ${(await r.text()).slice(0, 200)}`)
  return r.json()
}

// A busy runner sometimes takes longer than the driver waits for the browser to start: try again.
let b
for (let i = 1; !b; i++) {
  try { b = await launch({ software: true, width: 1280, height: 800, tmp: process.env.SMOKE_TMP, args: ['--disable-dev-shm-usage'] }) } catch (e) { if (i === 3) throw e; console.log('the browser did not start, trying again: ' + e.message.slice(0, 200)) }
}
const p = await b.open()
await p.send('Network.enable')
const failed = []
const netErrors = []
p.on('Network.responseReceived', (e) => { if (e.response.status >= 500) netErrors.push(`${e.response.status} ${e.response.url}`) })

async function step(name, fn) {
  if (only.length && !only.some((o) => name.toLowerCase().includes(o))) return
  const base = join(out, `${String(++n).padStart(2, '0')}-web-${name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/-$/, '')}`)
  const log = (s) => appendFileSync(base + '.log', s + '\n')
  writeFileSync(base + '.log', `--- ${name}\n`)
  p.logs.length = 0
  netErrors.length = 0
  const t = Date.now()
  let err
  console.log(`::group::web: ${name}`)
  try { await fn(log) } catch (e) { err = e }
  try { writeFileSync(base + '.png', await p.shot()) } catch (e) { log('no picture: ' + e.message) }
  log(`url: ${await p.eval('location.href').catch(() => '?')}`)
  if (p.logs.length) log('console:\n  ' + p.logs.join('\n  '))
  if (netErrors.length) log('server errors:\n  ' + netErrors.join('\n  '))
  console.log('::endgroup::')
  if (err) {
    log('FAILED: ' + (err.stack || err))
    console.log(`FAIL  web: ${name}: ${err.message}`)
    console.log(`::error title=Smoke web: ${name}::${err.message} (picture: ${base.split(/[\\/]/).pop()}.png)`)
    failed.push(name)
  } else console.log(`ok    web: ${name} (${((Date.now() - t) / 1000).toFixed(1)}s)`)
}

// Focus, then type as a keyboard would, so the page's own handlers see it.
async function type(selector, text) {
  await p.eval(`(() => { const e = document.querySelector(${JSON.stringify(selector)}); e.focus(); e.select?.(); return true })()`)
  await p.send('Input.insertText', { text })
}
// Clicks the first visible button or link whose text or label matches, waiting up to ms for one.
async function click(pattern, ms = 15000, scope = 'document') {
  const end = Date.now() + ms
  for (;;) {
  const ok = await p.eval(`(() => {
    const re = new RegExp(${JSON.stringify(pattern)}, 'i')
    const els = [...${scope}.querySelectorAll('button, a, [role=button], [role=link], [tabindex]')]
      .filter((e) => e.offsetParent !== null && re.test((e.getAttribute('aria-label') || '') + ' ' + e.textContent))
      // The most specific: a card's own button, not the row or page around it.
      .filter((e, _, all) => !all.some((o) => o !== e && e.contains(o)))
    if (!els.length) return false
    els[0].scrollIntoView({ block: 'center' }); els[0].click(); return true
  })()`)
    if (ok) return
    if (Date.now() > end) throw new Error(`nothing to press matching /${pattern}/ after ${ms / 1000}s`)
    await wait(250)
  }
}
// A media element that is really playing: its clock moves past `secs`.
const PLAYING = (kind, secs) => `[...document.querySelectorAll('${kind}')].some((m) => !m.paused && m.currentTime > ${secs} && m.readyState >= 2)`
// A picture on screen in a video or canvas: a frame of many colours, not one flat fill.
const picture = (share = 0.05) => `(() => {
  const c = document.createElement('canvas'); c.width = 64; c.height = 48; const g = c.getContext('2d', { willReadFrequently: true })
  for (const e of document.querySelectorAll('video, canvas')) {
    if (e.offsetWidth * e.offsetHeight < ${share} * innerWidth * innerHeight || (e.tagName === 'VIDEO' && !e.videoWidth)) continue
    try { g.drawImage(e, 0, 0, 64, 48) } catch { continue }
    const d = g.getImageData(0, 0, 64, 48).data, seen = new Set()
    for (let i = 0; i < d.length; i += 4) seen.add((d[i] >> 4) << 8 | (d[i + 1] >> 4) << 4 | d[i + 2] >> 4)
    if (seen.size >= 4) return true
  }
  return false
})()`

const libs = await api('/api/libraries')
const lib = (type) => libs.find((l) => l.type === type) || (() => { throw new Error('no ' + type + ' library') })()

await step('Sign in', async (log) => {
  await p.go(server + '/')
  await p.until(`document.querySelector('input[type=password]') || document.querySelector('.who-tile')`, 30000, 'the sign-in page')
  // A server with one account may first show who's watching: that account's password comes next.
  if (await p.eval(`!document.querySelector('input[type=password]')`)) await click('another account|smoke')
  await p.until(`document.querySelector('input[type=password]')`, 10000, 'the password field')
  if (await p.eval(`!!document.querySelector('input[autocomplete=username], input[name=name], input[type=text]')`)) {
    await type('input[autocomplete=username], input[name=name], input[type=text]', process.env.WMS_NAME)
  }
  await type('input[type=password]', process.env.WMS_PASS)
  await p.key('Enter')
  await p.until(`!document.querySelector('input[type=password]') && document.querySelector('nav, header, main')`, 30000, 'the signed-in app')
  log('signed in')
})

await step('Play the film', async (log) => {
  const items = await api(`/api/items?library=${lib('movies').id}`)
  const film = (items.items || items)[0]
  log('film: ' + JSON.stringify({ id: film.id, title: film.title || film.name }))
  await p.go(`${server}/item/${film.id}`)
  await click('^\\s*(Play|Watch)', 45000)
  await p.until(PLAYING('video', 1.5), 45000, 'the film playing past 1.5 s')
  await p.until(picture(0.3), 10000, 'a picture from the film')
})

await step('Play music', async (log) => {
  const music = lib('music')
  await p.go(`${server}/album/${music.id}?artist=${encodeURIComponent('Smoke Artist')}&album=${encodeURIComponent('Smoke Album')}`)
  await p.until(`document.body.innerText.includes('Tone 1')`, 30000, 'the album with its tracks')
  await click('^\\s*Play\\b')
  await p.until(PLAYING('audio', 1), 30000, 'a track playing past 1 s')
  log(await p.eval(`[...document.querySelectorAll('audio')].map((a) => a.currentSrc.replace(/token=[^&]+/, 'token=…') + ' ' + a.currentTime).join('\\n')`))
})

await step('Watch live TV from the demo tuner', async (log) => {
  await p.go(server + '/tv')
  await p.until(`document.body.innerText.includes('Demo')`, 45000, 'a demo channel in the guide')
  await click('Demo News|Demo Pattern|Demo')
  // The guide may play it in its preview, or open a channel page with a Watch button first.
  const started = await p.until(PLAYING('video', 0.5), 15000).catch(() => false)
  if (!started) { await click('^\\s*(Watch|Play|Watch live|Live)').catch(() => {}); await p.until(PLAYING('video', 0.5), 60000, 'the channel playing') }
  await p.until(picture(0.05), 15000, 'a picture from the channel')
  log('playing ' + await p.eval('location.pathname'))
})

await step('Start a WittyPlay game and get a frame', async (log) => {
  await p.go(server + '/games')
  await p.until(`document.body.innerText.match(/Adjustris/i)`, 60000, 'the game in the library')
  // WittyPlay's first visit asks which buttons to show (after a moment): the first choice is fine.
  if (await p.until(`/Which buttons should we show/.test(document.body.innerText)`, 6000).catch(() => false)) {
    await click('Continue')
    await p.until(`!/Which buttons should we show/.test(document.body.innerText)`, 10000, 'the buttons question gone')
  }
  await click('Adjustris')
  await wait(1500)
  log('after choosing the game: ' + await p.eval('location.pathname') + '\n' + (await p.eval('document.body.innerText')).slice(0, 600))
  if (!(await p.eval(picture(0.3)))) await click('^\\s*(Play|Start|Resume)\\b')
  await p.until(picture(0.3), 90000, 'a frame from the game, filling the screen')
  log('frame on screen at ' + await p.eval('location.pathname'))
})

await step('Open settings', async (log) => {
  await p.go(server + '/settings')
  await p.until(`/Settings/.test(document.body.innerText) && /Libraries/.test(document.body.innerText)`, 20000, 'the settings page')
  const crash = await p.eval(`document.body.innerText.match(/Something went wrong|went wrong/i)?.[0] || ''`)
  if (crash) throw new Error('settings shows: ' + crash)
  log(await p.eval(`[...document.querySelectorAll('h1, h2')].map((h) => h.textContent).join(' | ')`))
})

// Settings, Backup: a folder of one's own (typed, as a second drive would be), a backup made into it,
// and back to the data folder after.
await step('Back up to a folder of your own', async (log) => {
  const dir = join(process.env.SMOKE_TMP || tmpdir(), `smoke-backups-${process.pid}`)
  mkdirSync(dir, { recursive: true })
  const send = async (method, path, body) => {
    const r = await fetch(server + path, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    if (!r.ok) throw new Error(`${method} ${path}: ${r.status} ${(await r.text()).slice(0, 200)}`)
    return r.json()
  }
  try {
    await p.go(server + '/settings/backup')
    await click('^\\s*Schedule\\s*$')
    await p.until(`document.querySelector('.bk-folder input')`, 15000, 'the folder field')
    await type('.bk-folder input', dir)
    await p.until(`document.querySelector('.bk-folder input[type=password]')`, 5000, 'the password field')
    await type('.bk-folder input[type=password]', process.env.WMS_PASS)
    await click('^\\s*Use this folder')
    const end = Date.now() + 15000
    while (!(await api('/api/backup')).chosenFolder) {
      if (Date.now() > end) throw new Error('the folder was not chosen: ' + (await p.eval(`document.querySelector('[role=alert]')?.innerText || ''`)))
      await wait(300)
    }
    log('chosen: ' + (await api('/api/backup')).chosenFolder)
    await send('POST', '/api/backup/create', { parts: ['settings'] })
    const made = Date.now() + 120000
    while (!readdirSync(dir).some((f) => /^wms-backup-.*\.zip$/.test(f))) {
      if (Date.now() > made) throw new Error('no backup in the folder: ' + JSON.stringify((await api('/api/backup')).job))
      await wait(500)
    }
    log('in the folder: ' + readdirSync(dir).join(', '))
    await p.go(server + '/settings/backup')
    await p.until(`/In your folder/.test(document.body.innerText)`, 20000, 'the backup listed as in the chosen folder')
  } finally {
    await send('PUT', '/api/backup/folder', { folder: '', current: process.env.WMS_PASS }).catch((e) => log('could not put the folder back: ' + e.message))
    rmSync(dir, { recursive: true, force: true })
  }
})

await b.close()
if (failed.length) { console.log(`\n${failed.length} web step(s) failed: ${failed.join(', ')}`); process.exit(1) }
console.log('\nweb: every step passed')
process.exit(0)
