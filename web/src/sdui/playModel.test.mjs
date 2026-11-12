// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { buttonsOf } from './model.ts'
import { focusedOf, presenceClass } from './playModel.ts'

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
