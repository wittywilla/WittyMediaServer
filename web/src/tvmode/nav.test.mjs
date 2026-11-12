// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/tvmode/nav.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { backStep, stepTab, tabPath, tabRootOf, tvKey, TV_TABS } from './nav.ts'
import { moreTabs, tabsFromMenu } from './nav.ts'

test('the bar is Home, Libraries, Games, Live TV, Search, then You', () => {
  assert.deepEqual(TV_TABS.map((t) => t.label), ['Home', 'Libraries', 'Games', 'Live TV', 'Search', 'You'])
})

test('a tab\'s first screen has the bar; a screen opened from it has none', () => {
  assert.equal(tabRootOf('/'), 'home')
  assert.equal(tabRootOf('/libraries'), 'libraries')
  assert.equal(tabRootOf('/libraries/'), 'libraries')
  assert.equal(tabRootOf('/games'), 'games')
  assert.equal(tabRootOf('/tv'), 'live')
  assert.equal(tabRootOf('/search'), 'search')
  assert.equal(tabRootOf('/you'), 'you')
  for (const pushed of ['/item/4', '/library/2', '/games/library/7', '/games/game/9', '/tv/recordings', '/settings/users', '/show/3']) {
    assert.equal(tabRootOf(pushed), null, pushed)
  }
})

test('LB and RB step along the bar and stop at its ends', () => {
  assert.equal(stepTab('home', 1), 'libraries')
  assert.equal(stepTab('search', 1), 'you')
  assert.equal(stepTab('you', 1), null)
  assert.equal(stepTab('home', -1), null)
  assert.equal(stepTab('live', -1), 'games')
  assert.equal(tabPath('live'), '/tv')
})

test('Back: a pushed screen, then the bar, then Home, then out', () => {
  assert.equal(backStep({ tab: null, inBar: false }), 'history')
  assert.equal(backStep({ tab: 'games', inBar: false }), 'bar')
  assert.equal(backStep({ tab: 'home', inBar: false }), 'bar')
  assert.equal(backStep({ tab: 'games', inBar: true }), 'home')
  assert.equal(backStep({ tab: 'you', inBar: true }), 'home')
  assert.equal(backStep({ tab: 'home', inBar: true }), 'leave')
})

test('every remote\'s Back, and the controller\'s B, is Back', () => {
  for (const e of [{ key: 'Escape' }, { key: 'GoBack' }, { key: 'BrowserBack' }, { key: 'Unidentified', keyCode: 461 }, { key: 'XF86Back', keyCode: 10009 }]) {
    assert.equal(tvKey(e, false), 'back')
    assert.equal(tvKey(e, true), 'back')
  }
  // Backspace deletes in a text field and is Back everywhere else.
  assert.equal(tvKey({ key: 'Backspace' }, false), 'back')
  assert.equal(tvKey({ key: 'Backspace' }, true), null)
})

test('the bumpers and Start, but not while typing', () => {
  assert.equal(tvKey({ key: 'PageUp' }, false), 'prevTab')
  assert.equal(tvKey({ key: 'PageDown' }, false), 'nextTab')
  assert.equal(tvKey({ key: 'ContextMenu' }, false), null) // options are app-wide now (ui/shortcuts.ts)
  assert.equal(tvKey({ key: 'PageDown' }, true), null)
  assert.equal(tvKey({ key: 'Enter' }, false), null)
  assert.equal(tvKey({ key: 'ArrowUp' }, false), null)
})

test('a server\'s menu becomes the bar: its order, the end entries last, Settings behind You', () => {
  const menu = [
    { id: 'home', label: 'Home', to: '/', icon: 'home', placement: 'primary' },
    { id: 'search', label: 'Search', to: '/search', icon: 'search', placement: 'end' },
    { id: 'libraries', label: 'Libraries', to: '/libraries', icon: 'library', placement: 'primary' },
    { id: 'you', label: 'You', to: '/you', placement: 'end', screen: 'profiles' },
    { id: 'settings', label: 'Settings', to: '/settings', placement: 'end', screen: 'settings' },
  ]
  const tabs = tabsFromMenu(menu)
  assert.deepEqual(tabs.map((t) => t.id), ['home', 'search', 'libraries', 'you'])
  assert.equal(tabRootOf('/libraries', tabs), 'libraries')
  assert.equal(tabRootOf('/library/3', tabs), null)
  assert.equal(stepTab('home', 1, tabs), 'search')
  assert.equal(stepTab('you', 1, tabs), null)
  assert.equal(tabPath('libraries', tabs), '/libraries')
  assert.equal(backStep({ tab: 'home', inBar: true, first: 'home' }), 'leave')
  // With no You to reach it from, Settings stays in the bar.
  assert.deepEqual(tabsFromMenu(menu.filter((m) => m.id !== 'you')).map((t) => t.id), ['home', 'search', 'libraries', 'settings'])
})

test('a server menu\'s More entries are gathered under one More tab, before the end entries', () => {
  const menu = [
    { id: 'home', label: 'Home', to: '/', placement: 'primary' },
    { id: 'search', label: 'Search', to: '/search', placement: 'primary' },
    { id: 'lib-1', label: 'Films', to: '/library/1', placement: 'primary' },
    { id: 'collections', label: 'Collections', to: '/collections', placement: 'more' },
    { id: 'lib-2', label: 'Music', to: '/library/2', placement: 'more' },
    { id: 'you', label: 'You', to: '/you', placement: 'end', screen: 'profiles' },
    { id: 'settings', label: 'Settings', to: '/settings', placement: 'more', screen: 'settings' },
  ]
  assert.deepEqual(tabsFromMenu(menu).map((t) => t.id), ['home', 'search', 'lib-1', 'more', 'you'])
  assert.deepEqual(moreTabs(menu).map((t) => t.id), ['collections', 'lib-2'])
  // Nothing placed under More: no More tab.
  assert.deepEqual(tabsFromMenu(menu.filter((m) => m.placement !== 'more')).map((t) => t.id), ['home', 'search', 'lib-1', 'you'])
})
