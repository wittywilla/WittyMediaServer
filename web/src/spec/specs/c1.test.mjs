// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { connectSpec } from './connect.ts'
import { dayName, hours, yearSpec } from './year.ts'
import { problems, reachable, withIds } from '../model.ts'

test('connect: field, hint, Connect disabled until there is an address', () => {
  const s = { address: '', busy: false, error: '', insecure: false, hasCancel: true }
  const p = withIds(connectSpec(s))
  assert.deepEqual(problems(p), [])
  assert.equal(p.actions[0].disabled, true)
  assert.deepEqual(p.actions.map((a) => a.id), ['connect', 'cancel'])
  const q = withIds(connectSpec({ ...s, address: 'http://8.8.8.8', insecure: true, error: 'No answer', hasCancel: false }))
  assert.equal(q.actions[0].disabled, false)
  assert.equal(q.actions.length, 1)
  assert.equal(q.sections[0].kind, 'note')
  assert.equal(q.sections[0].tone, true)
  assert.equal(q.sections[1].items[0].error, 'No answer')
  assert.ok(reachable(q, new Set(['connect'])).includes('connect'))
})

test('hours and dayName', () => {
  assert.equal(hours(1800), '30 min')
  assert.equal(hours(7200), '2.0 h')
  assert.equal(hours(36000), '10 h')
  assert.match(dayName(20240104), /4/)
  assert.equal(dayName(), '')
})

const review = { year: 2024, seconds: 7200, items: 1, days: 1, longestRun: 1, busiestDay: 20240104, busiestSeconds: 7200, byType: [{ name: 'movies', seconds: 7200 }],
  byMonth: [7200, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0], topTitles: [{ id: 5, name: 'Harbour', seconds: 7200 }], topShows: [{ showId: 3, name: 'Orchard', seconds: 100 }], topArtists: [], topCreators: [], topGenres: [], years: [2023, 2024] }

test('year: totals, links, year picker, clear chip', () => {
  const p = withIds(yearSpec(review))
  assert.deepEqual(problems(p), [])
  assert.equal(p.title, 'Your 2024')
  assert.equal(p.sections[0].kind, 'fields')
  const tot = p.sections.find((s) => s.h === 'Totals')
  assert.equal(tot.items[0].value, '2.0 h')
  assert.equal(tot.items.at(-1).title, 'Biggest day')
  assert.equal(p.sections.find((s) => s.h === 'Movies and videos').items[0].to, '/item/5')
  assert.equal(p.sections.find((s) => s.h === 'Shows').items[0].to, '/show/3')
  assert.equal(p.sections.find((s) => s.h === 'What you spent it on').items[0].title, 'Movies')
  assert.equal(p.sections.find((s) => s.h === 'Artists'), undefined)
  assert.match(p.sections.find((s) => s.h === 'Month by month').text, /Busiest month: Jan, 2.0 h/)
  assert.equal(p.sections.at(-1).items[0].id, 'clear')
})

test('year: nothing yet', () => {
  const p = withIds(yearSpec({ ...review, seconds: 0, years: [2024] }))
  assert.equal(p.title, 'Nothing yet for 2024')
  assert.deepEqual(p.sections.map((s) => s.kind), ['note'])
  assert.deepEqual(problems(p), [])
})
