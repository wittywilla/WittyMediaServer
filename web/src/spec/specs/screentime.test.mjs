// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { askParentSpec } from './screentime.ts'
import { problems, withIds } from '../model.ts'

const o = { more: '30', byPassword: false, hasPin: true, name: '', password: '', code: '', error: '' }

test('ask a parent: PIN first when there is one, password when asked or when there is none', () => {
  const pin = askParentSpec(o)
  assert.deepEqual(pin.sections[2].items.map((f) => f.id), ['code'])
  assert.equal(pin.sections[3].items[0].title, 'Use a parent’s password instead')
  const pw = askParentSpec({ ...o, byPassword: true, hasPin: false, error: 'Wrong' })
  assert.deepEqual(pw.sections[2].items.map((f) => f.id), ['name', 'password'])
  assert.equal(pw.sections[2].items[1].error, 'Wrong')
  assert.equal(pw.sections.length, 3)
  for (const p of [pin, pw]) assert.deepEqual(problems(withIds(p)), [])
})

test('ask a parent: the chosen amount is marked', () => {
  const chips = askParentSpec({ ...o, more: '60' }).sections[1].items
  assert.deepEqual(chips.map((c) => [c.id, c.on]), [['more:15', false], ['more:30', false], ['more:60', true]])
})
