import type Hls from 'hls.js'
import { useEffect, useRef } from 'react'
import { getToken, mediaUrl, serverBase, type StreamingHints } from '../api'
import { reportClient, reportHlsError, watchPlayback } from '../errorReport'
import { mediaSourceWorks } from './mse'
import { rememberSpeed } from './streaming'

type HlsClass = typeof Hls
let loaded: HlsClass | null = null

// The full-screen players bring hls.js with them, as they're about to need it, and start at once.
// A guide or a photo page that may never play anything only asks for it when a stream starts.
export function haveHls(h: HlsClass) { loaded = h }
const loadHls = () => loaded ? Promise.resolve(loaded) : import('hls.js').then((m) => (loaded = m.default))

// What a robust stream's player did, for the viewer to be told: kind says what, text says it in words.
export type NetNote = { kind: 'down' | 'up' | 'lost' | 'back' | 'reopen' | 'stall'; text: string }

// Robust streams (see streaming.ts): what the player does on its own.
const STEP_DOWN_AFTER_MS = 3000 // stalled this long with a lower rung to go to: go there
const KICK_AFTER_MS = 10000 // stalled this long: ask for the segment again
const REOPEN_AFTER_MS = 25000 // and this long: ask the server for the stream afresh, at the same place
const CLIMB_AFTER_MS = 60000 // smooth this long after stepping down: let the player climb again

// Attaches an HLS (or direct) source to a <video>. Uses hls.js where MSE is
// available and falls back to native HLS (Safari/iOS). startAt makes the
// stream begin at that position, so the first frame shown is the right one
// (seeking after the start would briefly show the beginning).
//
// robust is the server's advice for a robust stream (a bigger buffer, patient retries with
// growing waits). With it the player also steps down a rung when the picture stalls, keeps
// the place when the connection drops and picks up there once it's back (onReopen asks
// for a new stream at that place when this one can't go on), and says what it did (onNote).
export function useSource(video: React.RefObject<HTMLVideoElement | null>, url: string | null,
  opts: { onReady?: () => void; onError?: (msg: string) => void; startAt?: () => number | null | undefined; logSession?: string | null; growing?: boolean
    robust?: StreamingHints | null; measuredKbps?: number; startHeight?: number; onNote?: (n: NetNote) => void; onReopen?: (at: number) => void } = {}) {
  const hlsRef = useRef<Hls | null>(null)
  const optsRef = useRef(opts)
  optsRef.current = opts
  // What goes wrong is reported to the server's log, under this stream's session.
  useEffect(() => (video.current && url ? watchPlayback(video.current, url, optsRef.current.logSession) : undefined), [url, video])
  useEffect(() => {
    const v = video.current
    if (!v || !url) return
    let destroyed = false
    const isHls = url.includes('.m3u8')
    const abs = url.startsWith('http') ? url : serverBase() + url
    const start = optsRef.current.startAt?.()
    const robust = optsRef.current.robust
    if (isHls && mediaSourceWorks()) {
      const attach = (Hls: HlsClass) => {
        // A rewrapped stream grows as it's made, which HLS calls live. It starts at its
        // start, however far ahead it has got.
        const startPos = optsRef.current.growing ? 0 : (start != null && start > 0.5 ? start : -1)
        const retry = robust && { maxNumRetry: robust.retries, retryDelayMs: robust.firstRetryMs, maxRetryDelayMs: robust.maxRetryMs, backoff: 'exponential' as const }
        const policy = (ttfb: number) => (retry ? { default: { maxTimeToFirstByteMs: ttfb, maxLoadTimeMs: ttfb * 2, timeoutRetry: retry, errorRetry: retry } } : undefined)
        const hls = new Hls({
          xhrSetup: (xhr) => { const t = getToken(); if (t) xhr.setRequestHeader('Authorization', 'Bearer ' + t); xhr.withCredentials = !serverBase() },
          maxBufferLength: 30, maxMaxBufferLength: 90, backBufferLength: 30,
          fragLoadingTimeOut: 60000, manifestLoadingTimeOut: 20000, fragLoadingMaxRetry: 6, levelLoadingMaxRetry: 6,
          startPosition: startPos, lowLatencyMode: false,
          ...(robust && retry ? {
            autoStartLoad: false, // until the start rung is set (below)
            maxBufferLength: robust.bufferSec, maxMaxBufferLength: robust.maxBufferSec,
            fragLoadPolicy: policy(robust.segmentTimeoutMs), playlistLoadPolicy: policy(20000), manifestLoadPolicy: policy(20000),
            // It starts at the rung the server listed first, chosen from the measured speed,
            // and is slow to climb: every climb starts a conversion on the server.
            abrEwmaDefaultEstimate: (optsRef.current.measuredKbps || 2400) * 1000, abrBandWidthFactor: 0.8, abrBandWidthUpFactor: 0.5,
          } : {}),
        })
        hlsRef.current = hls
        hls.loadSource(abs)
        hls.attachMedia(v)
        hls.on(Hls.Events.MANIFEST_PARSED, () => {
          // A robust stream starts at the rung the server chose from the measured speed, not
          // the one hls.js would pick from its own first guess.
          if (robust && retry) {
            const h = optsRef.current.startHeight
            const i = h ? hls.levels.findIndex((l) => l.height === h) : -1
            if (i >= 0) hls.startLevel = i
            hls.startLoad(startPos)
          }
          if (!destroyed) optsRef.current.onReady?.()
        })
        // A stream the server no longer has (it restarted) would be retried for
        // ever behind a spinner; a busy server (503) is worth waiting for.
        let gone = 0
        let mediaErrors = 0
        hls.on(Hls.Events.FRAG_LOADED, () => { gone = 0 })
        const watch = robust ? robustWatch(Hls, hls, v, optsRef) : null
        hls.on(Hls.Events.ERROR, (_e, data) => {
          if (!data.fatal) return
          reportHlsError(data)
          if (watch && data.type === Hls.ErrorTypes.NETWORK_ERROR) { watch.lost(data.response?.code); return }
          if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
            if (data.response?.code === 404 && ++gone >= 2) optsRef.current.onError?.('The stream has ended on the server. Start playback again.')
            else hls.startLoad()
          } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
            // Once is worth it; a stream this browser really can't decode would go round for ever.
            if (++mediaErrors <= 2) hls.recoverMediaError()
            else optsRef.current.onError?.(data.details || 'playback error')
          }
          else optsRef.current.onError?.(data.details || 'playback error')
        })
        return () => {
          watch?.stop()
          if (watch && hls.bandwidthEstimate > 0) rememberSpeed(hls.bandwidthEstimate / 1000)
          hls.destroy(); hlsRef.current = null
        }
      }
      let stop: (() => void) | undefined
      if (loaded) stop = attach(loaded)
      else loadHls().then((Hls) => { if (!destroyed) stop = attach(Hls) }, (err) => { if (!destroyed) optsRef.current.onError?.(String(err?.message || err)) })
      return () => { destroyed = true; stop?.() }
    }
    v.src = mediaUrl(abs)
    const ready = () => optsRef.current.onReady?.()
    v.addEventListener('loadedmetadata', ready)
    // A file sent as it is can't step down: when it stalls for long, or the connection
    // fails, the player asks again from where it was (onReopen), and the server may
    // answer with a robust stream instead.
    const stopFile = robust ? fileWatch(v, optsRef) : () => {}
    return () => { destroyed = true; stopFile(); v.removeEventListener('loadedmetadata', ready); v.removeAttribute('src'); v.load() }
  }, [url, video])
  return hlsRef
}

