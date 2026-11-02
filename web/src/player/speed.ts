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
