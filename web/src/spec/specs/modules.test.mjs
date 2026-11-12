// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { UPDATES, modulesTopSpec } from './modules.ts'
import { problems, withIds } from '../model.ts'

const v = { autoCheck: true, autoInstall: false, loaded: true, checkedAt: '', held: '', checkError: '', checking: false, server: '0.8.0', api: '3', filters: [{ id: 'all', label: 'All', count: 9 }, { id: 'updates', label: 'Updates', count: 2 }], filter: 'all' }
const fields = (p) => p.sections.filter((s) => s.kind === 'fields').flatMap((s) => s.items)

test('modules top: the two switches, the check chip and the filter', () => {
  const p = modulesTopSpec(v)
  assert.equal(p.sections[0].h, UPDATES)
  assert.deepEqual(fields(p).map((f) => [f.id, f.value, f.disabled]), [['autoCheck', true, false], ['autoInstall', false, false]])
  assert.match(fields(p)[0].hint, /hasn’t looked yet/)
  const show = p.sections.find((s) => s.h === 'Show')
  assert.deepEqual(show.items.map((c) => [c.id, c.title, c.on]), [['filter:all', 'All', true], ['filter:updates', 'Updates · 2', false]])
  assert.deepEqual(problems(withIds(p)), [])
})

test('modules top: auto install follows auto check; notes only when they are so; busy check is disabled', () => {
  const p = modulesTopSpec({ ...v, autoCheck: false, held: '3 PM', checkError: 'offline', checkedAt: 'today', checking: true, loaded: true })
  assert.equal(fields(p)[1].disabled, true)
  assert.match(fields(p)[0].hint, /Last checked today/)
  assert.equal(p.sections.filter((s) => s.kind === 'note' && s.tone).length, 1)
  assert.ok(p.sections.some((s) => s.kind === 'note' && /until 3 PM/.test(s.text)))
  assert.deepEqual(p.sections.find((s) => s.kind === 'chips' && s.items[0].id === 'check').items[0], { id: 'check', title: 'Checking…', disabled: true })
  assert.equal(fields(modulesTopSpec({ ...v, loaded: false }))[0].disabled, true)
})
