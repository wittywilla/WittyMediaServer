// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// What went wrong in the player, in plain words with a way forward (design canvas: "When something goes wrong").
// The player passes what it knows: the message, and the browser's media error code when there is one (1 stopped,
// 2 network, 3 decode, 4 not supported).
export type PlayErrorKind = 'network' | 'decode' | 'unsupported' | 'ended' | 'subtitles' | 'other'
export type PlayError = { kind: PlayErrorKind; title: string; text: string; again: boolean; convert: boolean }

export function classifyPlayError(message: string, code?: number): PlayError {
  const m = message || ''
  if (/subtitle|captions?/i.test(m)) return { kind: 'subtitles', title: 'The subtitles didn’t load', text: 'The picture and sound are fine. Try other subtitles, or turn them off.', again: false, convert: false }
  if (/stream has ended|session (has )?(ended|expired)|ended on the server/i.test(m)) return { kind: 'ended', title: 'The stream ended', text: 'The server stopped this stream. Start it again and it carries on from where you were.', again: true, convert: false }
  if (code === 2 || /network|offline|timed? ?out|connection|failed to fetch|manifest|unreachable|50[234]\b/i.test(m)) return { kind: 'network', title: 'Can’t reach the server', text: 'The connection dropped. Check that you are online, then try again: it picks up where you were.', again: true, convert: false }
  if (code === 3 || /decode|corrupt|bufferAppend|demux/i.test(m)) return { kind: 'decode', title: 'This can’t be shown here', text: 'This device couldn’t decode the picture or sound. Playing it converted usually works.', again: true, convert: true }
  if (code === 4 || /not supported|can.?t be played|unsupported/i.test(m)) return { kind: 'unsupported', title: 'This file can’t play here', text: 'This device doesn’t handle the file as it is. Playing it converted usually works.', again: true, convert: true }
  return { kind: 'other', title: 'This can’t play right now', text: m || 'Something went wrong.', again: true, convert: false }
}
