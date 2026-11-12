// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/remote/queue.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { currentId, jump, makeQueue, MAX_QUEUE, mirror, nextId, opened, previousId, reported, step } from './queue.ts'
import { queued, queuedBefore, queueFor, setVideoQueue, videoQueue } from './videoQueue.ts'

test('a queue is a list and where it is', () => {
  const q = makeQueue([812, 813, 814], 1)
  assert.deepEqual(q, { ids: [812, 813, 814], index: 1 })
  assert.equal(currentId(q), 813)
  assert.equal(nextId(q), 814)
  assert.equal(previousId(q), 812)
  assert.deepEqual(reported(q), { queue: [812, 813, 814], queueIndex: 1 })
})

test('one item is not a queue, and no queue reports nothing', () => {
  assert.equal(makeQueue([812], 0), null)
  assert.equal(makeQueue(undefined), null)
  assert.equal(makeQueue([]), null)
  assert.deepEqual(reported(null), {})
  assert.equal(currentId(null), null)
  assert.equal(nextId(null), null)
})

test('the protocol\'s limit is 200 items, and ids are items', () => {
  const many = Array.from({ length: MAX_QUEUE }, (_, i) => i + 1)
  assert.equal(makeQueue(many, 199).index, 199)
  assert.equal(makeQueue([...many, 201], 0), null)
  assert.equal(makeQueue([1, 0, 3]), null)
  assert.equal(makeQueue([1, -2]), null)
  assert.equal(makeQueue([1, 2.5]), null)
})

test('an index outside the list is brought inside', () => {
  assert.equal(makeQueue([1, 2, 3], 9).index, 2)
  assert.equal(makeQueue([1, 2, 3], -1).index, 0)
  assert.equal(makeQueue([1, 2, 3], 1.5).index, 0)
})

test('the queue is its own copy of the list', () => {
  const ids = [1, 2, 3]
  const q = makeQueue(ids, 0)
  ids.push(4)
  assert.deepEqual(q.ids, [1, 2, 3])
})

test('next and previous move along and stop at the ends', () => {
  const q = makeQueue([1, 2, 3], 0)
  assert.equal(step(q, -1), null)
  const b = step(q, 1)
  assert.equal(b.index, 1)
  const c = step(b, 1)
  assert.equal(c.index, 2)
  assert.equal(step(c, 1), null)
  assert.equal(nextId(c), null)
  assert.equal(previousId(q), null)
  assert.equal(step(null, 1), null)
})

test('jumping goes to any entry that exists', () => {
  const q = makeQueue([1, 2, 3], 0)
  assert.equal(jump(q, 2).index, 2)
  assert.equal(jump(q, 3), null)
  assert.equal(jump(q, -1), null)
  assert.equal(jump(null, 0), null)
})

test('opening the next item moves the queue; anything from outside ends it', () => {
  const q = makeQueue([10, 20, 30], 0)
  assert.equal(opened(q, 10), q)
  assert.equal(opened(q, 20).index, 1)
  assert.equal(opened(q, 30).index, 2)
  assert.equal(opened(makeQueue([10, 20, 30], 2), 10).index, 0)
  assert.equal(opened(q, 99), null)
  assert.equal(opened(null, 10), null)
})

test('an item in the queue twice plays twice', () => {
  const q = makeQueue([10, 20, 10, 30], 1)
  // After 20 comes the second 10, not the first.
  assert.equal(opened(q, 10).index, 2)
  // And from the end, the nearest one before.
  assert.equal(opened(makeQueue([10, 20, 10, 30], 3), 10).index, 2)
})

test('the music player\'s own list is reported as a queue', () => {
  assert.deepEqual(mirror([6, 11, 12], 1), { ids: [6, 11, 12], index: 1 })
  assert.equal(mirror([6], 0), null)
  assert.equal(mirror([6, 11], 5), null)
  // Tracks that aren't library items (ids of 0) leave nothing to report.
  assert.equal(mirror([6, 0], 0), null)
})

test('a list longer than the protocol allows is reported around what is playing', () => {
  const ids = Array.from({ length: 500 }, (_, i) => i + 1)
  const m = mirror(ids, 300)
  assert.equal(m.ids.length, MAX_QUEUE)
  assert.equal(m.ids[m.index], 301)
  assert.ok(m.index > 0 && m.index < MAX_QUEUE - 1)
  const start = mirror(ids, 0)
  assert.equal(start.index, 0); assert.equal(start.ids[0], 1)
  const end = mirror(ids, 499)
  assert.equal(end.ids[end.index], 500); assert.equal(end.ids.length, MAX_QUEUE)
})

test('the video player walks its queue through each item\'s "next"', () => {
  setVideoQueue(makeQueue([3, 4, 5], 0))
  // The item opening is the queue's: what follows it is the queue's next entry, whatever the server said.
  assert.deepEqual(queued({ id: 3, nextId: 77 }), { id: 3, nextId: 4 })
  assert.deepEqual(reported(queueFor(3)), { queue: [3, 4, 5], queueIndex: 0 })
  assert.equal(queueFor(4), null) // not there yet
  assert.equal(queuedBefore(3), null)
  assert.deepEqual(queued({ id: 4, nextId: 5 }), { id: 4, nextId: 5 })
  assert.equal(videoQueue().index, 1)
  assert.equal(queuedBefore(4), 3)
  // The last entry is the end: no next episode from outside the queue.
  assert.deepEqual(queued({ id: 5, nextId: 6 }), { id: 5, nextId: undefined })
  // Something else chosen by hand ends the queue and is left as it came.
  const other = { id: 9, nextId: 10 }
  assert.equal(queued(other), other)
  assert.equal(videoQueue(), null)
  assert.deepEqual(reported(queueFor(9)), {})
})
