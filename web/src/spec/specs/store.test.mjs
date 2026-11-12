// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { addStoreSpec, otherStoresSpec, storeSearchSpec } from './store.ts'
import { problems, withIds } from '../model.ts'

const base = { q: 'tv', cats: [{ id: 'tv', title: 'Live TV', n: 2 }], cat: 'tv', show: 'updates', updates: 3, publishers: ['Witty'], publisher: '', platforms: [{ value: 'linux', label: 'Linux' }], platform: '', kinds: [], moduleKind: '', statuses: [{ value: 'stable', label: 'Stable' }], status: '', checking: false }

test('store search: chips carry the choice, kind select only when kinds exist', () => {
  const p = storeSearchSpec(base)
  assert.deepEqual(problems(withIds(p)), [])
  const cats = p.sections.find((s) => s.h === 'Category').items
  assert.deepEqual(cats.map((c) => [c.id, c.on]), [['cat:', false], ['cat:tv', true]])
  assert.equal(p.sections.find((s) => s.h === 'Show').items[1].title, 'Updates available · 3')
  assert.deepEqual(p.sections.at(-1).items.map((f) => f.id), ['publisher', 'platform', 'status'])
  const k = storeSearchSpec({ ...base, kinds: [{ value: 'web', label: 'Web' }] })
  assert.ok(k.sections.at(-1).items.some((f) => f.id === 'kind'))
  assert.equal(storeSearchSpec({ ...base, checking: true }).sections.find((s) => s.h === 'Show').items[2].disabled, true)
})

test('other stores: empty says so; each store has switch, facts, buttons', () => {
  assert.equal(otherStoresSpec([]).sections[0].kind, 'note')
  const s = { id: 4, name: 'Mine', host: 'x.test', enabled: true, url: 'https://x.test/c.json', fingerprint: 'abc', modules: 2, added: 'today', checked: '', private: true, inUse: ['A'], working: false }
  const p = otherStoresSpec([s])
  assert.deepEqual(problems(withIds(p)), [])
  assert.deepEqual(p.sections.at(-1).items.map((c) => c.id), ['store4:check', 'store4:remove'])
  assert.equal(p.sections.at(-2).items.find((r) => r.title === 'Last checked').value, 'Not yet')
  assert.equal(otherStoresSpec([{ ...s, enabled: false }]).sections.at(-1).items[0].disabled, true)
})

test('add a store: look first, then agreement gates Add', () => {
  const st = { url: 'https://a/c.json', key: '', allowPrivate: false, needKey: false, working: false, trust: false }
  const a = addStoreSpec(st)
  assert.deepEqual(problems(withIds(a)), [])
  assert.equal(a.sections.at(-1).items[0].id, 'look')
  assert.equal(addStoreSpec({ ...st, url: ' ' }).sections.at(-1).items[0].disabled, true)
  const b = addStoreSpec({ ...st, preview: { name: 'S', host: 'h', count: 1, warning: ['Careful.'], fingerprint: 'f', keySource: 'offered', modules: [{ id: 'm', name: 'M', tag: 'In development' }] } })
  assert.deepEqual(problems(withIds(b)), [])
  assert.equal(b.sections.at(-1).items[0].disabled, true)
  assert.equal(addStoreSpec({ ...st, trust: true, preview: { name: 'S', host: 'h', count: 1, warning: [], fingerprint: 'f', keySource: 'pasted', modules: [] } }).sections.at(-1).items[0].disabled, false)
})