type Opts = React.RefObject<{ robust?: StreamingHints | null; onNote?: (n: NetNote) => void; onReopen?: (at: number) => void }>

// A level's height as a viewer would say it.
const lines = (l?: { height?: number }) => (l?.height ? `${l.height}p` : 'a lower quality')

// robustWatch is what a robust HLS stream's player does by itself: step down when the
// picture stalls, and when the connection is lost, keep the place and keep trying, with
// growing waits, until it's back.
function robustWatch(Hls: HlsClass, hls: Hls, v: HTMLVideoElement, opts: Opts) {
  const hints = opts.current.robust!
  const note = (n: NetNote) => opts.current.onNote?.(n)
  let lastTime = -1
  let stuckSince = 0
  let kicked = false
  let cappedAt = 0
  let lostTries = 0
  let lostTimer = 0
  let lostSince = 0
  let shownHeight = 0
  let reopened = false
  const reopen = (why: string) => {
    if (reopened) return
    reopened = true
    note({ kind: 'reopen', text: why })
    reportClient({ level: 'warn', kind: 'player', message: `Robust stream: ${why}` })
    opts.current.onReopen?.(v.currentTime)
  }
  // The connection failed and hls.js gave up for now: try again, waiting longer each time.
  // A stream the server has forgotten (404) or a sign-in that ran out (401, 403) won't
  // come back by itself: ask for a new one at the same place.
  const lost = (code?: number) => {
    if (code === 404 || code === 401 || code === 403) { reopen('The stream had ended on the server, so it was started again from the same place.'); return }
    if (!lostSince) { lostSince = Date.now(); note({ kind: 'lost', text: 'Connection lost. Trying again…' }) }
    if (lostTimer) return
    const wait = Math.min(hints.firstRetryMs * 2 ** lostTries, hints.maxRetryMs)
    lostTries++
    lostTimer = window.setTimeout(() => {
      lostTimer = 0
      if (Date.now() - lostSince > REOPEN_AFTER_MS * 2) { reopen('The connection was gone for a long time, so the stream was asked for again from the same place.'); return }
      hls.startLoad(v.currentTime)
    }, wait)
  }
  const online = () => { if (lostSince && lostTimer) { clearTimeout(lostTimer); lostTimer = 0; hls.startLoad(v.currentTime) } }
  window.addEventListener('online', online)
  const loaded = () => {
    kicked = false
    if (lostSince) {
      note({ kind: 'back', text: `Reconnected after ${Math.round((Date.now() - lostSince) / 1000)} s, at the same place.` })
      lostSince = 0
      lostTries = 0
    }
  }
  hls.on(Hls.Events.FRAG_LOADED, loaded)
  // Which rung is showing: the player says when it changed.
  const switched = (_e: unknown, d: { level: number }) => {
    const l = hls.levels[d.level]
    const h = l?.height || 0
    if (shownHeight && h && h !== shownHeight) {
      note(h < shownHeight ? { kind: 'down', text: `Picture lowered to ${lines(l)} to keep playing.` } : { kind: 'up', text: `Picture back up to ${lines(l)}.` })
    }
    shownHeight = h || shownHeight
  }
  hls.on(Hls.Events.LEVEL_SWITCHED, switched)
  const tick = window.setInterval(() => {
    const now = Date.now()
    const moving = v.currentTime !== lastTime
    lastTime = v.currentTime
    if (v.paused || v.ended || v.seeking || moving || v.readyState >= 3) {
      stuckSince = 0
      // Smooth for a good while after stepping down: the player may climb again.
      if (cappedAt && now - cappedAt > CLIMB_AFTER_MS) { hls.autoLevelCapping = -1; cappedAt = 0 }
      return
    }
    if (!stuckSince) { stuckSince = now; return }
    if (lostSince) return // the retries above are on it
    const stuck = now - stuckSince
    // Levels are sorted by bitrate, lowest first.
    const level = hls.loadLevel
    if (hints.stepDownOnStall && stuck >= STEP_DOWN_AFTER_MS && level > 0) {
      hls.autoLevelCapping = level - 1 // or the bandwidth estimate, slow to catch up, takes it straight back
      hls.nextLoadLevel = level - 1
      cappedAt = now
      stuckSince = now
      return
    }
    // hls.js can sit waiting on a request that will never be answered: ask again.
    if (stuck >= KICK_AFTER_MS && !kicked) { kicked = true; hls.startLoad(v.currentTime); return }
    if (stuck >= REOPEN_AFTER_MS) reopen('Nothing arrived for a while, so the stream was asked for again from the same place.')
  }, 1000)
  return {
    lost,
    stop: () => { clearInterval(tick); clearTimeout(lostTimer); window.removeEventListener('online', online); hls.off(Hls.Events.FRAG_LOADED, loaded); hls.off(Hls.Events.LEVEL_SWITCHED, switched) },
  }
}

