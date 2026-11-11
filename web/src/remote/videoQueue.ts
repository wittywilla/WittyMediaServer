// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The video player's play queue: what another device asked this one to play through
// (docs/REMOTE-PROTOCOL.md, "Queues"). The rules are queue.ts; this holds the one queue there
// is. The player page asks it what comes after each item it opens, so the player's own "next
// episode" (the countdown at the end, the Next button, the remote's next) walks the queue.
import { currentId, nextId, opened, previousId, type Queue } from './queue.ts'

let queue: Queue | null = null

export const videoQueue = (): Queue | null => queue
export function setVideoQueue(q: Queue | null) { queue = q }

// queueFor: the queue, if this item is the one it is at.
export const queueFor = (itemId: number): Queue | null => (currentId(queue) === itemId ? queue : null)
export const queuedBefore = (itemId: number): number | null => previousId(queueFor(itemId))

// queued: the player is opening this item. In a queue, what follows it is the queue's next
// entry and nothing else (the last entry is the end, whatever episode would have followed);
// an item from outside the queue ends the queue and is left as it came.
export function queued<T extends { id: number; nextId?: number }>(item: T): T {
  queue = opened(queue, item.id)
  return queue ? { ...item, nextId: nextId(queue) ?? undefined } : item
}
