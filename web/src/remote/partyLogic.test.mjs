// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A watch party's rules for one player, fed with made-up rooms, players and clocks.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  ADVANCE_AFTER, CHEAP_SEEK_MS, HOLD_TRIES, INVITE_POLL_MS, INVITE_POLL_OFF_MS, RESTART_SEEK_MS,
  invitePollDelay, keepDeclined, maySeek, newSeekMemo, openInvites, partyKind, partyStep, positionAt, queueAdd, queueEditable,
  queueMove, queuePlayNow, queueRemove, seasonQueue, seekAim, seekLanded, shouldAdvance, tidyCode,
} from './partyLogic.ts'

const T = 1_794_000_000_000
const state = (o = {}) => ({ rev: 5, seq: 2, itemId: 7, index: 0, positionSec: 100, atMs: T, paused: false, waiting: false, rate: 1, ...o })
const player = (o = {}) => ({ pos: 100, duration: 600, ended: false, seeking: false, canPlay: true, ...o })
const opts = { nudging: false, cheapSeek: true }

test('the room stands still while paused, waiting, or before its start, and moves at its rate after', () => {
  assert.equal(positionAt(state({ paused: true }), T + 9000), 100)
  assert.equal(positionAt(state({ waiting: true }), T + 9000), 100)
  assert.equal(positionAt(state(), T - 500), 100)
  assert.equal(positionAt(state(), T + 4000), 104)
  assert.equal(positionAt(state({ rate: 1.5 }), T + 4000), 106)
})

test('only videos and songs are kept in step: not discs with menus, books or games', () => {
  assert.equal(partyKind('video'), 'video')
  assert.equal(partyKind('audio'), 'music')
  for (const k of ['dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder', 'cd-image', 'book', 'game', '', undefined]) assert.equal(partyKind(k), 'none')
})

test('a paused room holds the player on its place: exactly on a file, nearly on a converted stream', () => {
  const s = state({ paused: true })
  let st = partyStep(s, T + 5000, player({ pos: 100.3 }), opts)
  assert.equal(st.play, false)
  assert.equal(st.seekTo, 100)
  st = partyStep(s, T + 5000, player({ pos: 100.3 }), { nudging: false, cheapSeek: false })
  assert.equal(st.seekTo, null)
  assert.equal(st.ready, true)
  st = partyStep(s, T + 5000, player({ pos: 103 }), { nudging: false, cheapSeek: false })
  assert.equal(st.seekTo, 100)
  assert.equal(st.ready, false) // too far off to say so
})

test('a start set in the future is held until its moment, to the millisecond', () => {
  const st = partyStep(state({ atMs: T + 800 }), T, player(), opts)
  assert.equal(st.play, false)
  assert.equal(st.startInMs, 800)
  assert.equal(st.seekTo, null)
  assert.equal(partyStep(state({ atMs: T + 800 }), T + 800, player(), opts).play, true)
})

test('while playing: left alone when close, nudged when a little off, sent there when far off', () => {
  let st = partyStep(state(), T + 10000, player({ pos: 109.95 }), opts)
  assert.deepEqual([st.play, st.seekTo, st.rate, st.nudging], [true, null, 1, false])
  st = partyStep(state(), T + 10000, player({ pos: 109 }), opts)
  assert.deepEqual([st.seekTo, st.rate, st.nudging], [null, 1.03, true])
  st = partyStep(state(), T + 10000, player({ pos: 111 }), opts)
  assert.deepEqual([st.seekTo, st.rate], [null, 0.97])
  st = partyStep(state(), T + 10000, player({ pos: 60 }), opts)
  assert.deepEqual([st.seekTo, st.rate, st.nudging, st.ready], [110, 1, false, false])
  // The room's own speed is what the nudge is a fraction of.
  st = partyStep(state({ rate: 2 }), T + 10000, player({ pos: 119 }), opts)
  assert.equal(st.rate, 2 * 1.03)
})

test('not ready while loading or mid-seek, however near', () => {
  assert.equal(partyStep(state({ waiting: true }), T, player({ canPlay: false }), opts).ready, false)
  assert.equal(partyStep(state({ waiting: true }), T, player({ seeking: true }), opts).ready, false)
  assert.equal(partyStep(state({ waiting: true }), T, player(), opts).ready, true)
})

test('a room that has run past the end of the title rests at the end and holds nobody up', () => {
  const s = state({ positionSec: 590 })
  let st = partyStep(s, T + 30000, player({ pos: 600, ended: true }), opts)
  assert.deepEqual([st.over, st.play, st.seekTo, st.ready], [true, false, null, true])
  // Someone who arrives after the end is taken near it once, not sent past it again and again.
  st = partyStep(s, T + 30000, player({ pos: 20 }), opts)
  assert.deepEqual([st.over, st.play, st.seekTo, st.ready], [true, false, 599, true])
  // With no length known there is no end to judge by.
  assert.equal(partyStep(s, T + 30000, player({ pos: 620, duration: 0 }), opts).over, false)
})

