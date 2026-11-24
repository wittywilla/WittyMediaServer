// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Watches a WittyPlay game over its WebSocket (stream version 2) for a while and says how many
// pictures and sounds came: `pictures N sounds N`, or `error: why`.
//   node scripts/test/stream-frames.mjs <ws url with ?token=> <h264|jpeg> <milliseconds>
const [url, video = 'jpeg', ms = '8000'] = process.argv.slice(2)
const MSG_JPEG = 2, MSG_PCM = 3, MSG_VIDEO = 0x11, MSG_OPUS = 0x12
let pictures = 0, sounds = 0, said = ''
const ws = new WebSocket(url)
ws.binaryType = 'arraybuffer'
const done = (why) => {
  console.log(why || `pictures ${pictures} sounds ${sounds}`)
  try { ws.close() } catch { /* gone already */ }
  process.exit(0)
}
const timer = setTimeout(() => done(pictures ? '' : `error: no picture in ${ms} ms${said ? ` (the server said ${said})` : ''}`), Number(ms))
ws.onopen = () => ws.send(JSON.stringify({ type: 'hello', v: 2, app: 'docker-games test', device: 'ci', watch: true, video: [video], audio: ['pcm'], seats: [0] }))
ws.onmessage = (m) => {
  if (typeof m.data === 'string') {
    try {
      const e = JSON.parse(m.data)
      if (e.type === 'error' || e.type === 'ended') { clearTimeout(timer); done(`error: ${e.code || e.type}: ${e.text || ''}`) }
      said = said || e.type
    } catch { /* not ours */ }
    return
  }
  const kind = new Uint8Array(m.data)[0]
  if (kind === MSG_JPEG || kind === MSG_VIDEO) pictures++
  else if (kind === MSG_PCM || kind === MSG_OPUS) sounds++
}
ws.onerror = (e) => { clearTimeout(timer); done(`error: the connection failed${e?.message ? `: ${e.message}` : ''}`) }
ws.onclose = (e) => { if (!pictures) { clearTimeout(timer); done(`error: closed ${e.code} ${e.reason || ''}`) } }
