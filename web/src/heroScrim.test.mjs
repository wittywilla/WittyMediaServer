// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

// Words over a picture read over any picture only because of a scrim in the page colour at the theme's
// --hero-scrim, which scripts/theme-contrast.py checks against a white and a black picture. These pin the
// rules that draw it, so a change to the billboard or a title's header can't quietly drop it.
const css = readFileSync(new URL('./styles.css', import.meta.url), 'utf8')
const rule = (sel) => {
  const i = css.indexOf(sel + ' {')
  assert.ok(i >= 0, `${sel} is in styles.css`)
  return css.slice(i, css.indexOf('}', i))
}

test('the billboard draws a page-colour scrim at --hero-scrim behind its words', () => {
  const r = rule('.hero .info::before')
  assert.match(r, /rgb\(var\(--bg-rgb\) \/ var\(--hero-scrim\)\)/)
  assert.match(r, /z-index: -1/)
  assert.match(rule('.hero .info'), /isolation: isolate/, 'the scrim stays above the picture and its shade')
  // Over the words the scrim is solid: its fade starts only past the right of the words (the box reaches that far).
  assert.match(r, /calc\(-1 \* var\(--hero-scrim-fade\)\)/)
  assert.match(r, /calc\(100% - var\(--hero-scrim-fade\)\)/)
})

test('no opacity thins the words over the billboard', () => {
  assert.doesNotMatch(rule('.hero p'), /opacity/)
})

test('a title header is at least --hero-scrim where its words overlap it', () => {
  const r = rule('.detail-hero .shade')
  assert.match(r, /var\(--hero-scrim\)\) 42%/)
  assert.doesNotMatch(css.match(/\.detail h1 \{[^}]*\}/)[0], /rgba\(0,\s*0,\s*0/, 'a black shadow under dark words on a light theme')
})

test('the default --hero-scrim is the one the server sends native clients', () => {
  const ours = css.match(/--hero-scrim:\s*([\d.]+)/)[1]
  const go = readFileSync(new URL('../../internal/themes/design.go', import.meta.url), 'utf8').match(/DefaultHeroScrim = ([\d.]+)/)[1]
  assert.equal(Number(ours), Number(go))
})
