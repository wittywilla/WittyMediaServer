// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Karaoke's voice settings: ranges, presets, what's kept, and what the nodes are given.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cleanFx, decodeFx, encodeFx, FX_CONTROLS, FX_OFF, FX_PRESETS, fxKey, fxNodes, fxValue, isOff, presetFx, VOICE_LEVELS, VOICE_PRESETS, VOICE_RANGES, whichPreset } from './voiceFx.ts'

test('off is every control at rest, and inside its range', () => {
  assert.ok(isOff(FX_OFF))
  for (const c of FX_CONTROLS) {
    assert.ok(FX_OFF[c.id] >= c.min && FX_OFF[c.id] <= c.max, c.id)
    assert.ok(c.step > 0 && c.max > c.min, c.id)
  }
  assert.equal(new Set(FX_CONTROLS.map((c) => c.id)).size, FX_CONTROLS.length)
  const n = fxNodes(FX_OFF)
  assert.equal(n.echo, 0); assert.equal(n.reverb, 0); assert.equal(n.double, 0); assert.equal(n.ratio, 1); assert.equal(n.threshold, 0)
  assert.equal(n.makeUp, 1); assert.equal(n.pocketDb, 0); assert.equal(n.pan, 0)
})

test('what comes back is checked: out of range clamped, junk dropped', () => {
  assert.deepEqual(cleanFx(null), FX_OFF)
  assert.deepEqual(cleanFx([1, 2]), FX_OFF)
  const f = cleanFx({ bass: 99, pan: -500, echo: 'loud', reverb: NaN, nonsense: 4, presence: 3 })
  assert.equal(f.bass, 12); assert.equal(f.pan, -100); assert.equal(f.echo, 0); assert.equal(f.reverb, 0); assert.equal(f.presence, 3)
  assert.ok(!('nonsense' in f))
  for (const junk of ['', null, 'nope', '7', '[]']) assert.deepEqual(decodeFx(junk), FX_OFF)
  assert.deepEqual(decodeFx(encodeFx({ ...FX_OFF, echo: 40 })), { ...FX_OFF, echo: 40 })
  assert.notEqual(fxKey(1), fxKey(2))
})

test('presets are themselves until something moves', () => {
  for (const p of FX_PRESETS) {
    const f = presetFx(p.id)
    assert.deepEqual(cleanFx(f), f, p.id) // every preset value is in range
    assert.equal(whichPreset(f), p.id)
  }
  assert.equal(whichPreset({ ...presetFx('mid-medium'), bass: 5 }), '')
  assert.equal(whichPreset(FX_OFF), 'off')
})

test('compression turns up the ratio and gives back some level', () => {
  const half = fxNodes({ ...FX_OFF, compress: 50 })
  assert.equal(half.threshold, -20); assert.equal(half.ratio, 4.5); assert.ok(half.makeUp > 1)
  const n = fxNodes({ ...FX_OFF, echoTime: 400, echoRepeats: 50, echo: 100, pocket: 6 })
  assert.equal(n.echoTime, 0.4); assert.equal(n.echoFeedback, 0.5); assert.ok(n.echo < 1); assert.equal(n.pocketDb, -6)
})

test('values read as people say them', () => {
  const byId = Object.fromEntries(FX_CONTROLS.map((c) => [c.id, c]))
  assert.equal(fxValue(byId.pan, 0), 'Centre')
  assert.equal(fxValue(byId.pan, -40), '40% left')
  assert.equal(fxValue(byId.lowCut, 20), 'Off')
  assert.equal(fxValue(byId.lowCut, 120), '120 Hz')
  assert.equal(fxValue(byId.bass, 3), '+3 dB')
  assert.equal(fxValue(byId.room, 1.6), '1.6 s')
  assert.equal(fxValue(byId.echo, 35), '35%')
})

test('nine voice presets: each voice range at each height, each one its own', () => {
  assert.equal(VOICE_PRESETS.length, 9)
  for (const r of VOICE_RANGES) for (const l of VOICE_LEVELS) {
    const p = VOICE_PRESETS.find((x) => x.range === r.id && x.level === l.id)
    assert.ok(p, `${r.id} ${l.id}`)
    assert.equal(p.id, `${r.id}-${l.id}`)
  }
  const shapes = new Set(VOICE_PRESETS.map((p) => encodeFx(presetFx(p.id))))
  assert.equal(shapes.size, 9, 'no two presets the same')
  assert.ok(!shapes.has(encodeFx(FX_OFF)))
  assert.deepEqual(FX_PRESETS.map((p) => p.id), ['off', ...VOICE_PRESETS.map((p) => p.id)])
})

test('voice presets follow the voice: the low cut and warmth go down with it, presence and compression up', () => {
  const at = (id) => presetFx(id)
  for (const r of VOICE_RANGES) {
    const [hi, mid, lo] = VOICE_LEVELS.map((l) => at(`${r.id}-${l.id}`))
    assert.ok(hi.lowCut > mid.lowCut && mid.lowCut > lo.lowCut, r.id + ' low cut')
    assert.ok(hi.bass <= mid.bass && mid.bass <= lo.bass, r.id + ' warmth')
    assert.ok(hi.presence <= mid.presence && mid.presence <= lo.presence, r.id + ' presence')
    assert.ok(hi.compress <= mid.compress && mid.compress <= lo.compress, r.id + ' compression')
  }
  // From the highest voice to the lowest, the low cut never goes up.
  const order = VOICE_RANGES.flatMap((r) => VOICE_LEVELS.map((l) => at(`${r.id}-${l.id}`).lowCut))
  for (let i = 1; i < order.length; i++) assert.ok(order[i] <= order[i - 1], 'low cut in order ' + i)
  // Nothing in a voice preset echoes: they're shaping, not effects.
  for (const p of VOICE_PRESETS) assert.equal(presetFx(p.id).echo, 0, p.id)
})

test('the nodes are never given anything but numbers, even from settings that are incomplete', () => {
  for (const fx of [{}, { compress: NaN }, { echoTime: Infinity, echo: 50 }, { ...FX_OFF, room: -4 }]) {
    for (const [k, v] of Object.entries(fxNodes(fx))) assert.ok(Number.isFinite(v), `${k} = ${v}`)
  }
})
