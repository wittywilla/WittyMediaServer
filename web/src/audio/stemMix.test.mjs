// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The karaoke mixing and sync rules, with made-up answers and clocks.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { decodeLevels, defaultLevel, drifted, DRIFT, encodeLevels, gainsFor, LEAD, levelsKey, partsAt, playable, pollDelay, qualityName, sliders, startPlan, statusLine, streamedParts, streamStep, MAX_SECONDS } from './stemMix.ts'

const stem = (id, role, defaultGain = 1) => ({ id, role, label: id[0].toUpperCase() + id.slice(1), url: '/api/media/1/stems/' + id, defaultGain })
const parts = { state: 'ready', quality: 'stems', mix: 'parts', version: 'a', improving: false, progress: -1, pollSec: 0, stems: [stem('vocals', 'vocals'), stem('drums', 'music'), stem('bass', 'music', 0.8), stem('other', 'music')] }
const blend = { state: 'ready', quality: 'approximate', mix: 'blend', version: 'b', improving: true, progress: 40, pollSec: 5, stems: [stem('full', 'full'), stem('instrumental', 'music')] }
const near = (a, b) => assert.ok(Math.abs(a - b) < 1e-9, `${a} is not ${b}`)

test('parts: each stem at its own level, the big slider is the voice', () => {
  assert.deepEqual(gainsFor(parts, {}, false), { vocals: 1, drums: 1, bass: 0.8, other: 1 })
  assert.deepEqual(gainsFor(parts, { vocals: 0.25, drums: 0.5 }, false), { vocals: 0.25, drums: 0.5, bass: 0.8, other: 1 })
  assert.deepEqual(sliders(parts).map((s) => s.id), ['vocals', 'drums', 'bass', 'other'])
})

test('Sing takes the voice to nothing and gives it back', () => {
  const levels = { vocals: 0.7 }
  assert.equal(gainsFor(parts, levels, true).vocals, 0)
  assert.equal(gainsFor(parts, levels, false).vocals, 0.7)
  assert.equal(gainsFor(blend, levels, true).full, 0)
  assert.equal(gainsFor(blend, levels, true).instrumental, 1)
})

test('a blend crosses the whole song against the instrumental in a straight line', () => {
  assert.deepEqual(gainsFor(blend, {}, false), { full: 1, instrumental: 0 })
  for (const v of [0, 0.3, 0.5, 1]) for (const m of [0, 0.4, 1]) {
    const g = gainsFor(blend, { vocals: v, instrumental: m }, false)
    near(g.full, m * v)
    near(g.instrumental, m * (1 - v))
    near(g.full + g.instrumental, m)
  }
  assert.deepEqual(sliders(blend), [{ id: 'vocals', label: 'Vocals' }, { id: 'instrumental', label: 'Music' }])
  assert.equal(defaultLevel({ mix: 'blend', stems: [stem('full', 'full', 0.6), stem('instrumental', 'music')] }, 'vocals'), 0.6)
})

test('levels out of range, and stems nobody knows, do no harm', () => {
  assert.equal(gainsFor(parts, { vocals: 7, drums: -1, bass: NaN }, false).vocals, 1)
  assert.equal(gainsFor(parts, { drums: -1 }, false).drums, 0)
  const odd = { ...parts, stems: [...parts.stems, stem('choir', 'backing')] }
  assert.equal(gainsFor(odd, {}, false).choir, 1)
  assert.equal(gainsFor({ mix: 'parts', stems: [stem('vocals', 'vocals'), stem('full', 'full')] }, {}, false).full, 0)
})

test('what can be played', () => {
  assert.ok(playable(parts) && playable(blend))
  assert.ok(!playable(null) && !playable({ ...parts, state: 'preparing' }))
  assert.ok(!playable({ ...blend, stems: [stem('full', 'full')] }))
  assert.ok(!playable({ ...parts, stems: [stem('vocals', 'vocals')] }))
  assert.ok(!playable({ ...parts, mix: 'something-new' }))
})

test('levels are kept by account, and what comes back is checked', () => {
  assert.notEqual(levelsKey(1), levelsKey(2))
  assert.deepEqual(decodeLevels(encodeLevels({ vocals: 0.333, drums: 2, 'bad key': 1 })), { vocals: 0.33, drums: 1 })
  for (const junk of ['', null, 'nonsense', '[]', '{"vocals":"loud","__proto__":1}', '7']) assert.deepEqual(decodeLevels(junk), {})
})

test('what the screen says', () => {
  assert.equal(qualityName(parts), 'Separated')
  assert.equal(qualityName(blend), 'Quick version')
  assert.equal(statusLine(blend), 'Preparing a better version… 40%')
  assert.equal(statusLine({ ...blend, progress: -1 }), 'Preparing a better version…')
  assert.equal(statusLine({ ...parts, state: 'preparing', progress: 12 }), 'Preparing karaoke… 12%')
  assert.equal(statusLine({ ...parts, state: 'unavailable', reason: 'mono', message: 'This song is mono.' }), 'This song is mono.')
  assert.equal(statusLine({ ...parts, state: 'unavailable', reason: 'not-made', message: 'x' }), '')
  assert.equal(statusLine(parts), '')
  assert.equal(statusLine(parts, 'The normal track is playing.'), 'The normal track is playing.')
})

test('asking again: as often as the server says, backing off when it can\'t be reached', () => {
  assert.equal(pollDelay(parts, 0), 0)
  assert.equal(pollDelay(blend, 0), 5000)
  assert.equal(pollDelay({ ...blend, pollSec: 9999 }, 0), 60000)
  assert.deepEqual([1, 2, 3, 4, 9].map((n) => pollDelay(blend, n)), [6000, 12000, 24000, 30000, 30000])
  assert.equal(pollDelay(null, 0), 0)
})

test('the parts stay with the song, and are restarted when they wander', () => {
  assert.equal(partsAt(10, 100, 103), 13)
  assert.equal(partsAt(10, 100, 99), 10)
  assert.ok(!drifted(10, 100, 103, 13.02))
  assert.ok(drifted(10, 100, 103, 13 + DRIFT + 0.01))
  assert.ok(drifted(10, 100, 103, 42)) // someone moved to another place in the song
  assert.ok(!drifted(10, 100, 99.99, 42)) // not started yet
  const plan = startPlan(30, 200)
  near(plan.when, 200 + LEAD)
  near(plan.offset, 30 + LEAD)
  assert.ok(!drifted(plan.offset, plan.when, 205, 30 + 5))
  assert.equal(startPlan(-1, 0).offset >= 0, true)
})

test('parts are streamed for another speed or a long song, decoded otherwise', () => {
  assert.equal(streamedParts(1, 200), false)
  assert.equal(streamedParts(1.25, 200), true)
  assert.equal(streamedParts(0.8, 200), true)
  assert.equal(streamedParts(1, MAX_SECONDS + 1), true)
  // Not known yet: decoded, as a song usually is short.
  assert.equal(streamedParts(1, NaN), false)
})

test('a streamed part is kept with the song by small changes of speed, and a jump when far off', () => {
  assert.deepEqual(streamStep(1, 0.01), { seek: false, rate: 1 })
  // The song is ahead: the part plays a little faster, never more than 5%.
  const faster = streamStep(1.5, 0.1)
  assert.equal(faster.seek, false)
  assert.ok(faster.rate > 1.5 && faster.rate <= 1.5 * 1.05)
  const slower = streamStep(1, -0.2)
  assert.ok(slower.rate < 1 && slower.rate >= 0.95)
  assert.equal(streamStep(1, 0.5).seek, true)
  assert.equal(streamStep(1, NaN).seek, true)
})
