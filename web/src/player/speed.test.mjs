// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'

const s = await import('./speed.ts')

test('a real change of speed lines sound and picture up again; a small step does not', () => {
  assert.equal(s.realignNeeded(4, 1), true) // the owner's case: back to normal from 4×
  assert.equal(s.realignNeeded(1, 4), true)
  assert.equal(s.realignNeeded(1, 2), true)
  assert.equal(s.realignNeeded(1, 1.25), false)
  assert.equal(s.realignNeeded(1.5, 1.75), false)
  assert.equal(s.realignNeeded(1, 1), false)
  assert.equal(s.realignNeeded(0, 1), false) // paused players report no rate
  assert.equal(s.realignNeeded(NaN, 2), false)
})

// A stand-in for a <video>: counts keepInStep's own seeks (currentTime set to where it is).
class FakeVideo extends EventTarget {
  playbackRate = 1
  paused = false
  seeking = false
  ended = false
  readyState = 4
  seeks = 0
  #t = 0
  get currentTime() { return this.#t }
  set currentTime(v) { this.seeks++; this.#t = v; this.dispatchEvent(new Event('seeked')) }
  rate(r) { this.playbackRate = r; this.dispatchEvent(new Event('ratechange')) }
}

test("live TV's own corrections never line up again: ten minutes of WittyTV's sync steps, no seek", async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  globalThis.window ??= globalThis
  const { syncStep } = await import('./sync.ts')
  const v = new FakeVideo()
  const stop = s.keepInStep(v)
  // What TvHost does every 2 s: behind or ahead of the server's clock by up to 2.9 s, nudging 3% either way.
  let nudging = false
  const behinds = [0.5, 0.3, 0.1, 0.05, -0.2, -0.6, -0.1, 0.02, 2.9, 1.2, 0.03, -2.5, -0.04]
  for (let i = 0; i < 300; i++) {
    const step = syncStep(behinds[i % behinds.length], nudging)
    if ('rate' in step) { v.rate(step.rate); nudging = step.rate !== 1 }
    t.mock.timers.tick(2000)
  }
  v.rate(1) // TvHost puts the speed back when it stops
  t.mock.timers.tick(31000)
  stop()
  assert.equal(v.seeks, 0)
})

test("a viewer's 1× → 4× → 1× lines up once each way", async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  globalThis.window ??= globalThis
  const v = new FakeVideo()
  const stop = s.keepInStep(v)
  v.rate(4)
  t.mock.timers.tick(1000)
  v.rate(1)
  t.mock.timers.tick(1000)
  stop()
  assert.equal(v.seeks, 2)
})
