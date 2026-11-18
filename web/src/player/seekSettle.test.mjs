// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Bursts of seeks, fed to settleSeeks with a pretend player and clock.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { SEEK_BURST_MS, SEEK_SETTLE_MS, inBuffer, settleSeeks } from './seekSettle.ts'

function rig(buffered = [[0, 20]]) {
  const calls = []
  const hls = { stopLoad: () => calls.push('stop'), startLoad: (at) => calls.push('start@' + at) }
  let fn = null
  const v = {
    currentTime: 0,
    buffered: { length: buffered.length, start: (i) => buffered[i][0], end: (i) => buffered[i][1] },
    addEventListener: (_t, f) => { fn = f },
    removeEventListener: () => { fn = null },
  }
  let now = 0
  let timers = []
  const clock = { now: () => now, set: (f, ms) => { const t = { f, at: now + ms }; timers.push(t); return t }, clear: (t) => { timers = timers.filter((x) => x !== t) } }
  const seek = (to) => { v.currentTime = to; fn?.() }
  const wait = (ms) => {
    now += ms
    for (const t of timers.filter((x) => x.at <= now)) { timers = timers.filter((x) => x !== t); t.f() }
  }
  const stop = settleSeeks(hls, v, clock)
  return { calls, seek, wait, stop, timers: () => timers }
}

test('a single seek is left to the player at once', () => {
  const r = rig()
  r.seek(300)
  r.wait(2000)
  assert.deepEqual(r.calls, [])
})

test('rapid skips load once, from where the viewer stopped', () => {
  const r = rig()
  for (let i = 1; i <= 8; i++) { r.seek(i * 10 + 100); r.wait(120) }
  assert.deepEqual(r.calls, ['stop'], 'held while the skips go on')
  r.wait(SEEK_SETTLE_MS)
  assert.deepEqual(r.calls, ['stop', 'start@180'])
})

test('dragging back and forth along the timeline asks for the last place only', () => {
  const r = rig()
  for (const to of [900, 3000, 1500, 4800, 2400]) { r.seek(to); r.wait(60) }
  r.wait(SEEK_SETTLE_MS)
  assert.deepEqual(r.calls, ['stop', 'start@2400'])
})

test('seeks within what is buffered load nothing, even quickly', () => {
  const r = rig([[0, 60]])
  for (const to of [10, 20, 30, 40]) { r.seek(to); r.wait(100) }
  r.wait(SEEK_SETTLE_MS)
  assert.deepEqual(r.calls, [])
})

test('a burst that ends inside the buffer still starts loading again', () => {
  const r = rig([[0, 60]])
  r.seek(500); r.wait(100); r.seek(900); r.wait(100); r.seek(30)
  r.wait(SEEK_SETTLE_MS)
  assert.deepEqual(r.calls, ['stop', 'start@30'])
})

test('seeks far enough apart are each their own', () => {
  const r = rig()
  r.seek(300); r.wait(SEEK_BURST_MS + 10); r.seek(900); r.wait(SEEK_BURST_MS + 10)
  assert.deepEqual(r.calls, [])
})

test('stopping leaves no timer behind', () => {
  const r = rig()
  r.seek(300); r.wait(50); r.seek(600)
  r.stop()
  assert.equal(r.timers().length, 0)
})

test('inBuffer leaves a little at the end, where nothing would play', () => {
  const b = { length: 1, start: () => 10, end: () => 20 }
  assert.ok(inBuffer(b, 15))
  assert.ok(!inBuffer(b, 19.8))
  assert.ok(!inBuffer(b, 5))
})
