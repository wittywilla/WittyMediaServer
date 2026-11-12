// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { radioAddressSpec, radioFilterSpec, radioPlaylistSpec, stationManageSpec } from './radio.ts'
import { handledIds, problems, withIds } from '../model.ts'

test('radio filters: country select and genre chips, only when there is a choice', () => {
  const p = radioFilterSpec(['DE', 'FR'], 'FR', ['Jazz', 'Rock'], 'Jazz')
  assert.deepEqual(p.sections.map((s) => s.kind), ['fields', 'chips'])
  assert.equal(p.sections[0].items[0].value, 'FR')
  assert.deepEqual(p.sections[1].items.map((c) => [c.id, c.on]), [['genre:', false], ['genre:Jazz', true], ['genre:Rock', false]])
  assert.equal(radioFilterSpec(['DE'], '', ['Jazz'], '').sections.length, 0)
  assert.deepEqual(problems(withIds(p)), [])
})

test('radio add by address: needs an address, shows the error on the field', () => {
  const p = radioAddressSpec({ url: '', name: '', genre: '' }, '', false)
  assert.equal(p.actions[0].disabled, true)
  const q = radioAddressSpec({ url: 'https://x/y', name: '', genre: '' }, 'No.', false)
  assert.equal(q.actions[0].disabled, false)
  assert.equal(q.sections[0].items[0].error, 'No.')
  assert.ok(handledIds(withIds(q)).has('add'))
  assert.equal(radioAddressSpec({ url: 'a', name: '', genre: '' }, '', true).actions[0].title, 'Adding…')
})

test('radio playlist: file field, add disabled until there is text', () => {
  const p = radioPlaylistSpec('', '', false)
  assert.equal(p.sections[0].items[0].kind, 'file')
  assert.equal(p.actions[0].disabled, true)
  assert.equal(radioPlaylistSpec('#EXTM3U', '', false).actions[0].disabled, false)
})

test('station manage: lookup only for a station added by address; all off while busy', () => {
  const a = stationManageSpec({ fromDirectory: false, address: 'https://x', busy: '' })
  assert.deepEqual(a.sections[1].items.map((c) => c.id), ['check', 'lookup', 'remove'])
  const b = stationManageSpec({ fromDirectory: true, lastCheck: 'today', busy: 'check' })
  assert.deepEqual(b.sections[1].items.map((c) => [c.id, c.disabled]), [['check', true], ['remove', true]])
  assert.equal(b.sections[0].items[0].value, 'Radio Browser directory')
  assert.deepEqual(problems(withIds(a)), [])
})
