import assert from 'node:assert/strict'
import { test } from 'node:test'
import { FlashGuard, GUARD, GUARD_CALM, flashMetric, worstFlashesPerSecond } from '../safety.ts'

const FPS = 60

// Plays a brightness signal through the guard, and gives what the screen would show.
function through(signal, seconds, limits = GUARD) {
  const g = new FlashGuard(limits)
  const values = [], times = []
  for (let f = 0; f < seconds * FPS; f++) {
    const t = f / FPS, m = signal(t)
    values.push(m * g.step(m, 1 / FPS))
    times.push(t)
  }
  return { values, times }
}

const square = (hz, low, high) => (t) => ((t * hz) % 1 < 0.5 ? high : low)

test('the counter itself recognises a flash when it sees one', () => {
  const t = [], v = []
  for (let f = 0; f < 240; f++) { t.push(f / FPS); v.push(square(8, 0.02, 0.9)(f / FPS)) }
  assert.equal(worstFlashesPerSecond(v, t), 8)
  const small = v.map((x) => x * 0.05)
  assert.equal(worstFlashesPerSecond(small, t), 0, 'a small change is not a flash')
})

test('the limits are small enough to keep any fast swing under a flash, by the sums', () => {
  for (const g of [GUARD, GUARD_CALM]) assert.ok(g.deadBand + g.rise / 3 < 0.1)
})

test('no strobe gets through at any rate, from any level', () => {
  for (const limits of [GUARD, GUARD_CALM]) {
    for (const hz of [1, 2, 3, 4, 5, 6, 8, 10, 12, 15, 20, 25, 30]) {
      for (const [low, high] of [[0, 1], [0.02, 0.9], [0.1, 0.4], [0.3, 0.75], [0.5, 1]]) {
        const { values, times } = through(square(hz, low, high), 8, limits)
        const n = worstFlashesPerSecond(values, times)
        assert.ok(n <= 3, `${hz} Hz between ${low} and ${high}: ${n} flashes in a second`)
      }
    }
  }
})

test('sudden flashes, random bursts and beats followed by darkness get through no better', () => {
  let seed = 7
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647
  const bursts = Array.from({ length: 2000 }, () => rnd())
  const random = (t) => bursts[Math.floor(t * 30) % bursts.length]
  const { values, times } = through(random, 20)
  assert.ok(worstFlashesPerSecond(values, times) <= 3)
  const pulses = (t) => 0.05 + 0.9 * Math.exp(-((t * 4) % 1) * 9)
  const p = through(pulses, 20)
  assert.ok(worstFlashesPerSecond(p.values, p.times) <= 3)
})

test('a low frame rate does not open a gap in the guard', () => {
  for (const fps of [15, 24, 30, 144]) {
    const g = new FlashGuard(GUARD)
    const v = [], ts = []
    for (let f = 0; f < fps * 8; f++) {
      const t = f / fps, m = square(5, 0.02, 0.9)(t)
      v.push(m * g.step(m, 1 / fps))
      ts.push(t)
    }
    assert.ok(worstFlashesPerSecond(v, ts) <= 3, `${fps} fps`)
  }
})

test('a steady picture is left alone once it has risen', () => {
  const { values } = through(() => 0.2, 3)
  assert.ok(values.at(-1) > 0.195)
})

test('falling needs no help, and the rise afterwards is slow', () => {
  const step = (t) => (t < 4 ? 0.5 : t < 5 ? 0.02 : 0.5)
  const { values, times } = through(step, 7)
  const at = (s) => values[times.findIndex((t) => t >= s)]
  assert.ok(at(4.1) <= 0.03)
  assert.ok(at(5.2) < 0.12, 'not straight back up')
  assert.ok(at(6.9) > at(5.2))
})

test('small movement below the flash size passes through untouched', () => {
  const wobble = (t) => 0.2 + 0.015 * Math.sin(t * 2 * Math.PI * 6)
  const { values } = through(wobble, 6)
  const tail = values.slice(-60)
  assert.ok(Math.max(...tail) - Math.min(...tail) > 0.02, 'the movement is still there')
})

test('saturated red counts for more, as the guideline says', () => {
  assert.ok(flashMetric(1, 0, 0) > flashMetric(0, 0, 1))
  assert.ok(flashMetric(1, 0, 0) >= 1.5, 'a change of 20/320 in R-G-B is scaled to compare with 0.1')
  assert.ok(Math.abs(flashMetric(1, 1, 1) - 1) < 0.001)
  assert.equal(flashMetric(0, 0, 0), 0)
})
