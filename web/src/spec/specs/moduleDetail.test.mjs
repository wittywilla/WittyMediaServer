// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { detailsSpec, manageSpec, permissionsSpec } from './moduleDetail.ts'
import { problems, withIds } from '../model.ts'

test('permissions: rows, or the plain none', () => {
  const p = permissionsSpec([{ name: 'Network', words: 'reaches the internet' }])
  assert.equal(p.sections[0].items[0].sub, 'reaches the internet')
  assert.equal(permissionsSpec([]).sections[0].items[0].title, 'No special permissions.')
  assert.deepEqual(problems(withIds(p)), [])
})

test('details: facts, and links for what it is built on', () => {
  const p = detailsSpec([{ title: 'Version', value: '1.2' }], [{ id: 'a', name: 'WittyInk', to: '/settings/modules/store/a' }])
  assert.equal(p.sections[1].items[0].to, '/settings/modules/store/a')
  assert.equal(detailsSpec([{ title: 'V', value: '1' }], []).sections.length, 1)
  assert.deepEqual(problems(withIds(p)), [])
})

test('manage: only what applies', () => {
  const base = { installed: true, canRollback: true, previous: '1.0', core: false, enabled: true, builtin: false, usingFork: false, working: false }
  const ids = (m) => manageSpec(m).sections[0].items.map((c) => c.id)
  assert.deepEqual(ids(base), ['rollback', 'off', 'remove', 'log'])
  assert.deepEqual(ids({ ...base, enabled: false, canRollback: false }), ['on', 'remove', 'log'])
  assert.deepEqual(ids({ ...base, core: true, builtin: true, usingFork: true }), ['rollback', 'official', 'log'])
  assert.deepEqual(ids({ ...base, installed: false }), ['log'])
  assert.equal(manageSpec(base).sections[0].items[0].title, 'Go back to version 1.0')
})
