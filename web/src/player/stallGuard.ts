// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A live picture that stands still while its sound plays on: a video decoder that couldn't start, or fell
// behind, at a tune-in or at a programme join (a graphics chip that waits for a frame it may start from,
// a stream it can't keep up with). The page's clock goes on with the sound, so the picture, when it comes,
// is late. These are the rules for noticing that and putting it right, apart from any player: a seek to
// where the clock is makes the browser start both decoders again together, from the keyframe before.
// The sound is silenced while the picture stands still, so nobody hears a programme they can't see.
//
// No imports, so it can be tested on its own.

// The clock running this long with no new picture, while playing, is a stall (a frame is 17 to 42 ms).
export const STALL_SEC = 1
// A stream gets this many seeks to put it right; after that, it is left as it is.
export const MAX_FIXES = 3

export type Sample = { t: number; frames: number; paused: boolean; ready: boolean }
export type GuardState = { since: number | null; fixes: number; silenced: boolean }
export type GuardStep = { kind: 'none' } | { kind: 'silence' } | { kind: 'seek' } | { kind: 'restore' }

export const freshGuard = (): GuardState => ({ since: null, fixes: 0, silenced: false })

// One look: the clock now (t), the pictures shown so far, whether it's paused or has nothing yet, and the
// last look. It says what to do, and the state to keep.
export function guardStep(prev: Sample | null, now: Sample, st: GuardState): { step: GuardStep; st: GuardState } {
  // A new picture: whatever stood still is over.
  if (prev && now.frames > prev.frames) {
    const step: GuardStep = st.silenced ? { kind: 'restore' } : { kind: 'none' }
    return { step, st: { since: null, fixes: st.fixes, silenced: false } }
  }
  // Paused, waiting for data, or the clock not moving: nothing playing to stand still.
  if (!prev || now.paused || !now.ready || now.t <= prev.t) return { step: { kind: 'none' }, st: { ...st, since: null } }
  const since = st.since ?? prev.t
  if (now.t - since < STALL_SEC) return { step: { kind: 'none' }, st: { ...st, since } }
  if (!st.silenced) return { step: { kind: 'silence' }, st: { ...st, since, silenced: true } }
  if (st.fixes >= MAX_FIXES) return { step: { kind: 'none' }, st: { ...st, since } }
  // Seek, and give the seek a moment of its own before looking again.
  return { step: { kind: 'seek' }, st: { since: now.t, fixes: st.fixes + 1, silenced: true } }
}
