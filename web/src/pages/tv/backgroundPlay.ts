// "Keep playing when this tab is in the background", one choice per device. On, a channel
// carries on with its sound in a background tab or a minimised window, for as long as it would
// in front, and the server keeps it going. Off, the player lets its channel go when the page is
// hidden, so a tuner is free, and tunes in again at what's on now when the page is shown.
//
// A device that hasn't chosen goes by the server: `backgroundPlay` in /guide and /tune, which
// is on unless the owner set new devices to stop. The apps keep the same choice under the same
// name. See docs/CLIENT-API.md, "Playing in the background". No imports, so it can be tested
// on its own.

// Where this device's own choice is kept: "1", "0", or nothing for the server's.
export const BACKGROUND_KEY = 'wms.tv.backgroundPlay'

// How often a player in the background tells the server it's still there, when its stream
// may have gone quiet (a muted or paused player, which browsers slow down). The server counts
// a viewer as gone after 30 seconds without a word, and stops a channel 45 seconds after the
// last request.
export const STILL_HERE_MS = 15000

export type Choice = '1' | '0' | null

export function readChoice(v: string | null | undefined): Choice {
  return v === '1' || v === '0' ? v : null
}

// Whether a player keeps its channel while the page is hidden.
export function keepsPlaying(choice: Choice, serverDefault: boolean | undefined): boolean {
  if (choice === '1') return true
  if (choice === '0') return false
  return serverDefault !== false
}

// Whether the player has let its channel go for now: only when hidden, and only when it doesn't
// keep playing. Shown again, it tunes in again.
export const letGo = (hidden: boolean, keep: boolean): boolean => hidden && !keep

// Whether a hidden player should tell the server it's still there: it keeps playing, and has a
// channel.
export const sayStillHere = (hidden: boolean, keep: boolean, key: string): boolean => hidden && keep && !!key

// A browser may pause a silent video in a background tab; that pause isn't the viewer's, so
// shown again, a player that keeps playing starts again. A pause the viewer asked for (on the
// page, or from the system's media controls while it played with sound) is theirs.
export const resumeOnShow = (keep: boolean, paused: boolean, viewerPaused: boolean): boolean => keep && paused && !viewerPaused
