// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { addToAlbumSpec, albumSpec } from './albums.ts'
import { handledIds, problems, withIds } from '../model.ts'

test('album details: name with the error, a switch for sharing', () => {
  const p = albumSpec({ name: 'Trip', shared: true }, 'Taken.')
  assert.deepEqual(p.sections[0].items.map((f) => [f.id, f.kind, f.value]), [['name', 'text', 'Trip'], ['shared', 'switch', true]])
  assert.equal(p.sections[0].items[0].error, 'Taken.')
  assert.deepEqual(problems(withIds(p)), [])
})

test('add to album: New album first, then albums with their counts', () => {
  const p = addToAlbumSpec([{ id: 3, name: 'Trip', count: 12 }], 2)
  assert.equal(p.lead, '2 pictures chosen.')
  assert.deepEqual([...handledIds(withIds(p))], ['new', 'album:3'])
  assert.equal(p.sections[1].items[0].value, '12')
  assert.equal(addToAlbumSpec([], 1).sections.length, 1)
  assert.equal(addToAlbumSpec([], 1).lead, '1 picture chosen.')
})
