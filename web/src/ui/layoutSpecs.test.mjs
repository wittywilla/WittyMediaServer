// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/ui/layoutSpecs.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { specFor, specFromWire } from './layoutSpecs.ts'

const table = JSON.parse(readFileSync(fileURLToPath(new URL('../../../pkg/sdui/conformance/experience-wire-table.json', import.meta.url)), 'utf8'))

test('every experience in the server\'s wire table has a wireframe on every form factor', () => {
  assert.ok(table.experiences.length >= 14)
  for (const e of table.experiences) {
    for (const ff of ['tv', 'phone', 'tablet', 'desktop', 'web', 'headset']) {
      assert.ok(e.wire[ff], `${e.id} ${ff}`)
      const s = specFromWire(e.wire[ff])
      assert.ok(s.nav && s.home && s.cards, `${e.id} ${ff}`)
    }
  }
})

test('the layouts the canvas draws come out as drawn', () => {
  const at = (id) => specFor(table.experiences.find((e) => e.id === id).wire, 'desktop')
  assert.equal(at('lumen').nav, 'top')
  assert.equal(at('lumen').home, 'billboard')
  assert.equal(at('classic').nav, 'sidebar')
  assert.equal(at('classicCable').nav, 'cable')
  assert.equal(at('classicSatellite').nav, 'bevel')
  assert.equal(at('retro').nav, 'tele')
  assert.equal(at('aero').nav, 'floating')
  assert.equal(at('marquee').home, 'carousel')
  assert.equal(at('crossbar').nav, 'crossbar')
  assert.equal(at('dashboard').home, 'widgets')
  assert.equal(at('bigpicture').home, 'tiles')
  assert.ok(['rail', 'railDrawer'].includes(at('rail').nav))
})

test('a look with no wire is a plain top-tabs layout', () => {
  assert.deepEqual(specFor(undefined), { nav: 'top', home: 'billboard', cards: 'poster' })
})
