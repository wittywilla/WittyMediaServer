// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  addAll, edgeLanding, windowStepSlots, canCatchup, catchupKey, channelOf, chooseSet, dialOutcome, filterChannels, moveItem, pageCount, pageLabel, pageStartOf,
  parseCatchupKey, prefsKey, readGuidePrefs, readPageSize, removeAll, rowsOnPage, sectionsOf, stepPage, stepSet, toggleIn, widerLookup, editableSets,
} from './guideLogic.ts'

const SETS = [
  { id: 'all', kind: 'all', name: 'All channels', count: 10423 },
  { id: 'fav', kind: 'favourites', name: 'Favourites', count: 12 },
  { id: 'set:7', kind: 'custom', name: 'Kids', count: 30, editable: true, mine: true },
  { id: 'set:9', kind: 'custom', name: 'Sports', count: 8, shared: true, editable: false, mine: false },
  { id: 'src:lib', kind: 'source', name: 'My library', count: 14 },
  { id: 'grp:News', kind: 'group', name: 'News', count: 120 },
  { id: 'grp:Kids News', kind: 'group', name: 'Kids News', count: 3 },
]

test('the remembered set is kept while the server has it, and is all channels once it has gone', () => {
  assert.equal(chooseSet('set:7', SETS), 'set:7')
  assert.equal(chooseSet('set:404', SETS), 'all')
  assert.equal(chooseSet(undefined, SETS), 'all')
  assert.equal(chooseSet('fav', []), 'all')
})

test('the next and previous set go round at either end', () => {
  assert.equal(stepSet(SETS, 'all', 1), 'fav')
  assert.equal(stepSet(SETS, 'all', -1), 'grp:Kids News')
  assert.equal(stepSet(SETS, 'grp:Kids News', 1), 'all')
  assert.equal(stepSet(SETS, 'gone', 1), 'all')
  assert.equal(stepSet([], 'x', 1), 'all')
})

test('the picker groups sets in sections and finds them by name', () => {
  assert.deepEqual(sectionsOf(SETS).map((s) => [s.title, s.sets.length]), [['Channels', 2], ['Your sets', 1], ['Shared sets', 1], ['Sources', 1], ['Groups', 2]])
  assert.deepEqual(sectionsOf(SETS, 'kids').map((s) => [s.title, s.sets.map((x) => x.id)]), [['Your sets', ['set:7']], ['Groups', ['grp:Kids News']]])
  assert.deepEqual(sectionsOf(SETS, 'nothing like it'), [])
  assert.deepEqual(editableSets(SETS).map((s) => s.id), ['set:7'])
})

test('pages: where a row is, how many, and what they are called', () => {
  assert.equal(pageStartOf(0, 50), 0)
  assert.equal(pageStartOf(49, 50), 0)
  assert.equal(pageStartOf(50, 50), 50)
  assert.equal(pageStartOf(10422, 100), 10400)
  assert.equal(pageStartOf(5000, 0), 0)
  assert.equal(pageCount(10423, 100), 105)
  assert.equal(pageCount(10423, 0), 1)
  assert.equal(pageCount(0, 25), 0)
  assert.equal(rowsOnPage(10400, 100, 10423), 23)
  assert.equal(rowsOnPage(0, 0, 10423), 10423)
  const fmt = (n) => n.toLocaleString('en-US')
  assert.equal(pageLabel(50, 50, 10423, fmt), 'Channels 51–100 of 10,423')
  assert.equal(pageLabel(10400, 100, 10423, fmt), 'Channels 10,401–10,423 of 10,423')
  assert.equal(pageLabel(0, 0, 1, fmt), '1 channel')
  assert.equal(pageLabel(0, 25, 0, fmt), 'No channels')
})

test('page keys keep the place on the page, and stop at the ends', () => {
  assert.equal(stepPage(7, 25, 100, 1), 32)
  assert.equal(stepPage(32, 25, 100, -1), 7)
  assert.equal(stepPage(7, 25, 100, -1), 0)
  assert.equal(stepPage(80, 25, 90, 1), 89)
  assert.equal(stepPage(60, 25, 90, 1), 85)
  assert.equal(stepPage(99, 100, 100, 1), 99)
  assert.equal(stepPage(12, 0, 100, 1), 12)
})

test('only a page size the guide offers is read back', () => {
  assert.equal(readPageSize('50'), 50)
  assert.equal(readPageSize(100), 100)
  assert.equal(readPageSize(30), 0)
  assert.equal(readPageSize(undefined), 0)
})

test('dialling: here, elsewhere to offer, or nowhere', () => {
  assert.equal(widerLookup('all', '', -1), false)
  assert.equal(widerLookup('all', 'bbc', -1), true)
  assert.equal(widerLookup('set:7', '', -1), true)
  assert.equal(widerLookup('set:7', '', 4), false)
  assert.deepEqual(dialOutcome(4, null), { kind: 'here', row: 4 })
  assert.deepEqual(dialOutcome(-1, 812), { kind: 'elsewhere', row: 812 })
  assert.deepEqual(dialOutcome(-1, -1), { kind: 'none' })
  assert.deepEqual(dialOutcome(-1, null), { kind: 'none' })
})

