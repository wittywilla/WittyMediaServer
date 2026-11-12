// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { ownerFormSpec, ownerNoticeSpec } from './legal.ts'
import { problems, withIds } from '../model.ts'

test('owner notice: nothing to show without words; notice and contact when given', () => {
  assert.equal(ownerNoticeSpec(null), null)
  assert.equal(ownerNoticeSpec({ notice: '', contact: '' }), null)
  const p = ownerNoticeSpec({ notice: 'House rules.', contact: 'me@example.test' })
  assert.equal(p.sections[0].anchor, 'this-server')
  assert.deepEqual(p.sections.map((s) => s.kind), ['note', 'note', 'rows'])
  assert.equal(p.sections[2].items[0].value, 'me@example.test')
  assert.deepEqual(ownerNoticeSpec({ notice: 'x', contact: '' }).sections.map((s) => s.kind), ['note', 'note'])
  assert.deepEqual(problems(withIds(p)), [])
})

test('owner form: anchored, contact is a text field', () => {
  const p = ownerFormSpec('a')
  assert.equal(p.sections[0].anchor, 'owner-form')
  assert.deepEqual(p.sections[1].items.map((f) => [f.id, f.kind, f.value]), [['contact', 'text', 'a']])
  assert.deepEqual(problems(withIds(p)), [])
})
