// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The SDUI conformance kit (pkg/sdui/conformance): every fixture's document, through the
// renderer's own layout step, must come out as the fixture's expected layout; and every
// fixture's further pages of items must land in their section as the spec says.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { applyItems, buildPage, gridColumns, normalise, plainText } from './model.ts'

const dir = fileURLToPath(new URL('../../../pkg/sdui/conformance/fixtures/', import.meta.url))
const files = readdirSync(dir).filter((f) => f.endsWith('.json') && f !== 'index.json').sort()

// A missing key and its zero value are the same (the kit's README).
function prune(v) {
  if (Array.isArray(v)) return v.map(prune)
  if (v && typeof v === 'object') {
    const out = {}
    for (const k of Object.keys(v).sort()) {
      const x = prune(v[k])
      if (x === undefined || x === null || x === '' || x === 0 || x === false) continue
      if (Array.isArray(x) && x.length === 0) continue
      if (typeof x === 'object' && !Array.isArray(x) && Object.keys(x).length === 0) continue
      out[k] = x
    }
    return out
  }
  return v
}

function find(node, id) {
  if (node.id === id) return node
  for (const c of node.children || []) { const hit = find(c, id); if (hit) return hit }
  return null
}

test('the kit is the one this renderer was written against', () => {
  const index = JSON.parse(readFileSync(dir + 'index.json', 'utf8'))
  assert.equal(index.kit, 1)
  assert.equal(files.length, 57)
})

for (const file of files) {
  const fx = JSON.parse(readFileSync(dir + file, 'utf8'))
  test(`conformance: ${fx.id}`, () => {
    if (fx.expected.notFound) { assert.equal(fx.expected.document ?? null, null); return }
    const got = prune(JSON.parse(JSON.stringify(normalise(buildPage(fx.expected.document)))))
    const want = prune(fx.expected.layout)
    assert.deepEqual(got.page, want.page, 'page')
    assert.equal(got.visible.length, want.visible.length, 'how many elements are drawn')
    want.visible.forEach((w, i) => {
      const g = got.visible[i]
      for (const k of new Set([...Object.keys(w), ...Object.keys(g)])) assert.deepEqual(g[k], w[k], `${w.id}: ${k}`)
    })
    assert.deepEqual(got.focus, want.focus, 'focus')
    for (const more of fx.expected.more || []) {
      const section = find(fx.expected.document.root, more.section)
      assert.ok(section, `section ${more.section}`)
      const before = (section.children || []).map((c) => c.id)
      const after = applyItems(section, more.answer)
      const ids = (more.answer.items || []).map((c) => c.id)
      assert.deepEqual(after.children.map((c) => c.id), more.answer.replace ? ids : [...before, ...ids], `${more.section} after cursor ${more.cursor}`)
      assert.equal(after.props.more?.url, more.answer.next, 'where the next page is')
    }
  })
}

test('a deferred section takes its first page whole, and an empty answer takes it off the page', () => {
  const section = { type: 'Shelf', id: 'r', state: 'deferred', props: { title: 'Row', more: { url: '/a' } } }
  const filled = applyItems(section, { items: [{ type: 'Card', id: 'r:1' }], props: { title: 'Row', total: 1 } })
  assert.equal(filled.state, undefined)
  assert.equal(filled.props.more, undefined)
  assert.deepEqual(filled.children.map((c) => c.id), ['r:1'])
  assert.equal(applyItems(section, { state: 'empty' }), null)
  assert.equal(applyItems(section, { state: 'empty', props: { emptyText: 'Nothing yet' } }).state, 'empty')
})

test('a grid recomputes its columns when the window changes inside a breakpoint', () => {
  const l = { formFactor: 'web', breakpoint: 'large', spacing: { xs: 4, s: 8, m: 16, l: 24 }, text: {} }
  assert.equal(gridColumns(l, 180, 1408), 7)
  assert.equal(gridColumns(l, 180, 300), 2)
  assert.equal(gridColumns({ ...l, breakpoint: 'compact' }, 120, 358), 3)
  assert.equal(gridColumns({ ...l, formFactor: 'tv', focusEngine: true }, 250, 1760, 6), 6)
})

test('Markdown is read as plain text', () => {
  assert.equal(plainText('## Friday\nPick **one**. The [full list](/collections/12) is longer.'), 'Friday Pick one. The full list is longer.')
})
