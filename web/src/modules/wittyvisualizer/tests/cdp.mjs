// A small driver for a headless Chromium browser, using nothing beyond Node. No window opens.
//   const b = await launch({ software: true }); const p = await b.open(url); await p.eval('1+1')
import { spawn } from 'node:child_process'
import { existsSync, mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const BROWSERS = [
  process.env.WV_BROWSER,
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  'C:/Program Files/Microsoft/Edge/Application/msedge.exe',
  '/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/google-chrome',
].filter(Boolean)

export async function launch({ software = false, width = 1280, height = 720, args = [], tmp } = {}) {
  const exe = BROWSERS.find(existsSync)
  if (!exe) throw new Error('no Chromium browser found; set WV_BROWSER')
  const dir = mkdtempSync(join(tmp || tmpdir(), 'wv-browser-'))
  const flags = [
    '--headless=new', `--user-data-dir=${dir}`, '--remote-debugging-port=0', '--no-first-run', '--no-default-browser-check', '--mute-audio',
    '--autoplay-policy=no-user-gesture-required', `--window-size=${width},${height}`, '--disable-background-networking', '--disable-sync',
    ...(software ? ['--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--disable-gpu-rasterization'] : ['--enable-gpu', '--use-angle=default', '--ignore-gpu-blocklist']),
    ...args, 'about:blank',
  ]
  const proc = spawn(exe, flags, { stdio: ['ignore', 'ignore', 'pipe'] })
  const ws = await new Promise((resolve, reject) => {
    let err = ''
    const t = setTimeout(() => reject(new Error('the browser did not start: ' + err)), 20000)
    proc.stderr.on('data', (d) => {
      err += d
      const m = /DevTools listening on (ws:\/\/\S+)/.exec(err)
      if (m) { clearTimeout(t); resolve(m[1]) }
    })
  })
  const http = ws.replace('ws://', 'http://').replace(/\/devtools.*/, '')
  return {
    async open(url) {
      const t = await (await fetch(`${http}/json/new?${encodeURIComponent('about:blank')}`, { method: 'PUT' })).json()
      const page = await connect(t.webSocketDebuggerUrl)
      await page.send('Page.enable'); await page.send('Runtime.enable'); await page.send('DOM.enable')
      if (url) await page.go(url)
      return page
    },
    async close() {
      proc.kill()
      await new Promise((d) => setTimeout(d, 400))
      try { rmSync(dir, { recursive: true, force: true }) } catch { /* the browser still holds a file */ }
    },
  }
}

async function connect(url) {
  const ws = new WebSocket(url)
  await new Promise((d, f) => { ws.onopen = d; ws.onerror = f })
  let id = 0
  const waiting = new Map(), logs = []
  const handlers = new Map()
  ws.onmessage = (m) => {
    const d = JSON.parse(m.data)
    if (d.id && waiting.has(d.id)) {
      const [ok, no] = waiting.get(d.id); waiting.delete(d.id)
      d.error ? no(new Error(JSON.stringify(d.error))) : ok(d.result)
    } else if (d.method === 'Runtime.consoleAPICalled') logs.push(`${d.params.type}: ${d.params.args.map((a) => a.value ?? a.description).join(' ')}`)
    else if (d.method === 'Runtime.exceptionThrown') logs.push('exception: ' + (d.params.exceptionDetails.exception?.description || d.params.exceptionDetails.text))
    handlers.get(d.method)?.(d.params)
  }
  const send = (method, params = {}) => new Promise((ok, no) => { waiting.set(++id, [ok, no]); ws.send(JSON.stringify({ id, method, params })) })
  const page = {
    send, logs,
    on(method, fn) { handlers.set(method, fn) },
    async go(url) {
      const loaded = new Promise((d) => handlers.set('Page.loadEventFired', d))
      await send('Page.navigate', { url })
      await loaded
    },
    async eval(expression) {
      const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true, userGesture: true })
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text)
      return r.result.value
    },
    async until(expression, ms = 15000, what = expression) {
      const end = Date.now() + ms
      for (;;) {
        const v = await page.eval(`!!(${expression})`)
        if (v) return v
        if (Date.now() > end) throw new Error('timed out waiting for ' + what)
        await new Promise((d) => setTimeout(d, 100))
      }
    },
    async shot() { return Buffer.from((await send('Page.captureScreenshot', { format: 'png' })).data, 'base64') },
    async key(key, code = key) {
      await send('Input.dispatchKeyEvent', { type: 'keyDown', key, code, windowsVirtualKeyCode: key.length === 1 ? key.toUpperCase().charCodeAt(0) : undefined })
      await send('Input.dispatchKeyEvent', { type: 'keyUp', key, code })
    },
    close() { ws.close() },
  }
  return page
}
