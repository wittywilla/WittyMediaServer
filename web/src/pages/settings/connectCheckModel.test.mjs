// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { byDevice, stepStates } from './connectCheckModel.ts'

const steps = JSON.parse(readFileSync(new URL('../../../../pkg/connectsteps/steps.json', import.meta.url), 'utf8')).steps

test('tries are grouped by device, the one that tried last first, with the name it has', () => {
  const a = (device, at, extra = {}) => ({ device, at, short: device, listed: false, kind: 'connect', ok: false, count: 1, ...extra })
  const g = byDevice([a('b', 30), a('a', 20, { name: 'Phone' }), a('b', 10, { name: 'TV' }), a('a', 5)])
  assert.deepEqual(g.map((x) => [x.device, x.name, x.tries.length]), [['b', 'TV', 2], ['a', 'Phone', 2]])
  assert.deepEqual(byDevice([]), [])
})

test('a failed step has ticks before it and nothing after it', () => {
  assert.deepEqual(steps.map((s) => s.id), ['read', 'found', 'secure', 'signin'])
  assert.deepEqual(stepStates(steps, 'secure'), ['ok', 'ok', 'fail', 'skip'])
  assert.deepEqual(stepStates(steps, 'read'), ['fail', 'skip', 'skip', 'skip'])
  assert.equal(stepStates(steps, undefined), null)
  assert.equal(stepStates(steps, 'nope'), null)
})
