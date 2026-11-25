// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Playing faster than 1×: how much the player keeps ahead, and what to do when the server
// can't convert the video as fast as it plays. Plain logic, no React, so the tests run it as it is.

// The player's buffer is counted in the film's seconds, which a player at 2× uses up twice as
// fast: 30 seconds ahead is 15 seconds of waiting room. It grows with the rate, so the room
// stays the same, up to a limit that keeps a long 4K stream from filling the browser's memory.
export const MAX_BUFFER_SEC = 300

export function bufferFor(rate: number, base: number, baseMax: number): { maxBufferLength: number; maxMaxBufferLength: number } {
  const r = Number.isFinite(rate) && rate > 1 ? rate : 1
  return { maxBufferLength: Math.min(Math.round(base * r), MAX_BUFFER_SEC), maxMaxBufferLength: Math.min(Math.round(baseMax * r), MAX_BUFFER_SEC * 2) }
}

// How the server's conversion keeps up (GET /hls/{id}/work, pace).
export type Pace = { speed: number; rate: number; leadSec: number; keepingUp: boolean; fastest: boolean }

// The sizes the player steps down through, largest first.
const STEPS = [2160, 1440, 1080, 720, 480, 360]

// lowerHeight is the next size down from what plays now, or 0 when there's none to go to.
export function lowerHeight(height: number): number {
  if (!height || height <= STEPS[STEPS.length - 1]) return 0
  return STEPS.find((h) => h < height - 8) || 0
}

// Too slow: what the player does about a conversion that can't keep up. 'lower' asks the server
// for a smaller picture; faster than 1× it does that once, to 720p, and then 'slower' brings the speed
// down to one the server can keep up with (sustainableRate), rather than a picture that stops and
// starts, each reopen a moment of black. 'tell' only says so, when neither helps (or the viewer chose
// this quality themself and plays at 1×); 'wait' means not yet: the server's own faster setting
// comes first, and one bad reading is not enough.
export function tooSlow(p: Pace | null, behindFor: number, height: number, viewerChose: boolean): 'ok' | 'wait' | 'lower' | 'slower' | 'tell' {
  if (!p || p.keepingUp || !p.speed) return 'ok'
  if (!p.fastest || behindFor < 2) return 'wait'
  const fast = p.rate > 1
  if (viewerChose || !lowerHeight(height) || (fast && height <= 720)) return fast && sustainableRate(p.speed, p.rate) < p.rate ? 'slower' : 'tell'
  return 'lower'
}

// The playback speeds a viewer can pick, the same list as the speed menu (speed.ts SPEEDS, which this file
// can't import and still run in the tests as it is).
const SPEED_STEPS = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3, 3.5, 4, 5, 6, 8, 10, 12, 16]

// sustainableRate is the fastest speed a conversion making `speed` seconds of film a second keeps up with,
// with a tenth to spare, and never below 1×: what the player slows to, and the top of its speed menu, when
// even the smallest picture at the server's fastest setting can't keep up at the chosen speed.
export function sustainableRate(speed: number, rate: number): number {
  const fits = SPEED_STEPS.filter((s) => s >= 1 && s <= rate && s * 1.1 <= speed)
  return fits.length ? fits[fits.length - 1] : 1
}

// Words for the viewer: how fast the server converts, against the speed they play at.
export const times = (n: number) => `${Math.round(n * 100) / 100}×`
