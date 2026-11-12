// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/ui/spatial.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { isBackKey, nearest, ranked, score } from './spatial.ts'

const box = (left, top, w = 100, h = 150) => ({ left, top, right: left + w, bottom: top + h })
const best = (from, all, dir) => ranked(from, all, (b) => b, dir)[0]

// Three rows of poster cards.
const grid = []
for (let y = 0; y < 3; y++) for (let x = 0; x < 6; x++) grid.push(box(x * 120, y * 180))
const at = (x, y) => grid[y * 6 + x]

test('when the focus has gone, the closest thing to where it was', () => {
  const b = (left, top) => ({ left, top, right: left + 100, bottom: top + 40 })
  const items = [b(0, 0), b(300, 410), b(0, 900)]
  assert.equal(nearest(b(300, 400), items, (x) => x), items[1])
  assert.equal(nearest(b(0, 0), [], (x) => x), undefined)
})

test('along a row it stays on the row', () => {
  assert.equal(best(at(2, 1), grid, 'right'), at(3, 1))
  assert.equal(best(at(2, 1), grid, 'left'), at(1, 1))
})

test('up and down keep the column', () => {
  assert.equal(best(at(4, 1), grid, 'down'), at(4, 2))
  assert.equal(best(at(4, 1), grid, 'up'), at(4, 0))
})

test('nothing that way means nowhere to go', () => {
  assert.equal(best(at(0, 0), grid, 'left'), undefined)
  assert.equal(best(at(5, 2), grid, 'down'), undefined)
  assert.equal(score(at(2, 1), at(2, 0), 'down'), null)
})

test('a far card on this row beats a near one on the next', () => {
  const here = box(0, 0)
  const sameLine = box(600, 0), nextLine = box(120, 180)
  assert.equal(best(here, [nextLine, sameLine], 'right'), sameLine)
})

test('from a narrow button down to a wide bar under it', () => {
  const button = box(500, 0, 40, 40)
  const bar = box(0, 60, 1000, 40), corner = box(0, 300, 40, 40)
  assert.equal(best(button, [corner, bar], 'down'), bar)
})

test('from a bar down to a very wide programme, not the footer straight below it', () => {
  const chip = box(437, 410, 52, 50)
  const programme = box(662, 500, 1195, 95), footer = box(372, 980, 90, 40)
  assert.equal(best(chip, [footer, programme], 'down'), programme)
})

test('down from a page title reaches the nearest thing under it, not the sidebar', () => {
  const title = box(300, 10, 200, 40)
  const sidebarLink = box(0, 70, 200, 40), firstCard = box(300, 90)
  assert.equal(best(title, [sidebarLink, firstCard], 'down'), firstCard)
})

test('left from the page reaches the sidebar link on the same line', () => {
  const card = box(300, 200)
  const links = [box(0, 40, 200, 40), box(0, 240, 200, 40), box(0, 500, 200, 40)]
  assert.equal(best(card, links, 'left'), links[1])
})

test('overlapping boxes still move', () => {
  const a = box(0, 0, 100, 100), b = box(50, 80, 100, 100)
  assert.equal(best(a, [b], 'down'), b)
})

test('the Back button on every remote', () => {
  for (const e of [{ key: 'GoBack' }, { key: 'BrowserBack' }, { key: 'Unidentified', keyCode: 461 }, { key: 'XF86Back', keyCode: 10009 }]) assert.ok(isBackKey(e))
  assert.ok(!isBackKey({ key: 'Enter', keyCode: 13 }))
})

test('the scoring weights come from the design tokens, and fall back to the defaults', async () => {
  const { DEFAULT_WEIGHTS, setWeights, weightsFrom, currentWeights } = await import('./spatial.ts')
  assert.deepEqual(weightsFrom(undefined), DEFAULT_WEIGHTS)
  assert.deepEqual(weightsFrom({ weights: { offLine: { up: 'x', down: -1, left: 9 } } }).offLine, { left: 9, right: 4, up: 1.2, down: 1.2 })
  assert.deepEqual(weightsFrom({ weights: { unknown: 1 }, other: 2 }), DEFAULT_WEIGHTS)
  const a = box(0, 0), far = box(900, 400)
  const before = score(a, far, 'down')
  try {
    // Sideways drift made free: going down scores only the gap.
    setWeights({ weights: { leavesLine: { left: 0, right: 0, up: 0, down: 0 }, offLine: { left: 0, right: 0, up: 0, down: 0 }, offCentre: { left: 0, right: 0, up: 0, down: 0 } } })
    assert.equal(score(a, far, 'down'), 250)
    assert.equal(currentWeights().offLine.right, 0)
  } finally { setWeights() }
  assert.equal(score(a, far, 'down'), before)
  assert.equal(currentWeights().offLine.right, 4)
})
