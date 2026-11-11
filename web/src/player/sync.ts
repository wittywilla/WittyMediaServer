// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Keeping a live picture at the same moment as everyone else's. Every segment of a channel says
// when on the air it starts (EXT-X-PROGRAM-DATE-TIME), so every player that keeps to "the server's
// time now, less three target durations" shows the same moment (the playlist's HOLD-BACK starts it
// there). These are the rules for that, apart from any player, so a watch-together feature can use
// the same ones.

// Under STILL seconds off, nothing is done; up to SEEK, the picture plays RATE faster or slower
// until it's within SETTLED (3% is too little to notice, and audio keeps its pitch); beyond
// that, or after a stall, it jumps. With these, two viewers rest within a quarter of a second
// of each other.
export const STILL = 0.12
export const SETTLED = 0.04
export const SEEK = 3
export const RATE = 0.03

export type SyncStep = { rate: number } | { seekBy: number }

// `behind`: seconds this player is behind where it should be (negative: ahead). `nudging`:
// whether it's already playing faster or slower, which carries on until it's settled.
export function syncStep(behind: number, nudging: boolean): SyncStep {
  const off = Math.abs(behind)
  if (off >= SEEK) return { seekBy: behind }
  if (off < (nudging ? SETTLED : STILL)) return { rate: 1 }
  return { rate: behind > 0 ? 1 + RATE : 1 - RATE }
}

// How far the server's clock is ahead of this one, in ms, from timed requests: the one with the
// shortest round trip is the one least delayed on the way, so its midpoint is the best guess.
export type ClockSample = { sent: number; server: number; got: number }
export function clockOffset(samples: ClockSample[]): number {
  let best: ClockSample | null = null
  for (const s of samples) if (s.got >= s.sent && (!best || s.got - s.sent < best.got - best.sent)) best = s
  return best ? best.server - (best.sent + best.got) / 2 : 0
}