test('a seek is given its time before another is tried: a short time on a file, a long one on a converted stream', () => {
  const ok = { seeking: false, canPlay: true, loaded: true }
  const busy = { seeking: false, canPlay: false, loaded: true }
  const m = newSeekMemo()
  assert.equal(maySeek(m, 1000, 50, true, ok, false), true)
  // The new stream hasn't arrived: asking again would only start it over.
  assert.equal(maySeek(m, 6000, 55, true, busy, false), false)
  assert.equal(maySeek(m, 1000 + RESTART_SEEK_MS - 1, 70, true, busy, false), false)
  assert.equal(maySeek(m, 1000 + RESTART_SEEK_MS, 75, true, busy, false), true)
  // A file gives up waiting sooner.
  const f = newSeekMemo()
  assert.equal(maySeek(f, 1000, 50, true, ok, true), true)
  assert.equal(maySeek(f, 1000 + CHEAP_SEEK_MS - 1, 50, true, busy, true), false)
  assert.equal(maySeek(f, 1000 + CHEAP_SEEK_MS, 50, true, busy, true), true)
  // Nothing is asked of a player with no stream open, nor in the middle of the viewer's own seek.
  assert.equal(maySeek(newSeekMemo(), 1000, 50, true, { ...ok, loaded: false }, true), false)
  assert.equal(maySeek(newSeekMemo(), 1000, 50, true, { ...ok, seeking: true }, true), false)
})

test('how long a seek took is remembered, and the next one while playing aims that far ahead', () => {
  const m = newSeekMemo()
  const ok = { seeking: false, canPlay: true, loaded: true }
  assert.equal(seekAim(m, 200, true, 600), 200)
  maySeek(m, 1000, 200, true, ok, false)
  // Straight after asking, the player still looks ready where it was: that isn't arriving.
  assert.equal(seekLanded(m, 1100, ok), false)
  assert.equal(seekLanded(m, 3000, { ...ok, canPlay: false }), false)
  assert.equal(seekLanded(m, 4500, ok), true)
  assert.equal(m.lag, 3.5)
  assert.equal(seekAim(m, 300, true, 600), 303.5)
  assert.equal(seekAim(m, 300, false, 600), 300) // a paused room isn't going anywhere
  assert.equal(seekAim(m, 598, true, 600), 599) // never past the end
  // A seek that took for ever doesn't send the next one minutes ahead.
  maySeek(m, 10000, 300, true, ok, false)
  seekLanded(m, 10000 + 60000, ok)
  assert.equal(m.lag, 12)
})

test('a paused player is sent to one place twice at most, and again when the place changes', () => {
  const m = newSeekMemo()
  const ok = { seeking: false, canPlay: true, loaded: true }
  let now = 1000
  for (let i = 0; i < HOLD_TRIES; i++) { assert.equal(maySeek(m, now, 90, false, ok, false), true); now += 1000; seekLanded(m, now, ok) }
  assert.equal(maySeek(m, now, 90, false, ok, false), false)
  assert.equal(maySeek(m, now, 120, false, ok, false), true)
})

test('the room moves on when a title ends, once, by whoever may control it', () => {
  const room = (o = {}, s = {}) => ({ canControl: true, queue: [7, 8], state: state(s), ...o })
  assert.equal(shouldAdvance(room(), T, { ended: true, duration: 600 }), true)
  assert.equal(shouldAdvance(room({ canControl: false }), T, { ended: true, duration: 600 }), false)
  assert.equal(shouldAdvance(room({ queue: [7] }), T, { ended: true, duration: 600 }), false)
  assert.equal(shouldAdvance(room({}, { paused: true }), T, { ended: true, duration: 600 }), false)
  // A player that never reached the end goes by the room's clock, a little after the end.
  const late = room({}, { positionSec: 590 })
  assert.equal(shouldAdvance(late, T + 10500, { ended: false, duration: 600 }), false)
  assert.equal(shouldAdvance(late, T + (10 + ADVANCE_AFTER) * 1000, { ended: false, duration: 600 }), true)
  assert.equal(shouldAdvance(late, T + 60000, { ended: false, duration: 0 }), false)
})

test('adding to a queue leaves what is playing where it is', () => {
  assert.deepEqual(queueAdd([1, 2, 3], 1, [9], 'end', 200), { items: [1, 2, 3, 9], index: 1 })
  assert.deepEqual(queueAdd([1, 2, 3], 1, [9, 8], 'next', 200), { items: [1, 2, 9, 8, 3], index: 1 })
  assert.deepEqual(queuePlayNow([1, 2, 3], 1, [9], 200), { items: [1, 2, 9, 3], index: 2 })
  assert.equal(queueAdd([1, 2, 3], 1, [9], 'end', 3), null) // full
  assert.equal(queueAdd([1, 2, 3], 1, [0], 'end', 200), null)
  assert.deepEqual(queueAdd([1], 0, [5, 5, 6], 'end', 200), { items: [1, 5, 6], index: 0 })
})