test('watch from the start: a started broadcast within the archive of a channel that keeps one', () => {
  const now = 1_000_000
  const ch = { kind: 'tuner', catchupDays: 2 }
  const p = (start, end, kind = 'broadcast') => ({ kind, start, end })
  assert.equal(canCatchup(ch, p(now - 600, now + 600), now), true)
  assert.equal(canCatchup(ch, p(now - 86400, now - 80000), now), true)
  assert.equal(canCatchup(ch, p(now - 3 * 86400, now - 3 * 86400 + 600), now), false)
  assert.equal(canCatchup(ch, p(now + 60, now + 600), now), false)
  assert.equal(canCatchup(ch, p(now - 600, now + 600, 'show'), now), false)
  assert.equal(canCatchup({ kind: 'tuner' }, p(now - 600, now + 600), now), false)
  assert.equal(canCatchup({ ...ch, encrypted: 'CA' }, p(now - 600, now + 600), now), false)
})

test('a stream from the start has a name of its own that leads back to its channel', () => {
  const k = catchupKey('tun-12', 1700000000.5, 1700003600)
  assert.equal(k, 'tun-12@1700000000-1700003600')
  assert.deepEqual(parseCatchupKey(k), { key: 'tun-12', start: 1700000000, end: 1700003600 })
  assert.equal(parseCatchupKey('tun-12'), null)
  assert.equal(channelOf(k), 'tun-12')
  assert.equal(channelOf('lib-3'), 'lib-3')
})

test('putting a set in order', () => {
  assert.deepEqual(moveItem(['a', 'b', 'c', 'd'], 0, 2), ['b', 'c', 'a', 'd'])
  assert.deepEqual(moveItem(['a', 'b', 'c', 'd'], 3, 0), ['d', 'a', 'b', 'c'])
  assert.deepEqual(moveItem(['a', 'b'], 1, 9), ['a', 'b'])
  assert.deepEqual(moveItem(['a', 'b'], 5, 0), ['a', 'b'])
  assert.deepEqual(toggleIn(['a', 'b'], 'c', true), ['a', 'b', 'c'])
  assert.deepEqual(toggleIn(['a', 'b'], 'a', true), ['a', 'b'])
  assert.deepEqual(toggleIn(['a', 'b'], 'a', false), ['b'])
  assert.deepEqual(addAll(['b'], ['a', 'b', 'c', 'a']), ['b', 'a', 'c'])
  assert.deepEqual(removeAll(['a', 'b', 'c'], ['c', 'a']), ['b'])
})

test('finding a channel by number, name or group', () => {
  const list = [{ number: '101', name: 'BBC One', group: 'UK' }, { number: '1010', name: 'News 24' }, { number: '7', name: 'Kids Zone', group: 'Kids' }]
  assert.deepEqual(filterChannels(list, '101').map((c) => c.number), ['101', '1010'])
  assert.deepEqual(filterChannels(list, 'kids').map((c) => c.number), ['7'])
  assert.deepEqual(filterChannels(list, ' bbc ').map((c) => c.number), ['101'])
  assert.equal(filterChannels(list, '').length, 3)
})

test('what this device remembers is read safely', () => {
  assert.equal(prefsKey('https://tv.example', 4), 'wms.tv.guide.https://tv.example.4')
  assert.equal(prefsKey('', null), 'wms.tv.guide.here.none')
  assert.deepEqual(readGuidePrefs('{"set":"set:7","pageSize":50}'), { set: 'set:7', pageSize: 50 })
  assert.deepEqual(readGuidePrefs('{"set":3,"pageSize":33}'), { set: undefined, pageSize: 0 })
  assert.deepEqual(readGuidePrefs('not json'), { set: undefined, pageSize: 0 })
  assert.deepEqual(readGuidePrefs(null), { set: undefined, pageSize: 0 })
})

test('past the edge the time window pages by half its width', () => {
  assert.deepEqual([3, 4, 5, 6, 8].map(windowStepSlots), [1, 2, 2, 3, 4])
  assert.equal(windowStepSlots(1), 1)
  assert.equal(windowStepSlots(0), 1)
})

test('the highlight lands just past the edge it left, inside the paged window', () => {
  const from = 1000, to = 1000 + 6 * 1800, step = windowStepSlots(6) * 1800
  // Right: the cell after the window, which the new window now holds.
  const right = edgeLanding(1, from, to, to + 600)
  assert.ok(right >= from + step && right < to + step)
  // A programme that ended inside the window still lands at its edge.
  assert.equal(edgeLanding(1, from, to, to - 100), to)
  // Left: the moment just before the window, inside the window that moved back.
  const left = edgeLanding(-1, from, to)
  assert.ok(left >= from - step && left < to - step)
})
