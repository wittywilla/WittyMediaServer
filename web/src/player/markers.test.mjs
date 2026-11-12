// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { anyByHand, autoSkipPrefs, autoSkipStart, autoSkipStep, clockSeconds, clockText, markerForm, markerPut, markerSourceWords, skipAt } from './markers.ts'

const m = { recap: { startSec: 0, endSec: 40 }, intro: { startSec: 40, endSec: 100 }, credits: { startSec: 1300, endSec: 1380 }, commercials: [{ startSec: 600, endSec: 780 }] }

test('the part playing now is the one offered', () => {
  assert.deepEqual(skipAt(m, 10, 1500), { kind: 'recap', label: 'Skip recap', to: 40 })
  assert.deepEqual(skipAt(m, 60, 1500), { kind: 'intro', label: 'Skip intro', to: 100 })
  assert.equal(skipAt(m, 700, 1500)?.kind, 'commercial')
  assert.equal(skipAt(m, 1310, 1500)?.to, 1380)
  assert.equal(skipAt(m, 300, 1500), null)
})

test('nothing is offered for the last second of a part, or with no markers', () => {
  assert.equal(skipAt(m, 99.5, 1500), null)
  assert.equal(skipAt(null, 10, 1500), null)
  assert.equal(skipAt({ intro: { startSec: 50, endSec: 50 } }, 50, 1500), null)
})

test('credits that run to the end are left to "next episode"', () => {
  assert.equal(skipAt({ credits: { startSec: 1300, endSec: 1500 } }, 1310, 1500), null)
  assert.equal(skipAt({ credits: { startSec: 1300, endSec: 1380 } }, 1310, 0), null)
})

// ---------------------------------------------------------------- skipping by itself

const ALL = { intro: true, recap: true, credits: true, adverts: true }
const HOME = { duration: 1500, hasNext: false, party: 'none' }

// play feeds positions one after another, as the player does, and lists what was done.
function play(positions, markers = m, prefs = ALL, where = HOME, state = autoSkipStart()) {
  const acts = []
  for (const p of positions) {
    const r = autoSkipStep(state, markers, prefs, p, where)
    state = r.state
    if (r.act) acts.push(r.act)
  }
  return { acts, state }
}

test('the stored choice is off unless it says on', () => {
  assert.deepEqual(autoSkipPrefs(undefined), { intro: false, recap: false, credits: false, adverts: false })
  assert.deepEqual(autoSkipPrefs({ intro: true, recap: 'yes', credits: 1, adverts: true }), { intro: true, recap: false, credits: false, adverts: true })
  assert.deepEqual(autoSkipPrefs('nonsense'), { intro: false, recap: false, credits: false, adverts: false })
})

test('playing into a part skips it, once, and only the kinds asked for', () => {
  const intro = { intro: { startSec: 5, endSec: 30 } }
  const { acts, state } = play([3, 4, 4.8, 5.1], intro)
  assert.deepEqual(acts, [{ kind: 'intro', key: 'intro:5:30', label: 'Skipped intro', from: 5.1, to: 30, next: false }])
  assert.equal(state.last, 30)
  assert.deepEqual(play([3, 4, 5.1, 6], intro, { ...ALL, intro: false }).acts, [])
  assert.deepEqual(play([3, 4, 5.1, 6], intro, autoSkipPrefs(null)).acts, [])
  assert.deepEqual(play([3, 4, 5.1], null).acts, [])
})

test('starting inside a part skips it, and so does a part that follows straight on', () => {
  // The recap from the very start, then the intro it lands in.
  const { acts } = play([0.2, 40, 40.3])
  assert.deepEqual(acts.map((a) => [a.kind, a.to]), [['recap', 40], ['intro', 100]])
  // Resuming in the middle of the intro.
  assert.equal(play([60]).acts[0].to, 100)
})

test('after an undo the part plays, and is never skipped again', () => {
  const first = play([38, 39.5, 40.2], { intro: m.intro })
  assert.equal(first.acts.length, 1)
  // Undo seeks back to where it was; the picture then plays through the intro.
  const after = play([40.2, 40.5, 41, 60, 99, 101], { intro: m.intro }, ALL, HOME, first.state)
  assert.deepEqual(after.acts, [])
  // Even when someone later seeks to before it and plays into it again.
  assert.deepEqual(play([20, 21, 39, 40.5, 41], { intro: m.intro }, ALL, HOME, after.state).acts, [])
})

test('seeking back into a part plays it; seeking forward into one skips it', () => {
  // Past the intro, then back into the middle of it.
  const back = play([200, 200.3, 60, 60.3, 60.6, 80], { intro: m.intro })
  assert.deepEqual(back.acts, [])
  assert.deepEqual(back.state.spent, ['intro:40:100'])
  // From before it, forward into it: skipped once playing carries on there.
  const forward = play([0.5, 1, 60, 60.3], { intro: m.intro })
  assert.deepEqual(forward.acts.map((a) => a.from), [60.3])
  // The same position seen twice is not playing on.
  assert.deepEqual(play([0.5, 1, 60, 60, 60], { intro: m.intro }).acts, [])
  // One position that makes no sense (a stream being swapped) is not acted on.
  assert.deepEqual(play([200, 200.3, 700, 200.6, 200.9]).acts, [])
})

test('adverts are skipped break by break', () => {
  const tv = { commercials: [{ startSec: 600, endSec: 780 }, { startSec: 1000, endSec: 1100 }] }
  const { acts } = play([599, 600.2, 780, 780.3, 999.8, 1000.1], tv)
  assert.deepEqual(acts.map((a) => [a.kind, a.label, a.to]), [['commercial', 'Skipped adverts', 780], ['commercial', 'Skipped adverts', 1100]])
})

