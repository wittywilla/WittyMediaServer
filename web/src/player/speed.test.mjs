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
