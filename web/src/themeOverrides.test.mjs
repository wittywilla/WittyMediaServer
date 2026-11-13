// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { moduleForPath, overrideDeclarations, safeColour, themeQuery, tokenVar } from './themeOverrides.ts'

test('a colour name becomes the CSS variable themes use', () => {
  assert.equal(tokenVar('accentFg'), '--accent-fg')
  assert.equal(tokenVar('bg2'), '--bg-2')
  assert.equal(tokenVar('borderStrong'), '--border-strong')
  assert.equal(tokenVar('fg'), '--fg')
  // The hovered accent is --accent-2 in every stylesheet; --accent-hover is nothing.
  assert.equal(tokenVar('accentHover'), '--accent-2')
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

test('a module\'s own colours (where its palette differs) apply on its pages, under the administrator\'s', () => {
  const t = {
    id: 'x', source: 'server', overrides: {}, tokens: { accent: '#111111', bg: '#000000', fg: '#ffffff' },
    modules: { wittyboardgames: { overrides: { fg: '#eeeeee' }, tokens: { accent: '#ff8800', bg: '#000000', fg: '#eeeeee' } } },
  }
  const got = Object.fromEntries(overrideDeclarations(t, 'wittyboardgames', true))
  assert.equal(got['--accent'], '#ff8800')
  assert.equal(got['--fg'], '#eeeeee')
  assert.equal(got['--bg'], undefined, 'the same as everyone\'s is left to the theme on screen')
  assert.deepEqual(overrideDeclarations(t, '', true), [])
})

test('the profile\'s own colours go over everything, and its colours for a scope over those', () => {
  const t = {
    id: 'x', source: 'server', overrides: { accent: '#222222' }, tokens: { accent: '#222222', fg: '#ffffff' }, personal: { fg: '#fafafa' },
    modules: { wittytv: { overrides: { fg: '#eeeeee' }, tokens: { accent: '#ff8800', fg: '#eeeeee' }, personal: { accent: '#00aa00' } } },
  }
  const on = Object.fromEntries(overrideDeclarations(t, 'wittytv', true))
  assert.equal(on['--fg'], '#fafafa', 'over the scope\'s administrator colour')
  assert.equal(on['--accent'], '#00aa00')
  // A device showing its own theme still gets the profile's own colours.
  const other = Object.fromEntries(overrideDeclarations(t, '', false))
  assert.equal(other['--fg'], '#fafafa')
  assert.equal(other['--accent'], undefined)
})

test('a finish colour from a theme file is a plain colour and nothing that could fetch or break out', () => {
  for (const ok of ['#fff', '#7fd8ff', '#0a6aa8cc', 'rgba(255,255,255,.9)', 'rgb(20 140 80 / 25%)', 'hsl(200, 80%, 50%)', ' #123456 ']) assert.equal(safeColour(ok), ok.trim(), ok)
  for (const bad of ['url(https://x/y.png)', 'red; background: url(x)', 'var(--accent)', 'rgba(1,2,3) }', 'expression(1)', '', 42, null, undefined]) assert.equal(safeColour(bad), '', String(bad))
})

test('the theme this device chose is asked of the server, so its scene comes with it', () => {
  assert.equal(themeQuery('', '', false), '')
  assert.equal(themeQuery('abc', '', false), '?device=abc')
  assert.equal(themeQuery('abc', 'paper', false), '?device=abc&theme=paper')
  assert.equal(themeQuery('', 'auto', true), '?theme=light')
  assert.equal(themeQuery('', 'auto', false), '?theme=default')
  // Made on this device: the server has no such theme, and it wears no scene.
  assert.equal(themeQuery('abc', 'custom', false), '?device=abc')
})
