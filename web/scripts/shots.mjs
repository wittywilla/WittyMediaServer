// Takes the pictures in the docs and on the GitHub pages, from a test server filled from
// D:\WittyDemoMedia. No window opens. WMS_ONLY=dvd,games takes only those scenes.
//
//   WMS_SERVER=http://demo-media.localhost:19701 WMS_PASSWORD=... node scripts/shots.mjs out-folder
import { open, wait } from './shots-lib.mjs'

const server = process.env.WMS_SERVER
const [out = 'shots'] = process.argv.slice(2)
const only = process.env.WMS_ONLY ? process.env.WMS_ONLY.split(',') : null
const s = await open(server, out, { path: '/help' })
const { page, press, shot, card, call, go, until, size, maskAddresses } = s
const login = await call('POST', '/login', { name: process.env.WMS_USER || 'owner', password: process.env.WMS_PASSWORD })
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)
await call('POST', '/legal/acknowledge')

const libs = async () => { const l = await call('GET', '/libraries'); return l.libraries || l }
const lib = async (name) => (await libs()).find((l) => l.name === name)
const items = async (name, q = '') => {
  const l = await lib(name)
  const r = await call('GET', `/items?library=${l.id}&limit=2000${q}`)
  return r.items || r
}
const item = async (name, rx) => (await items(name)).find((i) => rx.test(i.title))
const setTheme = async (theme) => { await call('PUT', '/settings', { theme }); await page.eval(`localStorage.setItem('wms.theme', ${JSON.stringify(theme)})`) }
const showControls = async () => { await page.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: 900, y: 600 }); await wait(250) }
const key = async (k, code = k, windowsVirtualKeyCode = 0) => {
  await page.send('Input.dispatchKeyEvent', { type: 'keyDown', key: k, code, windowsVirtualKeyCode, text: k === 'Enter' ? '\r' : undefined })
  await page.send('Input.dispatchKeyEvent', { type: 'keyUp', key: k, code, windowsVirtualKeyCode })
}
const hasVideo = (ms = 40000) => until(`[...document.querySelectorAll('video')].some((v) => v.readyState >= 2 && v.currentTime > 0.4 && v.videoWidth > 0)`, ms, 'the video to play')

const scenes = {
  async dvd() {
    const it = await item('Discs', /Sintel/)
    await go(`/item/${it.id}`)
    await shot('dvd-item')
    await press(/Insert DVD/)
    await wait(1500)
    await shot('dvd-tray')
    await wait(24000)
    await shot('dvd-playing')
    console.log(await page.eval(`document.body.innerText.slice(0, 300)`))
  },
  async game() {
    const games = await items('Games')
    console.log(games.map((g) => [g.id, g.title]))
    const g = games.find((x) => /City/i.test(x.title))
    await go(`/game/${g.id}`)
    await wait(20000)
    await shot('game-page')
    console.log(await page.eval(`document.body.innerText.slice(0, 500)`))
    for (const k of ['Enter', 'Enter', 'Enter']) { await key(k, k, 13); await wait(2500) }
    await shot('game-page2')
    console.log(await page.eval(`JSON.stringify([...document.querySelectorAll('video,canvas')].map((v) => [v.tagName, v.width, v.height, v.videoWidth, v.currentTime]))`))
  },
}

try {
  for (const [name, run] of Object.entries(scenes)) {
    if (only && !only.includes(name)) continue
    console.log('scene', name)
    try { await run() } catch (e) { console.log('FAILED', name, e.message); await shot(`failed-${name}`).catch(() => {}); process.exitCode = 1 }
  }
} finally {
  page.close()
  await s.browser.close()
}
