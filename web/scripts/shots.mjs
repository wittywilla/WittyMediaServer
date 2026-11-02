// Takes the pictures in the docs and on the GitHub pages, from a test server filled from
// D:\WittyDemoMedia. No window opens. WMS_ONLY=dvd,games takes only those scenes, and without it
// every scene runs. A scene that fails is reported, and the others still run.
//
//   WMS_SERVER=http://demo-media.localhost:19701 WMS_PASSWORD=... node scripts/shots.mjs out-folder
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const [out = 'shots'] = process.argv.slice(2)
const only = process.env.WMS_ONLY ? process.env.WMS_ONLY.split(',') : null
const s = await open(server, out, { path: '/help' })
const { page, press, shot, call, go, until, size, maskAddresses } = s
const login = await call('POST', '/login', { name: process.env.WMS_USER || 'owner', password: process.env.WMS_PASSWORD })
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)
await call('POST', '/legal/acknowledge')

const libs = async () => { const l = await call('GET', '/libraries'); return l.libraries || l }
const lib = async (name) => (await libs()).find((l) => l.name === name)
const openLib = async (name, settle = 3500) => { const l = await lib(name); await go(`/library/${l.id}`, settle); return l }
const items = async (name) => {
  const l = await lib(name)
  const r = await call('GET', `/items?library=${l.id}&limit=3000`)
  return r.items || r
}
const item = async (name, rx) => (await items(name)).find((i) => rx.test(i.title))
const shows = async (name) => { const l = await lib(name); const r = await call('GET', `/shows?library=${l.id}&limit=500`); return r.shows || r.items || r }
const setTheme = async (theme) => {
  await call('PUT', '/settings', { theme })
  await page.eval(`(async () => { const t = localStorage.getItem('wms.token'); await fetch('/api/me/preferences', { method: 'PUT', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + t }, body: JSON.stringify({ theme: ${JSON.stringify(theme)} }) }).catch(() => {}) })()`)
}
const move = async (x = 900, y = 600) => { await page.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x, y }); await wait(300) }
const key = async (k, code = k, windowsVirtualKeyCode = 0) => {
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, windowsVirtualKeyCode, text: k === 'Enter' ? '\r' : undefined })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code, windowsVirtualKeyCode })
}
const hasVideo = (ms = 60000) => until(`[...document.querySelectorAll('video')].some((v) => v.readyState >= 2 && v.currentTime > 0.5 && v.videoWidth > 0)`, ms, 'the video to play')
const seek = (t) => page.eval(`(() => { const v = document.querySelector('video'); if (v) v.currentTime = ${t} })()`)
const playerGone = async () => { await key('Escape', 'Escape', 27); await wait(400); await press(/Stop playing and close/); await wait(600) }

