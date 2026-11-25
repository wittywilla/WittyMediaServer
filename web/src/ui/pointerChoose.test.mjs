// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { clickOpens, pointerChooser } from './pointerChoose.ts'

// A row of cards as ShelfView keeps it: the chosen one, set by the focus (arrows, a remote, a pad) or by a click.
function row() {
  const r = { chosen: 'a', opened: [] }
  const handlers = pointerChooser(() => r.chosen, (id) => { r.chosen = id })
  // A press: pointerdown (the focus it brings chooses the card, as onFocus does), then the click, which opens unless stopped.
  r.press = (id, detail = 1) => {
    const h = handlers(id)
    h.onPointerDownCapture()
    r.chosen = id // the focus the press brings
    let stopped = false
    h.onClickCapture({ detail, preventDefault() { stopped = true }, stopPropagation() {} })
    if (!stopped) r.opened.push(id)
  }
  // Enter on the focused card, from a keyboard, a remote or a pad: a click with no mouse behind it.
  r.enter = (id) => {
    let stopped = false
    handlers(id).onClickCapture({ detail: 0, preventDefault() { stopped = true }, stopPropagation() {} })
    if (!stopped) r.opened.push(id)
  }
  return r
}

test('a pointer moving across cards chooses nothing: no handler runs on hover', () => {
  const handlers = pointerChooser(() => 'a', () => assert.fail('hovering chose a card'))
  const h = handlers('b')
  assert.deepEqual(Object.keys(h).sort(), ['onClickCapture', 'onPointerDownCapture'])
})

test('a click on another card chooses it; a second click opens it', () => {
  const r = row()
  r.press('c')
  assert.equal(r.chosen, 'c')
  assert.deepEqual(r.opened, [])
  r.press('c')
  assert.deepEqual(r.opened, ['c'])
})

test('a click on the card already chosen opens it at once', () => {
  const r = row()
  r.press('a')
  assert.deepEqual(r.opened, ['a'])
})

test('a double-click opens the card it lands on', () => {
  const r = row()
  // A browser sends a double-click as two clicks: the first (detail 1) chooses, the second (detail 2) opens.
  r.press('d', 1)
  r.press('d', 2)
  assert.deepEqual(r.opened, ['d'])
  assert.equal(r.chosen, 'd')
})

test('Enter (a keyboard, a remote or a pad) opens the focused card, chosen or not', () => {
  const r = row()
  r.enter('b')
  assert.deepEqual(r.opened, ['b'])
})

test('clickOpens', () => {
  assert.equal(clickOpens(0, false), true) // Enter
  assert.equal(clickOpens(1, false), false) // first click on another card
  assert.equal(clickOpens(1, true), true) // click on the chosen card
  assert.equal(clickOpens(2, false), true) // double-click
})

// Recent lost this once to a merge (#558 put back onPointerEnter={() => setFocusId(...)} in ShelfView). Every row with a
// chosen card goes through pointerChooser, and none chooses on hover.
test('ShelfView chooses with pointerChooser, never on hover', async () => {
  const { readFileSync } = await import('node:fs')
  const src = readFileSync(new URL('../sdui/render/components.tsx', import.meta.url), 'utf8')
  assert.match(src, /pointerChooser\(/)
  assert.doesNotMatch(src, /onPointer(Enter|Over|Move)=\{[^}]*setFocusId/)
  assert.doesNotMatch(src, /onMouse(Enter|Over|Move)=\{[^}]*setFocusId/)
})
