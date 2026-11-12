// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { bulkRequest, bulkSpec, bulkTitle } from './bulkedit.ts'
import { applyRuleField, collectionSpec, genresOf, toggleType } from './smartlist.ts'
import { activitySpec } from './activity.ts'
import { problems, withIds } from '../model.ts'

test('bulk edit: title, fields, done and request', () => {
  assert.equal(bulkTitle(1), 'Edit 1 title together')
  const p = bulkSpec({ count: 6, year: '1999', setRating: true, lock: 'lock', error: '', result: null })
  assert.equal(p.title, 'Edit 6 titles together')
  assert.deepEqual(problems(withIds(p)), [])
  assert.deepEqual(p.sections[1].items.map((f) => f.id), ['year', 'setRating'])
  const d = bulkSpec({ count: 6, year: '', setRating: false, lock: '', error: '', result: { done: 4, failed: ['A', 'B'] } })
  assert.match(d.sections[0].text, /Changed 4\. 2 couldn't/)
  assert.equal(d.sections[1].items.length, 2)
  const r = bulkRequest({ year: ' 2001 ', setRating: true, rating: 'PG', lock: 'unlock' })
  assert.deepEqual(r.set, { year: '2001', contentRating: 'PG' })
  assert.equal(r.unlock, true)
  assert.equal(bulkRequest({ year: '', setRating: false, rating: '', lock: '' }).nothing(false), true)
  assert.equal(bulkRequest({ year: '', setRating: false, rating: '', lock: '' }).nothing(true), false)
})

const view = (o = {}) => ({ name: 'Short', desc: '', kind: 'smart', canChooseKind: true, rule: { sort: 'added', desc: true, limit: 100 }, genresText: '', shared: false, canShare: true, preview: { count: 3 }, error: '', ...o })

test('collection dialog: manual is small, smart carries the rule and the count', () => {
  const m = collectionSpec(view({ kind: 'manual', canChooseKind: false, canShare: false, preview: null }))
  assert.deepEqual(m.sections.map((s) => s.kind), ['fields'])
  const p = collectionSpec(view())
  assert.deepEqual(problems(withIds(p)), [])
  assert.equal(p.sections.find((s) => s.choice).items.find((r) => r.on).id, 'kind:smart')
  assert.ok(p.sections.some((s) => s.kind === 'note' && s.text === '3 matches'))
  assert.equal(p.sections.find((s) => s.h === 'Everything that matches').items.length, 13)
})

test('collection rule changes', () => {
  let r = { sort: 'added' }
  r = applyRuleField(r, 'yearFrom', 1990); r = applyRuleField(r, 'ratingMin', ''); r = applyRuleField(r, 'order', 'a'); r = applyRuleField(r, 'liked', true)
  assert.deepEqual(r, { sort: 'added', yearFrom: 1990, ratingMin: undefined, desc: false, liked: true })
  assert.deepEqual(genresOf(' Comedy, ,Drama '), ['Comedy', 'Drama'])
  assert.deepEqual(toggleType(toggleType({}, 'music'), 'music').libraryTypes, [])
})

test('activity: notices and history filter', () => {
  const base = { tab: 'now', connected: true, loaded: true, readOnly: false, update: '', isAdmin: true, failedOnly: false, canClear: true }
  assert.equal(activitySpec(base).sections.length, 0)
  assert.equal(activitySpec({ ...base, connected: false, readOnly: true, update: '0.9' }).sections.length, 3)
  const h = activitySpec({ ...base, tab: 'history', failedOnly: true })
  assert.deepEqual(h.sections[0].items.map((c) => [c.id, c.on]), [['show:all', false], ['show:failed', true], ['clear', false]])
  assert.deepEqual(problems(withIds(h)), [])
})