test('removing and moving keep the index on the same title', () => {
  assert.deepEqual(queueRemove([1, 2, 3], 1, 0), { items: [2, 3], index: 0 })
  assert.deepEqual(queueRemove([1, 2, 3], 1, 2), { items: [1, 2], index: 1 })
  // What is playing itself: on to what came after, or back when it was last.
  assert.deepEqual(queueRemove([1, 2, 3], 1, 1), { items: [1, 3], index: 1 })
  assert.deepEqual(queueRemove([1, 2, 3], 2, 2), { items: [1, 2], index: 1 })
  assert.equal(queueRemove([1], 0, 0), null) // a party is always playing something
  assert.deepEqual(queueMove([1, 2, 3, 4], 1, 3, 0), { items: [4, 1, 2, 3], index: 2 })
  assert.deepEqual(queueMove([1, 2, 3, 4], 1, 0, 3), { items: [2, 3, 4, 1], index: 0 })
  assert.deepEqual(queueMove([1, 2, 3, 4], 1, 1, 3), { items: [1, 3, 4, 2], index: 3 })
  assert.deepEqual(queueMove([1, 2, 3, 4], 1, 2, 3), { items: [1, 2, 4, 3], index: 1 })
  assert.equal(queueMove([1, 2, 3], 1, 0, 0), null)
  assert.equal(queueMove([1, 2, 3], 1, 0, 3), null)
  for (const [q, i, from, to] of [[[1, 2, 3, 4, 5], 2, 0, 4], [[1, 2, 3, 4, 5], 2, 4, 0], [[1, 2, 3, 4, 5], 2, 2, 0], [[1, 2, 3, 4, 5], 0, 3, 1]]) {
    const e = queueMove(q, i, from, to)
    assert.equal(e.items[e.index], q[i])
  }
})

test('a queue holding something this account may not see can be read and stepped through, not edited', () => {
  assert.equal(queueEditable([3, 4]), true)
  assert.equal(queueEditable([3, 0, 4]), false)
  assert.equal(queueEditable([]), false)
})

test('a party started on an episode plays through the rest of that season, in order', () => {
  const eps = [
    { id: 30, season: 1, episode: 3 }, { id: 10, season: 1, episode: 1 }, { id: 20, season: 1, episode: 2 },
    { id: 40, season: 2, episode: 1 }, { id: 50, season: null, episode: null },
  ]
  assert.deepEqual(seasonQueue(eps, 20, 200), [20, 30])
  assert.deepEqual(seasonQueue(eps, 10, 2), [10, 20])
  assert.deepEqual(seasonQueue(eps, 40, 200), [40])
  assert.deepEqual(seasonQueue(eps, 50, 200), [50])
  assert.deepEqual(seasonQueue(eps, 99, 200), [99]) // not in the list: itself alone
})

test('a code is tidied as the server tidies it', () => {
  assert.equal(tidyCode(' kxm-4t7 '), 'KXM4T7')
})

test('an invitation shows until it is answered, on this device', () => {
  const rooms = [
    { id: 'a', name: 'A', hostName: 'Sam', members: 1, invited: true },
    { id: 'b', name: 'B', hostName: 'Ada', members: 2, invited: true },
    { id: 'c', name: 'C', hostName: 'Me', members: 1, memberId: 'seat' },
    { id: 'd', name: 'D', hostName: 'Lu', members: 1, invited: true, memberId: 'seat2' },
  ]
  assert.deepEqual(openInvites(rooms, [], '').map((r) => r.id), ['a', 'b'])
  assert.deepEqual(openInvites(rooms, ['a'], '').map((r) => r.id), ['b'])
  assert.deepEqual(openInvites(rooms, [], 'b').map((r) => r.id), ['a'])
  // A declined party that has ended is forgotten; one still listed stays declined.
  assert.deepEqual(keepDeclined(['a', 'gone', 'c'], rooms), ['a'])
})

test('invitations are asked about sparingly, and less still when off or failing', () => {
  assert.equal(invitePollDelay(true, 0), INVITE_POLL_MS)
  assert.equal(invitePollDelay(false, 0), INVITE_POLL_OFF_MS)
  assert.ok(INVITE_POLL_MS >= 30000)
  assert.equal(invitePollDelay(true, 1), 2 * INVITE_POLL_MS)
  assert.equal(invitePollDelay(true, 9), INVITE_POLL_OFF_MS)
})
