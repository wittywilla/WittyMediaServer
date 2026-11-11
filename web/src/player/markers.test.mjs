// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { skipAt } from './markers.ts'

const m = { recap: { startSec: 0, endSec: 40 }, intro: { startSec: 40, endSec: 100 }, credits: { startSec: 1300, endSec: 1380 }, commercials: [{ startSec: 600, endSec: 780 }] }

test('the part playing now is the one offered', () => {
  assert.deepEqual(skipAt(m, 10, 1500), { kind: 'recap', label: 'Skip recap', to: 40 })
  assert.deepEqual(skipAt(m, 60, 1500), { kind: 'intro', label: 'Skip intro', to: 100 })
  assert.equal(skipAt(m, 700, 1500)?.kind, 'commercial')
  assert.equal(skipAt(m, 1310, 1500)?.to, 1380)
  assert.equal(skipAt(m, 300, 1500), null)
})

test('nothing is offered for the last second of a part, or with no markers', () => {
  assert.equal(skipAt(m, 99.5, 1500), null)
  assert.equal(skipAt(null, 10, 1500), null)
  assert.equal(skipAt({ intro: { startSec: 50, endSec: 50 } }, 50, 1500), null)
})

test('credits that run to the end are left to "next episode"', () => {
  assert.equal(skipAt({ credits: { startSec: 1300, endSec: 1500 } }, 1310, 1500), null)
  assert.equal(skipAt({ credits: { startSec: 1300, endSec: 1380 } }, 1310, 0), null)
})
