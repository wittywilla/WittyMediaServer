// Tries the music pages and player in a headless browser, on a running test server. No window
// opens. It expects these libraries, with generated media and read-only real music:
//   "Test music": Generated Artist/Generated Album (3 tracks), Signal Test/Signals with "Quiet tag"
//   (ReplayGain -9 dB track, -3 dB album) and "No tag" and a Booklet.pdf, and a .cue CD image.
//   "Discs" with the same image, "Radio", "Podcasts", and optionally "Books".
//
//   WMS_SERVER=http://127.0.0.1:9811 WMS_NAME=... WMS_PASS=... node scripts/test-music.mjs out-folder [section ...]
import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
const { launch } = await import(new URL((process.env.WITTYVISUALIZER || "../../../WittyVisualizer") + "/tests/cdp.mjs", import.meta.url))

const server = process.env.WMS_SERVER || 'http://127.0.0.1:9811'
const [out = '.', ...only] = process.argv.slice(2)
mkdirSync(out, { recursive: true })
const wait = (ms) => new Promise((d) => setTimeout(d, ms))
const call = async (method, path, body, token) => {
  const r = await fetch(server + '/api' + path, { method, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) }, body: body ? JSON.stringify(body) : undefined })
  const data = await r.json().catch(() => ({}))
  return { status: r.status, data, headers: r.headers }
}
const { data: { token } } = await call('POST', '/login', { name: process.env.WMS_NAME, password: process.env.WMS_PASS })
const libs = (await call('GET', '/libraries', null, token)).data
const lib = (name) => libs.find((l) => l.name === name)
const testLib = lib('Test music')
const albumUrl = (artist, album) => `${server}/album/${testLib.id}?artist=${encodeURIComponent(artist)}&album=${encodeURIComponent(album)}`
const AUDIO = `[...document.querySelectorAll('audio')].find((a) => a.src)`

// What the player can't show from outside: how often the backdrop draws, and the sound graph's levels.
const HOOKS = `
  window.__draws = 0
  const clear = CanvasRenderingContext2D.prototype.clearRect
  CanvasRenderingContext2D.prototype.clearRect = function (...a) { window.__draws++; return clear.apply(this, a) }
  window.__analysers = []
  const make = BaseAudioContext.prototype.createAnalyser
  BaseAudioContext.prototype.createAnalyser = function () { const n = make.call(this); window.__analysers.push(n); return n }
  window.__level = async () => {
    // The sound graph makes three in one context, and the first follows the volume stages.
    const byCtx = new Map()
    for (const n of window.__analysers) { if (!byCtx.has(n.context)) byCtx.set(n.context, []); byCtx.get(n.context).push(n) }
    const list = [...byCtx.values()].find((l) => l.length >= 3)
    if (!list) return null
    const f = new Float32Array(2048)
    let s = 0, c = 0
    for (let i = 0; i < 20; i++) { list[0].getFloatTimeDomainData(f); for (const v of f) { s += v * v; c++ } await new Promise((d) => setTimeout(d, 40)) }
    return Math.sqrt(s / c)
  }
  window.__rate = async (ms = 3000) => { const a = window.__draws; await new Promise((d) => setTimeout(d, ms)); return Math.round((window.__draws - a) / (ms / 1000)) }
`

const results = {}
const b = await launch({ width: 1400, height: 900, tmp: process.env.WMS_TMP })
const size = (p, w, h, mobile = false) => p.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile })
const section = async (name, fn, { width = 1400, height = 900, mobile = false, media } = {}) => {
  if (only.length && !only.includes(name)) return
  const p = await b.open()
  try {
    await p.send('Page.addScriptToEvaluateOnNewDocument', { source: HOOKS })
    await size(p, width, height, mobile)
    if (media) await p.send('Emulation.setEmulatedMedia', { features: media })
    await p.go(server + '/')
    // Signed in as the page really is, with its cookie: some addresses take nothing else.
    await p.eval(`fetch('/api/login', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: ${JSON.stringify(process.env.WMS_NAME)}, password: ${JSON.stringify(process.env.WMS_PASS)} }) }).then((r) => r.json()).then((d) => localStorage.setItem('wms.token', d.token))`)
    results[name] = await fn(p)
    results[name].errors = p.logs.filter((l) => /^(error|exception)/.test(l)).slice(0, 6)
  } catch (e) {
    results[name] = { ...(results[name] || {}), failed: String(e) }
    writeFileSync(join(out, name + '-failed.png'), await p.shot())
  }
  console.log(name, JSON.stringify(results[name], null, 1))
  p.close()
}
const shot = async (p, name) => writeFileSync(join(out, name + '.png'), await p.shot())
const noMotion = (p) => p.eval(`(() => { const s = document.createElement('style'); s.textContent = '*{animation:none!important}'; document.head.appendChild(s) })()`)
const playAlbum = async (p, url) => {
  await p.go(url)
  await p.until(`document.querySelector('.track-list button[aria-label^="Play"]')`, 20000, 'the album')
}
const clickText = (p, sel, re) => p.eval(`[...document.querySelectorAll(${JSON.stringify(sel)})].find((b) => ${re}.test(b.textContent)).click()`)

