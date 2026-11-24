// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Time-shift for everyone (#648): what a channel player decides when the viewer starts over,
// rewinds, skips a break or comes back from a long pause. Times are the schedule's, in Unix
// seconds; the stream's program date-times say where the picture is in it. No React here, so
// node:test can check it.

export type Span = { start: number; end: number }
export type Timeline = {
  programme: { start: number; end: number; title: string; kind: string }
  breaks: Span[]
  from?: number
  live: number
  earliest: number
}

// A start over closer to live than this is live: the player goes back to the shared stream.
export const LIVE_EDGE_SEC = 20

// The break the picture is in at moment t, if any.
export function breakAt(breaks: Span[] | undefined, t: number | null): Span | null {
  if (t == null || !breaks) return null
  for (const b of breaks) if (b.start <= t && t < b.end) return b
  return null
}

// Where live is now, given where it was when the server said (liveAt, at serverTime) and the
// clock since.
export const liveNowFrom = (live: number, serverTime: number, nowSec: number) => live + Math.max(0, nowSec - serverTime)

// What a seek to schedule moment target does. The picture is at media time `cur`, which is
// schedule moment `sched`; the stream has media times [winStart, winEnd] loaded.
// - 'live': it's at (or past) live, so back to the channel everyone shares
// - 'seek': within what's loaded; `to` is the media time
// - 'restart': further back than the window: the viewer's stream starts again from `at`
export type SeekPlan = { kind: 'live' } | { kind: 'seek'; to: number } | { kind: 'restart'; at: number }
export function planSeek(target: number, sched: number, cur: number, win: { start: number; end: number }, live: number, earliest = -Infinity): SeekPlan {
  if (target >= live - LIVE_EDGE_SEC) return { kind: 'live' }
  const at = Math.max(earliest, target)
  const to = cur + (at - sched)
  if (to >= win.start - 0.5 && to <= win.end) return { kind: 'seek', to: Math.max(win.start, to) }
  return { kind: 'restart', at: Math.floor(at) }
}

// After a pause: a viewer who paused longer than the stream keeps (bufferSec, less a margin
// for the window moving on while they decide) starts again from where they paused, rather
// than being pushed forward to whatever is left.
export function resumePlan(pausedAtMs: number, nowMs: number, bufferSec: number): 'resume' | 'restart' {
  const keep = Math.max(30, bufferSec - 30)
  return (nowMs - pausedAtMs) / 1000 > keep ? 'restart' : 'resume'
}

// The address that starts this channel over from a moment, ending with the programme.
export const startOverPath = (key: string, from: number, end: number) => `/tv/watch/${key}?from=${Math.floor(from)}&to=${Math.floor(Math.max(end, from + 1))}`

// Whether a key is a library channel's: those start over on a stream of the viewer's own;
// a tuner channel starts over from its archive.
export const isLibraryKey = (key: string) => key.startsWith('lib-')
