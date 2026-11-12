// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { collectionsSpec, saveSpec } from './collections.ts'
import { problems, withIds } from '../model.ts'

const list = [
  { id: 1, ownerId: 7, owner: 'Willow', name: 'Watch later', kind: 'manual', shared: false, count: 1, system: 'watchlater' },
  { id: 2, ownerId: 7, owner: 'Willow', name: 'Friday night', kind: 'manual', shared: true, count: 6 },
  { id: 3, ownerId: 9, owner: 'Pip', name: 'Short films', kind: 'smart', shared: true, count: 23 },
]

test('collections: lists then smart playlists, linking to their pages', () => {
  const p = collectionsSpec(list, 7, true)
  assert.equal(p.title, 'Your lists')
  assert.deepEqual(p.sections.map((s) => s.h), ['Lists', 'Smart playlists'])
  assert.deepEqual(p.sections[0].items.map((r) => [r.title, r.value, r.to]), [['Watch later', '1 title', '/collections/1'], ['Friday night', '6 titles', '/collections/2']])
  assert.equal(p.sections[0].items[1].sub, 'Shared with the other profiles')
  assert.equal(p.sections[1].items[0].value, '23 match')
  assert.equal(p.sections[1].items[0].sub, 'From Pip')
  assert.deepEqual(p.actions.map((a) => a.id), ['new'])
  assert.deepEqual(problems(withIds(p)), [])
})

test('collections: nobody who cannot make lists gets New; empty says what to do', () => {
  const p = collectionsSpec([], 7, false)
  assert.equal(p.actions.length, 0)
  assert.equal(p.sections[0].kind, 'note')
  assert.match(p.sections[0].text, /no collections/)
  assert.match(collectionsSpec([], 7, true).sections[0].text, /smart playlist/)
})

test('save to a list: ticked where it is, Watch later offered until it exists', () => {
  const p = saveSpec([{ id: 2, name: 'Friday night' }, { id: 4, name: 'Sea' }], [4], false)
  assert.deepEqual(p.sections[0].items.map((r) => [r.id, r.on, r.value]), [['later', false, undefined], ['list:2', false, undefined], ['list:4', true, '✓']])
  const q = saveSpec([{ id: 1, name: 'Watch later' }], [1], true)
  assert.deepEqual(q.sections[0].items.map((r) => r.id), ['list:1'])
  assert.deepEqual(problems(withIds(p)), [])
  assert.equal(saveSpec([], [], true).sections.length, 0)
})
