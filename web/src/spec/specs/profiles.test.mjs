// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { allowanceSpec, allowedAt, agesFor, collectionRulesSpec, featuresSpec, kindLimitsSpec, parentPinSpec } from './profiles.ts'
import { problems, withIds } from '../model.ts'

const systems = [
  { kinds: ['film'], ratings: [{ label: 'G', age: 0 }, { label: 'PG', age: 8 }, { label: 'PG-13', age: 13 }] },
  { kinds: ['film', 'tv'], ratings: [{ label: 'FSK 6', age: 6 }, { label: 'FSK 12', age: 12 }] },
]

test('features: off ones unticked, share links only for adults', () => {
  const kid = featuresSpec(['games'], 'kids').sections[0].items
  assert.equal(kid.find((f) => f.id === 'feature:games').value, false)
  assert.equal(kid.find((f) => f.id === 'feature:downloads').value, true)
  const sharing = kid.find((f) => f.id === 'feature:sharing')
  assert.equal(sharing.disabled, true); assert.equal(sharing.value, false); assert.equal(sharing.hint, 'Only for adult accounts.')
  assert.equal(featuresSpec([], 'adult').sections[0].items.find((f) => f.id === 'feature:sharing').value, true)
  assert.deepEqual(problems(withIds(featuresSpec([], 'adult'))), [])
})

test('rating helpers', () => {
  assert.deepEqual(agesFor(systems, 'film'), [0, 6, 8, 12, 13])
  assert.deepEqual(agesFor(systems, 'tv'), [6, 12])
  assert.equal(allowedAt(systems, 'film', 12), 'PG, FSK 12')
})

test('kind limits: same by default, unrated switch only with an age, a stray age is kept', () => {
  const p = kindLimitsSpec(['film', 'tv'], systems, { film: { maxAge: 9, allowUnrated: true } })
  const items = p.sections[0].items
  assert.deepEqual(items.map((i) => i.id), ['kind:film', 'unrated:film', 'kind:tv'])
  assert.equal(items[0].value, '9'); assert.ok(items[0].options.some((o) => o.value === '9'))
  assert.equal(items[2].value, 'same'); assert.equal(items[2].options[0].label, 'The same as everything else')
  assert.equal(kindLimitsSpec(['film'], systems, { film: { maxAge: -1, allowUnrated: false } }).sections[0].items.length, 1)
  assert.deepEqual(problems(withIds(p)), [])
})

test('collections and allowances', () => {
  const p = collectionRulesSpec([{ id: 1, name: 'A', count: 1 }, { id: 2, name: 'B', count: 3 }], [1], [2])
  assert.deepEqual(p.sections[0].items.map((i) => [i.value, i.hint]), [['allow', '1 title'], ['block', '3 titles']])
  const a = allowanceSpec(75, 0).sections[0].items
  assert.ok(a[0].options.some((o) => o.value === '75' && o.label === '1 h 15 min'))
  assert.equal(a[1].options[0].label, 'No limit')
})

test('parents PIN: save needs a PIN, the password and, with two-factor, a code', () => {
  const o = { isSet: false, pin: '', current: '', code: '', twoFactor: false, error: '', busy: false }
  assert.equal(parentPinSpec(o).actions[0].disabled, true)
  assert.equal(parentPinSpec({ ...o, pin: '1234', current: 'x' }).actions[0].disabled, false)
  assert.equal(parentPinSpec({ ...o, pin: '123', current: 'x' }).actions[0].disabled, true)
  assert.equal(parentPinSpec({ ...o, pin: '1234', current: 'x', twoFactor: true }).actions[0].disabled, true)
  const set = parentPinSpec({ ...o, isSet: true, current: 'x', twoFactor: true })
  assert.deepEqual(set.actions.map((a) => a.id), ['save', 'remove']); assert.equal(set.sections[0].items.length, 3)
  assert.deepEqual(problems(withIds(set)), [])
})
