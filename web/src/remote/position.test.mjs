// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/remote/position.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { clockAhead, partDone, positionNow, seekTarget } from './position.ts'

const T = 1794000008200

test('a playing device has moved on since it last said, at its speed', () => {
  // The protocol's own example: positionSec + (serverNow - stateAt) / 1000 × rate.
  assert.equal(positionNow({ status: 'playing', positionSec: 1312.4, durationSec: 5400, rate: 1 }, T, T + 1800), 1314.2)
  assert.equal(positionNow({ status: 'playing', positionSec: 100, durationSec: 5400, rate: 2 }, T, T + 3000), 106)
  assert.equal(positionNow({ status: 'playing', positionSec: 100, durationSec: 5400, rate: 0.5 }, T, T + 3000), 101.5)
})

test('only a playing device moves', () => {
  for (const status of ['paused', 'buffering', 'idle']) assert.equal(positionNow({ status, positionSec: 40, durationSec: 100, rate: 1 }, T, T + 9000), 40, status)
})

test('nothing goes past the end, or before the start', () => {
  assert.equal(positionNow({ status: 'playing', positionSec: 95, durationSec: 100, rate: 1 }, T, T + 60000), 100)
  assert.equal(positionNow({ status: 'paused', positionSec: -3, durationSec: 100 }, T, T), 0)
  // No length known (a stream still opening): it just counts on.
  assert.equal(positionNow({ status: 'playing', positionSec: 10, durationSec: 0, rate: 1 }, T, T + 5000), 15)
})

test('clocks a little apart never move a device backwards', () => {
  assert.equal(positionNow({ status: 'playing', positionSec: 50, durationSec: 100, rate: 1 }, T, T - 400), 50)
})

test('a missing or broken rate is normal speed; a missing place is the start', () => {
  assert.equal(positionNow({ status: 'playing', positionSec: 10, durationSec: 100 }, T, T + 2000), 12)
  assert.equal(positionNow({ status: 'playing', positionSec: 10, durationSec: 100, rate: 0 }, T, T + 2000), 12)
  assert.equal(positionNow({ status: 'playing', positionSec: 10, durationSec: 100, rate: NaN }, T, T + 2000), 12)
  assert.equal(positionNow({ status: 'playing' }, T, T + 2000), 2)
  // No report time at all: where it said, and no further.
  assert.equal(positionNow({ status: 'playing', positionSec: 10 }, 0, T), 10)
})

test('the server\'s clock, from its time in an answer', () => {
  assert.equal(clockAhead(T + 250, T), 250)
  assert.equal(clockAhead(T - 250, T), -250)
  // serverNow = local now + ahead.
  const ahead = clockAhead(T + 1000, T)
  assert.equal(positionNow({ status: 'playing', positionSec: 0, durationSec: 100, rate: 1 }, T + 1000, (T + 5000) + ahead), 5)
})

test('a seek by some seconds stays inside the item', () => {
  assert.equal(seekTarget(100, -10, 5400), 90)
  assert.equal(seekTarget(5, -10, 5400), 0)
  assert.equal(seekTarget(5390, 30, 5400), 5400)
  assert.equal(seekTarget(50, 30, 0), 80)
})

test('a book\'s place is a share of it', () => {
  assert.equal(partDone(30, 120), 25)
  assert.equal(partDone(0, 120), 0)
  assert.equal(partDone(500, 120), 100)
  assert.equal(partDone(3, 0), null)
  assert.equal(partDone(undefined, 10), null)
})
