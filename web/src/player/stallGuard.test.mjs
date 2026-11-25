// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The stall guard's rules, fed with made-up clocks and frame counts.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { freshGuard, guardStep, MAX_FIXES, STALL_SEC } from './stallGuard.ts'

const s = (t, frames, paused = false, ready = true) => ({ t, frames, paused, ready })
// Runs a list of looks through the guard and lists what it did.
function run(looks) {
  let st = freshGuard(), prev = null
  const did = []
  for (const now of looks) {
    const r = guardStep(prev, now, st)
    st = r.st
    if (r.step.kind !== 'none') did.push(`${now.t}:${r.step.kind}`)
    prev = now
  }
  return did
}

test('a picture that keeps coming is left alone', () => {
  assert.deepEqual(run([s(0, 0), s(0.25, 6), s(0.5, 12), s(0.75, 18), s(1, 24)]), [])
})

test('the clock running on with no new picture silences the sound, then seeks', () => {
  const looks = [s(0, 10)]
  for (let t = 0.25; t <= 2.5; t += 0.25) looks.push(s(t, 10))
  const did = run(looks)
  assert.equal(did[0], `${STALL_SEC}:silence`)
  assert.equal(did[1], '1.25:seek')
})

test('the sound comes back with the next picture', () => {
  assert.deepEqual(run([s(0, 10), s(0.5, 10), s(1, 10), s(1.25, 10), s(1.5, 11)]), ['1:silence', '1.25:seek', '1.5:restore'])
})

test('paused, waiting for data or the clock standing still is no stall', () => {
  assert.deepEqual(run([s(0, 10), s(1, 10, true), s(2, 10, true), s(3, 10, true)]), [])
  assert.deepEqual(run([s(0, 10), s(1, 10, false, false), s(2, 10, false, false)]), [])
  assert.deepEqual(run([s(5, 10), s(5, 10), s(5, 10), s(5, 10), s(5, 10)]), [])
})

test('a stream that stays stuck gets a few seeks, not endless ones', () => {
  const looks = [s(0, 10)]
  for (let t = 0.25; t <= 20; t += 0.25) looks.push(s(t, 10))
  assert.equal(run(looks).filter((x) => x.endsWith('seek')).length, MAX_FIXES)
})
