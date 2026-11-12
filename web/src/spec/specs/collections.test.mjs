// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { saveSpec } from './collections.ts'
import { problems, withIds } from '../model.ts'

test('save to a list: ticked where it is, Watch later offered until it exists', () => {
  const p = saveSpec([{ id: 2, name: 'Friday night' }, { id: 4, name: 'Sea' }], [4], false)
  assert.deepEqual(p.sections[0].items.map((r) => [r.id, r.on, r.value]), [['later', false, undefined], ['list:2', false, undefined], ['list:4', true, '✓']])
  const q = saveSpec([{ id: 1, name: 'Watch later' }], [1], true)
  assert.deepEqual(q.sections[0].items.map((r) => r.id), ['list:1'])
  assert.deepEqual(problems(withIds(p)), [])
  assert.equal(saveSpec([], [], true).sections.length, 0)
})
