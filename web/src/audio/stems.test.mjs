// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  backingOf, carryChoice, carryPosition, decodedBytes, defaultLevels, enginePosition, gainFor, levelsAreDefault, multitrackPlan,
  present, reasonWords, sourcesOf, startPlan, stemGains, stemsToLoad, tracksEnabled, voiceChoices, END_GUARD_SEC, START_LEAD_SEC,
} from './stems.ts'

const stem = (name, extra = {}) => ({ name, url: `/api/media/1/stems/${name}`, mime: 'audio/flac', size: 1000, source: 'sidecar', ...extra })
const from = (...names) => sourcesOf(names.map((n) => stem(n)))
const ids = (choices) => choices.map((c) => c.id)

test('what a song has is read from the listing', () => {
  const s = sourcesOf([stem('vocals'), stem('instrumental', { approximate: true, source: 'approximate' }), stem('multitrack', { multitrack: true }), stem('choir')])
  assert.deepEqual(s, { vocals: true, instrumental: true, drums: false, bass: false, other: false, approximate: true, multitrack: true })
  assert.deepEqual(present(s), ['vocals', 'instrumental'])
  assert.equal(sourcesOf([stem('instrumental')]).approximate, false)
})

test('the music without the voice is the three parts when all are there, else the instrumental', () => {
  assert.equal(backingOf(from('vocals', 'drums', 'bass', 'other')), 'parts')
  assert.equal(backingOf(from('vocals', 'instrumental', 'drums', 'bass', 'other')), 'parts')
  // Two of the three would leave a hole in the music.
  assert.equal(backingOf(from('vocals', 'instrumental', 'drums', 'bass')), 'instrumental')
  assert.equal(backingOf(from('vocals', 'drums', 'bass')), 'none')
  assert.equal(backingOf(from('vocals')), 'none')
})

test('real stems replace the browser\'s trick; without them it stays as it was', () => {
  const none = voiceChoices(from())
  assert.deepEqual(ids(none), ['on', 'lower', 'lowest'])
  assert.deepEqual(none.map((c) => [c.engine, c.vocalCut]), [['mix', 0], ['mix', 0.6], ['mix', 1]])

  const real = voiceChoices(from('vocals', 'instrumental'))
  assert.deepEqual(ids(real), ['on', 'lower', 'off'])
  assert.deepEqual(real.map((c) => c.engine), ['mix', 'stems', 'stems'])
  assert.ok(real.every((c) => c.vocalCut === 0), 'the middle of the mix is never cancelled on top of real stems')

  // Music without the voice, and no voice part: the trick stays, and the instrumental is one more choice.
  const approx = voiceChoices(sourcesOf([stem('instrumental', { approximate: true })]))
  assert.deepEqual(ids(approx), ['on', 'lower', 'lowest', 'instrumental'])
  assert.equal(approx[3].name, 'Instrumental (approximate)')
  assert.equal(approx[3].approximate, true)
  assert.equal(voiceChoices(from('instrumental'))[3].name, 'Instrumental')

  // A voice part alone can't be taken away from anything.
  assert.deepEqual(ids(voiceChoices(from('vocals'))), ['on', 'lower', 'lowest'])
  // Parts this browser couldn't decode count as not there.
  assert.deepEqual(ids(voiceChoices(from('vocals', 'instrumental'), false)), ['on', 'lower', 'lowest'])
})

test('a choice carries to the next song as the nearest thing it offers', () => {
  const real = voiceChoices(from('vocals', 'instrumental')), none = voiceChoices(from()), inst = voiceChoices(from('instrumental'))
  assert.equal(carryChoice('off', real), 'off')
  assert.equal(carryChoice('off', none), 'lowest')
  assert.equal(carryChoice('off', inst), 'instrumental')
  assert.equal(carryChoice('lowest', real), 'off')
  assert.equal(carryChoice('instrumental', real), 'off')
  assert.equal(carryChoice('instrumental', none), 'lowest')
  assert.equal(carryChoice('lower', real), 'lower')
  assert.equal(carryChoice('lower', none), 'lower')
  assert.equal(carryChoice('on', real), 'on')
})

