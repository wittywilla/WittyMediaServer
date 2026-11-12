// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { discSpec, pastSpec, storageSpec } from './diagnoser.ts'
import { problems, withIds } from '../model.ts'

const base = { path: '', knownDiscs: [], crawl: true, damage: false, busy: false, error: '', browsing: false }

test('disc: start needs a disc; options are switches; known discs only when there are some', () => {
  let p = discSpec(base)
  assert.equal(p.actions[0].disabled, true)
  assert.equal(p.sections.length, 2)
  assert.deepEqual(p.sections[1].items.map((f) => [f.id, f.value]), [['crawl', true], ['damage', false]])
  p = discSpec({ ...base, path: '/m/Films/Alien.iso', knownDiscs: [{ title: 'Alien', path: '/m/Films/Alien.iso' }], error: 'Nope' })
  assert.equal(p.actions[0].disabled, false)
  assert.equal(p.sections[0].items[0].value, 'Alien.iso')
  assert.equal(p.sections[1].items[0].value, '/m/Films/Alien.iso')
  assert.equal(p.sections.at(-1).tone, true)
  assert.deepEqual(problems(withIds(p)), [])
})

test('storage: folders are rows toggled by id; start needs one', () => {
  const folders = [{ id: 'a', name: 'Films', path: '/f', group: 'library' }, { id: 'd', name: 'Data', path: '/d', group: 'data' }]
  const s = { folders, chosen: new Set(['a']), walk: true, readTest: true, writeTest: false, busy: false, error: '' }
  const p = storageSpec(s)
  assert.deepEqual(p.sections.map((x) => x.h), ['Library folders', 'The server’s own folders', 'Options'])
  assert.deepEqual(p.sections[0].items.map((r) => [r.id, r.on]), [['dir:a', true]])
  assert.equal(p.actions[0].disabled, false)
  assert.equal(p.actions[1].title, 'Choose all')
  assert.equal(storageSpec({ ...s, chosen: new Set() }).actions[0].disabled, true)
  assert.equal(storageSpec({ ...s, chosen: new Set(['a', 'd']) }).actions[1].title, 'Clear all')
  assert.deepEqual(problems(withIds(p)), [])
})

test('past reports: words by state, empty says so', () => {
  const p = pastSpec([{ id: '1', kind: 'disc', subject: 'Alien', running: false, state: 'done', worst: 'warn', finished: 'Today' }, { id: '2', kind: 'storage', subject: 'Storage', running: true, state: 'running' }], '1', { warn: 'Warnings' })
  assert.deepEqual(p.sections[0].items.map((r) => [r.id, r.value, r.on]), [['job:1', 'Warnings', true], ['job:2', 'Running', false]])
  assert.equal(pastSpec([], '', {}).sections[0].kind, 'note')
})
