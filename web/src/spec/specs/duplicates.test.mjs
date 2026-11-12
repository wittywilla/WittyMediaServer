// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { duplicatesSpec } from './duplicates.ts'
import { problems, withIds } from '../model.ts'

const g = [{ reason: 'same name', suggested: 2, channels: [{ id: 1, number: '4', name: 'Kids', tuner: 'A', health: 'failing' }, { id: 2, number: '104', name: 'Kids', tuner: 'B', health: 'ok' }] }]

test('duplicates: the suggested one is kept until another is chosen', () => {
  const p = duplicatesSpec(g, {}, -1)
  assert.equal(p.sections[0].h, 'Kids · Same name')
  assert.deepEqual(p.sections[0].items.map((r) => [r.id, r.on, r.sub]), [['keep:0:1', false, 'A · failing'], ['keep:0:2', true, 'B · opens']])
  assert.equal(p.sections[0].items[1].value, 'Keep in the guide')
  assert.equal(duplicatesSpec(g, { 0: 1 }, -1).sections[0].items[0].on, true)
  assert.equal(p.sections[1].items[0].id, 'merge:0')
  assert.equal(duplicatesSpec(g, {}, 0).sections[1].items[0].disabled, true)
  assert.deepEqual(problems(withIds(p)), [])
})

test('duplicates: loading, none, and an error each say so', () => {
  assert.equal(duplicatesSpec(null, {}, -1).sections[0].text, 'Looking…')
  assert.match(duplicatesSpec([], {}, -1).sections[0].text, /No duplicates/)
  assert.equal(duplicatesSpec(null, {}, -1, 'No')?.sections[0].text, 'No')
})