const scenes = {
  async home() {
    await go('/', 4000)
    await shot('home')
    await page.eval(`window.scrollTo(0, 900)`)
    await wait(1500)
    await shot('home-shelves')
  },
  async movies() {
    await openLib('Movies', 5000)
    await shot('movies')
    const it = await item('Movies', /Sintel/)
    await go(`/item/${it.id}`, 3000)
    await shot('film-page')
    await press(/^\s*Play/); await wait(1500)
    await hasVideo()
    await seek(210)
    await until(`(() => { const v = document.querySelector('video'); return v && v.currentTime > 205 && v.readyState >= 3 })()`, 60000, 'the seek').catch((e) => console.log(e.message))
    await wait(4000)
    await move()
    await shot('player')
  },
  async tv() {
    await openLib('TV Shows', 5000)
    await shot('tv-shows')
    const all = await shows('TV Shows')
    const es = all.find((x) => /ESOcast|Hubblecast/.test(x.title)) || all[0]
    await go(`/show/${es.id}`, 3000)
    await shot('tv-show-page')
    await openLib('Anime', 4000)
    await shot('anime')
  },
  async discs() {
    await openLib('Discs', 5000)
    await shot('discs')
    const it = await item('Discs', /^Sintel$/)
    await go(`/item/${it.id}`, 3000)
    await shot('dvd-item')
    await press(/Insert DVD/)
    await wait(1800)
    await shot('dvd-tray')
    await wait(26000)
    await shot('dvd-menu')
  },
  async bluray() {
    const refused = await item('Discs', /^Big Buck Bunny$/)
    await go(`/item/${refused.id}`, 3000)
    await shot('bluray-refused')
    const it = await item('Discs', /^Sita Sings the Blues$/)
    await go(`/item/${it.id}`, 3000)
    await shot('bluray-item')
    await press(/Insert Blu-ray/)
    await wait(45000)
    await move()
    await shot('bluray-menu')
    console.log((await page.eval(`document.body.innerText`)).slice(0, 200))
  },
  async cd() {
    const it = (await items('Discs')).find((i) => i.kind === 'cd-image') || (await items('Music')).find((i) => i.kind === 'cd-image')
    await go(`/cd/${it.id}?play=1`, 12000)
    await shot('cd-player')
  },
  async music() {
    const l = await openLib('Music', 4000)
    await shot('music-home')
    await press(/^\s*Albums/); await wait(2000)
    await press(/^\s*Artist\s*$/); await wait(3000)
    await shot('music-albums')
    await press(/^\s*Artists/); await wait(3000)
    await shot('music-artists')
    const song = (await items('Music')).find((i) => /^Collide$/.test(i.album || ''))
    await go(`/album/${l.id}?album=${encodeURIComponent(song.album)}&artist=${encodeURIComponent(song.albumArtist || song.artist || '')}`, 3000)
    await press(/^\s*Play\b/)
    await wait(5000)
    await press(/Show the full player/)
    await wait(3000)
    await shot('music-full')
    // It remembers being on, so pressing it blindly could turn it off.
    await page.eval(`(() => { const b = document.querySelector('.np-viz-toggle'); if (b && b.getAttribute('aria-pressed') !== 'true') b.click() })()`)
    await wait(1000)
    await press(/^\s*Turn on\s*$/)
    await wait(6000)
    await shot('music-visualizer-bars')
    await press(/^\s*Drift/); await wait(7000)
    await shot('music-visualizer-drift')
    let n = 0
    for (const name of ['Warp Field', 'Molten Glass', 'Northern Veil']) {
      await press(/More visualizer styles/); await wait(1000)
      await page.eval(`[...document.querySelectorAll('.np-more-styles button, [role=menu] button, [role=dialog] button')].find((b) => b.textContent.trim() === ${JSON.stringify(name)})?.click()`)
      await wait(1000)
      await press(/^\s*Turn on\s*$/)
      await wait(9000)
      n++
      await shot(n === 1 ? 'music-visualizer' : `music-visualizer-${n}`)
    }
    await key('Escape', 'Escape', 27)
    await playerGone()
  },
  async lyrics() {
    const l = await lib('Music')
    await go(`/library/${l.id}`, 2500)
    await page.eval(`(() => { const i = document.querySelector('input[type=search], input[placeholder*="Search"]'); if (i) i.value = '' })()`)
    const r = await call('GET', `/items?library=${l.id}&limit=3000&search=${encodeURIComponent('Take Me Out')}`)
    const song = (r.items || r)[0]
    console.log('song', song && song.title, song && song.id)
    await page.eval(`window.__wmsDemoSong = ${song.id}`)
    await go(`/album/${l.id}?album=${encodeURIComponent(song.album)}&artist=${encodeURIComponent(song.albumArtist || song.artist || '')}`, 3000)
    await shot('music-album')
    await press(/Play Take Me Out/)
    await wait(4000)
    await press(/Show the full player/)
    await wait(1500)
    await page.eval(`(() => { const a = document.querySelector('audio'); if (a) a.currentTime = 42 })()`)
    await press(/Lyrics \(L\)/); await wait(4000)
    await shot('music-lyrics')
    await press(/Karaoke \(K\)/); await wait(4000)
    await shot('music-karaoke')
    await playerGone()
  },
  async audio() {
    await openLib('Audiobooks', 4000)
    await shot('audiobooks')
    await openLib('Podcasts', 4000)
    await shot('podcasts')
  },
  async reading() {
    await openLib('Books', 4000)
    await shot('books')
    const book = await item('Books', /Alice/)
    await go(`/read/${book.id}`, 7000)
    for (let i = 0; i < 9; i++) { await key('ArrowRight', 'ArrowRight', 39); await wait(500) }
    await wait(2500)
    await shot('reader-epub')
    const pdf = await item('PDFs', /Earth as Art/)
    await go(`/read/${pdf.id}`, 9000)
    await page.eval(`(() => { const e = [...document.querySelectorAll('*')].filter((x) => x.scrollHeight > x.clientHeight + 500 && /auto|scroll/.test(getComputedStyle(x).overflowY)).sort((x, y) => y.scrollHeight - x.scrollHeight)[0]; if (e) e.scrollTop = e.scrollHeight * 0.3 })()`)
    await wait(7000)
    await shot('reader-pdf')
    for (const [i, f] of [[1, 0.2], [2, 0.35], [3, 0.6]]) {
      await page.eval(`(() => { const e = [...document.querySelectorAll('*')].filter((x) => x.scrollHeight > x.clientHeight + 500 && /auto|scroll/.test(getComputedStyle(x).overflowY)).sort((a, b) => b.scrollHeight - a.scrollHeight)[0]; if (e) e.scrollTop = e.scrollHeight * ${f} })()`)
      await wait(6000)
      await shot(`reader-pdf-${i}`)
    }
    await openLib('Comics', 4000)
    await shot('comics')
    await page.eval(`window.scrollTo(0, 0)`)
    const comic = (await items('Comics')).find((i) => /Pepper&Carrot 0?6\b|Pepper&Carrot 06/.test(i.title)) || (await items('Comics'))[0]
    await go(`/read/${comic.id}`, 6000)
    await key('ArrowRight', 'ArrowRight', 39); await wait(2500)
    await shot('reader-comic')
  },
  async photos() {
    await openLib('Photos', 6000)
    await page.eval(`window.scrollTo(0, 3000)`); await wait(5000)
    await page.eval(`window.scrollTo(0, 0)`); await wait(9000)
    await shot('photos')
    await press(/^\s*Map/); await wait(7000)
    await shot('photos-map')
    await press(/^\s*Timeline/); await wait(2500)
    await page.eval(`document.querySelector('main img, .photo img, img[loading]')?.closest('button,a')?.click()`)
    await wait(4000)
    await shot('photos-viewer')
  },
  async other() {
    await openLib('Vertical Videos', 5000)
    await wait(3000)
    await shot('shorts')
    await openLib('Creators', 4000)
    await shot('creators')
    await openLib('Music Videos', 4000)
    await shot('music-videos')
    await openLib('Home Videos', 4000)
    await shot('home-videos')
    await go('/collections', 3000)
    await shot('collections')
  },
  async stats() {
    const film = await item('Movies', /^Sintel/)
    const other = await s.browser.open(`${server}/item/${film.id}`)
    await wait(3500)
    await other.eval(`[...document.querySelectorAll('button, a')].find((b) => /^\\s*Play\\b/.test(b.textContent))?.click()`)
    await wait(14000)
    console.log('other', await other.eval(`(() => { const v = document.querySelector('video'); return v ? [v.currentTime, v.paused] : document.body.innerText.slice(0, 200) })()`))
    await page.send('Page.bringToFront')
    await go('/m/wittystats/stats', 4000)
    await until(`!/Waiting for the first/.test(document.body.innerText)`, 60000, 'the first readings').catch((e) => console.log(e.message))
    await other.eval(`(() => { const v = document.querySelector('video'); if (v) { v.muted = true; v.play() } return v ? [v.currentTime, v.paused] : 'no video' })()`).then(console.log)
    await until(`/Sintel/.test(document.body.innerText)`, 30000, 'Sintel to show as playing').catch((e) => console.log(e.message))
    await wait(2000)
    await page.eval(`[...document.querySelectorAll('*')].filter((e) => e.children.length === 0 && /^\\/(usr|init|run|snap|boot|mnt\\/wsl)/.test(e.textContent.trim())).forEach((e) => { let r = e; while (r.parentElement && !/Used/.test(r.innerText)) r = r.parentElement; r.style.display = 'none' })`)
    await wait(500)
    await shot('stats')
    await other.send('Page.close').catch(() => {})
    await page.send('Page.bringToFront')
  },
  async pages() {
    await go('/couch', 3000); await shot('couch-page')
    await go('/controllers', 3000); await shot('controllers')
    await go('/search?q=moon', 4000); await shot('search')
    await go('/fix-matches', 3000); await shot('fix-matches')
    await go('/review', 3000); await shot('review')
    await go('/help/first-time-setup', 3000); await shot('help-setup-guide')
    const m = await call('GET', '/collections')
    const c = (m.collections || m).find((x) => /Silent/.test(x.name))
    if (c) { await go(`/collections/${c.id}`, 3500); await shot('collection-page') }
    const clip = (await items('Home Videos').catch(() => []))[0]
    if (clip) { await go(`/item/${clip.id}`, 3000); await shot('home-video-page') }
    const book = (await items('Audiobooks').catch(() => []))[0]
    if (book) { await go(`/item/${book.id}`, 3000); await shot('audiobook-page') }
    const album = (await items('Photos').catch(() => []))[0]
    void album
  },
  async tvguide() {
    const look = async (theme) => {
      await page.eval(`localStorage.setItem('wms.tv.look.${login.user.id}', ${JSON.stringify(JSON.stringify({ theme }))})`)
      await call('PUT', '/me/ui', { tvLook: { theme } })
    }
    // How bright the picture is, so a fade to black or an end card doesn't end up in the docs.
    const shown = () => page.eval(`(() => {
      const v = document.querySelector('video')
      if (!v || v.readyState < 2 || !v.videoWidth || v.paused || v.currentTime < 1) return 0
      const c = document.createElement('canvas'); c.width = 32; c.height = 18
      const x = c.getContext('2d'); x.drawImage(v, 0, 0, 32, 18)
      const d = x.getImageData(0, 0, 32, 18).data
      let sum = 0, lo = 255, hi = 0
      for (let i = 0; i < d.length; i += 4) { const y = (d[i] + d[i + 1] + d[i + 2]) / 3; sum += y; if (y < lo) lo = y; if (y > hi) hi = y }
      return hi - lo < 40 ? 0 : sum / (d.length / 4)
    })()`)
    const FILMS = /Sintel|Big Buck|Tears of Steel|Sita Sings|Spring|Caminandes|Cosmos|Agent 327|Hero|Coffee Run|Glass Half/i
    const toFilm = async () => {
      for (let i = 0; i < 15; i++) {
        const now = await page.eval(`document.querySelector('.ctv-hero-title')?.getAttribute('aria-label') || document.querySelector('.ctv-hero-title')?.textContent || ''`)
        if (FILMS.test(now)) return now
        await key('ArrowDown', 'ArrowDown', 40)
        await wait(250)
      }
      return ''
    }
    const playing = async (name, tries = 8) => {
      for (let i = 0; i < tries; i++) {
        for (let w = 0; w < 25; w++) {
          const b = await shown()
          if (b > 45) { await wait(1500); await shot(name); console.log(name, 'video brightness', Math.round(b), 'channel', i); return true }
          await wait(1000)
        }
        await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'ArrowDown', code: 'ArrowDown', windowsVirtualKeyCode: 40 })
        await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'ArrowDown', code: 'ArrowDown', windowsVirtualKeyCode: 40 })
        await wait(500)
      }
      console.log(name, 'NO VIDEO')
      await shot(`failed-${name}`)
      return false
    }
    for (const theme of ['modern', 'classic', 'satellite', 'teletext', 'listings', 'bigpicture']) {
      await look(theme)
      await go('/tv', 3000)
      console.log(theme, await toFilm())
      await playing(`wittytv-${theme}`)
    }
    await look('modern')
    await go('/tv', 3000)
    console.log(await toFilm())
    await playing('wittytv-guide')
    await page.eval(`document.querySelector('.ctv-preview-bar button[title="Full screen"]').click()`)
    await wait(3000)
    await playing('wittytv-watching', 2)
    await move()
    await wait(800)
    await shot('wittytv-watching-info')
    await key('g', 'KeyG', 71)
    await wait(1500)
    await shot('wittytv-miniguide')
    await go('/tv/recordings', 2500)
    await shot('wittytv-recordings')
    await go('/settings/channels/new', 2500)
    await shot('wittytv-new-channel')
  },
  async games() {
    await openLib('Games', 5000)
    await shot('games')
    await go('/settings/gaming', 3000)
    await shot('settings-consoles')
  },
  async play() {
    const games = await items('Games')
    const want = [['gameplay-ucity', /City/, [13, 13]], ['gameplay-thwaite', /Thwaite/, [13, 13]], ['gameplay-libbet', /Libbet/, [13]], ['gameplay-dragon-blaster', /Dragon Blaster/, [13]], ['gameplay-datastorm', /Data ?Storm/, [13]], ['gameplay-rhde', /RHDE/, [13]], ['gameplay-super-tilt-bro', /Super Tilt/, [13]]]
    for (const [name, rx, keys] of want) {
      const g = games.find((x) => rx.test(x.title))
      if (!g) { console.log('no game', rx); continue }
      await go(`/game/${g.id}`, 1500)
      await hasVideo(50000).catch((e) => console.log(name, e.message))
      await wait(9000)
      for (const code of keys) { await key('Enter', 'Enter', code); await wait(3000) }
      await shot(name)
    }
  },
  // A picture of every game as it runs. WMS_FRAMES says where they go, to make its poster from.
  async gameart() {
    const games = await items('Games')
    const dir = process.env.WMS_FRAMES || out
    for (const g of games) {
      const slug = String(g.title).replace(/\W+/g, '-').replace(/^-|-$/g, '').toLowerCase()
      await go(`/game/${g.id}`, 1000)
      await until(`[...document.querySelectorAll('video')].some((v) => v.readyState >= 2 && v.videoWidth > 0)`, 60000, g.title).catch((e) => console.log('NO PICTURE', g.title, e.message))
      await wait(15000)
      await key('Enter', 'Enter', 13)
      await wait(5000)
      const data = await page.eval(`(() => { const v = document.querySelector('video'); if (!v || !v.videoWidth) return ''; const c = document.createElement('canvas'); c.width = v.videoWidth * 4; c.height = v.videoHeight * 4; const x = c.getContext('2d'); x.imageSmoothingEnabled = false; x.drawImage(v, 0, 0, c.width, c.height); return c.toDataURL('image/png').split(',')[1] })()`)
      console.log(g.title, data ? 'picture' : 'NO PICTURE')
      if (data) (await import('node:fs')).writeFileSync((await import('node:path')).join(dir, `${slug}.png`), Buffer.from(data, 'base64'))
      await shot(`frame-${slug}`)
    }
  },
  async settings() {
    const have = await call('GET', '/users')
    for (const [name, displayName, kind, password] of [['alex', 'Alex', 'adult', 'demo-only-password-77'], ['robin', 'Robin', 'guest', ''], ['kids', 'Kids', 'kids', '']]) {
      if (have.some((u) => u.name === name)) continue
      const body = { name, displayName, password, kind, isAdmin: false }
      if (kind !== 'adult') body.access = { maxAge: kind === 'kids' ? 7 : 13, allowUnrated: false, liveTv: kind === 'kids', libraryChannels: kind === 'kids', tunerChannels: false }
      console.log('profile', name, JSON.stringify(await call('POST', '/users', body)).slice(0, 80))
    }
    await call('DELETE', '/sessions')
    for (const path of ['libraries', 'metadata', 'playback', 'channels', 'couch', 'transcoding', 'library-transcoding', 'remote', 'users', 'security', 'games', 'activity', 'appearance', 'sharing', 'wittyrigs', 'logs', 'diagnoser', 'modules', 'backup', 'repair', 'reset', 'about', 'legal']) {
      await go(`/settings/${path}`, 3500)
      await maskAddresses()
      await shot(`settings-${path}`)
    }
    await go('/settings/logs', 3000)
    await shot('settings-logs-viewer')
    await press(/^\s*What to log/); await wait(1500)
    await shot('settings-logs')
    await go('/settings/gaming', 3500)
    await shot('settings-consoles')
    for (const [name, path] of [['store-module', 'modules/store/wittyplay'], ['store-sources', 'modules/stores']]) {
      await go(`/settings/${path}`, 3500)
      await shot(name)
    }
    await go('/settings/libraries', 2500)
    await press(/Add library/); await wait(1500)
    await press(/^\s*Movies|Movies/, '[role=dialog]'); await wait(800)
    await press(/Browse folders|Browse folder/); await wait(2500)
    await shot('folder-browser')
    await go('/settings/modules', 2500)
    await press(/Browse the store/); await wait(3500)
    await shot('module-store')
    const scanning = await lib('Music')
    await call('POST', `/libraries/${scanning.id}/scan`)
    await go('/', 1500)
    await press(/^Activity|Activity:/); await wait(1500)
    await shot('activity')
    await go('/m/wittystats/stats', 6000)
    await go('/help', 2500)
    await shot('help')
    await go('/help/first-time-setup', 3000)
    await shot('help-guide')
  },
  async themes() {
    await setTheme('light'); await go('/', 4000); await shot('theme-light-home')
    const l = await lib('Movies')
    await go(`/library/${l.id}`, 4000); await shot('theme-light-movies')
    await setTheme('cinema'); await go('/', 4000); await shot('theme-cinema-home')
    await setTheme('paper'); await go(`/library/${l.id}`, 4000); await shot('theme-paper-movies')
    await setTheme('midnight'); await go('/', 4000); await shot('theme-midnight-home')
    await setTheme('witty')
  },
  async phone() {
    await size(390, 844, true, 2)
    await go('/', 3500); await shot('phone-home')
    const l = await lib('Movies')
    await go(`/library/${l.id}`, 3000); await press(/^\s*All films/); await wait(3500); await shot('phone-movies')
    const it = await item('Movies', /Sintel/)
    await go(`/item/${it.id}`, 3000); await shot('phone-film')
    await go('/tv', 3000); await until(`(() => { const v = document.querySelector('.ctv-preview video'); return v && !document.querySelector('.ctv-preview-msg') && v.readyState >= 2 && v.currentTime > 1 })()`, 60000, 'the preview').catch((e) => console.log(e.message)); await wait(2000); await shot('phone-guide')
    const m = await lib('Music')
    await go(`/library/${m.id}`, 3500); await shot('phone-music')
    await size(1920, 1080)
  },
  async big() {
    await go('/big/', 5000)
    await shot('big-screen')
  },
}

try {
  for (const [name, run] of Object.entries(scenes)) {
    if (only && !only.includes(name)) continue
    await go('/', 1200)
    await press(/Stop playing and close/)
    console.log('scene', name)
    try { await run() } catch (e) { console.log('FAILED', name, e.message); await shot(`failed-${name}`).catch(() => {}); process.exitCode = 1 }
  }
} finally {
  page.close()
  await s.browser.close()
}
