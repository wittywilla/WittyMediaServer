// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { exitWords, isPlayPads, isPlayPath, isPlayTop, padsFor, PLAY_EXIT, PLAY_TIP, swapFor, tipDue } from './playPaths.ts'

test('WittyPlay Online pages take over the window; nothing else does', () => {
  assert.ok(isPlayPath('/page/wittyplay-online'))
  assert.ok(isPlayPath('/page/wittyplay-controllers'))
  assert.ok(isPlayPath('/page/wittyplay-privacy'))
  assert.ok(isPlayPath('/design/play'))
  assert.ok(isPlayPath('/page/wittyplay'))
  assert.ok(isPlayPath('/wittyplay/settings/emulators'))
  assert.ok(!isPlayPath('/'))
  assert.ok(!isPlayPath('/page/home'))
  assert.ok(!isPlayPath('/game/12'))
  assert.ok(!isPlayPath('/page/wittyplayer'))
})

test('the exit tip shows until the profile has seen it, and names every way out', () => {
  assert.ok(tipDue(undefined))
  assert.ok(tipDue(false))
  assert.ok(!tipDue(true))
  assert.deepEqual(PLAY_TIP.ways.map((w) => w.key), ['Home', 'Esc', 'B'])
})

test('the buttons shown are guessed from a connected pad, else the keyboard', () => {
  assert.equal(padsFor([]), 'keyboard')
  assert.equal(padsFor(['Xbox Wireless Controller (STANDARD GAMEPAD Vendor: 045e Product: 0b13)']), 'xbox')
  assert.equal(padsFor(['DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)']), 'playstation')
  assert.equal(padsFor(['Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)']), 'nintendo')
  assert.equal(padsFor(['Generic USB Joystick']), 'xbox')
  assert.ok(isPlayPads('nintendo') && !isPlayPads('generic'))
})

test('a Nintendo choice makes the right-hand button confirm; the keyboard leaves pads to their ids', () => {
  assert.equal(swapFor('nintendo'), true)
  assert.equal(swapFor('xbox'), false)
  assert.equal(swapFor('playstation'), false)
  assert.equal(swapFor('keyboard'), null)
  assert.match(exitWords('playstation'), /Circle/)
  assert.match(exitWords('keyboard'), /Esc/)
})

test('Back exits to Home only from the top page', () => {
  assert.ok(isPlayTop('/page/wittyplay-online'))
  assert.ok(!isPlayTop('/page/wittyplay-privacy'))
  assert.equal(PLAY_EXIT, '/')
})
