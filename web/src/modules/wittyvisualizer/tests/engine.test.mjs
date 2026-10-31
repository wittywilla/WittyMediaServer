import assert from 'node:assert/strict'
import { test } from 'node:test'
import { Budget, Cycle, DEFAULT_SETTINGS, Palettes, eligible, normalizeSettings } from '../engine.ts'

const seeded = (s = 1) => () => (s = (s * 48271) % 2147483647) / 2147483647
const ids = ['a', 'b', 'c', 'd', 'e']

test('random order plays everything before anything comes round again, and never twice running', () => {
  const c = new Cycle(seeded())
  let last = null
  for (let round = 0; round < 40; round++) {
    const seen = new Set()
    for (let i = 0; i < ids.length - 1; i++) {
      const n = c.next(ids, true)
      assert.notEqual(n, last)
      assert.ok(!seen.has(n))
      seen.add(n)
      last = n
    }
  }
})

test('in order it goes round, and previous really goes back', () => {
  const c = new Cycle(seeded())
  assert.deepEqual([c.next(ids, false), c.next(ids, false), c.next(ids, false)], ['a', 'b', 'c'])
  assert.equal(c.previous(ids, false), 'b')
  const r = new Cycle(seeded())
  const first = r.next(ids, true), second = r.next(ids, true)
  assert.notEqual(first, second)
  assert.equal(r.previous(ids, true), first)
  assert.equal(r.next(ids, true), second, 'and forward again returns')
})

test('a list that shrinks or has one thing in it is fine', () => {
  const c = new Cycle(seeded())
  assert.equal(c.next(['only'], true), 'only')
  assert.equal(c.next(['only'], true), 'only')
  assert.equal(c.previous(['only'], true), 'only')
  assert.equal(c.next([], true), 'only')
})

const items = [
  { id: 'gentle1', motion: 'gentle', cost: 1 }, { id: 'gentle2', motion: 'gentle', cost: 3 },
  { id: 'lively1', motion: 'lively', cost: 1 }, { id: 'lively2', motion: 'lively', cost: 2 },
]

test('calm mode, favourites and a weak machine each narrow the running', () => {
  const names = (l) => l.map((e) => e.id)
  const none = { favourites: [], favouritesOnly: false }
  assert.deepEqual(names(eligible(items, none, false, false)), ['gentle1', 'gentle2', 'lively1', 'lively2'])
  assert.deepEqual(names(eligible(items, none, true, false)), ['gentle1', 'gentle2'])
  assert.deepEqual(names(eligible(items, none, false, true)), ['gentle1', 'lively1', 'lively2'])
  assert.deepEqual(names(eligible(items, { favourites: ['lively2'], favouritesOnly: true }, false, false)), ['lively2'])
  // A choice that leaves nothing isn't taken literally.
  assert.deepEqual(names(eligible(items, { favourites: ['lively2'], favouritesOnly: true }, true, false)), ['gentle1', 'gentle2'])
  assert.deepEqual(names(eligible(items, { favourites: [], favouritesOnly: true }, false, false)).length, 4)
})

test('resolution falls under load, frames fall only when that is not enough, and it recovers', () => {
  const b = new Budget(1, 0.25, 1)
  let changed = 0
  for (let i = 0; i < 400; i++) if (b.frame(1 / 15)) changed++
  assert.ok(changed > 0)
  assert.equal(b.scale, 0.25, 'all the way down')
  assert.equal(b.frameGap, 1 / 30, 'then fewer frames')

  const up = new Budget(0.5, 0.25, 1)
  for (let i = 0; i < 60 * 60; i++) up.frame(1 / 60)
  assert.equal(up.scale, 1, 'back up on a fast machine')
  const fine = new Budget(1, 0.25, 1)
  for (let i = 0; i < 60 * 30; i++) assert.equal(fine.frame(1 / 60), false, 'and left alone at full speed')
})

test('aiming at thirty frames a second, on purpose, is not taken for being slow', () => {
  const b = new Budget(0.33, 0.2, 1)
  b.frameGap = 1 / 30
  for (let i = 0; i < 30 * 90; i++) b.frame(1 / 30)
  assert.equal(b.scale, 1, 'it can use more pixels when there is time')
  const held = new Budget(0.33, 0.2, 1)
  held.frameGap = 1 / 30
  for (let i = 0; i < 400; i++) held.frame(1 / 20)
  assert.equal(held.scale, 0.2, 'and fewer when there is not')
})

test('it stops trying for more pixels when they keep failing', () => {
  const b = new Budget(0.5, 0.25, 1)
  let changes = 0
  // A machine that manages 60 at this resolution and 40 at any more.
  const limit = b.scale
  for (let i = 0; i < 60 * 300; i++) if (b.frame(b.scale > limit + 1e-6 ? 1 / 40 : 1 / 60)) changes++
  assert.ok(changes < 16, `changed resolution ${changes} times in five minutes`)
})

test('a chosen resolution stays, and a long pause is not taken for slowness', () => {
  const b = new Budget(0.5, 0.5, 0.5)
  for (let i = 0; i < 200; i++) b.frame(1 / 15)
  assert.equal(b.scale, 0.5)
  const p = new Budget(1, 0.25, 1)
  for (let i = 0; i < 100; i++) p.frame(1 / 60)
  assert.equal(p.frame(5), false)
  assert.equal(p.scale, 1)
})

test('settings from anywhere are made safe', () => {
  assert.deepEqual(normalizeSettings(null), DEFAULT_SETTINGS)
  assert.deepEqual(normalizeSettings('x'), DEFAULT_SETTINGS)
  const s = normalizeSettings({ notice: 'yes', on: 1, effect: 'x'.repeat(99), random: 'no', cycleSec: 7, favourites: ['a', 3, {}, 'b'], intensity: 90, sensitivity: -4, calm: 'on', scale: 0.2, stats: true })
  assert.equal(s.notice, false, 'only true counts as having read the notice')
  assert.equal(s.on, false)
  assert.equal(s.effect, null)
  assert.equal(s.random, true)
  assert.equal(s.cycleSec, 30)
  assert.deepEqual(s.favourites, ['a', 'b'])
  assert.equal(s.intensity, 1.5)
  assert.equal(s.sensitivity, 0.25)
  assert.equal(s.calm, null)
  assert.equal(s.scale, 0)
  assert.equal(normalizeSettings({ intensity: NaN }).intensity, 1)
  assert.equal(normalizeSettings({ favourites: Array(500).fill('a') }).favourites.length, 64)
  assert.equal(DEFAULT_SETTINGS.on, false, 'off until someone turns it on')
  assert.equal(DEFAULT_SETTINGS.notice, false, 'and the notice comes first')
})

test('colours move with the beats, slowly, and stay in range', () => {
  const list = [
    { id: 'x', a: [0, 0, 0], b: [1, 1, 1], c: [1, 1, 1], d: [0, 0, 0] },
    { id: 'y', a: [1, 1, 1], b: [0, 0, 0], c: [1, 1, 1], d: [1, 1, 1] },
  ]
  const p = new Palettes(list, seeded(5))
  const start = p.out.a[0]
  p.step(1 / 60, 24, false)
  const first = p.out.a[0]
  assert.ok(Math.abs(first - start) < 0.01, 'a change begins gently')
  for (let i = 0; i < 300; i++) p.step(1 / 60, 24, false)
  assert.notEqual(p.out.a[0], start)
})
