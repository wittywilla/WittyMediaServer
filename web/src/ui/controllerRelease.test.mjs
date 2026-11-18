// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { installControllerRelease } from './controllerRelease.ts'

function rig(tenFoot = false) {
  const classes = new Set(['using-controller', ...(tenFoot ? ['ten-foot'] : [])])
  const html = { classList: { contains: (c) => classes.has(c), remove: (c) => classes.delete(c) } }
  const handlers = {}
  const win = { addEventListener: (t, fn) => { (handlers[t] ||= []).push(fn) } }
  const timers = []
  installControllerRelease(win, html, (fn, ms) => { timers.push({ fn, ms }) })
  const fire = (t, e = {}) => (handlers[t] || []).forEach((h) => h(e))
  const run = () => { while (timers.length) timers.shift().fn() }
  return { classes, fire, run, timers }
}

test('a press does not change the layout: the class is still on between press and click', () => {
  const r = rig()
  r.fire('pointerdown', { pointerType: 'mouse' })
  assert.ok(r.classes.has('using-controller'), 'still on after the press')
  r.fire('pointerup', {})
  assert.ok(r.classes.has('using-controller'), 'still on after the release')
  r.fire('click', {})
  assert.ok(r.classes.has('using-controller'), 'still on while the click is delivered')
  r.run()
  assert.ok(!r.classes.has('using-controller'), 'gone a task after the click')
})

test('a mouse that moves releases the styling before any press', () => {
  const r = rig()
  r.fire('pointermove', { pointerType: 'mouse', movementX: 0, movementY: 0 })
  assert.ok(r.classes.has('using-controller'), 'no movement, no change')
  r.fire('pointermove', { pointerType: 'mouse', movementX: 3, movementY: 0 })
  assert.ok(!r.classes.has('using-controller'))
})

test('a touch move (scrolling) does not release it; the touch does, after its click', () => {
  const r = rig()
  r.fire('pointermove', { pointerType: 'touch', movementX: 0, movementY: 9 })
  assert.ok(r.classes.has('using-controller'))
  r.fire('pointerdown', { pointerType: 'touch' }); r.fire('click', {}); r.run()
  assert.ok(!r.classes.has('using-controller'))
})

test('a press that is never a click still releases, later', () => {
  const r = rig()
  r.fire('pointerdown', {}); r.fire('pointerup', {})
  assert.equal(r.timers.length, 1)
  assert.equal(r.timers[0].ms, 600)
  r.run()
  assert.ok(!r.classes.has('using-controller'))
})

test('a screen set up for across the room never leaves controller styling', () => {
  const r = rig(true)
  r.fire('pointermove', { pointerType: 'mouse', movementX: 5, movementY: 5 })
  r.fire('pointerdown', {}); r.fire('click', {}); r.run()
  assert.ok(r.classes.has('using-controller'))
})
