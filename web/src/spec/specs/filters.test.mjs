// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { facetLabel, filterSpec, pressFilter } from './filters.ts'
import { handledIds, problems, withIds } from '../model.ts'

const facets = {
  genres: [{ value: 'Drama', count: 9 }, { value: 'Comedy', count: 4 }], studios: Array.from({ length: 20 }, (_, i) => ({ value: 'S' + i, count: i + 1 })), networks: [], decades: [{ value: '2020', count: 3 }],
  years: [], content: [{ value: 'PG', count: 2 }], resolutions: [{ value: '4k', count: 1 }], audio: [{ value: 'en', count: 5 }], subtitles: [], formats: [{ value: 'none', count: 1 }, { value: 'Dolby', count: 2 }], tags: [], editions: [], kinds: [], platforms: [],
}

test('facet words', () => {
  assert.equal(facetLabel('decade', '2020'), '2020s')
  assert.equal(facetLabel('minRating', '7'), '7 or more')
  assert.equal(facetLabel('audio', 'en', () => 'English'), 'English')
  assert.equal(facetLabel('watched', 'no'), 'Not watched yet')
  assert.equal(facetLabel('resolution', '4k'), '4K')
})

test('the filter page: wording of the canvas, chips with counts, chosen first', () => {
  const p = filterSpec({ facets, params: { genre: 'Drama', decade: '2020' } })
  assert.equal(p.title, 'Narrow this library')
  assert.equal(p.eyebrow, 'Library · Filters')
  assert.equal(p.sections[0].h, 'Chosen')
  assert.deepEqual(p.sections[0].items.map((c) => c.title), ['Category: Drama ✕', 'Decade: 2020s ✕'])
  const cat = p.sections.find((s) => s.h === 'Category')
  assert.deepEqual(cat.items.map((c) => [c.title, c.on]), [['Drama (9)', true], ['Comedy (4)', false]])
  assert.ok(!p.sections.find((s) => s.h === 'Network'), 'empty facets are left out')
  assert.deepEqual(p.sections.find((s) => s.h === 'Sound').items.map((c) => c.id), ['f:format:Dolby'])
  assert.deepEqual(p.sections.map((s) => s.h).filter((h) => ['Score', 'Watched', 'Picture'].includes(h)), ['Score', 'Watched', 'Picture'])
  assert.deepEqual(problems(withIds(p)), [])
})

test('"N more" until expanded', () => {
  const p = filterSpec({ facets, params: {} })
  const st = p.sections.find((s) => s.h === 'Studio')
  assert.equal(st.items.length, 17)
  assert.equal(st.items.at(-1).title, '4 more')
  const q = filterSpec({ facets, params: {}, expanded: new Set(['studio']) })
  assert.equal(q.sections.find((s) => s.h === 'Studio').items.length, 20)
  assert.ok(!p.sections.some((s) => s.h === 'Chosen'))
})

test('Clear and Save are off until something is chosen', () => {
  const none = filterSpec({ facets, params: {} })
  assert.deepEqual(none.actions.map((a) => [a.id, !!a.disabled]), [['done', false], ['clear', true], ['save', true]])
  const some = filterSpec({ facets, params: { genre: 'Drama' } })
  assert.ok(handledIds(withIds(some)).has('save'))
  assert.ok(!handledIds(withIds(none)).has('clear'))
})

test('pressing', () => {
  assert.deepEqual(pressFilter('f:genre:Drama', {}), { set: ['genre', 'Drama'] })
  assert.deepEqual(pressFilter('f:genre:Drama', { genre: 'drama' }), { set: ['genre', ''] })
  assert.deepEqual(pressFilter('chosen:decade', { decade: '2020' }), { set: ['decade', ''] })
  assert.deepEqual(pressFilter('more:studio', {}), { more: 'studio' })
  assert.deepEqual(pressFilter('clear', {}), { verb: 'clear' })
  assert.equal(pressFilter('nonsense', {}), null)
})
