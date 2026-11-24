// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { breakAt, isLibraryKey, knownEndAfter, liveNowFrom, planSeek, resumePlan, startOverPath, LIVE_EDGE_SEC } from './timeshift.ts'

const breaks = [{ start: 600, end: 660 }, { start: 1200, end: 1290 }]

test('the break the picture is in', () => {
  assert.deepEqual(breakAt(breaks, 600), breaks[0])
  assert.deepEqual(breakAt(breaks, 1289), breaks[1])
  assert.equal(breakAt(breaks, 660), null)
  assert.equal(breakAt(breaks, null), null)
  assert.equal(breakAt(undefined, 600), null)
})

test('a seek within what is loaded moves the picture', () => {
  // At media 100 = schedule 1000, with media 40..130 loaded.
  assert.deepEqual(planSeek(970, 1000, 100, { start: 40, end: 130 }, 5000), { kind: 'seek', to: 70 })
})

test('a seek further back than the window starts the stream again from there', () => {
  assert.deepEqual(planSeek(500.7, 1000, 100, { start: 40, end: 130 }, 5000), { kind: 'restart', at: 500 })
  // Not before the earliest the server keeps.
  assert.deepEqual(planSeek(100, 1000, 100, { start: 40, end: 130 }, 5000, 300), { kind: 'restart', at: 300 })
})

test('skipping a break that ends ahead of what is loaded restarts after it', () => {
  assert.deepEqual(planSeek(660.5, 610, 100, { start: 40, end: 110 }, 5000), { kind: 'restart', at: 660 })
})

test('a seek to live, or nearly, goes back to the shared channel', () => {
  assert.deepEqual(planSeek(5000 - LIVE_EDGE_SEC, 1000, 100, { start: 40, end: 130 }, 5000), { kind: 'live' })
  assert.deepEqual(planSeek(9999, 1000, 100, { start: 40, end: 130 }, 5000), { kind: 'live' })
})

test('a long pause starts again from where it paused', () => {
  assert.equal(resumePlan(0, 60_000, 300), 'resume')
  assert.equal(resumePlan(0, 280_000, 300), 'restart')
  // A tiny buffer still allows a short pause.
  assert.equal(resumePlan(0, 25_000, 10), 'resume')
})

test('live moves on with the clock', () => {
  assert.equal(liveNowFrom(1000, 900, 950), 1050)
  assert.equal(liveNowFrom(1000, 900, 800), 1000)
})

test('start-over addresses and keys', () => {
  assert.equal(startOverPath('lib-4', 1000.9, 2000), '/tv/watch/lib-4?from=1000&to=2000')
  assert.equal(startOverPath('lib-4', 1000, 900), '/tv/watch/lib-4?from=1000&to=1001')
  assert.ok(isLibraryKey('lib-4'))
  assert.ok(!isLibraryKey('tun-4'))
})

test('a restart past the programme it knows of asks for the next one, not to=from+1 (#669)', () => {
  // Skip break on the break that ends the programme: at is the old programme's end.
  assert.equal(knownEndAfter(2000.5, 2000, 2000), null)
  assert.equal(knownEndAfter(1500, 2000, 1800), 2000)
  assert.equal(knownEndAfter(2100, null, 2000, 3800), 3800)
  assert.equal(knownEndAfter(1999.5, 2000), null)
})
