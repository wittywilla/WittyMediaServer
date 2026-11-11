// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A play queue as the Remote Protocol means it (docs/REMOTE-PROTOCOL.md, "Queues"): a list of
// item ids and which one is playing. Plain rules, tested in queue.test.mjs. The video player
// keeps one of these (videoQueue.ts); the music player has its own queue and mirrors it.

export const MAX_QUEUE = 200

export type Queue = { ids: number[]; index: number }

const valid = (id: unknown): id is number => typeof id === 'number' && Number.isInteger(id) && id > 0

// makeQueue: a queue from what a `play` command carried, or null when there is none worth
// keeping: a single item is just that item. Ids that aren't items are an error, as the
// server would have refused them; the index is brought inside the list.
export function makeQueue(ids: readonly number[] | undefined, index = 0): Queue | null {
  if (!ids || ids.length < 2) return null
  if (ids.length > MAX_QUEUE || !ids.every(valid)) return null
  const i = Number.isInteger(index) ? Math.max(0, Math.min(ids.length - 1, index)) : 0
  return { ids: [...ids], index: i }
}

export const currentId = (q: Queue | null): number | null => (q ? q.ids[q.index] ?? null : null)
export const nextId = (q: Queue | null): number | null => (q && q.index + 1 < q.ids.length ? q.ids[q.index + 1] : null)
export const previousId = (q: Queue | null): number | null => (q && q.index > 0 ? q.ids[q.index - 1] : null)

// step: the queue moved one along, or null at either end (it doesn't wrap round).
export function step(q: Queue | null, dir: 1 | -1): Queue | null {
  if (!q) return null
  const i = q.index + dir
  return i >= 0 && i < q.ids.length ? { ids: q.ids, index: i } : null
}

// jump: the queue at another entry, or null when there is no such entry.
export function jump(q: Queue | null, index: number): Queue | null {
  return q && Number.isInteger(index) && index >= 0 && index < q.ids.length ? { ids: q.ids, index } : null
}

// opened: what becomes of the queue when the player opens an item. One of the queue's own
// moves it there: the entry after the one playing if that is it (the usual case, and the right
// one when an item is in the queue twice), otherwise the nearest later entry, otherwise an
// earlier one. Anything else was chosen by hand, and ends the queue.
export function opened(q: Queue | null, id: number): Queue | null {
  if (!q) return null
  if (q.ids[q.index] === id) return q
  let at = q.ids.indexOf(id, q.index + 1)
  if (at < 0) at = q.ids.lastIndexOf(id, q.index)
  return at >= 0 ? { ids: q.ids, index: at } : null
}

// reported: the queue as a state report carries it, or nothing for no queue.
export function reported(q: Queue | null): { queue?: number[]; queueIndex?: number } {
  return q ? { queue: q.ids, queueIndex: q.index } : {}
}

// mirror: a queue from a player that keeps its own list (the music player), for the report.
// A list too long for the protocol is reported as the part around what's playing, so the
// index still names the right entry; ids that aren't library items leave nothing to report.
export function mirror(ids: readonly number[], index: number): Queue | null {
  if (ids.length < 2 || index < 0 || index >= ids.length || !ids.every(valid)) return null
  if (ids.length <= MAX_QUEUE) return { ids: [...ids], index }
  const start = Math.max(0, Math.min(ids.length - MAX_QUEUE, index - Math.floor(MAX_QUEUE / 4)))
  return { ids: ids.slice(start, start + MAX_QUEUE), index: index - start }
}
