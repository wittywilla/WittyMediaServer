// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Karaoke's voice settings as audio nodes: whatever the controls say, only a finite number inside each node's limits
// reaches the sound (#517), and turning things off and on again never fails.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { FxChain, safeValue } from './fxChain.ts'
import { FX_CONTROLS, FX_OFF, FX_PRESETS, presetFx } from './voiceFx.ts'

// Just enough of an audio engine to see what each AudioParam is given.
function fakeEngine() {
  const writes = []
  const param = (name, value, min = -3.4e38, max = 3.4e38) => ({
    name, value, minValue: min, maxValue: max,
    setTargetAtTime(v) { writes.push([name, v]); if (!Number.isFinite(v)) throw new TypeError(name + ' is not finite'); this.value = v },
  })
  let made = 0
  const node = (kind, params = {}) => {
    const n = { kind, id: made++, outs: new Set(), ...params }
    n.connect = (to) => { n.outs.add(to); return to }
    n.disconnect = (to) => {
      if (to === undefined) { n.outs.clear(); return }
      if (!n.outs.has(to)) { const e = new Error('not connected'); e.name = 'InvalidAccessError'; throw e }
      n.outs.delete(to)
    }
    return n
  }
  const ctx = {
    currentTime: 0, sampleRate: 48000,
    createGain: () => node('gain', { gain: param('gain', 1) }),
    createBiquadFilter: () => node('biquad', { type: '', frequency: param('frequency', 350, 0, 24000), Q: param('Q', 1), gain: param('biquad.gain', 0, -3.4e38, 1541) }),
    createDynamicsCompressor: () => node('comp', { threshold: param('threshold', -24, -100, 0), ratio: param('ratio', 12, 1, 20), attack: param('attack', 0), release: param('release', 0), knee: param('knee', 0) }),
    createStereoPanner: () => node('pan', { pan: param('pan', 0, -1, 1) }),
    createDelay: (max) => node('delay', { delayTime: param('delayTime', 0, 0, max) }),
    createAnalyser: () => node('analyser', { fftSize: 256, getFloatTimeDomainData(buf) { buf.fill(ctx.poisoned ? NaN : 0) } }),
    createConvolver: () => node('convolver', { buffer: null }),
    createBuffer: (ch, len) => ({ getChannelData: () => new Float32Array(Math.min(len, 64)) }),
    poisoned: false,
  }
  return { ctx, writes, out: node('out') }
}

test('safeValue: a number in range, or nothing at all', () => {
  assert.equal(safeValue(5, 0, 10), 5)
  assert.equal(safeValue(-5, 0, 10), 0)
  assert.equal(safeValue(50, 0, 10), 10)
  for (const bad of [NaN, Infinity, -Infinity, undefined, null, '3']) assert.equal(safeValue(bad, 0, 10), null)
  assert.equal(safeValue(1, 5, 0), null)
})

test('every value FxChain gives a node is finite and inside that node\'s limits, whatever comes in', () => {
  const { ctx, writes, out } = fakeEngine()
  const chain = new FxChain(ctx, out)
  const junk = [NaN, Infinity, -Infinity, 1e9, -1e9, undefined, null, 'loud']
  const tries = [FX_OFF, ...FX_PRESETS.map((p) => presetFx(p.id))]
  for (const c of FX_CONTROLS) for (const v of [c.min, c.max, c.min - 1000, c.max + 1000, ...junk]) tries.push({ ...FX_OFF, echo: 50, reverb: 50, [c.id]: v })
  tries.push(Object.fromEntries(FX_CONTROLS.map((c) => [c.id, NaN])))
  tries.push({})
  for (const fx of tries) assert.doesNotThrow(() => chain.set(fx))
  assert.ok(writes.length > 0)
  for (const [name, v] of writes) assert.ok(Number.isFinite(v), `${name} was given ${v}`)
  const delays = writes.filter(([n]) => n === 'delayTime').map(([, v]) => v)
  assert.ok(delays.every((v) => v >= 0.02 && v <= 1.4))
  const ratio = writes.filter(([n]) => n === 'ratio').map(([, v]) => v)
  assert.ok(ratio.every((v) => v >= 1 && v <= 20))
  chain.close()
})

test('reverb off and on again, and closing twice, never throw', () => {
  const { ctx, out } = fakeEngine()
  const chain = new FxChain(ctx, out)
  chain.set({ ...FX_OFF, reverb: 60 })
  chain.set(FX_OFF)
  chain.set(FX_OFF)
  chain.set({ ...FX_OFF, reverb: 60, room: 3 })
  chain.set({ ...FX_OFF, reverb: 60, room: 4 })
  chain.set(FX_OFF)
  assert.doesNotThrow(() => chain.close())
  assert.doesNotThrow(() => chain.close())
})

test('a chain with something not a number in it says so', () => {
  const { ctx, out } = fakeEngine()
  const chain = new FxChain(ctx, out)
  assert.equal(chain.broken(), false)
  ctx.poisoned = true
  assert.equal(chain.broken(), true)
  chain.close()
})

test('a new chain is neutral before any settings reach it: no echo fed back, no compression (#517)', () => {
  const { ctx, out } = fakeEngine()
  const chain = new FxChain(ctx, out)
  // The echo's loop at the node default (1) built on the voice until it was NaN, and every sound after it with it.
  assert.equal(chain.echoBack.gain.value, 0)
  assert.equal(chain.comp.ratio.value, 1)
  assert.equal(chain.comp.threshold.value, 0)
  assert.equal(chain.echoLevel.gain.value, 0); assert.equal(chain.reverbLevel.gain.value, 0); assert.equal(chain.doubleLevel.gain.value, 0)
  chain.close()
})

test('the echo never comes back at full level, whatever is asked', () => {
  const { ctx, writes, out } = fakeEngine()
  const chain = new FxChain(ctx, out)
  for (const fx of [{ ...FX_OFF, echo: 100, echoRepeats: 80 }, { ...FX_OFF, echo: 100, echoRepeats: 1e6 }, { ...FX_OFF, echo: 100, echoRepeats: Infinity }]) {
    chain.set(fx)
    assert.ok(chain.echoBack.gain.value <= 0.85, 'fed back at ' + chain.echoBack.gain.value)
  }
  chain.set(FX_OFF)
  assert.equal(chain.echoBack.gain.value, 0)
  chain.close()
})
