// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { arrange, entries, fitCount, groupOf, isActive, moved, frameFor } from './navModel.ts'

const items = [
  { id: 'home', label: 'Home', to: '/', end: true },
  { id: 'tv', label: 'TV guide', to: '/tv' },
  { id: 'lib:1', label: 'Films', to: '/library/1', kind: 'movies' },
  { id: 'lib:2', label: 'Series', to: '/library/2', kind: 'shows' },
  { id: 'lib:3', label: 'Music', to: '/library/3', kind: 'music' },
  { id: 'lib:4', label: 'Comics', to: '/library/4', kind: 'books' },
]
const ids = (l) => l.map((x) => x.id)

test('with no choices made, everything shows in its natural order', () => {
  assert.deepEqual(ids(arrange(items, {})), ids(items))
})

test('order and hiding apply, and a library that came later joins at the end', () => {
  const got = arrange(items, { order: ['lib:3', 'home', 'lib:1'], hidden: ['tv'] })
  assert.deepEqual(ids(got), ['lib:3', 'home', 'lib:1', 'lib:2', 'lib:4'])
})

test('an id that no longer exists is ignored', () => {
  assert.deepEqual(ids(arrange(items, { order: ['lib:99', 'lib:4'] }))[0], 'lib:4')
})

test('groups sit where their first member is; a group of one stays a link', () => {
  const got = entries(arrange(items, {}), { grouped: true })
  assert.deepEqual(got.map((e) => e.key), ['home', 'tv', 'group:Watch', 'lib:3', 'lib:4'])
  assert.deepEqual(ids(got[2].items), ['lib:1', 'lib:2'])
})

test('a person can move something into another group or out of them all', () => {
  const prefs = { grouped: true, groups: { 'lib:3': 'Watch', 'lib:1': '' } }
  assert.equal(groupOf(items[4], prefs), 'Watch')
  assert.equal(groupOf(items[2], prefs), '')
  assert.deepEqual(entries(arrange(items, {}), prefs).map((e) => e.key), ['home', 'tv', 'lib:1', 'group:Watch', 'lib:4'])
})

test('everything fits: no More', () => {
  assert.equal(fitCount([50, 50, 50], 2, 154, 40), 3)
})

test('too many: as many as leave room for More', () => {
  assert.equal(fitCount([50, 50, 50], 2, 153, 40), 2)
  assert.equal(fitCount([50, 50, 50], 2, 90, 40), 0)
  assert.equal(fitCount([50, 50, 50], 2, 92, 40), 1)
})

test('when the open page is under More, its longer name gets the room', () => {
  assert.equal(fitCount([50, 50, 50, 50], 2, 150, 40, 3, 90), 1)
  assert.equal(fitCount([50, 50, 50, 50], 2, 150, 40, 0, 90), 2)
})

test('active: home only on its own page, the rest on anything beneath them', () => {
  assert.equal(isActive(items[0], '/'), true)
  assert.equal(isActive(items[0], '/tv'), false)
  assert.equal(isActive(items[1], '/tv/recordings'), true)
  assert.equal(isActive(items[1], '/tvx'), false)
})

test('moved takes one item to a new place', () => {
  assert.deepEqual(moved(['a', 'b', 'c'], 0, 2), ['b', 'c', 'a'])
  assert.deepEqual(moved(['a', 'b', 'c'], 2, 0), ['c', 'a', 'b'])
})

test('the frame follows the server\'s nav style, whichever of the thirteen it is', () => {
  assert.deepEqual(frameFor(undefined), { nav: 'sidebar', style: 'side', rail: false, drawer: false, overlay: false, tabBar: true, sidebar: undefined })
  assert.deepEqual(frameFor({ nav: 'sidebar', size: 240 }), { nav: 'sidebar', style: 'side', rail: false, drawer: false, overlay: false, tabBar: true, sidebar: 240 })
  const rail = frameFor({ nav: 'railDrawer', size: 76, open: 240, drawer: 360 })
  assert.deepEqual([rail.rail, rail.drawer, rail.railWidth, rail.open, rail.drawerWidth], [true, true, 76, 240, 360])
  assert.equal(frameFor({ nav: 'rail', size: 80 }).drawer, false)
  for (const nav of ['topTabs', 'floatingGlassBar', 'cableBar', 'bevelBar', 'teleKeys', 'crossbar', 'ornamentTabs']) {
    const f = frameFor({ nav, size: 56, overlay: nav === 'topTabs' })
    assert.deepEqual([f.style, f.tabBar, f.nav], ['top', false, nav], nav)
  }
  assert.equal(frameFor({ nav: 'topTabs', overlay: true, size: 56 }).overlay, true)
  assert.deepEqual([frameFor({ nav: 'bottomTabs', size: 56 }).style, frameFor({ nav: 'bottomTabs', size: 56 }).tabBar], ['side', true])
})
