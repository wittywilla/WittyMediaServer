// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The karaoke mixing and sync rules, with made-up answers and clocks.
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { Hush, takeOver, decodeLevels, defaultLevel, drifted, DRIFT, encodeLevels, gainsFor, LEAD, levelsKey, partsAt, playable, pollDelay, qualityName, routesFor, sliders, startPlan, statusLine, streamedParts, streamStep, MAX_SECONDS } from './stemMix.ts'

const stem = (id, role, defaultGain = 1) => ({ id, role, label: id[0].toUpperCase() + id.slice(1), url: '/api/media/1/stems/' + id, defaultGain })
const parts = { state: 'ready', quality: 'stems', mix: 'parts', version: 'a', improving: false, progress: -1, pollSec: 0, stems: [stem('vocals', 'vocals'), stem('drums', 'music'), stem('bass', 'music', 0.8), stem('other', 'music')] }
const blend = { state: 'ready', quality: 'approximate', mix: 'blend', version: 'b', improving: true, progress: 40, pollSec: 5, stems: [stem('full', 'full'), stem('instrumental', 'music')] }
const near = (a, b) => assert.ok(Math.abs(a - b) < 1e-9, `${a} is not ${b}`)

test('parts: Voice is the singer, Background Music every other part, Track all of it', () => {
  assert.deepEqual(gainsFor(parts, {}, false), { vocals: 1, drums: 1, bass: 0.8, other: 1 })
  assert.deepEqual(gainsFor(parts, { vocals: 0.25, music: 0.5 }, false), { vocals: 0.25, drums: 0.5, bass: 0.4, other: 0.5 })
  const g = gainsFor(parts, { vocals: 1, music: 0.2, track: 0.5 }, false)
  near(g.vocals, 0.5); near(g.drums, 0.1); near(g.bass, 0.08)
  assert.deepEqual(sliders(parts).map((s) => s.id), ['vocals', 'music', 'track'])
  assert.deepEqual(sliders(blend).map((s) => s.label), ['Voice', 'Background Music', 'Track'])
})

test('the two sends a stem add up to the mix, and the voice chain gets only the voice', () => {
  for (const a of [parts, blend]) for (const v of [0, 0.4, 1]) for (const m of [0, 0.3, 1]) for (const t of [0.5, 1]) {
    const lv = { vocals: v, music: m, track: t }
    const g = gainsFor(a, lv, false), r = routesFor(a, lv, false)
    for (const s of a.stems) near(r[s.id].voice + r[s.id].music, g[s.id])
  }
  // A blend's voice send is the whole song less the music; its music send is the music alone.
  const r = routesFor(blend, { vocals: 0.5, music: 1, track: 1 }, false)
  near(r.full.voice, 0.5); near(r.instrumental.voice, -0.5); near(r.instrumental.music, 1); near(r.full.music, 0)
  assert.equal(routesFor(parts, {}, true).vocals.voice, 0)
})

test('Sing takes the voice to nothing and gives it back', () => {
  const levels = { vocals: 0.7 }
  assert.equal(gainsFor(parts, levels, true).vocals, 0)
  assert.equal(gainsFor(parts, levels, false).vocals, 0.7)
  assert.equal(gainsFor(blend, levels, true).full, 0)
  assert.equal(gainsFor(blend, levels, true).instrumental, 1)
})

test('a blend: the voice comes from the whole song, the music adds up to the Background Music slider', () => {
  assert.deepEqual(gainsFor(blend, {}, false), { full: 1, instrumental: 0 })
  for (const v of [0, 0.3, 0.5, 1]) for (const m of [0, 0.4, 1]) for (const t of [0.5, 1]) {
    const g = gainsFor(blend, { vocals: v, music: m, track: t }, false)
    near(g.full, t * v) // the voice
    near(g.full + g.instrumental, t * m) // the music, which is in both
  }
  assert.equal(defaultLevel({ mix: 'blend', stems: [stem('full', 'full', 0.6), stem('instrumental', 'music')] }, 'vocals'), 0.6)
  assert.equal(defaultLevel(blend, 'music'), 1)
  assert.equal(defaultLevel(blend, 'track'), 1)
})

test('levels out of range, and stems nobody knows, do no harm', () => {
  assert.equal(gainsFor(parts, { vocals: 7, music: NaN }, false).vocals, 1)
  assert.equal(gainsFor(parts, { music: -1 }, false).drums, 0)
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

test('a set taking over plays at the current levels and voice settings from its first sound (#517)', () => {
  const calls = []
  const player = { setRoutes: (r, v) => calls.push(['routes', r, v]), setFx: (fx) => calls.push(['fx', fx]) }
  const a = { mix: 'parts', stems: [{ id: 'vocals', role: 'vocals' }, { id: 'drums', role: 'music' }] }
  const fx = { echo: 30 }
  takeOver(player, a, { levels: { vocals: 0.4 }, sing: false, fx, volume: 0.8 })
  assert.deepEqual(calls.map((c) => c[0]), ['routes', 'fx'])
  assert.equal(calls[0][1].vocals.voice, 0.4)
  assert.equal(calls[0][2], 0.8)
  assert.equal(calls[1][1], fx)
  // The singer out, as karaoke starts: the new set starts without them too.
  calls.length = 0
  takeOver(player, a, { levels: {}, sing: true, fx, volume: 1 })
  assert.equal(calls[0][1].vocals.voice, 0)
})

test('the song stays hushed while one set of parts hands over to the next (#517)', () => {
  const h = new Hush()
  const quick = {}, separated = {}
  assert.equal(h.hold(quick), true)
  // The separated set takes over before the quick one lets go: the song must not come back in between, or after.
  assert.equal(h.hold(separated), true)
  assert.equal(h.release(quick), true)
  assert.equal(h.release(quick), true) // letting go twice changes nothing
  assert.equal(h.release(separated), false)
})

test('while the separated version loads, the quick one playing keeps its levels: they come from its own parts (#517)', () => {
  const quick = { mix: 'blend', stems: [stem('full', 'full'), stem('instrumental', 'music')] }
  const separated = { mix: 'parts', stems: [stem('vocals', 'vocals'), stem('drums', 'music'), stem('bass', 'music'), stem('other', 'music')] }
  // Routes from the newer answer name none of the playing set's parts: a set playing at those is silent.
  const wrong = routesFor(separated, {}, false)
  for (const s of quick.stems) assert.equal(wrong[s.id], undefined)
  // From the answer it was opened with, its music is heard.
  const right = routesFor(quick, {}, true)
  assert.ok(right.instrumental.music > 0)
})
