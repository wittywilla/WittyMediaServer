// The live sync rules, fed with made-up drifts and clocks.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { clockOffset, RATE, SEEK, syncStep } from './sync.ts'

test('a small drift is left alone', () => {
  assert.deepEqual(syncStep(0.3, false), { rate: 1 })
  assert.deepEqual(syncStep(-0.49, false), { rate: 1 })
})

test('a medium drift plays a little faster when behind and slower when ahead', () => {
  assert.deepEqual(syncStep(1.2, false), { rate: 1 + RATE })
  assert.deepEqual(syncStep(-2, false), { rate: 1 - RATE })
})

test('catching up carries on until it has settled, then stops', () => {
  assert.deepEqual(syncStep(0.3, true), { rate: 1 + RATE })
  assert.deepEqual(syncStep(0.1, true), { rate: 1 })
})

test('a large drift, or a stall, jumps straight there', () => {
  assert.deepEqual(syncStep(SEEK, false), { seekBy: SEEK })
  assert.deepEqual(syncStep(-7.5, true), { seekBy: -7.5 })
})

test('the clock offset comes from the quickest round trip', () => {
  // The server is 5 s ahead. The slow sample was held up on the way back, which would make it
  // look less far ahead than it is.
  const samples = [
    { sent: 1000, server: 6060, got: 1120 },
    { sent: 2000, server: 7010, got: 2020 },
    { sent: 3000, server: 8010, got: 3600 },
  ]
  assert.equal(clockOffset(samples), 5000)
  assert.equal(clockOffset([]), 0)
  assert.equal(clockOffset([{ sent: 10, server: 99, got: 5 }]), 0) // a clock that went back is no use
})