test('credits go on to the next episode, or are skipped when something follows them', () => {
  const next = play([1299, 1300.2, 1300.5], m, ALL, { ...HOME, hasNext: true })
  assert.deepEqual(next.acts.map((a) => [a.kind, a.next]), [['credits', true]])
  // No next episode: skipped to what comes after.
  assert.deepEqual(play([1299, 1300.2]).acts.map((a) => [a.next, a.to]), [[false, 1380]])
  // Credits that run to the end of a film with nothing after it are left alone.
  assert.deepEqual(play([1299, 1300.2, 1301], { credits: { startSec: 1300, endSec: 1500 } }).acts, [])
  // With a next episode they are not: that is what going on to it is for.
  assert.equal(play([1299, 1300.2], { credits: { startSec: 1300, endSec: 1500 } }, ALL, { ...HOME, hasNext: true }).acts[0].next, true)
})

test('in a watch party only someone who may control it skips, and the room chooses what is next', () => {
  assert.deepEqual(play([38, 39.5, 40.2], { intro: m.intro }, ALL, { ...HOME, party: 'follow' }).acts, [])
  assert.equal(play([38, 39.5, 40.2], { intro: m.intro }, ALL, { ...HOME, party: 'control' }).acts.length, 1)
  assert.deepEqual(play([1299, 1300.2, 1301], m, ALL, { ...HOME, hasNext: true, party: 'control' }).acts, [])
  // Someone else's undo arrives as a seek back: this player doesn't skip it again.
  const guest = play([38, 39.5, 40.2, 100, 100.3, 40.3, 40.6, 41, 50], { intro: m.intro }, ALL, { ...HOME, party: 'control' })
  assert.equal(guest.acts.length, 1)
})

test('fast playback still counts as playing', () => {
  const fast = { ...HOME, maxStep: 24 }
  assert.equal(play([20, 24, 28, 32, 36, 40.5], { intro: m.intro }, ALL, fast).acts.length, 1)
})

// ---------------------------------------------------------------- setting them by hand

test('times are written and read as minutes and seconds', () => {
  assert.equal(clockText(65), '1:05')
  assert.equal(clockText(151.5), '2:31.5')
  assert.equal(clockText(3723), '1:02:03')
  assert.equal(clockSeconds('1:05'), 65)
  assert.equal(clockSeconds('2:31.5'), 151.5)
  assert.equal(clockSeconds('90'), 90)
  assert.equal(clockSeconds(' '), null)
  assert.ok(Number.isNaN(clockSeconds('soon')))
})

const fromFiles = { intro: { startSec: 5, endSec: 30 }, recap: null, credits: { startSec: 130, endSec: 150 }, commercials: [{ startSec: 60, endSec: 70 }], source: 'edl', sources: { intro: 'chapters', credits: 'chapters', commercials: 'edl' } }

test('the form shows every range, and says where each came from', () => {
  const f = markerForm(fromFiles)
  assert.deepEqual(f, { intro: { start: '0:05', end: '0:30' }, recap: { start: '', end: '' }, credits: { start: '2:10', end: '2:30' }, commercials: [{ start: '1:00', end: '1:10' }] })
  assert.equal(markerSourceWords(fromFiles, 'intro'), 'From the file’s chapters')
  assert.equal(markerSourceWords(fromFiles, 'commercials'), 'From an .edl beside the file')
  assert.equal(markerSourceWords(fromFiles, 'recap'), '')
  assert.equal(anyByHand(fromFiles), false)
  assert.deepEqual(markerForm(null), { intro: { start: '', end: '' }, recap: { start: '', end: '' }, credits: { start: '', end: '' }, commercials: [] })
})

test('only what was changed, or was already set by hand, is sent', () => {
  const f = markerForm(fromFiles)
  // One change: the rest keeps following the files.
  assert.deepEqual(markerPut(fromFiles, { ...f, recap: { start: '', end: '0:04' } }).body, { recap: { start: 0, end: 4 } })
  // Something already set by hand is sent again, as the server keeps the hand-set ones whole.
  const hand = { ...fromFiles, intro: { startSec: 8, endSec: 25 }, sources: { ...fromFiles.sources, intro: 'manual' } }
  assert.equal(anyByHand(hand), true)
  assert.deepEqual(markerPut(hand, { ...markerForm(hand), credits: { start: '2:00', end: '' } }).body, { intro: { start: 8, end: 25 }, credits: { start: 120 } })
  // Emptying the only hand-set one goes back to what the files say.
  assert.deepEqual(markerPut(hand, { ...markerForm(hand), intro: { start: '', end: '' } }).body, { reset: true })
  // Advert breaks are sent whole, in order, without the empty rows.
  assert.deepEqual(markerPut(fromFiles, { ...f, commercials: [{ start: '5:00', end: '6:00' }, { start: '', end: '' }, { start: '1:00', end: '1:10' }] }).body, { commercials: [{ start: 60, end: 70 }, { start: 300, end: 360 }] })
})

test('times that make no sense are refused in words', () => {
  const f = markerForm(fromFiles)
  assert.match(markerPut(fromFiles, { ...f, intro: { start: '0:30', end: '0:10' } }).error, /after its start/)
  assert.match(markerPut(fromFiles, { ...f, recap: { start: '0:10', end: '' } }).error, /where the recap ends/)
  assert.match(markerPut(fromFiles, { ...f, intro: { start: 'x', end: '1:00' } }).error, /minutes and seconds/)
  assert.match(markerPut(fromFiles, { ...f, commercials: [{ start: '2:00', end: '1:00' }] }).error, /advert break/)
})
