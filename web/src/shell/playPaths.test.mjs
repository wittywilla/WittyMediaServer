// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { moveTab, tabsPref, PLAY_TABS, libraryFor, yearsWords, layoutOf, exitWords, isPlayPads, isPlayPath, isPlayTop, padsFor, PLAY_EXIT, PLAY_TIP, swapFor, tipDue } from './playPaths.ts'
import { cardRoute, openOf, playGamePath } from './playPaths.ts'

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
  assert.deepEqual(PLAY_TIP.ways.map((w) => w.key), ['server', 'Esc', 'B'])
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

test('a console is browsed in its own library, else a mixed one', () => {
  const libs = [{ id: 1, type: 'movies' }, { id: 2, type: 'games', console: 'snes' }, { id: 3, type: 'games', console: 'auto' }]
  assert.equal(libraryFor(libs, 'snes').id, 2)
  assert.equal(libraryFor(libs, 'nes').id, 3)
  assert.equal(libraryFor([{ id: 1, type: 'movies' }], 'nes'), undefined)
})

test('years and layouts read as people say them', () => {
  assert.equal(yearsWords(1990, 1997), '1990 to 1997')
  assert.equal(yearsWords(2017), 'Since 2017')
  assert.equal(yearsWords(), '')
  assert.equal(layoutOf('grid'), 'grid')
  assert.equal(layoutOf('tiles'), 'shelf')
})

test('the tabs start Recent, Consoles, Classics, and each profile can move and hide them (never Recent)', () => {
  assert.deepEqual(PLAY_TABS.slice(0, 3).map((t) => t.id), ['recent', 'consoles', 'classics'])
  assert.deepEqual(tabsPref(undefined).order.slice(0, 3), ['recent', 'consoles', 'classics'])
  assert.deepEqual(tabsPref({ hidden: ['recent', 'discover'] }).hidden, ['discover'])
  assert.deepEqual(moveTab(['recent', 'consoles', 'classics'], 'classics', -1).slice(0, 3), ['recent', 'classics', 'consoles'])
  assert.deepEqual(moveTab(['recent', 'consoles'], 'recent', -1).slice(0, 2), ['recent', 'consoles'])
})

test('a game card opens its WittyPlay page first unless the profile starts playing', () => {
  assert.equal(cardRoute('/game/745', 'info'), '/wittyplay/game/745')
  assert.equal(cardRoute('/game/745', 'play'), '/game/745')
  assert.equal(cardRoute('/game/745?resume=1', 'info'), '/game/745?resume=1')
  assert.equal(cardRoute('/item/745', 'info'), '/item/745')
  assert.equal(openOf('play'), 'play')
  assert.equal(openOf(undefined), 'info')
  assert.equal(openOf('nonsense'), 'info')
  assert.equal(playGamePath(12), '/wittyplay/game/12')
})