test('each choice plays the music once, and the voice as asked', () => {
  const four = from('vocals', 'drums', 'bass', 'other')
  assert.deepEqual(defaultLevels(four, 'lower'), { vocals: 0.5, instrumental: 0, drums: 1, bass: 1, other: 1 })
  assert.deepEqual(defaultLevels(four, 'off'), { vocals: 0, instrumental: 0, drums: 1, bass: 1, other: 1 })

  // Both the instrumental and all three parts: the parts play, the instrumental doesn't.
  const both = from('vocals', 'instrumental', 'drums', 'bass', 'other')
  assert.deepEqual(defaultLevels(both, 'off'), { vocals: 0, instrumental: 0, drums: 1, bass: 1, other: 1 })

  // The instrumental and only some parts: the instrumental plays, the parts wait at nothing.
  const some = from('vocals', 'instrumental', 'drums', 'bass')
  assert.deepEqual(defaultLevels(some, 'lower'), { vocals: 0.5, instrumental: 1, drums: 0, bass: 0, other: 0 })
  assert.deepEqual(stemGains(some, defaultLevels(some, 'lower')), { vocals: 0.25, instrumental: 1, drums: 0, bass: 0 })
  assert.deepEqual(stemGains(some, defaultLevels(some, 'off')), { vocals: 0, instrumental: 1, drums: 0, bass: 0 })

  const inst = from('instrumental')
  assert.deepEqual(stemGains(inst, defaultLevels(inst, 'instrumental')), { instrumental: 1 })
})

test('a slider is heard as loudness: squared, and kept between nothing and full', () => {
  assert.equal(gainFor(0), 0)
  assert.equal(gainFor(1), 1)
  assert.equal(gainFor(0.5), 0.25)
  assert.ok(Math.abs(20 * Math.log10(gainFor(0.5)) + 12.04) < 0.01, 'half-way is 12 dB down')
  assert.equal(gainFor(-3), 0)
  assert.equal(gainFor(7), 1)
  assert.equal(gainFor(NaN), 0)
  for (let l = 0; l < 1; l += 0.05) assert.ok(gainFor(l + 0.05) > gainFor(l), 'it only ever rises')
})

test('moved sliders are noticed', () => {
  const s = from('vocals', 'instrumental')
  const l = defaultLevels(s, 'lower')
  assert.equal(levelsAreDefault(s, 'lower', l), true)
  assert.equal(levelsAreDefault(s, 'lower', { ...l, vocals: 0.3 }), false)
  // A part the song doesn't have can't make it "moved".
  assert.equal(levelsAreDefault(s, 'lower', { ...l, drums: 0.9 }), true)
})

test('all the parts are fetched when they fit, the heard ones when they don\'t, none when the song is too long', () => {
  const s = from('vocals', 'instrumental', 'drums', 'bass')
  const l = defaultLevels(s, 'off')
  assert.equal(decodedBytes(60), 60 * 48000 * 8)
  assert.deepEqual(stemsToLoad(s, l, 240), { ok: true, names: ['vocals', 'instrumental', 'drums', 'bass'], all: true })
  // Twenty minutes: four parts are too much, the one that is heard isn't.
  assert.deepEqual(stemsToLoad(s, l, 1200), { ok: true, names: ['instrumental'], all: false })
  assert.deepEqual(stemsToLoad(s, l, 3 * 3600), { ok: false, names: [], all: false })
  assert.deepEqual(stemsToLoad(from(), l, 60), { ok: false, names: [], all: true })
})

test('every part starts at the same moment from the same place', () => {
  const p = startPlan(100, 12.5, 200)
  assert.deepEqual(p, { when: 100 + START_LEAD_SEC, offset: 12.5 })
  // One plan serves every part: there is nothing per part to drift.
  assert.equal(Object.keys(p).length, 2)
  assert.equal(startPlan(100, -4, 200).offset, 0)
  assert.equal(startPlan(100, 500, 200).offset, 200 - END_GUARD_SEC)
})

