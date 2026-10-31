// The parts of the player with no browser in them. Run with: node --test
// (Node 22 needs --experimental-strip-types.)
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { computeLayout } from './layout.ts'
import { Hotkeys } from './hotkeys.ts'

const base = { areaW: 1920, areaH: 1080, srcW: 256, srcH: 240, aspect: 4 / 3, mode: 'original', custom: 0, integer: false, cropX: 0, cropY: 0, turns: 0, inset: 0 }

test('the picture fills the room it has, in its own shape', () => {
  const l = computeLayout(base)
  assert.equal(l.boxH, 1080)
  assert.equal(l.boxW, 1440)
})

test('fill uses all of it, whatever the shape', () => {
  const l = computeLayout({ ...base, mode: 'stretch' })
  assert.deepEqual([l.boxW, l.boxH], [1920, 1080])
})

test('square pixels are the source shape', () => {
  const l = computeLayout({ ...base, mode: 'square' })
  assert.equal(l.boxH, 1080)
  assert.equal(l.boxW, Math.round((1080 * 256) / 240))
})

test('whole-number scale gives every game pixel the same size', () => {
  const l = computeLayout({ ...base, integer: true })
  assert.equal(l.picH % 240, 0)
  assert.equal(l.picH, 960) // 4x, because 5x wouldn't fit
  assert.equal(l.scale, 4)
})

test('whole-number scale with square pixels has whole pixels in both directions', () => {
  const l = computeLayout({ ...base, integer: true, mode: 'square' })
  assert.equal(l.picW % 256, 0)
  assert.equal(l.picH % 240, 0)
})

test('it never gets smaller than one to one, even with no room', () => {
  const l = computeLayout({ ...base, areaW: 100, areaH: 100, integer: true })
  assert.ok(l.picW >= 1 && l.picH >= 1)
  assert.ok(l.boxW <= 100 + 1)
})

test('a sideways picture swaps the box', () => {
  const l = computeLayout({ ...base, turns: 1 })
  assert.equal(l.picW, l.boxH)
  assert.equal(l.picH, l.boxW)
  assert.ok(l.boxH <= 1080 && l.boxW <= 1920)
})

test('a crop shows less of the picture at the same size', () => {
  const l = computeLayout({ ...base, cropX: 10, cropY: 10 })
  assert.ok(Math.abs(l.scaleX - 1 / 0.8) < 1e-9)
  assert.ok(l.boxH === 1080 || l.boxW === 1920)
})

test('what surrounds the picture takes its share of the room', () => {
  const l = computeLayout({ ...base, inset: 25 })
  assert.equal(l.boxH, 810)
})

test('zero sizes before the page has any room do no harm', () => {
  const l = computeLayout({ ...base, areaW: 0, areaH: 0, srcW: 0, srcH: 0, aspect: 0 })
  assert.ok(l.boxW >= 1 && l.boxH >= 1 && Number.isFinite(l.scale))
})

const BIT = { b: 1, select: 1 << 2, a: 1 << 8, l: 1 << 10, r: 1 << 11 }
const keys = { 'hotkeys.padCombos': true, 'hotkeys.padModifier': 'select', 'hotkeys.pad.fastForward': 'r', 'hotkeys.pad.rewind': 'l', 'hotkeys.pad.quickSave': 'none', 'hotkeys.pad.screenshot': 'a' }

function hot() {
  const fired = []
  const h = new Hotkeys((a, down) => fired.push(`${a}:${down ? 'down' : 'up'}`), () => false)
  h.configure(keys)
  return { h, fired }
}

test('a button alone reaches the game, and nothing else happens', () => {
  const { h, fired } = hot()
  assert.equal(h.filter(BIT.r), BIT.r)
  assert.equal(h.filter(0), 0)
  assert.deepEqual(fired, [])
})

test('with the shortcut button held, a button does the shortcut instead of reaching the game', () => {
  const { h, fired } = hot()
  assert.equal(h.filter(BIT.select), BIT.select)
  assert.equal(h.filter(BIT.select | BIT.r), BIT.select)
  assert.deepEqual(fired, ['fastForward:down'])
  assert.equal(h.filter(BIT.select), BIT.select)
  assert.deepEqual(fired, ['fastForward:down', 'fastForward:up'])
})

test('letting go of the shortcut button first still keeps the other from the game until released', () => {
  const { h, fired } = hot()
  h.filter(BIT.select)
  h.filter(BIT.select | BIT.r)
  assert.equal(h.filter(BIT.r), 0, 'R is still kept from the game')
  assert.deepEqual(fired, ['fastForward:down'], 'and fast-forward holds')
  assert.equal(h.filter(0), 0)
  assert.deepEqual(fired, ['fastForward:down', 'fastForward:up'])
  assert.equal(h.filter(BIT.r), BIT.r, 'R is the game’s again')
})

test('pressing the shortcut button after another button does nothing', () => {
  const { h, fired } = hot()
  h.filter(BIT.r)
  assert.equal(h.filter(BIT.r | BIT.select), BIT.r | BIT.select)
  assert.deepEqual(fired, [])
})

test('something that happens once only fires once', () => {
  const { h, fired } = hot()
  h.filter(BIT.select)
  h.filter(BIT.select | BIT.a)
  h.filter(BIT.select | BIT.a)
  h.filter(BIT.select)
  h.filter(BIT.select | BIT.a)
  assert.deepEqual(fired, ['screenshot:down', 'screenshot:down'])
})

test('turning shortcuts off leaves every button to the game', () => {
  const { h, fired } = hot()
  h.configure({ ...keys, 'hotkeys.padCombos': false })
  assert.equal(h.filter(BIT.select | BIT.r), BIT.select | BIT.r)
  assert.deepEqual(fired, [])
})

test('a button with no shortcut stays with the game while the shortcut button is held', () => {
  const { h, fired } = hot()
  h.filter(BIT.select)
  assert.equal(h.filter(BIT.select | BIT.b), BIT.select | BIT.b)
  assert.deepEqual(fired, [])
})

test('a disabled menu state passes everything through', () => {
  const { h, fired } = hot()
  h.enabled = false
  assert.equal(h.filter(BIT.select | BIT.r), BIT.select | BIT.r)
  assert.deepEqual(fired, [])
})
