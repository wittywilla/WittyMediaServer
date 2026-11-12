// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { gameFiltersSpec, letterSpec, splitFilterId } from './gameLibrary.ts'
import { handledIds, problems, withIds } from '../model.ts'

test('game filters: sort first, empty groups left out, chosen value on', () => {
  const p = gameFiltersSpec('year', [{ id: 'title', label: 'Title' }, { id: 'year', label: 'Year' }], [
    { name: 'genre', title: 'Genre', options: [{ value: 'RPG', label: 'RPG (3)' }] }, { name: 'series', title: 'Series', options: [] },
  ], { genre: 'RPG' })
  assert.deepEqual(p.sections.map((s) => s.h), ['Sort', 'Genre'])
  assert.deepEqual(p.sections[0].items.map((c) => c.on), [false, true])
  assert.equal(p.sections[1].items[0].on, true)
  assert.deepEqual(problems(withIds(p)), [])
})

test('filter ids split at the first colon', () => {
  assert.deepEqual(splitFilterId('genre:Action: Adventure'), ['genre', 'Action: Adventure'])
  assert.deepEqual(splitFilterId('sort:title'), ['sort', 'title'])
})

test('letters: a chip per letter carrying its offset', () => {
  const p = letterSpec([{ label: 'A', offset: 0, count: 5 }, { label: 'B', offset: 5, count: 2 }])
  assert.deepEqual([...handledIds(withIds(p))], ['letter:0', 'letter:5'])
})
