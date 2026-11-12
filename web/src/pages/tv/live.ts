// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useLayoutEffect, useRef, useSyncExternalStore } from 'react'
import type Hls from 'hls.js'
import { BACKGROUND_KEY, STILL_HERE_MS, keepsPlaying, letGo, readChoice, resumeOnShow, sayStillHere, type Choice } from './backgroundPlay'
import { leaveChannel, makeViewer, preflight, tune, type TuneInfo } from './tvApi'

// One player for the whole TV section: one <video>, one stream, for as long as someone stays
// on a channel. The guide's preview and the watch view are only places to draw it (slots), so
// going from one to the other, or full screen and back, never tunes again. TvHost attaches the
// stream; the pages say which channel they want and where to show it, with useLive.
//
// A page that goes (the preview giving way to the watch view, or back) isn't leaving yet: the
// picture is put aside, silent, and the channel is left LEAVE_MS later, unless someone wants it
// again first. A page that stays but stops wanting the channel (the guide's highlight moving
// on), changing to another channel, and closing the tab all leave at once, so a tuner is free
// for the next.
//
// A tab in the background keeps playing, sound and all, unless this device has turned off
// "Keep playing when this tab is in the background" (backgroundPlay.ts). Kept, the player tells
// the server every so often that it's still there, from a worker, whose timer browsers don't
// slow down the way they do a background page's, so the server never takes a quiet background
// player for one that has gone. Turned off, the channel is left when the page is hidden, and
// tuned again when it's shown.

export const LEAVE_MS = 3000

export type Live = {
  key: string; info: TuneInfo | null; url: string | null; err: string
  // The stream is attached (ready) and has shown a picture (playing).
  ready: boolean; playing: boolean
  // The viewer paused or went back, so live sync leaves this player be until Back to live.
  away: boolean
}
const IDLE: Live = { key: '', info: null, url: null, err: '', ready: false, playing: false, away: false }

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
  document.addEventListener('visibilitychange', onVisibility)
  return video
}

// ------------------------------------------------------------ playing in the background

let hidden = typeof document !== 'undefined' && document.hidden
let serverDefault: boolean | undefined
const savedChoice = (): Choice => { try { return readChoice(localStorage.getItem(BACKGROUND_KEY)) } catch { return null } }
let choice: Choice = savedChoice()
export type BackgroundPlay = { keep: boolean; choice: Choice; serverDefault: boolean }
let bg: BackgroundPlay = { keep: keepsPlaying(choice, serverDefault), choice, serverDefault: serverDefault !== false }
const bgSubs = new Set<() => void>()
// Marked on the page while it's off, for a desktop app that keeps its window looking visible
// to stop background throttling: it lets the page be hidden again when this says off.
function markPage() {
  if (typeof document === 'undefined') return
  if (bg.keep) delete document.documentElement.dataset.backgroundPlay
  else document.documentElement.dataset.backgroundPlay = 'off'
}
markPage()
function bgChanged() {
  bg = { keep: keepsPlaying(choice, serverDefault), choice, serverDefault: serverDefault !== false }
  markPage()
  bgSubs.forEach((f) => f())
  onVisibility()
}
export const keepPlaying = () => bg.keep
const bgSubscribe = (f: () => void) => { bgSubs.add(f); return () => { bgSubs.delete(f) } }
export const useBackgroundPlay = () => useSyncExternalStore(bgSubscribe, () => bg)
// This device's own choice; null goes back to the server's.
export function setBackgroundChoice(c: Choice) {
  choice = c
  try { if (c) localStorage.setItem(BACKGROUND_KEY, c); else localStorage.removeItem(BACKGROUND_KEY) } catch { /* kept for this visit */ }
  bgChanged()
}
// What the server says a device that hasn't chosen does: backgroundPlay in /guide and /tune.
export function noteBackgroundDefault(b: boolean | undefined) {
  if (typeof b === 'boolean' && b !== serverDefault) { serverDefault = b; bgChanged() }
}
if (typeof window !== 'undefined') window.addEventListener('storage', (e) => { if (e.key === BACKGROUND_KEY) { choice = readChoice(e.newValue); bgChanged() } })

function onVisibility() {
  if (typeof document === 'undefined') return
  const was = hidden
  hidden = document.hidden
  if (!video) return // no player yet: nothing to keep or let go
  if (sayStillHere(hidden, bg.keep, live.key)) startTicker(); else stopTicker()
  if (letGo(hidden, bg.keep) || (was && !hidden)) settle()
  if (was && !hidden && video && resumeOnShow(bg.keep, video.paused, live.away) && live.ready) video.play().catch(() => {})
}

// The worker's tick, in place of a timer on the page.
let ticker: Worker | null = null
let tickerUrl = ''
let tickerFallback: ReturnType<typeof setInterval> | undefined
function startTicker() {
  if (ticker || tickerFallback !== undefined) return
  try {
    tickerUrl = URL.createObjectURL(new Blob([`setInterval(() => postMessage(0), ${STILL_HERE_MS})`], { type: 'text/javascript' }))
    ticker = new Worker(tickerUrl)
    ticker.onmessage = stillHere
  } catch {
    stopTicker()
    tickerFallback = setInterval(stillHere, STILL_HERE_MS)
  }
}
function stopTicker() {
  ticker?.terminate()
  ticker = null
  if (tickerUrl) URL.revokeObjectURL(tickerUrl)
  tickerUrl = ''
  clearInterval(tickerFallback)
  tickerFallback = undefined
}
// Asking for the playlist is what a player does anyway, so it counts as this viewer watching.
function stillHere() {
  if (!sayStillHere(hidden, bg.keep, live.key)) { stopTicker(); return }
  if (live.url) preflight(live.url, new AbortController().signal).catch(() => { /* the next tick tries again */ })
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

function settle(leaveNow = false) {
  let top: Slot | undefined
  for (let i = slots.length - 1; i >= 0 && !top; i--) if (slots[i].key) top = slots[i]
  // Hidden, on a device that doesn't keep playing: the channel goes now, and comes back when shown.
  if (letGo(hidden, bg.keep)) { top = undefined; leaveNow = true }
  place(top?.el || null)
  if (top) {
    clearTimeout(leaveTimer)
    leaveTimer = undefined
    if (top.key !== live.key) start(top.key)
    return
  }
  if (!live.key) return
  if (leaveNow) { clearTimeout(leaveTimer); stop(); return }
  if (leaveTimer === undefined) {
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
      noteBackgroundDefault(i.backgroundPlay)
      setLive({ info: i })
      const u = i.url + (i.url.includes('?') ? '&' : '?') + 'viewer=' + viewer
      await preflight(u, a.signal)
      if (!a.signal.aborted) setLive({ url: u })
    })
    .catch((e) => { if (!a.signal.aborted) setLive({ err: e.message }) })
}

function stop() {
  stopTicker()
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
    const had = s.key
    s.key = key
    s.el = slot.current
    settle(!!had && !key)
  }, [key, slot])
  return useLiveState()
}
