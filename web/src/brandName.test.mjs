// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { accentMask, brandParts, pickFromMask, validColouring } from './brandName.ts'

// The server's own table of cases, so the two agree on every name.
const table = JSON.parse(readFileSync(new URL('../../internal/brandname/testdata/cases.json', import.meta.url), 'utf8'))

const marked = (parts) => parts.map((p) => (p.accent ? `[${p.text}]` : p.text)).join('')

test('names are coloured as the server colours them', () => {
  for (const c of table.cases) {
    const parts = brandParts(c.name, c.colouring ?? '')
    assert.equal(marked(parts), c.want, `${JSON.stringify(c.name)} with ${JSON.stringify(c.colouring ?? '')}`)
    assert.equal(parts.map((p) => p.text).join(''), c.name)
    parts.forEach((p, i) => {
      assert.ok(p.text !== '', 'no empty parts')
      if (i > 0) assert.notEqual(!!parts[i - 1].accent, !!p.accent, 'neighbours differ')
    })
  }
})

test('the settings accept the same colourings', () => {
  for (const v of table.valid) assert.ok(validColouring(v), v)
  for (const v of table.invalid) assert.ok(!validColouring(v), v)
})

test('a chosen colouring is saved and comes back the same', () => {
  for (const c of table.cases) {
    const r = Array.from(c.name)
    const mask = accentMask(c.name, c.colouring ?? '')
    const saved = pickFromMask(r, mask)
    assert.ok(validColouring(saved), saved)
    assert.equal(marked(brandParts(c.name, saved)), marked(brandParts(c.name, c.colouring ?? '')).replace(/\[(\s+)\]/gu, '$1'), c.name)
  }
  // Spaces at the ends of a coloured run are left out.
  assert.equal(pickFromMask(Array.from('A B'), [true, true, false]), 'pick:0-1')
  assert.equal(pickFromMask(Array.from('🎬 Movie'), [false, false, true, true, true, true, true]), 'pick:2-7')
})