try {
  // ---- the shared header, on every kind of library
  await section('header', async (p) => {
    const r = {}
    await p.go(`${server}/library/${testLib.id}?tab=albums`)
    await p.until(`document.querySelector('.album-grid .mh-album')`, 15000, 'albums')
    r.hasHeader = await p.eval(`!!document.querySelector('.mh > .lh .lh-sort')`)
    const order = `[...document.querySelectorAll('.album-grid .mh-album b')].map((b) => b.textContent)`
    r.titles = await p.eval(order)
    await p.eval(`document.querySelector('.lh-sort .btn.square').click()`)
    await wait(600)
    r.reversed = await p.eval(order)
    r.reversedOk = JSON.stringify([...r.titles].reverse()) === JSON.stringify(r.reversed)
    await shot(p, 'header-albums')
    await p.go(`${server}/library/${testLib.id}?tab=songs`)
    await p.until(`document.querySelector('.track-list li')`, 15000, 'songs')
    const songs = `[...document.querySelectorAll('.track-list .tl-t b')].map((b) => b.textContent)`
    const a = await p.eval(songs)
    await p.eval(`document.querySelector('.lh-sort .btn.square').click()`)
    await wait(800)
    r.songsReversed = JSON.stringify([...a].reverse()) === JSON.stringify(await p.eval(songs))
    await clickText(p, '.lh-sort-btn', '/Length/')
    await wait(800)
    r.byLength = await p.eval(`[...document.querySelectorAll('.track-list .tl-d')].slice(0, 3).map((b) => b.textContent)`)
    r.url = await p.eval('location.search')
    await p.go(`${server}/library/${testLib.id}?tab=artists`)
    await p.until(`document.querySelector('.mh-artist')`, 15000, 'artists')
    r.artistsSort = await p.eval(`[...document.querySelectorAll('.lh-sort-btn')].map((b) => b.textContent)`)
    for (const [name, label] of [['Discs', 'discs'], ['Books', 'books']]) {
      const l = lib(name)
      if (!l) { r[label] = 'no such library here'; continue }
      await p.go(`${server}/library/${l.id}${label === 'discs' ? '?view=all' : ''}`)
      await p.until(`document.querySelector('.lh')`, 15000, label)
      await wait(800)
      await clickText(p, '.lh-sort-btn', '/^Title/')
      await wait(800)
      r[label] = await p.eval(`({ sorts: [...document.querySelectorAll('.lh-sort-btn')].map((b) => b.textContent), direction: !!document.querySelector('.lh-sort .btn.square') })`)
      await shot(p, 'header-' + label)
    }
    return r
  })

  // ---- the queue comes back after a reload, and waits to be asked
  await section('queue', async (p) => {
    const r = {}
    await playAlbum(p, albumUrl('Generated Artist', 'Generated Album'))
    await p.eval(`document.querySelectorAll('.track-list button[aria-label^="Play"]')[1].click()`)
    await p.until(`${AUDIO}?.currentTime > 0.3`, 15000, 'the music')
    await p.eval(`${AUDIO}.currentTime = 20`)
    await wait(1500)
    await p.eval(`document.querySelector('[aria-label="Shuffle"]').click()`)
    await p.eval(`document.querySelector('[aria-label="Repeat off"]').click()`)
    await wait(5500) // the place is saved every few seconds, and when the page goes
    r.before = await p.eval(`({ title: document.querySelector('.audio-bar .ab-title').textContent, t: ${AUDIO}.currentTime })`)
    const fetches = `performance.getEntriesByType('resource').filter((e) => /\\/stream/.test(e.name)).length`
    await p.go(`${server}/library/${testLib.id}`)
    await p.until(`document.querySelector('.audio-bar')`, 10000, 'the bar')
    await wait(800)
    r.after = await p.eval(`({ title: document.querySelector('.audio-bar .ab-title').textContent, time: document.querySelector('.ab-time')?.textContent, playing: [...document.querySelectorAll('audio')].some((a) => !a.paused), sources: [...document.querySelectorAll('audio')].filter((a) => a.src).length, streams: ${fetches}, pressed: document.querySelector('[aria-label="Shuffle"]').getAttribute('aria-pressed'), repeat: !!document.querySelector('[aria-label="Repeat all"]') })`)
    await shot(p, 'queue-after-reload')
    await p.eval(`document.querySelector('.audio-bar [aria-label="Play"]').click()`)
    await p.until(`${AUDIO}?.currentTime > 0.2`, 10000, 'it to start again')
    r.resumedAt = await p.eval(`${AUDIO}.currentTime`)
    r.keptQueue = await p.eval(`(document.querySelector('.ab-expand').click(), new Promise((d) => setTimeout(d, 900))).then(() => { document.querySelector('.np-top-right [aria-label="Queue"]').click(); return new Promise((d) => setTimeout(d, 300)).then(() => document.querySelectorAll('.np-queue li').length) })`)
    await p.eval(`document.querySelector('.np-close').click()`)
    await wait(500)
    await p.eval(`document.querySelector('[aria-label="Stop playing and close"]').click()`)
    await wait(300)
    await p.go(`${server}/library/${testLib.id}`)
    await wait(1500)
    r.stoppedStaysGone = await p.eval(`!document.querySelector('.audio-bar')`)
    return r
  })

  // ---- Slow, and shuffling across every effect
  await section('visual', async (p) => {
    const r = {}
    await call('PUT', '/me/ui', {}, token) // as someone who has never seen the notice
    await playAlbum(p, albumUrl('Generated Artist', 'Generated Album'))
    await p.eval(`document.querySelector('.track-list button[aria-label^="Play"]').click()`)
    await p.until(`${AUDIO}?.currentTime > 0.3`, 15000, 'the music')
    await p.eval(`document.querySelector('.ab-expand').click()`)
    await p.until(`document.querySelector('.np-canvas')`, 5000, 'the bars')
    await wait(1500)
    r.drawsPerSecond = await p.eval('window.__rate()')
    await clickText(p, '.np-chip', '/Slow/')
    await wait(500)
    r.drawsPerSecondSlow = await p.eval('window.__rate()')
    await noMotion(p)
    await shot(p, 'bars-slow')
    await clickText(p, '.np-chip', '/Slow/')
    // Accept the notice once, with an effect from the menu, then back to Bars.
    await p.eval(`document.querySelector('[aria-label="More visualizer styles"]').click()`)
    await p.until(`document.querySelectorAll('.np-style-menu button').length > 2`, 15000, 'effects')
    r.effects = await p.eval(`document.querySelectorAll('.np-style-menu button').length - 1`)
    await clickText(p, '.np-style-menu button', '/Molten Glass/')
    await p.until(`document.querySelector('.wv-dialog')`, 10000, 'the notice')
    await clickText(p, '.wv-dialog button', '/Turn on/')
    await p.until(`document.querySelector('.np-shader .wv-stage')?.dataset.fps > 0`, 15000, 'the shader')
    // Slow reaches a shader too: its calm mode, saved with its settings.
    await clickText(p, '.np-chip', '/Slow/')
    await wait(1200)
    r.shaderCalm = (await call('GET', '/me/ui', null, token)).data.visualizer?.calm
    await clickText(p, '.np-chip', '/Slow/')
    await wait(800)
    await clickText(p, '.np-styles > button', '/Bars/')
    await wait(500)
    // Shuffle: every 15 seconds, among every effect.
    await p.eval(`document.querySelector('.np-chip-pop').click()`)
    await wait(300)
    await clickText(p, '.np-style-menu button', '/Every effect/')
    await wait(200)
    await clickText(p, '.np-style-menu button', '/15 seconds/')
    const seen = []
    const showing = `document.querySelector('.np-shader .wv-stage')?.dataset.effect || document.querySelector('.np-styles [aria-checked=true]')?.textContent`
    const started = Date.now()
    while (Date.now() - started < 50000) {
      const v = await p.eval(showing)
      if (seen[seen.length - 1]?.[0] !== v) seen.push([v, Math.round((Date.now() - started) / 1000)])
      await wait(250)
    }
    r.shuffleSeen = seen
    r.reachedShader = seen.some(([v]) => !['Bars', 'Waves', 'Drift'].includes(v))
    r.stillPlaying = await p.eval(`!${AUDIO}.paused`)
    const ui = (await call('GET', '/me/ui', null, token)).data
    r.savedStyleUnchanged = ui.musicPlayer?.style
    await p.eval(`document.querySelector('.np-chip-pop').click()`)
    await wait(300)
    await clickText(p, '.np-style-menu button', '/^.?Off/')
    await clickText(p, '.np-styles > button', '/Bars/')
    await wait(800)
    return r
  })

  // ---- reduced motion
  await section('calm', async (p) => {
    const r = {}
    await playAlbum(p, albumUrl('Generated Artist', 'Generated Album'))
    await p.eval(`document.querySelector('.track-list button[aria-label^="Play"]').click()`)
    await p.until(`${AUDIO}?.currentTime > 0.3`, 15000, 'the music')
    await p.eval(`document.querySelector('.ab-expand').click()`)
    await p.until(`document.querySelector('.np-canvas')`, 5000, 'the bars')
    await wait(1000)
    r.drawsPerSecond = await p.eval('window.__rate()')
    r.noOpeningMove = await p.eval(`getComputedStyle(document.querySelector('.np')).animationName`)
    r.slowPressed = await p.eval(`[...document.querySelectorAll('.np-chip')].find((b) => /Slow/.test(b.textContent)).getAttribute('aria-pressed')`)
    await clickText(p, '.np-chip', '/Lyrics/')
    await wait(800)
    r.lineGrows = await p.eval(`getComputedStyle(document.querySelector('.np-lines button') || document.body).transform`)
    await clickText(p, '.np-chip', '/Karaoke/')
    await wait(800)
    r.karaokeOpens = await p.eval(`getComputedStyle(document.querySelector('.kar')).animationName`)
    await shot(p, 'calm-karaoke')
    return r
  }, { media: [{ name: 'prefers-reduced-motion', value: 'reduce' }] })

  // ---- ReplayGain from tags in a test file
  await section('replaygain', async (p) => {
    const r = {}
    await playAlbum(p, albumUrl('Signal Test', 'Signals'))
    await p.eval(`document.querySelectorAll('.track-list button[aria-label^="Play"]')[0].click()`)
    await p.until(`${AUDIO}?.currentTime > 0.3`, 15000, 'the music')
    const id = await p.eval(`Number(${AUDIO}.src.match(/items\\/(\\d+)/)[1])`)
    r.tags = (await call('GET', `/music/loudness/${id}`, null, token)).data
    await p.eval(`document.querySelector('.ab-expand').click()`)
    await p.until(`document.querySelector('.np')`, 5000, 'the player')
    await p.eval(`document.querySelector('[aria-label="More"]').click()`)
    await wait(300)
    await clickText(p, '.mp-menu button', '/Sound/')
    await wait(400)
    const pick = (v) => p.eval(`(() => { const s = [...document.querySelectorAll('.np-sound select')].find((x) => [...x.options].some((o) => o.value === 'track')); Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set.call(s, ${JSON.stringify(v)}); s.dispatchEvent(new Event('change', { bubbles: true })) })()`)
    // Something has to bring the graph in first, as nothing was using it.
    await pick('track')
    await wait(1500)
    r.track = await p.eval('window.__level()')
    await pick('album')
    await wait(1000)
    r.album = await p.eval('window.__level()')
    await p.eval(`document.querySelector('[aria-label="Next"]').click()`)
    await wait(2000)
    r.title = await p.eval(`document.querySelector('.np-title h2').textContent`)
    r.noTagLevel = await p.eval('window.__level()')
    await pick('off')
    await wait(800)
    r.offLevel = await p.eval('window.__level()')
    const db = (x) => Math.round(20 * Math.log10(x / r.noTagLevel) * 10) / 10
    r.trackDbBelowNoTag = db(r.track)
    r.albumDbBelowNoTag = db(r.album)
    return r
  })

  // ---- the booklet
  await section('booklet', async (p) => {
    const r = {}
    await playAlbum(p, albumUrl('Signal Test', 'Signals'))
    await p.eval(`document.querySelectorAll('.track-list button[aria-label^="Play"]')[0].click()`)
    await p.until(`${AUDIO}?.currentTime > 0.3`, 15000, 'the music')
    await p.eval(`document.querySelector('.ab-expand').click()`)
    await p.until(`document.querySelector('.np-chip')`, 5000, 'the player')
    await clickText(p, '.np-chip', '/Read/')
    await p.until(`document.querySelector('.np-extras a')`, 5000, 'the booklet link')
    const href = await p.eval(`document.querySelector('.np-extras a').href`)
    r.target = await p.eval(`document.querySelector('.np-extras a').target`)
    const res = await fetch(href, { headers: { Authorization: 'Bearer ' + token } })
    const bytes = Buffer.from(await res.arrayBuffer())
    r.status = res.status; r.type = res.headers.get('content-type'); r.magic = bytes.slice(0, 5).toString(); r.csp = res.headers.get('content-security-policy')
    const tab = await fetch(href)
    r.withTokenInAddress = tab.status
    const bare = await fetch(href.replace(/[?&]token=[^&]*/, ''))
    r.withoutSignIn = bare.status
    return r
  })

  // ---- a CD image, in a music library and a disc library
  await section('cd', async (p) => {
    const r = {}
    await p.go(`${server}/library/${testLib.id}?tab=albums`)
    await p.until(`document.querySelector('.album-grid .mh-album')`, 15000, 'albums')
    r.albums = await p.eval(`[...document.querySelectorAll('.album-grid .mh-album b')].map((b) => b.textContent)`)
    await playAlbum(p, albumUrl('A Made Up Band', 'A Made Up Disc'))
    r.tracks = await p.eval(`[...document.querySelectorAll('.track-list .tl-t b')].map((b) => b.textContent)`)
    r.insert = await p.eval(`!!document.querySelector('a[href^="/cd/"]')`)
    await p.eval(`document.querySelector('a[href^="/cd/"]').click()`)
    await p.until(`location.pathname.startsWith('/cd/')`, 5000, 'the CD player')
    await p.until(`${AUDIO}?.currentTime > 0.5`, 20000, 'the disc to play')
    r.playing = await p.eval(`({ t: ${AUDIO}.currentTime, paused: ${AUDIO}.paused, decks: document.querySelectorAll('audio').length })`)
    await noMotion(p)
    await shot(p, 'cd-player')
    await p.go(`${server}/library/${testLib.id}`)
    await wait(1500)
    await shot(p, 'cd-after-leaving')
    r.afterReload = await p.eval(`({ bar: !!document.querySelector('.audio-bar') })`)
    // In the bar, while browsing.
    await playAlbum(p, albumUrl('A Made Up Band', 'A Made Up Disc'))
    await p.eval(`document.querySelector('a[href^="/cd/"]').click()`)
    await p.until(`${AUDIO}?.currentTime > 0.5`, 20000, 'the disc to play')
    await p.eval(`document.querySelector('.sidebar a[href="/"]').click()`)
    await p.until(`document.querySelector('.audio-bar .ab-art.cd')`, 5000, 'the disc in the bar')
    r.barShowsDisc = await p.eval(`document.querySelector('.audio-bar .ab-title').textContent`)
    await p.eval(`document.querySelector('.audio-bar [aria-label="Next"]').click()`)
    await wait(1500)
    r.nextTrack = await p.eval(`({ title: document.querySelector('.audio-bar .ab-title').textContent, t: ${AUDIO}.currentTime })`)
    await p.eval(`document.querySelector('.ab-expand').click()`)
    await wait(600)
    r.expandGoesToDisc = await p.eval('location.pathname')
    await p.eval(`(document.querySelector('[aria-label="Stop playing and close"]') || { click() {} }).click()`)
    return r
  })

  // ---- an HLS radio station, from the directory
  await section('hls', async (p) => {
    const r = {}
    const radio = lib('Radio')
    // Chrome now plays HLS itself, and so does Safari. Others get hls.js, so that's what's tried.
    await p.send('Page.addScriptToEvaluateOnNewDocument', { source: `const can = HTMLMediaElement.prototype.canPlayType; HTMLMediaElement.prototype.canPlayType = function (t) { return /mpegurl/i.test(t) ? '' : can.call(this, t) }` })
    const found = (await call('GET', '/radio/directory/search?q=news&limit=100', null, token)).data
    const hls = (Array.isArray(found) ? found : found.stations || []).filter((s) => s.hls && s.working !== false).slice(0, 6)
    r.candidates = hls.length
    for (const s of hls) {
      const added = await call('POST', '/radio', { libraryId: radio.id, name: 'HLS test: ' + s.name.slice(0, 40), url: s.url }, token)
      if (added.status >= 300) continue
      await p.go(`${server}/library/${radio.id}`)
      await p.until(`document.querySelector('.rd-play')`, 10000, 'the station')
      const label = 'Play HLS test: ' + s.name.slice(0, 40)
      await p.eval(`document.querySelector(${JSON.stringify(`[aria-label="${label.replace(/"/g, '\\"')}"]`)})?.click() ?? [...document.querySelectorAll('.rd-play')].pop().click()`)
      const ok = await p.until(`${AUDIO}?.currentTime > 1`, 25000, 'the stream').then(() => true).catch(() => false)
      if (!ok) { r.tried = (r.tried || 0) + 1; await call('DELETE', `/radio/${added.data.id}`, null, token); continue }
      r.station = s.name
      r.state = await p.eval(`({ blobSource: ${AUDIO}.src.startsWith('blob:'), t: ${AUDIO}.currentTime, hlsLoaded: performance.getEntriesByType('resource').some((e) => /hls/.test(e.name)) })`)
      await p.eval(`document.querySelector('.ab-expand').click()`)
      await p.until(`document.querySelector('.np-canvas')`, 5000, 'the player')
      await wait(1500)
      r.drawsPerSecond = await p.eval('window.__rate(2000)')
      await noMotion(p)
      await shot(p, 'hls-full-player')
      r.playingWithPlayerOpen = await p.eval(`({ paused: ${AUDIO}.paused, t: ${AUDIO}.currentTime })`)
      await p.eval(`document.querySelector('.np-close').click()`)
      await wait(400)
      await p.eval(`document.querySelector('.audio-bar [aria-label="Pause"]').click()`)
      await wait(500)
      r.pauseHangsUp = await p.eval(`!document.querySelector('audio').getAttribute('src') && ![...document.querySelectorAll('audio')].some((a) => a.src && !a.paused)`)
      await p.eval(`document.querySelector('.audio-bar [aria-label="Play"]').click()`)
      r.playsAgain = await p.until(`${AUDIO}?.currentTime > 0.5`, 20000, 'it to start again').then(() => true).catch(() => false)
      await p.eval(`document.querySelector('[aria-label="Stop playing and close"]').click()`)
      break
    }
    return r
  })

  // ---- a podcast home, with a show followed
  await section('podcast', async (p) => {
    const r = {}
    const pods = lib('Podcasts')
    const have = (await call('GET', `/music/albums?library=${pods.id}`, null, token)).data
    if (!have.length) {
      const hits = (await call('GET', '/podcasts/search?q=science', null, token)).data
      const hit = (Array.isArray(hits) ? hits : hits.results || [])[0]
      r.show = hit?.title || hit?.name
      const added = await call('POST', '/podcasts', { library: pods.id, url: hit.feed || hit.feedUrl || hit.url }, token)
      r.followed = added.status
      if (added.status >= 300) throw new Error('following: ' + JSON.stringify(added.data).slice(0, 200))
    }
    await p.go(`${server}/library/${pods.id}`)
    await p.until(`document.querySelector('.mh-hero')`, 30000, 'the podcast home')
    await wait(1000)
    await noMotion(p)
    await shot(p, 'podcast-home')
    r.hero = await p.eval(`document.querySelector('.mh-hero-text h2').textContent`)
    r.actions = await p.eval(`[...document.querySelectorAll('.mh-hero-actions button')].map((b) => b.textContent)`)
    await clickText(p, '.mh-hero-actions button', '/Play this show/')
    await p.until(`${AUDIO}?.currentTime > 1`, 30000, 'an episode')
    r.playing = await p.eval(`({ t: ${AUDIO}.currentTime, title: document.querySelector('.audio-bar .ab-title').textContent })`)
    await p.eval(`document.querySelector('.ab-expand').click()`)
    await p.until(`document.querySelector('.np')`, 5000, 'the player')
    await wait(600)
    r.player = await p.eval(`({ label: document.querySelector('.np-label').textContent, transport: [...document.querySelectorAll('.np-transport button')].map((b) => b.getAttribute('aria-label')), like: !!document.querySelector('.np-top-right [aria-label="Add to liked songs"]') })`)
    await p.eval(`${AUDIO}.currentTime = 60`)
    await wait(16500) // the place is saved every 15 seconds
    await p.eval(`document.querySelector('.np-close').click()`)
    await wait(300)
    await p.go(`${server}/library/${pods.id}`)
    await p.until(`document.querySelector('.mh-hero')`, 15000, 'the podcast home')
    await wait(800)
    r.continueShelf = await p.eval(`document.querySelectorAll('.mh-resume').length`)
    await shot(p, 'podcast-continue')
    await p.eval(`(document.querySelector('[aria-label="Stop playing and close"]') || { click() {} }).click()`)
    return r
  })

  // ---- the bar and the tab bar together on a phone
  await section('phone', async (p) => {
    const r = {}
    await playAlbum(p, albumUrl('Generated Artist', 'Generated Album'))
    await p.eval(`document.querySelector('.track-list button[aria-label^="Play"]').click()`)
    await p.until(`${AUDIO}?.currentTime > 0.3`, 15000, 'the music')
    await p.go(`${server}/library/${lib('Real music').id}`)
    await p.until(`document.querySelector('.mh-hero') && document.querySelector('.audio-bar')`, 15000, 'home and the bar')
    await noMotion(p)
    await wait(500)
    const geometry = `(() => { const bar = document.querySelector('.audio-bar').getBoundingClientRect(), tabs = document.querySelector('.tabbar').getBoundingClientRect(); return { barTop: bar.top, barBottom: bar.bottom, barLeft: bar.left, barRight: bar.right, tabTop: tabs.top, tabHeight: tabs.height, gap: tabs.top - bar.bottom, tabbarSpace: getComputedStyle(document.documentElement).getPropertyValue('--tabbar-space').trim(), vw: innerWidth, sideways: document.documentElement.scrollWidth > innerWidth } })()`
    r.geometry = await p.eval(geometry)
    await shot(p, 'phone-home-top')
    await p.eval(`window.scrollTo(0, document.documentElement.scrollHeight)`)
    await wait(500)
    await shot(p, 'phone-home-bottom')
    r.lastThingClear = await p.eval(`(() => { const kids = [...document.querySelectorAll('.content *')].filter((e) => e.getBoundingClientRect().height > 0); const last = Math.max(...kids.map((e) => e.getBoundingClientRect().bottom)); return { lastBottom: last, barTop: document.querySelector('.audio-bar').getBoundingClientRect().top } })()`)
    // Toasts and the like are kept clear of both.
    await p.eval(`document.querySelector('.audio-bar .phone-only').click()`)
    await p.until(`document.querySelector('.np')`, 5000, 'the player')
    await noMotion(p)
    await wait(600)
    await shot(p, 'phone-full-player')
    r.fullCoversTabs = await p.eval(`(() => { const t = document.querySelector('.tabbar')?.getBoundingClientRect(); const at = t ? document.elementFromPoint(t.left + 20, t.top + 10) : null; return !!at?.closest('.np') })()`)
    await p.eval(`document.querySelector('.np-close').click()`)
    await wait(500)
    // Collapsed: still above the tab bar.
    await size(p, 800, 900)
    await wait(300)
    await p.eval(`document.querySelector('[aria-label="Collapse the player"]').click()`)
    await size(p, 375, 812, true)
    await wait(400)
    r.collapsed = await p.eval(geometry)
    await p.eval(`document.querySelector('.audio-bar .ab-art').click()`)
    await wait(300)
    // Between phone and desktop: no tab bar there, and nothing kept for one.
    await size(p, 800, 900)
    await wait(400)
    r.tablet = await p.eval(`({ tabbarShown: getComputedStyle(document.querySelector('.tabbar')).display !== 'none', bottomGap: innerHeight - document.querySelector('.audio-bar').getBoundingClientRect().bottom, sideways: document.documentElement.scrollWidth > innerWidth })`)
    await shot(p, 'between-sizes')
    await p.eval(`document.querySelector('[aria-label="Stop playing and close"]')?.click()`)
    return r
  }, { width: 375, height: 812, mobile: true })
} finally {
  writeFileSync(join(out, 'results.json'), JSON.stringify(results, null, 1))
  await b.close()
}
