// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A library switch's words agree with the switch. Run with:
//   node --experimental-strip-types --test web/src/pages/settings/librarySwitches.test.mjs
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { commercialsSwitchText, nfoOn, nfoSwitchText } from './librarySwitches.ts'

test('.nfo files from other programs: on unless the library says off', () => {
  assert.equal(nfoOn({ foreignNfo: true }), true)
  assert.equal(nfoOn({ foreignNfo: false }), false)
  assert.equal(nfoOn({}), true, 'the default is on')
})

test('the words under a switch say what it is now', () => {
  assert.match(nfoSwitchText(true), /^On: /)
  assert.match(nfoSwitchText(false), /^Off: /)
  assert.match(commercialsSwitchText(true), /^On: /)
  assert.match(commercialsSwitchText(false), /^Off: /)
  // An on switch never reads as off.
  for (const lib of [{ foreignNfo: true }, {}]) assert.doesNotMatch(nfoSwitchText(nfoOn(lib)), /^Off/)
})

test('Settings, Libraries draws the switch and its words from the same value', () => {
  const page = readFileSync(new URL('../SettingsPage.tsx', import.meta.url), 'utf8')
  assert.match(page, /nfoSwitchText\(on\)/)
  assert.match(page, /checked=\{on\}/)
  assert.doesNotMatch(page, /<small className="muted">Off: /, 'no fixed "Off:" text under a switch')
})
