// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { runPatch, runSettingsSpec, settleLabel } from './libtranscode.ts'
import { problems, withIds } from '../model.ts'

const d = { pauseWhileStreaming: true, lowPriority: false, hoursOn: false, hoursFrom: 1, hoursTo: 6, settleSeconds: 300, threads: 0, maxJobs: 1, useHardware: false, minFreeGb: 10 }
const th = [{ value: '0', label: 'Half' }]

test('run settings: hours shown only when on; values as strings for selects', () => {
  const p = runSettingsSpec(d, th)
  assert.deepEqual(problems(withIds(p)), [])
  const ids = (q) => q.sections[0].items.map((f) => f.id)
  assert.ok(!ids(p).includes('from'))
  assert.ok(ids(runSettingsSpec({ ...d, hoursOn: true }, th)).includes('to'))
  assert.equal(p.sections[0].items.find((f) => f.id === 'settle').value, '300')
  assert.ok(runSettingsSpec({ ...d, minFreeGb: 7 }, th).sections[0].items.find((f) => f.id === 'free').options.some((o) => o.value === '7'))
})

test('patches change the right setting and numbers become numbers', () => {
  assert.deepEqual(runPatch('jobs', '3'), { maxJobs: 3 })
  assert.deepEqual(runPatch('hardware', true), { useHardware: true })
  assert.deepEqual(runPatch('from', '22'), { hoursFrom: 22 })
  assert.equal(runPatch('nope', 1), null)
  assert.equal(settleLabel(60), '1 min')
  assert.equal(settleLabel(3600), '1 hour')
})
