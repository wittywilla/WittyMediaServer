// Controller presses, two ways.
//
// With no arguments: web/tests/pads.test.ts, the real navigation code on a simulated machine.
// Nothing else is needed.
//
// With "browser": real presses in a real game's menu, in a headless browser on a running test
// server, with half the machine's cores busy and frames held back as browsers were seen doing.
// No window opens. It expects a library holding games of the consoles named, with their cores
// installed:
//
//   WMS_SERVER=http://127.0.0.1:9811 WMS_NAME=... WMS_PASS=... node scripts/test-pads.mjs browser [nes n64 ...]
import { spawnSync } from 'node:child_process'
import { rmSync } from 'node:fs'
import { cpus } from 'node:os'
import { fileURLToPath } from 'node:url'
import { Worker } from 'node:worker_threads'
import { build } from 'esbuild'

const [mode, ...consoles] = process.argv.slice(2)

if (mode !== 'browser') {
  const entry = fileURLToPath(new URL('../tests/pads.test.ts', import.meta.url))
  const out = entry.replace(/\.ts$/, '.tmp.mjs')
  await build({ entryPoints: [entry], outfile: out, bundle: true, format: 'esm', platform: 'node', logLevel: 'warning' })
  const r = spawnSync(process.execPath, ['--test', '--test-timeout=30000', '--test-force-exit', out], { stdio: 'inherit' })
  rmSync(out, { force: true })
  process.exit(r.status === 0 ? 0 : 1)
}

const { launch } = await import(new URL((process.env.WITTYVISUALIZER || '../../../WittyVisualizer') + '/tests/cdp.mjs', import.meta.url))
const server = process.env.WMS_SERVER || 'http://127.0.0.1:9811'
const HOLDS = [16, 33, 50, 80] // ms; 33 is a very quick tap, 16 is shorter than a person manages
const TRIES = 24
const wait = (ms) => new Promise((d) => setTimeout(d, ms))

const login = await (await fetch(server + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: process.env.WMS_NAME, password: process.env.WMS_PASS }) })).json()
const browser = await launch({ width: 1920, height: 1080, software: true, args: ['--disable-renderer-backgrounding', '--disable-background-timer-throttling', '--disable-backgrounding-occluded-windows'] })
const page = await browser.open()
await page.send('Emulation.setFocusEmulationEnabled', { enabled: true }) // or a hidden page has no frames at all
await page.send('Page.addScriptToEvaluateOnNewDocument', { source: `
  window.__pads = []
  navigator.getGamepads = () => { const out = [null, null, null, null]; window.__pads.forEach((p, i) => { out[i] = p }); return out }
` })
await page.go(server + '/')
await page.eval(`localStorage.setItem('wms.token', ${JSON.stringify(login.token)})`)

const busy = Array.from({ length: Math.max(1, cpus().length >> 1) }, () => new Worker('while (true) {}', { eval: true }))
let lost = 0
try {
  for (const con of consoles.length ? consoles : ['nes', 'n64']) {
    const found = await page.eval(`fetch('/api/items?kind=game&console=${con}&sort=title&limit=1', { headers: { Authorization: 'Bearer ' + localStorage.getItem('wms.token') } }).then((r) => r.json())`)
    const game = found.items?.[0]
    if (!game) { console.log(con, 'no game to try'); continue }
    await page.go(server + '/game/' + game.id)
    await page.eval(`(() => {
      const p = { id: 'Test pad (STANDARD GAMEPAD)', index: 0, connected: true, mapping: 'standard', timestamp: 0, axes: [0, 0, 0, 0], buttons: Array.from({ length: 17 }, () => ({ pressed: false, touched: false, value: 0 })) }
      window.__pads[0] = p
      const e = new Event('gamepadconnected'); e.gamepad = p; window.dispatchEvent(e)
    })()`)
    await page.until(`document.querySelector('.wp-playing')`, 60000, 'the game to start')
    await wait(3000)
    await page.key('Escape')
    await page.until(`document.querySelector('.wp-menu')`, 5000, 'its menu')
    await wait(500)
    await page.eval(`(() => {
      window.__seen = 0
      window.addEventListener('keydown', (e) => { if (!e.isTrusted && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) window.__seen++ }, true)
      // Frames held back for 450ms in every second, whatever the page asks.
      const real = window.requestAnimationFrame.bind(window)
      let queued = []
      window.requestAnimationFrame = (cb) => { if (performance.now() % 1000 < 450) { queued.push(cb); return 0 } return real(cb) }
      setInterval(() => { if (performance.now() % 1000 >= 450 && queued.length) { const q = queued; queued = []; for (const cb of q) real(cb) } }, 10)
    })()`)
    for (const hold of HOLDS) {
      await page.eval(`window.__seen = 0`)
      // Timed inside the page, so this script's own delays don't count.
      await page.eval(`(async () => {
        const b = window.__pads[0].buttons, sleep = (ms) => new Promise((d) => setTimeout(d, ms))
        for (let i = 0; i < ${TRIES}; i++) {
          const dir = i % 2 ? 12 : 13
          b[dir].pressed = true; b[dir].value = 1
          await sleep(${hold})
          b[dir].pressed = false; b[dir].value = 0
          await sleep(160)
        }
      })()`)
      await wait(100)
      const seen = await page.eval(`window.__seen`)
      console.log(con, game.title, `${hold}ms:`, `${seen}/${TRIES}`)
      lost += TRIES - seen
    }
    await page.eval(`document.querySelector('[data-tile=quit]')?.focus()`)
    await page.key('Enter', 'Enter')
    await wait(1500)
  }
} finally {
  await Promise.all(busy.map((w) => w.terminate()))
  await browser.close()
}
console.log(lost ? `${lost} presses lost` : 'every press counted')
process.exit(lost ? 1 : 0)
