// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/ui/inputModel.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { legend, nextKind, padFamily, shortcut } from './inputModel.ts'

test('the keyboard shortcuts of the input model', () => {
  assert.equal(shortcut({ key: '/' }, false), 'search')
  assert.equal(shortcut({ key: 'k', ctrlKey: true }, false), 'search')
  assert.equal(shortcut({ key: 'K', metaKey: true }, true), 'search') // from inside a field too
  assert.equal(shortcut({ key: '?' }, false), 'help')
  assert.equal(shortcut({ key: '[', ctrlKey: true }, false), 'tabPrev')
  assert.equal(shortcut({ key: ']', ctrlKey: true }, false), 'tabNext')
  assert.equal(shortcut({ key: '.' }, false), 'options')
  assert.equal(shortcut({ key: 'F10', shiftKey: true }, false), 'options')
  assert.equal(shortcut({ key: 'ContextMenu' }, false), 'options')
  assert.equal(shortcut({ key: 'ArrowLeft', altKey: true }, false), 'back')
  assert.equal(shortcut({ key: 'Home' }, false), 'top')
  assert.equal(shortcut({ key: 'PageDown' }, false), 'pageDown')
  assert.equal(shortcut({ key: 'F6' }, false), 'dock')
})

test('typing in a field keeps its letters, punctuation and Backspace', () => {
  for (const key of ['/', '?', '.', 'Backspace', 'Home', 'PageUp']) assert.equal(shortcut({ key }, true), null, key)
  assert.equal(shortcut({ key: 'Escape' }, true), 'back')
  assert.equal(shortcut({ key: 'Backspace' }, false), 'back')
  assert.equal(shortcut({ key: 'x', ctrlKey: true }, false), null)
  assert.equal(shortcut({ key: 'GoBack' }, false), 'back')
  assert.equal(shortcut({ key: 'Unidentified', keyCode: 10009 }, false), 'back')
})

test('which input is in use', () => {
  assert.equal(nextKind('keys', 'pad'), 'pad')
  assert.equal(nextKind('pad', 'touch'), 'touch')
  assert.equal(nextKind('pad', 'pointer'), 'keys') // the mouse moved: back to a keyboard and mouse
  assert.equal(nextKind('touch', 'pointer'), 'touch') // a touch screen's synthetic mouse events stay touch
  assert.equal(nextKind('keys', 'key', { tenFoot: true }), 'remote')
  assert.equal(nextKind('keys', 'key', { key: 'a' }), 'keys')
  assert.equal(nextKind('pad', 'key', { key: 'Ctrl+[' }), 'pad')
})

test('the legend matches the input, and touch has none', () => {
  assert.deepEqual(legend('touch'), [])
  const pad = legend('pad', 'xbox')
  assert.equal(pad.find((i) => i.action === 'select').glyph, 'A')
  assert.equal(pad.find((i) => i.action === 'search').glyph, 'Y')
  assert.equal(legend('pad', 'playstation').find((i) => i.action === 'select').glyph, 'Cross')
  assert.equal(legend('keys').find((i) => i.action === 'search').glyph, '/')
  assert.equal(legend('remote').find((i) => i.action === 'back').glyph, 'Back')
  // A list adds its paging keys; a screen does not show them.
  assert.ok(!legend('keys').some((i) => i.action === 'pageDown'))
  assert.ok(legend('keys', 'generic', { scope: 'list' }).some((i) => i.action === 'pageDown'))
  // The same order each time.
  assert.deepEqual(legend('pad', 'xbox').map((i) => i.action), legend('pad', 'playstation').map((i) => i.action))
})

test('controllers are told apart by their ids', () => {
  assert.equal(padFamily('Xbox Wireless Controller (STANDARD GAMEPAD Vendor: 045e Product: 0b13)'), 'xbox')
  assert.equal(padFamily('DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)'), 'playstation')
  assert.equal(padFamily('Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)'), 'nintendo')
  assert.equal(padFamily('Some Pad'), 'generic')
})

test('Ctrl+Shift+L opens the layout editor', () => {
  assert.equal(shortcut({ key: 'L', ctrlKey: true, shiftKey: true }, false), 'layout')
  assert.equal(shortcut({ key: 'l', ctrlKey: true }, false), null)
})
