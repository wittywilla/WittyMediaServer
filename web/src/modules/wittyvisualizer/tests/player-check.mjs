// Tries the visualizer through the real audio player of a running test server, in a headless
// browser, with a library of generated music (tests/make-audio.mjs). Never for a real library.
//
//   WV_SERVER=http://127.0.0.1:9962 WV_NAME=... WV_PASS=... node tests/player-check.mjs out-folder [--software] [--shots] [--flash]
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { launch } from './cdp.mjs'
import { worstFlashesPerSecond } from '../safety.ts'

const server = process.env.WV_SERVER || 'http://127.0.0.1:9962'
const args = process.argv.slice(2)
const out = args.find((a) => !a.startsWith('--')) || '.'
const has = (f) => args.includes(f)
mkdirSync(out, { recursive: true })

const post = async (path, body, token) => {
  const r = await fetch(server + path, { method: 'POST', headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) }, body: JSON.stringify(body) })
  if (!r.ok) throw new Error(`${path}: ${r.status} ${await r.text()}`)
  return r.json().catch(() => ({}))
}
const login = await post('/api/login', { name: process.env.WV_NAME, password: process.env.WV_PASS })
const token = login.token
// Starts as someone who has never seen it.
await fetch(server + '/api/me/ui', { method: 'PUT', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: '{}' })

