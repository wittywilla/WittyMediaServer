// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyPlay's game stream as an app away from home uses it: a game is started, and on its WebSocket
// a button press arrives before the hello, as it does when a socket is slow to open (a phone on
// mobile data queues its pad state meanwhile). The stream must still start and send pictures.
// WittyPlay 0.7.12 and older refused it ("the first message must be the hello, as text").
//
//   WMS_URL=... WMS_TOKEN=... node scripts/smoke/game-stream.mjs <out folder>
import { appendFileSync, mkdirSync, readdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const server = process.env.WMS_URL
const token = process.env.WMS_TOKEN
const out = process.argv[2] || 'smoke-out'
mkdirSync(out, { recursive: true })
const n = readdirSync(out).filter((f) => /^\d\d-.*\.log$/.test(f)).length + 1
const logPath = join(out, `${String(n).padStart(2, '0')}-game-stream-input-before-hello.log`)
writeFileSync(logPath, '--- A game stream with a button press before the hello\n')
const log = (s) => appendFileSync(logPath, s + '\n')
const call = async (method, path, body) => {
  const r = await fetch(server + path, { method, headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined })
  const text = await r.text()
  if (!r.ok) throw new Error(`${method} ${path}: ${r.status} ${text.slice(0, 300)}`)
  return text ? JSON.parse(text) : null
}

let sid
const verdict = await (async () => {
  const libs = await call('GET', '/api/libraries')
  const games = libs.find((l) => l.type === 'games')
  const items = await call('GET', `/api/items?library=${games.id}`)
  const game = (items.items || items)[0]
  log(`game: ${game.id} ${game.title}`)
  const started = await call('POST', `/api/v1/modules/wittyplay/games/${game.id}/play`, { client: 'android', device: 'smoke-probe' })
  sid = started.id || started.sid || started.session?.id
  log(`session: ${sid}`)
  if (!sid) throw new Error('no session in ' + JSON.stringify(started).slice(0, 300))
  const url = `${server.replace(/^http/, 'ws')}/api/modules/wittyplay/play/${sid}/stream?token=${encodeURIComponent(token)}`
  return await new Promise((resolve) => {
    const ws = new WebSocket(url)
    ws.binaryType = 'arraybuffer'
    let frames = 0
    const t = setTimeout(() => { ws.close(); resolve(`no picture within 45 s (${frames} pictures)`) }, 45000)
    ws.onopen = () => {
      // What an app queued while the socket was opening: a pad state, then a keyframe request.
      ws.send(new Uint8Array([0x01, 0, 0, 0, 0, 0]))
      ws.send(new Uint8Array([0x02]))
      ws.send(JSON.stringify({ type: 'hello', v: 2, app: 'smoke', device: 'smoke-probe', watch: true, video: ['jpeg'], audio: ['pcm'] }))
    }
    ws.onmessage = (m) => {
      if (typeof m.data === 'string') {
        log('text: ' + m.data.slice(0, 300))
        const e = JSON.parse(m.data)
        if (e.type === 'error') { clearTimeout(t); ws.close(); resolve(`refused: ${e.code}: ${e.text}`) }
        return
      }
      const b = new Uint8Array(m.data)
      if (b[0] < 0xf0 && b.length > 100 && ++frames >= 3) { clearTimeout(t); log(`${frames} pictures, the last ${b.length} bytes`); ws.close(); resolve('') }
    }
    ws.onerror = () => { clearTimeout(t); resolve('the WebSocket failed to open') }
    ws.onclose = (e) => { if (frames < 3) { clearTimeout(t); resolve(`closed: ${e.code} ${e.reason}`) } }
  })
})().catch((e) => e.message)

if (sid) await call('DELETE', `/api/v1/modules/wittyplay/play/${sid}`).catch((e) => log('stopping it: ' + e.message))
if (verdict) {
  log('FAILED: ' + verdict)
  console.log(`FAIL  game stream: a button press before the hello: ${verdict}`)
  console.log(`::error title=Smoke: game stream::A button press before the hello: ${verdict}`)
  process.exit(1)
}
console.log('ok    game stream: a button press before the hello, and the pictures still come')
process.exit(0)
