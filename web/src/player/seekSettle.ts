// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Seeks come in bursts: skip pressed five times, the arrow key held down, clicks along the
// timeline. For a converted stream, every seek past what is buffered makes the server stop
// FFmpeg and start it again somewhere else, and a burst of them kept it restarting while the
// player's requests for the earlier places failed one after another. Here the first seek of a
// burst loads at once (a single seek waits for nothing), and the rest only load once the
// viewer has stopped seeking, from wherever they ended up.

export const SEEK_BURST_MS = 500 // a seek this soon after the one before is part of a burst
export const SEEK_SETTLE_MS = 350 // and the burst is over once no seek has come for this long

export type Loader = { stopLoad(): void; startLoad(at?: number): void }
export type Media = {
  currentTime: number
  buffered: { length: number; start(i: number): number; end(i: number): number }
  addEventListener(type: 'seeking', fn: () => void): void
  removeEventListener(type: 'seeking', fn: () => void): void
}
type Clock = { now(): number; set(fn: () => void, ms: number): unknown; clear(t: unknown): void }
const realClock: Clock = { now: () => performance.now(), set: (fn, ms) => setTimeout(fn, ms), clear: (t) => clearTimeout(t as number) }

// inBuffer: t is in what the player already holds, so seeking there loads nothing.
export function inBuffer(b: Media['buffered'], t: number) {
  for (let i = 0; i < b.length; i++) if (t >= b.start(i) && t < b.end(i) - 0.5) return true
  return false
}

// settleSeeks watches v's seeks for hls, and returns what stops watching. It must be called
// before hls.js is attached to v, so that it hears of each seek first: a seek it holds back
// stops loading before hls.js would ask the server for that place.
export function settleSeeks(hls: Loader, v: Media, clock: Clock = realClock) {
  let last = -Infinity
  let held = false
  let timer: unknown = null
  const resume = () => { timer = null; held = false; hls.startLoad(v.currentTime) }
  const onSeeking = () => {
    const now = clock.now()
    const burst = now - last < SEEK_BURST_MS
    last = now
    if (!held && (!burst || inBuffer(v.buffered, v.currentTime))) return
    if (!held) { held = true; hls.stopLoad() } // also cancels the request for the burst's first place
    if (timer !== null) clock.clear(timer)
    timer = clock.set(resume, SEEK_SETTLE_MS)
  }
  v.addEventListener('seeking', onSeeking)
  return () => { v.removeEventListener('seeking', onSeeking); if (timer !== null) clock.clear(timer) }
}
