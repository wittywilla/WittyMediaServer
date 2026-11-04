import { useLayoutEffect, useRef, useSyncExternalStore } from 'react'
import type Hls from 'hls.js'
import { leaveChannel, makeViewer, preflight, tune, type TuneInfo } from './tvApi'

// One player for the whole TV section: one <video>, one stream, for as long as someone stays
// on a channel. The guide's preview and the watch view are only places to draw it (slots), so
// going from one to the other, or full screen and back, never tunes again. TvHost attaches the
// stream; the pages say which channel they want and where to show it, with useLive.
//
// Nobody wanting a channel any more isn't leaving yet: the picture is put aside, silent, and
// the channel is left LEAVE_MS later, unless someone wants it again first. Changing to another
// channel leaves the old one at once, so a tuner is free for the next. Closing the tab leaves
// at once too.

export const LEAVE_MS = 3000

export type Live = {
  key: string; info: TuneInfo | null; url: string | null; err: string
  // The stream is attached (ready) and has shown a picture (playing).
  ready: boolean; playing: boolean
}
const IDLE: Live = { key: '', info: null, url: null, err: '', ready: false, playing: false }

let live = IDLE
const viewer = makeViewer()
let ac: AbortController | null = null
let leaveTimer: ReturnType<typeof setTimeout> | undefined
const subs = new Set<() => void>()

export function setLive(p: Partial<Live>) {
  live = { ...live, ...p }
  subs.forEach((f) => f())
}
const subscribe = (f: () => void) => { subs.add(f); return () => { subs.delete(f) } }
export const useLiveState = () => useSyncExternalStore(subscribe, () => live)
export const liveNow = () => live
export const liveViewer = () => viewer

// The video, made once. Until a slot wants it, it waits in a corner of the page: still in the
// document, as taking a video out of it pauses it, and still on screen, as some browsers stop
// a muted video they think nobody can see.
let video: HTMLVideoElement | null = null
let parking: HTMLDivElement | null = null
export function liveVideo(): HTMLVideoElement {
  if (video) return video
  video = document.createElement('video')
  video.playsInline = true
  video.preload = 'auto'
  video.addEventListener('playing', () => { if (!live.playing) setLive({ playing: true }) })
  parking = document.createElement('div')
  parking.setAttribute('aria-hidden', 'true')
  parking.style.cssText = 'position:fixed;right:0;bottom:0;width:2px;height:2px;overflow:hidden;opacity:0;pointer-events:none;z-index:-1'
  document.body.appendChild(parking)
  parking.appendChild(video)
  window.addEventListener('pagehide', () => { if (live.key) leaveChannel(live.key, viewer) })
  return video
}

// hls.js, while there is a stream: TvHost lends its handle.
let hlsRef: React.RefObject<Hls | null> | null = null
export const lendHls = (r: React.RefObject<Hls | null> | null) => { hlsRef = r }
export const liveHls = () => hlsRef?.current ?? null

type Slot = { id: number; key: string; el: HTMLElement | null }
const slots: Slot[] = []
let seq = 0

// Moving the video keeps it playing: moveBefore where the browser has it, which keeps
// everything about it, else insertBefore, which puts it back before the browser looks.
function place(el: HTMLElement | null) {
  const v = liveVideo()
  const to = el || parking!
  if (v.parentElement === to) return
  const m = (to as HTMLElement & { moveBefore?: (n: Node, ref: Node | null) => void }).moveBefore
  if (m && v.isConnected && to.isConnected) {
    try { m.call(to, v, to.firstChild); return } catch { /* fall back */ }
  }
  to.insertBefore(v, to.firstChild)
}

function settle() {
  let top: Slot | undefined
  for (let i = slots.length - 1; i >= 0 && !top; i--) if (slots[i].key) top = slots[i]
  place(top?.el || null)
  if (top) {
    clearTimeout(leaveTimer)
    leaveTimer = undefined
    if (top.key !== live.key) start(top.key)
    return
  }
  if (live.key && leaveTimer === undefined) {
    liveVideo().muted = true
    leaveTimer = setTimeout(stop, LEAVE_MS)
  }
}

function start(key: string) {
  if (live.key) leaveChannel(live.key, viewer)
  ac?.abort()
  const a = (ac = new AbortController())
  setLive({ ...IDLE, key })
  tune(key, viewer, a.signal)
    .then(async (i) => {
      if (a.signal.aborted) return
      setLive({ info: i })
      const u = i.url + '?viewer=' + viewer
      await preflight(u, a.signal)
      if (!a.signal.aborted) setLive({ url: u })
    })
    .catch((e) => { if (!a.signal.aborted) setLive({ err: e.message }) })
}

function stop() {
  leaveTimer = undefined
  ac?.abort()
  ac = null
  if (live.key) leaveChannel(live.key, viewer)
  setLive(IDLE)
}

// A page's place for the picture. `key` is the channel it wants, or '' for none yet; `slot`
// is the element the video goes in while this is the page that wants it.
export function useLive(key: string, slot: React.RefObject<HTMLElement | null>): Live {
  const me = useRef<Slot | null>(null)
  useLayoutEffect(() => {
    const s: Slot = { id: ++seq, key: '', el: slot.current }
    me.current = s
    slots.push(s)
    return () => {
      const i = slots.indexOf(s)
      if (i >= 0) slots.splice(i, 1)
      me.current = null
      settle()
    }
  }, [slot])
  useLayoutEffect(() => {
    const s = me.current
    if (!s) return
    s.key = key
    s.el = slot.current
    settle()
  }, [key, slot])
  return useLiveState()
}