// fileWatch: a file sent as it is, on a robust connection. It can't step down, so a stall
// that goes on, or a failed connection, is handed to onReopen with the place.
function fileWatch(v: HTMLVideoElement, opts: Opts) {
  let lastTime = -1
  let stuckSince = 0
  let done = false
  const reopen = (text: string) => {
    if (done) return
    done = true
    opts.current.onNote?.({ kind: 'stall', text })
    opts.current.onReopen?.(v.currentTime)
  }
  const tick = window.setInterval(() => {
    const moving = v.currentTime !== lastTime
    lastTime = v.currentTime
    if (v.paused || v.ended || v.seeking || moving || v.readyState >= 3) { stuckSince = 0; return }
    if (!stuckSince) stuckSince = Date.now()
    else if (Date.now() - stuckSince >= 6000) reopen('The file wasn’t arriving fast enough, so a steadier stream was asked for, from the same place.')
  }, 1000)
  const failed = () => { if (v.error?.code === MediaError.MEDIA_ERR_NETWORK) reopen('The connection failed, so the video was asked for again from the same place.') }
  v.addEventListener('error', failed)
  return () => { clearInterval(tick); v.removeEventListener('error', failed) }
}

// Auto-hide controls after inactivity (ms <= 0: never hide). Pointer, touch,
// keyboard and controller input all count as activity.
export function useIdle(ms = 3000, deps: unknown[] = []) {
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    let t: any
    const show = () => {
      el.classList.remove('controls-hidden', 'hide-cursor')
      clearTimeout(t)
      if (ms > 0) t = setTimeout(() => el.classList.add('controls-hidden', 'hide-cursor'), ms)
    }
    show()
    el.addEventListener('mousemove', show)
    el.addEventListener('touchstart', show, { passive: true })
    window.addEventListener('keydown', show)
    return () => { clearTimeout(t); el.removeEventListener('mousemove', show); el.removeEventListener('touchstart', show); window.removeEventListener('keydown', show) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ms, ...deps])
  return ref
}

export function toggleFullscreen(el: HTMLElement | null) {
  if (!el) return
  if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
  else el.requestFullscreen?.().catch(() => {})
}
