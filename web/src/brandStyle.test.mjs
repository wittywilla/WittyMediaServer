// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { ACCENT, FG, PRESETS, cellsFromSpans, contrast, fixed, gradient, markFromSetting, markSetting, markShown, nameWarnings, parseColour, partsFromCells, spanCount, spansFromCells, cellsFromColouring } from './brandStyle.ts'

test('cells become spans and come back the same', () => {
  const name = 'WittyMediaServer'
  const cells = PRESETS.find((p) => p.id === 'rainbow').cells(name)
  assert.deepEqual(cellsFromSpans(16, spansFromCells(cells)), cells)
  const auto = cellsFromColouring(name, 'auto')
  assert.deepEqual(spansFromCells(auto), [{ from: 5, to: 10, color: ACCENT }])
})

test('a name with only the text colour is still one span, so the server does not colour it itself', () => {
  assert.deepEqual(spansFromCells(PRESETS.find((p) => p.id === 'plain').cells('Home')), [{ from: 0, to: 4, color: FG }])
})

test('parts join letters of the same colour, a gradient stays one part', () => {
  const g = gradient(['#0a6aa8', '#3aa84a'])
  const parts = partsFromCells('Abc', [g, g, fixed('#ff0000')])
  assert.equal(parts.length, 2)
  assert.equal(parts[0].text, 'Ab')
  assert.equal(spanCount([g, g, fixed('#ff0000')]), 2)
})

test('the mark: first letter, initials, own characters', () => {
  assert.equal(markShown('WittyMediaServer', { kind: 'letter' }), 'W')
  assert.equal(markShown('WittyMediaServer', { kind: 'initials' }), 'WMS')
  assert.equal(markShown('x', { kind: 'text', text: ' ab ' }), 'ab')
  assert.deepEqual(markFromSetting(markSetting({ kind: 'text', text: 'ab' })), { kind: 'text', text: 'ab' })
  assert.deepEqual(markFromSetting(''), { kind: 'letter' })
})

test('contrast and the warnings', () => {
  assert.ok(Math.abs(contrast(parseColour('#000'), parseColour('#fff')) - 21) < 0.01)
  assert.deepEqual(parseColour('color(srgb 1 0 0)'), [255, 0, 0])
  const tokens = { fg: '#e8eaf0', brandAccent: '#9174ff' }
  const parts = partsFromCells('ab', [fixed('#3b2a1a'), FG])
  const w = nameWarnings(parts, [{ name: 'the side menu', colour: '#171a21' }], (t) => tokens[t] || '#ffffff')
  assert.equal(w.length, 1)
  assert.equal(w[0].text, 'a')
})
