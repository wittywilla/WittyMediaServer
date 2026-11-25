// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Live TV's Auto quality: a channel at a rate the link carries. A live stream can't be made ahead and
// stored, so a stream faster than the link stalls every few seconds: a home server's upload, reached
// peer to peer from away, carries 5 to 20 Mbit/s, and a 1080p channel converted on a small machine came
// at 6 to 7. When the viewer hasn't chosen a quality, the channel starts on a step of LADDER (lower over
// peer to peer), and steps down when the stream needs more than the link gives, or keeps stalling; up
// again, a step at a time, once the link has had plenty of room for a while. The step is kept on this
// device, so the next channel starts where the last one settled.
//
// No imports, so it can be tested on its own.

export type Step = { height: number; kbps: number }
// 0 is no cap: the profile's own.
export const LADDER: Step[] = [{ height: 0, kbps: 0 }, { height: 1080, kbps: 6000 }, { height: 720, kbps: 3000 }, { height: 540, kbps: 1500 }, { height: 360, kbps: 800 }]
// Where a device reached peer to peer starts, before it has learned anything: 720p at 3 Mbit/s fits a
// 5 Mbit/s upload with room for its own spikes.
export const PEER_START = 2
// The stream may use this much of what the link gives; the rest is room for a burst and the network's own.
export const SHARE = 0.8
// Stalls in a window that mean the step is too high, whatever the arithmetic says.
export const STALLS = 2
// Up a step only with this much room for the next one (its cap times this), and never sooner than UP_AFTER_SEC.
export const UP_ROOM = 2.5
export const UP_AFTER_SEC = 90

export type Frag = { bytes: number; durationSec: number; loadMs: number }
export type Look = { frags: Frag[]; stalls: number; sinceChangeSec: number }

// The choice string the server reads for a step (as player/quality.ts writes it): h<height>-b<kbps>.
export function stepChoice(step: number): string {
  const s = LADDER[Math.max(0, Math.min(LADDER.length - 1, step))]
  return s.kbps ? `h${s.height}-b${s.kbps}` : ''
}

// The step a channel starts on: the one this device settled on, or by how it reaches the server.
export function startStep(saved: number | null, via: string): number {
  if (saved != null && saved >= 0 && saved < LADDER.length) return saved
  return via.startsWith('p2p') ? PEER_START : 0
}

// The rate a stream comes at, and what the link gave while loading it, in bits a second, from its last
// few fragments (sizes, their length in time, and how long each took to come).
export function rates(frags: Frag[]): { stream: number; link: number } {
  const bits = frags.reduce((n, f) => n + f.bytes * 8, 0)
  const sec = frags.reduce((n, f) => n + f.durationSec, 0)
  const load = frags.reduce((n, f) => n + f.loadMs, 0) / 1000
  return { stream: sec > 0 ? bits / sec : 0, link: load > 0 ? bits / load : 0 }
}

// What to do now: a step down (higher number), up, or nothing.
export function autoStep(step: number, look: Look): 'down' | 'up' | 'keep' {
  if (look.frags.length < 3) return 'keep'
  const { stream, link } = rates(look.frags)
  const last = LADDER.length - 1
  if (step < last && (look.stalls >= STALLS || (link > 0 && stream > SHARE * link))) return 'down'
  if (step > 0 && look.stalls === 0 && look.sinceChangeSec >= UP_AFTER_SEC) {
    const next = LADDER[step - 1]
    // Up to no cap only with a great deal of room: a 1080p channel at the profile's own rate.
    const need = (next.kbps || 12000) * 1000 * UP_ROOM
    if (link >= need) return 'up'
  }
  return 'keep'
}

// ------------------------------------------------------------------- this device's step

const KEY = 'wms.tv.auto'
export function savedStep(): number | null {
  try {
    const v = Number(localStorage.getItem(KEY))
    return localStorage.getItem(KEY) != null && Number.isInteger(v) && v >= 0 && v < LADDER.length ? v : null
  } catch { return null }
}
export function saveStep(step: number) {
  try { localStorage.setItem(KEY, String(step)) } catch { /* this visit only */ }
}
// How this device last reached the server (the tune answer's quality.via), so the next tune can start on
// the right step before the server says.
const VIA = 'wms.tv.via'
export function lastVia(): string { try { return localStorage.getItem(VIA) || '' } catch { return '' } }
export function noteVia(via: string | undefined) { if (via) try { localStorage.setItem(VIA, via) } catch { /* not kept */ } }

// The step live TV asks for now, when the viewer hasn't chosen a quality.
export const autoChoice = () => stepChoice(startStep(savedStep(), lastVia()))