const b = await launch({ software: has('--software'), width: 1600, height: 900, tmp: process.env.WV_TMP })
const results = {}
try {
  const p = await b.open(server + '/')
  await p.eval(`localStorage.setItem('wms.token', ${JSON.stringify(token)})`)
  if (process.env.WV_THEME) await p.eval(`localStorage.setItem('wms.theme', ${JSON.stringify(process.env.WV_THEME)})`)
  // Costs nothing until it's opened: note what has loaded so far.
  await p.go(server + '/')
  await p.until(`!!document.querySelector('.card, .sidebar, nav')`, 20000, 'the app')
  const libs = await (await fetch(server + '/api/libraries', { headers: { Authorization: 'Bearer ' + token } })).json()
  const music = (Array.isArray(libs) ? libs : libs.libraries).find((l) => l.type === 'music')
  await p.go(`${server}/album/${music.id}?artist=${encodeURIComponent('Generated Artist')}&album=${encodeURIComponent('Generated Album')}`)
  await p.until(`[...document.querySelectorAll('button')].some((b) => /^(Play|Play all)/.test(b.textContent.trim()))`, 20000, 'the album page').catch(async (e) => { writeFileSync(join(out, 'album.png'), await p.shot()); throw e })
  results.loadedBeforeOpening = await p.eval(`performance.getEntriesByType('resource').some((r) => /wittyvisualizer|Stage/.test(r.name))`)
  await p.eval(`[...document.querySelectorAll('button')].find((b) => /^(Play|Play all)/.test(b.textContent.trim())).click()`)
  await p.until(`!!document.querySelector('audio') && document.querySelector('audio').currentTime > 0.5`, 15000, 'the music to start')
  results.playingBefore = await p.eval(`({ t: document.querySelector('audio').currentTime, paused: document.querySelector('audio').paused })`)

  // Off until someone turns it on.
  results.canvasBeforeOn = await p.eval(`!!document.querySelector('.wv-canvas')`)

  await p.eval(`document.querySelector('.ab-viz-btn').click()`)
  await p.until(`!!document.querySelector('.wv-dialog')`, 15000, 'the notice')
  writeFileSync(join(out, 'notice.png'), await p.shot())
  results.noticeRole = await p.eval(`document.querySelector('.wv-dialog').getAttribute('role')`)
  await p.eval(`[...document.querySelectorAll('.wv-dialog button')].find((b) => b.textContent === 'Turn on').click()`)
  await p.until(`document.querySelector('.wv-stage')?.dataset.fps > 0`, 20000, 'the first frames')
  await new Promise((d) => setTimeout(d, 1500))
  const stage = `document.querySelector('.wv-stage').dataset`
  results.stage = await p.eval(`({ ...${stage} })`)
  // Set to full place by the button in the bar.
  const stillPlays = async () => {
    const a = await p.eval(`document.querySelector('audio').currentTime`)
    await new Promise((d) => setTimeout(d, 1000))
    const c = await p.eval(`({ t: document.querySelector('audio').currentTime, paused: document.querySelector('audio').paused, muted: document.querySelector('audio').muted, volume: document.querySelector('audio').volume })`)
    return { advanced: Math.round((c.t - a) * 100) / 100, paused: c.paused, muted: c.muted, volume: c.volume }
  }
  results.playbackWhileOn = await stillPlays()

  // Is it really hearing the music? Count beats, in the real sound, on a 120 BPM track.
  results.tap = await p.eval(stage + '.tap')
  const beats0 = Number(await p.eval(stage + '.beats'))
  await new Promise((d) => setTimeout(d, 8000))
  const beats1 = Number(await p.eval(stage + '.beats'))
  results.beatsPerSecondAt120Bpm = (beats1 - beats0) / 8

  // Screen readers: the canvas is hidden from them, and every control has a name.
  results.a11y = await p.eval(`({
    canvasHidden: document.querySelector('.wv-canvas').getAttribute('aria-hidden'),
    unnamed: [...document.querySelectorAll('.wv-stage button, .wv-stage select')].filter((e) => !(e.getAttribute('aria-label') || e.textContent.trim())).length,
  })`)

  const setEffect = (id) => p.eval(`(() => { const s = document.querySelector('.wv-bar select'); const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set; set.call(s, ${JSON.stringify(id)}); s.dispatchEvent(new Event('change', { bubbles: true })) })()`)
  const ids = await p.eval(`[...document.querySelectorAll('.wv-bar select option')].map((o) => o.value)`)
  results.effects = ids.length

  // Stay on what's chosen, while pictures are taken.
  await p.eval(`document.querySelector('[aria-label="Visualizer settings"]').click()`)
  await p.until(`!!document.querySelector('.wv-dialog.wide')`, 3000, 'the settings')
  writeFileSync(join(out, 'settings.png'), await p.shot())
  await p.eval(`(() => { const s = document.querySelector('.wv-dialog.wide select'); const set = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set; set.call(s, '0'); s.dispatchEvent(new Event('change', { bubbles: true })) })()`)
  await p.eval(`[...document.querySelectorAll('.wv-check input')].find((i) => /frame rate/.test(i.parentElement.textContent)).click()`)
  await p.eval(`document.querySelector('[aria-label="Close settings"]').click()`)
  await new Promise((d) => setTimeout(d, 200))

  await p.eval(`document.querySelector('[aria-label="Full screen"]')?.click()`)
  await p.until(`document.querySelector('.wv-stage.whole')`, 5000, 'full screen')
  await new Promise((d) => setTimeout(d, 3000))
  results.fullScreen = await p.eval(`({ ...${stage}, fullscreen: !!document.fullscreenElement })`)
  results.playbackFull = await stillPlays()

  // Keys
  const before = await p.eval(stage + '.effect')
  await p.key('ArrowRight')
  await new Promise((d) => setTimeout(d, 300))
  const after = await p.eval(stage + '.effect')
  await p.key('ArrowLeft')
  await new Promise((d) => setTimeout(d, 300))
  results.keys = { rightChanged: before !== after, leftReturned: (await p.eval(stage + '.effect')) === before }

  if (has('--shots')) {
    const seen = {}
    for (const id of ids) {
      await setEffect(id)
      await new Promise((d) => setTimeout(d, 5500))
      const png = await p.shot()
      writeFileSync(join(out, `effect-${id}.png`), png)
      seen[id] = await p.eval(`({ fps: +${stage}.fps, w: +${stage}.width, h: +${stage}.height, ms: +${stage}.frameMs })`)
    }
    results.perEffect = seen
  }

  if (has('--flash')) {
    // Watches the real canvas, on a real page, playing the most hostile music we can make,
    // with each effect: one brightness per frame, in tiles the size the guideline uses.
    await p.eval(`(() => {
      window.__wvRec = { tiles: [], t: [] }
      // Enough pixels that a tile is a real average of its area, not a few samples of it.
      const c2 = document.createElement('canvas'); c2.width = 160; c2.height = 100
      const x = c2.getContext('2d', { willReadFrequently: true })
      x.imageSmoothingQuality = 'high'
      const lum = (v) => Math.pow(v / 255, 2.2)
      const loop = () => {
        const c = document.querySelector('.wv-canvas')
        if (c && window.__wvRec) {
          x.drawImage(c, 0, 0, 160, 100)
          const d = x.getImageData(0, 0, 160, 100).data, row = new Array(40).fill(0)
          let m = 0
          for (let py = 0; py < 100; py++) for (let px = 0; px < 160; px++) {
            const i = (py * 160 + px) * 4, l = 0.2126 * lum(d[i]) + 0.7152 * lum(d[i + 1]) + 0.0722 * lum(d[i + 2])
            row[Math.floor(py / 20) * 8 + Math.floor(px / 20)] += l / 400
            m += l
          }
          row.push(m / 16000)
          window.__wvRec.tiles.push(row); window.__wvRec.t.push(performance.now() / 1000)
          requestAnimationFrame(loop)
        }
      }
      requestAnimationFrame(loop)
    })()`)
    const worst = {}
    // The third track is the hostile one: full-range bursts ten times a second.
    await p.eval(`document.querySelector('[aria-label="Next"]').click()`)
    await new Promise((d) => setTimeout(d, 300))
    await p.eval(`document.querySelector('[aria-label="Next"]').click()`)
    await p.until(`document.querySelector('audio').currentTime > 0.1 && /03|Hostile/.test(document.querySelector('.ab-title')?.textContent || '')`, 8000, 'the hostile track')
    for (const id of ids) {
      await setEffect(id)
      await new Promise((d) => setTimeout(d, 400))
      await p.eval(`window.__wvRec.tiles.length = 0; window.__wvRec.t.length = 0`)
      await new Promise((d) => setTimeout(d, 4500))
      const r = await p.eval(`window.__wvRec`)
      // The whole screen, and the worst single tile of the 40.
      const whole = r.tiles.map((row) => row[40])
      let tile = 0
      for (let k = 0; k < 40; k++) tile = Math.max(tile, worstFlashesPerSecond(r.tiles.map((row) => row[k]), r.t))
      worst[id] = { whole: worstFlashesPerSecond(whole, r.t), worstTile: tile, frames: r.t.length, high: Math.round(Math.max(...whole) * 1000) / 1000 }
    }
    await p.eval(`window.__wvRec = null`)
    results.flashCheck = worst
  }

  // Off is really off.
  await p.key('Escape')
  await new Promise((d) => setTimeout(d, 400))
  await p.eval(`document.querySelector('.ab-viz-btn').click()`)
  await new Promise((d) => setTimeout(d, 500))
  results.afterOff = await p.eval(`({ stage: !!document.querySelector('.wv-stage'), canvas: !!document.querySelector('.wv-canvas') })`)
  results.playbackAfterOff = await stillPlays()

  // Its settings were saved for this person.
  await new Promise((d) => setTimeout(d, 900))
  const ui = await (await fetch(server + '/api/me/ui', { headers: { Authorization: 'Bearer ' + token } })).json()
  results.saved = ui.visualizer
  results.errors = p.logs.filter((l) => /^(error|exception|warning)/.test(l)).slice(0, 12)
  writeFileSync(join(out, 'results.json'), JSON.stringify(results, null, 1))
  console.log(JSON.stringify(results, null, 1))
  p.close()
} catch (e) {
  console.log(String(e))
  console.log(JSON.stringify(results, null, 1))
  throw e
} finally {
  await b.close()
}
