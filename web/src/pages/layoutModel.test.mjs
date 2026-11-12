// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { LAYOUT_PRESETS, applyPreset, deviceOf, zoneOf } from './layoutModel.ts'

const regions = [
  { id: 'brand', label: 'Server name', positions: ['start', 'center', 'top'] },
  { id: 'nav', label: 'Menu', positions: ['start', 'end', 'top', 'bottom'], locked: true },
  { id: 'search', label: 'Search', positions: ['nav', 'top', 'floating'] },
  { id: 'profile', label: 'You', positions: ['nav', 'top'], locked: true },
  { id: 'clock', label: 'Clock', positions: ['top', 'dock'] },
]

test('a region is drawn where its position puts it', () => {
  assert.equal(zoneOf('nav', 'start', 'start', 'top'), 'start')
  assert.equal(zoneOf('search', 'nav', 'bottom', 'top'), 'bottom')
  assert.equal(zoneOf('search', 'floating', 'start', 'top'), 'main')
  assert.equal(zoneOf('clock', 'dock', 'start', 'bottom'), 'bottom')
  assert.equal(zoneOf('hero', undefined, 'start', 'top'), 'main')
  assert.equal(zoneOf('brand', 'center', 'start', 'top'), 'top')
})

test('presets only set what a region can have, and never hide what is locked', () => {
  const out = applyPreset(regions, {}, LAYOUT_PRESETS.find((p) => p.id === 'right'))
  assert.equal(out.nav.position, 'end')
  const calm = applyPreset(regions, { nav: { position: 'top' } }, LAYOUT_PRESETS.find((p) => p.id === 'calm'))
  assert.equal(calm.clock.visible, false)
  assert.equal(calm.legend, undefined)
  assert.equal(calm.nav.position, 'top')
})

test('this window is one of the five devices', () => {
  assert.equal(deviceOf('web'), 'desktop')
  assert.equal(deviceOf('headset'), 'headset')
})
