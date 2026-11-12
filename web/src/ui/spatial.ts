// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Spatial navigation: given where focus is and a direction, which control is
// next? Nothing here touches the page, so the rule is easy to test
// (spatial.test.mjs) and to copy into a native app. docs/DESIGN.md has the contract;
// tv.ts is the web app's use of it.

export type Dir = 'up' | 'down' | 'left' | 'right'
export type Box = { left: number; top: number; right: number; bottom: number }

// Moving along a line (left, right) staying on that line matters most: a card two
// along on this row beats a nearer one on the next row. Moving up or down the eye
// is happy to shift sideways, but still prefers what is straight ahead.
// The numbers are design tokens (`nav.weights` in themes/tokens.json) so the Swift and Kotlin ports
// score the same way; these are the defaults, and setWeights() takes the server's.
export type Weights = { offLine: Record<Dir, number>; offCentre: Record<Dir, number>; leavesLine: Record<Dir, number> }
// Kept as these three constants: scripts/design-tokens.py --check compares them with `nav.weights` in the tokens.
const OFF_LINE: Record<Dir, number> = { left: 4, right: 4, up: 1.2, down: 1.2 }
const OFF_CENTRE: Record<Dir, number> = { left: 0.1, right: 0.1, up: 0.3, down: 0.3 }
const LEAVES_LINE: Record<Dir, number> = { left: 4000, right: 4000, up: 0, down: 0 }
export const DEFAULT_WEIGHTS: Weights = { offLine: OFF_LINE, offCentre: OFF_CENTRE, leavesLine: LEAVES_LINE }
let W: Weights = DEFAULT_WEIGHTS

// weightsFrom: the weights out of a tokens document's `nav` block. Anything missing or not a finite
// number keeps its default, and unknown keys are ignored, as every token reader must.
export function weightsFrom(nav: unknown): Weights {
  const src = (nav && typeof nav === 'object' ? (nav as { weights?: unknown }).weights : null) as Partial<Record<keyof Weights, Partial<Record<Dir, unknown>>>> | null
  const out = {} as Weights
  for (const k of Object.keys(DEFAULT_WEIGHTS) as (keyof Weights)[]) {
    const row = {} as Record<Dir, number>
    for (const d of ['left', 'right', 'up', 'down'] as Dir[]) {
      const v = src?.[k]?.[d]
      row[d] = typeof v === 'number' && Number.isFinite(v) && v >= 0 ? v : DEFAULT_WEIGHTS[k][d]
    }
    out[k] = row
  }
  return out
}
export function setWeights(nav?: unknown) { W = nav ? weightsFrom(nav) : DEFAULT_WEIGHTS }
export const currentWeights = () => W

// score: how far b is from a in that direction, smaller being better; null when b
// isn't that way at all.
export function score(a: Box, b: Box, dir: Dir): number | null {
  const ax = (a.left + a.right) / 2, ay = (a.top + a.bottom) / 2
  const bx = (b.left + b.right) / 2, by = (b.top + b.bottom) / 2
  // How far from straight ahead, to the nearest part of it: a programme in a guide can be the width of the
  // screen, and its middle is no measure of how close it is.
  const offCentre = (p: number, lo: number, hi: number) => (p < lo ? lo - p : p > hi ? p - hi : 0)
  let gap: number, near: number, far: number, bNear: number, bFar: number, centre: number
  switch (dir) {
    case 'right': if (bx <= ax + 1) return null; gap = b.left - a.right; near = a.top; far = a.bottom; bNear = b.top; bFar = b.bottom; centre = offCentre(ay, b.top, b.bottom); break
    case 'left': if (bx >= ax - 1) return null; gap = a.left - b.right; near = a.top; far = a.bottom; bNear = b.top; bFar = b.bottom; centre = offCentre(ay, b.top, b.bottom); break
    case 'down': if (by <= ay + 1) return null; gap = b.top - a.bottom; near = a.left; far = a.right; bNear = b.left; bFar = b.right; centre = offCentre(ax, b.left, b.right); break
    default: if (by >= ay - 1) return null; gap = a.top - b.bottom; near = a.left; far = a.right; bNear = b.left; bFar = b.right; centre = offCentre(ax, b.left, b.right)
  }
  // How far off to the side it is: nothing if the two overlap across the direction of travel.
  const aside = Math.max(0, Math.max(near, bNear) - Math.min(far, bFar))
  return Math.max(0, gap) + aside * W.offLine[dir] + (aside > 0 ? W.leavesLine[dir] : 0) + centre * W.offCentre[dir]
}

// ranked: every candidate that way, best first.
export function ranked<T>(from: Box, candidates: T[], box: (c: T) => Box, dir: Dir): T[] {
  const scored: [number, T][] = []
  for (const c of candidates) {
    const s = score(from, box(c), dir)
    if (s !== null) scored.push([s, c])
  }
  return scored.sort((x, y) => x[0] - y[0]).map((x) => x[1])
}

// nearest: whichever candidate is closest to a place, in any direction. For when what had
// the focus has gone, so there's no direction to go in.
export function nearest<T>(from: Box, candidates: T[], box: (c: T) => Box): T | undefined {
  const ax = (from.left + from.right) / 2, ay = (from.top + from.bottom) / 2
  let best: T | undefined, bestDist = Infinity
  for (const c of candidates) {
    const b = box(c)
    const d = Math.hypot((b.left + b.right) / 2 - ax, (b.top + b.bottom) / 2 - ay)
    if (d < bestDist) { best = c; bestDist = d }
  }
  return best
}

// The Back button, as each kind of remote reports it. webOS and Tizen TVs send a
// key code with no name for it.
export const isBackKey = (e: { key: string; keyCode?: number }) => e.key === 'GoBack' || e.key === 'BrowserBack' || e.keyCode === 461 || e.keyCode === 10009