test('the position while the parts play follows what is heard', () => {
  const st = { offset: 30, startedAt: 100.06, playing: true }
  assert.equal(enginePosition(st, 100, 0, 200), 30, 'not started yet: still where it starts from')
  assert.ok(Math.abs(enginePosition(st, 110.06, 0, 200) - 40) < 1e-9)
  // A speaker a quarter of a second behind: the words wait for it.
  assert.ok(Math.abs(enginePosition(st, 110.06, 0.25, 200) - 39.75) < 1e-9)
  assert.equal(enginePosition(st, 100.1, 0.25, 200), 30, 'never before where it started')
  assert.equal(enginePosition(st, 9999, 0, 200), 200, 'never past the end')
  assert.equal(enginePosition({ ...st, playing: false }, 9999, 0, 200), 30, 'paused: it stays')
  assert.ok(Math.abs(enginePosition(st, 110.06, NaN, 200) - 40) < 1e-9, 'a browser that can\'t say its delay')
  assert.ok(Math.abs(enginePosition(st, 110.06, 30, 200) - 39) < 1e-9, 'an absurd delay is held to a second')
})

test('going between the song and its parts keeps the same second of the song', () => {
  // Song to parts and back, with time passing in between: nothing is lost or gained.
  const dur = 180
  const fromSong = 42.137
  const plan = startPlan(500, fromSong, dur)
  assert.equal(plan.offset, fromSong)
  const st = { offset: plan.offset, startedAt: plan.when, playing: true }
  const later = enginePosition(st, plan.when + 10, 0, dur)
  assert.ok(Math.abs(later - 52.137) < 1e-9)
  assert.ok(Math.abs(carryPosition(later, dur) - 52.137) < 1e-9)
  // And again, any number of times.
  let pos = 5
  for (let i = 0; i < 50; i++) {
    const pl = startPlan(i * 3, pos, dur)
    pos = carryPosition(enginePosition({ offset: pl.offset, startedAt: pl.when, playing: true }, pl.when + 1, 0, dur), dur)
  }
  assert.ok(Math.abs(pos - 55) < 1e-6, `fifty switches a second apart moved it ${pos - 5} seconds`)

  assert.equal(carryPosition(NaN, dur), 0)
  assert.equal(carryPosition(-1, dur), 0)
  assert.equal(carryPosition(dur, dur), dur - END_GUARD_SEC)
  assert.equal(carryPosition(12, 0), 12, 'a length not known yet')
})

test('a multitrack file\'s tracks: the format\'s custom, unless it names them', () => {
  assert.equal(multitrackPlan(['a', 'b']), null, 'one or two tracks is not a stem file')
  assert.deepEqual(multitrackPlan(['', '', '', '', '']), { mix: 0, parts: { drums: 1, bass: 2, other: 3, vocals: 4 } })
  assert.deepEqual(multitrackPlan(['Mix', 'Vocals', 'Drums', 'Bass', 'Synths']), { mix: 0, parts: { vocals: 1, drums: 2, bass: 3, other: 4 } })
  assert.deepEqual(multitrackPlan(['Master', 'Beat', 'Low end bass', 'Keys', 'Lead voice']), { mix: 0, parts: { drums: 1, bass: 2, other: 3, vocals: 4 } })
})

test('a multitrack file\'s parts are on or off', () => {
  const plan = multitrackPlan(['', '', '', '', ''])
  const all = { vocals: 1, instrumental: 0, drums: 1, bass: 1, other: 1 }
  assert.deepEqual(tracksEnabled(plan, all, 5), [true, false, false, false, false], 'everything on is the whole mix')
  assert.deepEqual(tracksEnabled(plan, { ...all, vocals: 0 }, 5), [false, true, true, true, false], 'voice off')
  assert.deepEqual(tracksEnabled(plan, { ...all, vocals: 0, drums: 0.2 }, 5), [false, true, true, true, false], 'part-way is on')
  assert.deepEqual(tracksEnabled(plan, { ...all, drums: 0 }, 5), [false, false, true, true, true])
})

test('why the server can\'t make stems is said in words', () => {
  assert.match(reasonWords('not-configured', 'external'), /No stem separator is set up/)
  assert.match(reasonWords('no-ffmpeg', 'approximate'), /FFmpeg/)
  assert.match(reasonWords('admin-only', 'external'), /administrator/)
  assert.match(reasonWords(undefined, 'approximate'), /can't make one/)
  assert.match(reasonWords('something-new', 'external'), /can't separate/)
})
