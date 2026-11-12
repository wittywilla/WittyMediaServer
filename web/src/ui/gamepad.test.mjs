// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/ui/gamepad.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { isNintendo, padKeys } from './gamepad.ts'

// A pad with these buttons held down and its sticks where they're given.
const pad = (id, mapping, held = [], axes = [0, 0, 0, 0], count = 17) => ({
  id, mapping, axes, buttons: Array.from({ length: count }, (_, i) => ({ pressed: held.includes(i) })),
})
const keys = (pads, o = {}) => [...padKeys(pads, { hats: new Set(), ...o })].sort()

const XBOX = 'Xbox Wireless Controller (STANDARD GAMEPAD Vendor: 045e Product: 0b13)'
const PRO = 'Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)'

test('a standard pad: A chooses, B goes back, the D-pad moves', () => {
  assert.deepEqual(keys([pad(XBOX, 'standard', [0])]), ['Enter'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [1])]), ['Escape'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [12])]), ['ArrowUp'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [13])]), ['ArrowDown'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [14])]), ['ArrowLeft'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [15])]), ['ArrowRight'])
})

test('in the player the buttons are the shared model in the player\'s words', () => {
  const p = (held) => keys([pad(XBOX, 'standard', held)])
  assert.deepEqual(p([0]), ['Enter']) // A: select, and play or pause on the picture (tv.ts)
  assert.deepEqual(p([1]), ['Escape']) // B: the Back ladder
  assert.deepEqual(p([2]), ['ContextMenu']) // X: options, or the disc's menu
  assert.deepEqual(p([3]), ['i']) // Y: info
  assert.deepEqual(p([4]), ['j']) // LB, RB: seek
  assert.deepEqual(p([5]), ['l'])
  assert.deepEqual(p([6, 7]), ['PageDown', 'PageUp']) // LT, RT: chapters
  assert.deepEqual(p([8]), ['t'])
  assert.deepEqual(p([9]), ['ContextMenu'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [16])]), [])
})

test('Nintendo pads swap A and B, so the right-hand button still confirms', () => {
  assert.ok(isNintendo(PRO))
  assert.ok(isNintendo('057e-2009-Pro Controller'))
  assert.ok(isNintendo('57e-2009-Wireless Gamepad'))
  assert.ok(!isNintendo(XBOX))
  assert.deepEqual(keys([pad(PRO, 'standard', [0])]), ['Escape'])
  assert.deepEqual(keys([pad(PRO, 'standard', [1])]), ['Enter'])
})

test('the left stick moves past its dead zone, along the stronger axis', () => {
  assert.deepEqual(keys([pad(XBOX, 'standard', [], [0.5, 0, 0, 0])]), [])
  assert.deepEqual(keys([pad(XBOX, 'standard', [], [0.9, 0.2, 0, 0])]), ['ArrowRight'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [], [-0.7, 0, 0, 0])]), ['ArrowLeft'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [], [0.7, -0.9, 0, 0])]), ['ArrowUp'])
  assert.deepEqual(keys([pad(XBOX, 'standard', [], [0, 0.95, 0, 0])]), ['ArrowDown'])
  // The right stick is for games, not for moving around.
  assert.deepEqual(keys([pad(XBOX, 'standard', [], [0, 0, 1, 1])]), [])
})

test('two controllers at once', () => {
  assert.deepEqual(keys([pad(XBOX, 'standard', [0]), null, pad(PRO, 'standard', [15])]), ['ArrowRight', 'Enter'])
})

test('combined Joy-Cons, which have no standard layout, are laid out by name', () => {
  // Their D-pad comes after everything else, from button 14.
  const id = 'Nintendo Co., Ltd. Combined Joy-Cons (Vendor: 057e Product: 2008)'
  assert.deepEqual(keys([pad(id, '', [14], [0, 0, 0, 0], 18)]), ['ArrowUp'])
  assert.deepEqual(keys([pad(id, '', [17], [0, 0, 0, 0], 18)]), ['ArrowRight'])
  assert.deepEqual(keys([pad(id, '', [5], [0, 0, 0, 0], 18)]), ['j']) // the left bumper
})

test('Android lays every pad out the standard way, whatever it says', () => {
  const dualSense = pad('DualSense Wireless Controller', '', [13])
  assert.deepEqual(keys([dualSense], { android: true }), ['ArrowDown'])
})

test('an unknown pad: its first two buttons, and its D-pad on its last axes', () => {
  const id = 'Generic USB Joystick'
  assert.deepEqual(keys([pad(id, '', [0], [0, 0, 0, 0], 10)]), ['Enter'])
  assert.deepEqual(keys([pad(id, '', [1], [0, 0, 0, 0], 10)]), ['Escape'])
  assert.deepEqual(keys([pad(id, '', [], [0, 0, -1, 0], 10)]), ['ArrowLeft'])
})

test('a D-pad on one axis is found at rest and read round the clock', () => {
  const id = 'Retro Pad'
  const hats = new Set()
  // At rest the axis is out of range, which gives it away: nothing is held.
  assert.deepEqual(keys([pad(id, '', [], [0, 0, 1.2857], 10)], { hats }), [])
  assert.ok(hats.has(id))
  assert.deepEqual(keys([pad(id, '', [], [0, 0, -1], 10)], { hats }), ['ArrowUp'])
  assert.deepEqual(keys([pad(id, '', [], [0, 0, -0.4286], 10)], { hats }), ['ArrowRight'])
  assert.deepEqual(keys([pad(id, '', [], [0, 0, 0.1429], 10)], { hats }), ['ArrowDown'])
  assert.deepEqual(keys([pad(id, '', [], [0, 0, 0.7143], 10)], { hats }), ['ArrowLeft'])
  assert.deepEqual(keys([pad(id, '', [], [0, 0, -0.7143], 10)], { hats }), ['ArrowRight', 'ArrowUp'])
})

test('in the app (not the player) the buttons follow the one input model', () => {
  const ui = (held) => keys([pad(XBOX, 'standard', held)], { context: 'ui' })
  assert.deepEqual(ui([0]), ['Enter'])
  assert.deepEqual(ui([1]), ['Escape'])
  assert.deepEqual(ui([2]), ['ContextMenu']) // X: options
  assert.deepEqual(ui([3]), ['/']) // Y: search
  assert.deepEqual(ui([4]), ['Ctrl+[']) // LB, RB: the tab to the left, right
  assert.deepEqual(ui([5]), ['Ctrl+]'])
  assert.deepEqual(ui([6, 7]), ['PageDown', 'PageUp']) // LT, RT: page a long list
  assert.deepEqual(ui([9]), ['F6']) // Start: the Dock
  assert.deepEqual(ui([11]), ['Home']) // right stick click: top
  assert.deepEqual(ui([8]), []) // Back alone does nothing
  assert.deepEqual(ui([12, 15]), ['ArrowRight', 'ArrowUp'])
})

test('Back and Start together ask for help, and neither does its own job', () => {
  assert.deepEqual(keys([pad(XBOX, 'standard', [8, 9])], { context: 'ui' }), ['?'])
  // In the player the same two buttons keep their player meanings.
  assert.deepEqual(keys([pad(XBOX, 'standard', [8, 9])], { context: 'player' }), ['ContextMenu', 't'])
})

test('a Nintendo pad still swaps A and B in the app, and the other buttons stay in place', () => {
  assert.deepEqual(keys([pad(PRO, 'standard', [0, 1, 2, 3])], { context: 'ui' }), ['/', 'ContextMenu', 'Enter', 'Escape'])
})
