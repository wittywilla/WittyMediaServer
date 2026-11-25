// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Playback speeds, apart from the player's controls: the music player needs these on every page.

export const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, 2.5, 3, 3.5, 4, 5, 6, 8, 10, 12, 16]

// setSpeed changes the playback rate without changing the pitch of voices
// (the browser time-stretches the audio). defaultPlaybackRate keeps the
// speed when the player swaps streams (audio track, angle, next clip).
export function setSpeed(v: HTMLMediaElement, rate: number) {
  const a = v as HTMLMediaElement & { mozPreservesPitch?: boolean; webkitPreservesPitch?: boolean }
  a.preservesPitch = true
  a.mozPreservesPitch = true
  a.webkitPreservesPitch = true
  v.defaultPlaybackRate = rate
  v.playbackRate = rate
}

// A change of speed can leave the sound and the picture apart for good: the browser's time stretch
// keeps its own buffer, and a decoder that fell behind at 4× doesn't catch up by itself when the
// speed comes down again (the owner's "4× then back to normal, and they're out of step"). A seek to
// where it is lines them up again: the decoder and the stretch both start afresh at the same place.
// Done once the speed has stayed put for a moment, so stepping through the menu seeks once, and only
// for a real change. A seek to the same place costs a frame or two, and only the picture's own
// buffer, not the network.
export const REALIGN_AFTER_MS = 400
export const REALIGN_MIN_CHANGE = 0.5
// How many times, REALIGN_AFTER_MS apart, it waits for a player that can't seek yet (30 s).
export const REALIGN_TRIES = 75

// realignNeeded says a change of speed from one rate to another is big enough to line up after.
export function realignNeeded(from: number, to: number): boolean {
  return Number.isFinite(from) && Number.isFinite(to) && from > 0 && to > 0 && Math.abs(to - from) >= REALIGN_MIN_CHANGE
}

const aligning = new WeakSet<HTMLMediaElement>()

// isRealigning: the element's current seek is keepInStep's own, not the viewer's (no "At 1:23").
export const isRealigning = (v: HTMLMediaElement) => aligning.has(v)

// keepInStep lines the sound and picture of a video up again after each real change of speed.
export function keepInStep(v: HTMLVideoElement): () => void {
  let settled = v.playbackRate || 1
  let timer = 0
  let tries = 0
  const attempt = () => {
    const now = v.playbackRate || 1
    if (!realignNeeded(settled, now)) return
    // Only once it can: a player that is seeking, waiting for data (a converting server behind at
    // 4×) or paused tries again shortly, rather than giving up and leaving them apart.
    if (v.paused || v.seeking || v.ended || v.readyState < 3) {
      if (++tries < REALIGN_TRIES) timer = window.setTimeout(attempt, REALIGN_AFTER_MS)
      return
    }
    settled = now
    aligning.add(v)
    v.addEventListener('seeked', () => aligning.delete(v), { once: true })
    v.currentTime = v.currentTime
  }
  const changed = () => {
    clearTimeout(timer)
    tries = 0
    timer = window.setTimeout(attempt, REALIGN_AFTER_MS)
  }
  v.addEventListener('ratechange', changed)
  return () => { clearTimeout(timer); v.removeEventListener('ratechange', changed) }
}
