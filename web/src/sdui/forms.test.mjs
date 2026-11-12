// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { controlOf, saveAction, withValue } from './formsModel.ts'

test('a control is one of four kinds, else the row is plain', () => {
  assert.equal(controlOf({ control: { type: 'switch', value: true } }).type, 'switch')
  assert.equal(controlOf({ control: { type: 'dial' } }), null)
  assert.equal(controlOf({}), null)
})

test('a switch runs its row as the server worked it out; the others add the new value', () => {
  const a = { type: 'command', op: 'x.y', params: { value: false } }
  assert.deepEqual(withValue(a, { type: 'switch' }, true), a)
  assert.deepEqual(withValue({ type: 'command', op: 'x.y' }, { type: 'slider' }, 3), { type: 'command', op: 'x.y', params: { value: 3 } })
})

test('Save runs the Form\'s command with every named value', () => {
  assert.deepEqual(saveAction({ type: 'command', op: 'wittyplay.online.set' }, { sharePlaying: false }), { type: 'command', op: 'wittyplay.online.set', params: { sharePlaying: false } })
  assert.equal(saveAction(undefined, {}), undefined)
})
