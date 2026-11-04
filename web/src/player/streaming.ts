import { getToken, serverBase } from '../api'

// Robust streaming, from the player's side. The server decides the mode (its setting for
// viewers away from home, or this device's own choice) and says what to do in it; this is
// the device's choice and the measurement the server chooses a robust stream's start from.

export type DeviceStreaming = 'auto' | 'standard' | 'robust'
const KEY = 'wms.streaming'
const SPEED = 'wms.streaming.speed'

export function deviceStreaming(): DeviceStreaming {
  try { const v = localStorage.getItem(KEY); return v === 'standard' || v === 'robust' ? v : 'auto' } catch { return 'auto' }
}
export function setDeviceStreaming(v: DeviceStreaming) {
  try { if (v === 'auto') localStorage.removeItem(KEY); else localStorage.setItem(KEY, v) } catch { /* private window: it stays as it was */ }
}

// A measurement is good for a few minutes: a connection that was slow a minute ago
// probably still is, and measuring again costs the viewer a moment.
const FRESH_MS = 10 * 60 * 1000

type Speed = { kbps: number; at: number }
function saved(): Speed | null {
  try { const s = JSON.parse(sessionStorage.getItem(SPEED) || 'null') as Speed | null; return s && Date.now() - s.at < FRESH_MS ? s : null } catch { return null }
}

// rememberSpeed keeps what the player saw while playing (hls.js's estimate), for the
// next video. A short or unrealistic figure is ignored.
export function rememberSpeed(kbps: number) {
  if (!(kbps > 50 && kbps < 10_000_000)) return
  try { sessionStorage.setItem(SPEED, JSON.stringify({ kbps: Math.round(kbps), at: Date.now() })) } catch { /* not kept */ }
}

// measureSpeed times a download of bytes that don't compress (GET /playback/speedtest),
// giving up after a second and a half and going by what arrived by then. 0 = unknown.
export async function measureSpeed(): Promise<number> {
  const s = saved()
  if (s) return s.kbps
  const ctl = new AbortController()
  const limit = setTimeout(() => ctl.abort(), 1500)
  let got = 0
  const t0 = performance.now()
  let t1 = t0
  try {
    const tok = getToken()
    const r = await fetch(serverBase() + '/api/playback/speedtest?kb=1024', {
      headers: tok ? { Authorization: 'Bearer ' + tok } : {}, credentials: serverBase() ? 'omit' : 'same-origin', cache: 'no-store', signal: ctl.signal,
    })
    if (!r.ok || !r.body) return 0
    const reader = r.body.getReader()
    let first = true
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      // From the first bytes on: the time to answer is latency, not speed.
      if (first) { first = false; t1 = performance.now(); continue }
      got += value.length
    }
  } catch { /* cut short: go by what arrived */ } finally { clearTimeout(limit) }
  const ms = performance.now() - t1
  if (got < 32 * 1024 || ms < 50) return 0
  const kbps = (got * 8) / ms
  rememberSpeed(kbps)
  return Math.round(kbps)
}

// Whether a stream may be robust, so the connection is worth measuring first: this device
// asked for it, or it follows the server, whose setting is robust (for viewers away from
// home, which only the server can tell).
let serverMode: Promise<string> | null = null
export async function mayBeRobust(): Promise<boolean> {
  const d = deviceStreaming()
  if (d !== 'auto') return d === 'robust'
  serverMode ??= fetch(serverBase() + '/api/capabilities', { cache: 'no-store' }).then((r) => r.json())
    .then((c: { playback?: { remoteStreaming?: string } }) => c.playback?.remoteStreaming || 'standard').catch(() => { serverMode = null; return 'standard' })
  return (await serverMode) === 'robust'
}
