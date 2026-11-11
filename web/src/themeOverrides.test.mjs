// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { moduleForPath, overrideDeclarations, tokenVar } from './themeOverrides.ts'

test('a colour name becomes the CSS variable themes use', () => {
  assert.equal(tokenVar('accentFg'), '--accent-fg')
  assert.equal(tokenVar('bg2'), '--bg-2')
  assert.equal(tokenVar('borderStrong'), '--border-strong')
  assert.equal(tokenVar('fg'), '--fg')
})

test('a page belongs to a module or to the server', () => {
  assert.equal(moduleForPath('/tv'), 'wittytv')
  assert.equal(moduleForPath('/tv/watch/lib-3'), 'wittytv')
  assert.equal(moduleForPath('/tvshows'), '')
  assert.equal(moduleForPath('/game/12'), 'wittyplay')
  assert.equal(moduleForPath('/m/wittydvd/drives'), 'wittydvd')
  assert.equal(moduleForPath('/m/../etc/x'), '')
  assert.equal(moduleForPath('/library/1'), '')
})

const theme = {
  id: 'default', source: 'server', overrides: { accent: '#FF8800', fg: '#EEEEEE' },
  modules: { wittytv: { overrides: { accent: '#00A2FF', bg: '#001020' } } },
}

test('global overrides apply everywhere, and a module tints only its own pages', () => {
  const home = Object.fromEntries(overrideDeclarations(theme, '', true))
  assert.deepEqual(home, { '--accent': '#FF8800', '--accent-rgb': '255 136 0', '--fg': '#EEEEEE' })
  const tv = Object.fromEntries(overrideDeclarations(theme, 'wittytv', true))
  assert.equal(tv['--accent'], '#00A2FF')
  assert.equal(tv['--accent-rgb'], '0 162 255')
  assert.equal(tv['--bg'], '#001020')
  assert.equal(tv['--bg-rgb'], '0 16 32')
  assert.equal(tv['--fg'], '#EEEEEE')
  // A module with no overrides gets the global ones only.
  assert.deepEqual(overrideDeclarations(theme, 'wittyplay', true), overrideDeclarations(theme, '', true))
})

test('with another theme on this device, only the module colours apply', () => {
  assert.deepEqual(overrideDeclarations(theme, '', false), [])
  assert.equal(Object.fromEntries(overrideDeclarations(theme, 'wittytv', false))['--fg'], undefined)
  assert.equal(Object.fromEntries(overrideDeclarations(theme, 'wittytv', false))['--accent'], '#00A2FF')
})

test('nothing but a plain colour reaches the page', () => {
  const bad = { id: 'x', source: 'server', overrides: { accent: 'red;} body{display:none', 'bad-name': '#FFFFFF', fg: 'url(https://example.org/x)', ok: '#12345678' }, modules: {} }
  assert.deepEqual(overrideDeclarations(bad, '', true), [['--ok', '#12345678']])
  assert.deepEqual(overrideDeclarations(null, 'wittytv', true), [])
})
