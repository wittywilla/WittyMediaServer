// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/ui/zonesModel.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { backLadder, cycleZone } from './zonesModel.ts'

test('the zone key goes Stage, Dock, Bar and round, skipping what the screen lacks', () => {
  const all = ['bar', 'stage', 'dock']
  assert.equal(cycleZone('stage', all), 'dock')
  assert.equal(cycleZone('dock', all), 'bar')
  assert.equal(cycleZone('bar', all), 'stage')
  assert.equal(cycleZone(null, all), 'stage')
  assert.equal(cycleZone('stage', ['bar', 'stage']), 'bar')
  assert.equal(cycleZone('bar', ['bar']), 'bar')
  assert.equal(cycleZone('stage', []), null)
})

test('Back unwinds a Layer, then the Dock, then the Stage to the Bar, then to Home, then leaves', () => {
  const at = { layer: false, dock: false, tab: 'live', inBar: false, first: 'home' }
  assert.equal(backLadder({ ...at, layer: true, dock: true }), 'layer')
  assert.equal(backLadder({ ...at, dock: true }), 'dock')
  assert.equal(backLadder(at), 'bar')
  assert.equal(backLadder({ ...at, inBar: true }), 'home')
  assert.equal(backLadder({ ...at, tab: 'home', inBar: true }), 'leave')
  assert.equal(backLadder({ ...at, tab: null }), 'history')
  // The dock comes before history too: a Dock on a pushed screen closes first.
  assert.equal(backLadder({ ...at, tab: null, dock: true }), 'dock')
})
