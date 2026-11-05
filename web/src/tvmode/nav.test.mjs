// node --experimental-strip-types --test web/src/tvmode/nav.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { backStep, stepTab, tabPath, tabRootOf, tvKey, TV_TABS } from './nav.ts'

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
  assert.equal(tvKey({ key: 'ContextMenu' }, false), 'options')
  assert.equal(tvKey({ key: 'PageDown' }, true), null)
  assert.equal(tvKey({ key: 'Enter' }, false), null)
  assert.equal(tvKey({ key: 'ArrowUp' }, false), null)
})
