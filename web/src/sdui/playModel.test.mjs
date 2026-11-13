// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { buttonsOf } from './model.ts'
import { arrangeTabs, consolesOf, dotFor, focusedOf, onConsole, presenceClass } from './playModel.ts'

test('presence is a dot colour', () => {
  assert.equal(presenceClass('Playing'), 'on')
  assert.equal(presenceClass('online'), 'on')
  assert.equal(presenceClass(' Away '), 'away')
  assert.equal(presenceClass('Offline'), 'off')
  assert.equal(presenceClass(''), 'off')
})

test('the detail panel is about the focused item, else the first', () => {
  const items = [{ id: 'a' }, { id: 'b' }]
  assert.equal(focusedOf(items, 'b').id, 'b')
  assert.equal(focusedOf(items, 'gone').id, 'a')
  assert.equal(focusedOf([], 'a'), undefined)
})

test('an action carries the key it is drawn with, and only the four there are', () => {
  const b = buttonsOf({ actions: [{ id: 'p', label: 'Resume', glyph: 'a', primary: true }, { id: 'o', label: 'Options', glyph: 'q' }] })
  assert.equal(b[0].glyph, 'a')
  assert.equal(b[1].glyph, undefined)
})

test('the catalogue\'s tones decide the dot when the card carries them', () => {
  const tones = { Playing: 'ok', Online: 'ok', Away: 'warn', Offline: 'muted' }
  assert.equal(dotFor('Playing', tones), 'on')
  assert.equal(dotFor('Away', tones), 'away')
  assert.equal(dotFor('Offline', tones), 'off')
  assert.equal(dotFor('Online'), 'on', 'without tones, the names this app knows')
  assert.equal(dotFor('Busy', { Busy: 'warn' }), 'away')
})

test('a games row offers its consoles, most games first, and narrows to one', () => {
  const row = [{ props: { console: 'SNES' } }, { props: { console: 'NES' } }, { props: { console: 'SNES' } }, { props: {} }]
  assert.deepEqual(consolesOf(row), [{ name: 'SNES', count: 2 }, { name: 'NES', count: 1 }])
  assert.equal(onConsole(row, 'SNES').length, 2)
  assert.equal(onConsole(row, '').length, 4)
  assert.deepEqual(consolesOf([{ props: { console: 'SNES' } }]), [])
})

test('tabs follow the person’s order and leave out those hidden, but never the one showing', () => {
  const t = ['recent', 'classics', 'consoles', 'together'].map((value) => ({ value }))
  assert.deepEqual(arrangeTabs(t, ['consoles', 'recent']).map((x) => x.value), ['consoles', 'recent', 'classics', 'together'])
  assert.deepEqual(arrangeTabs(t, [], ['together', 'recent'], 'recent').map((x) => x.value), ['recent', 'classics', 'consoles'])
  assert.deepEqual(arrangeTabs(t).map((x) => x.value), ['recent', 'classics', 'consoles', 'together'])
})
