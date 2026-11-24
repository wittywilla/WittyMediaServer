// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'

const p = await import('./pace.ts')

test('the buffer grows with the rate, and only above 1×', () => {
  assert.deepEqual(p.bufferFor(1, 30, 90), { maxBufferLength: 30, maxMaxBufferLength: 90 })
  assert.deepEqual(p.bufferFor(0.5, 30, 90), { maxBufferLength: 30, maxMaxBufferLength: 90 })
  assert.deepEqual(p.bufferFor(2, 30, 90), { maxBufferLength: 60, maxMaxBufferLength: 180 })
  assert.deepEqual(p.bufferFor(3, 60, 120), { maxBufferLength: 180, maxMaxBufferLength: 360 })
  assert.deepEqual(p.bufferFor(16, 30, 90), { maxBufferLength: 300, maxMaxBufferLength: 600 })
  assert.deepEqual(p.bufferFor(NaN, 30, 90), { maxBufferLength: 30, maxMaxBufferLength: 90 })
})

test('the next size down', () => {
  assert.equal(p.lowerHeight(2160), 1440)
  assert.equal(p.lowerHeight(1080), 720)
  assert.equal(p.lowerHeight(800), 720) // a wide film's 1080p is 800 lines
  assert.equal(p.lowerHeight(720), 480)
  assert.equal(p.lowerHeight(360), 0)
  assert.equal(p.lowerHeight(0), 0)
})

test('a conversion that can not keep up: the server tries first, then a smaller picture, then words', () => {
  const slow = { speed: 0.7, rate: 2, leadSec: 4, keepingUp: false, fastest: false }
  assert.equal(p.tooSlow(null, 5, 1080, false), 'ok')
  assert.equal(p.tooSlow({ ...slow, keepingUp: true }, 5, 1080, false), 'ok')
  assert.equal(p.tooSlow({ ...slow, speed: 0 }, 5, 1080, false), 'ok') // not measured yet
  assert.equal(p.tooSlow(slow, 5, 1080, false), 'wait') // the server's faster setting first
  assert.equal(p.tooSlow({ ...slow, fastest: true }, 1, 1080, false), 'wait') // one reading
  assert.equal(p.tooSlow({ ...slow, fastest: true }, 2, 1080, false), 'lower')
  assert.equal(p.tooSlow({ ...slow, fastest: true }, 2, 1080, true), 'tell') // the viewer's own choice
  assert.equal(p.tooSlow({ ...slow, fastest: true }, 2, 360, false), 'tell')
  assert.equal(p.times(0.666), '0.67×')
})
