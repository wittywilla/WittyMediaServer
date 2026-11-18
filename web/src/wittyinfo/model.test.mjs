// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { changed, check, draftOf, filterAt, filterPrefs, manifestOf, markAt, moveRange, newSegment, problemWords, timeAt } from './model.ts'

const file = {
  format: 'wittyinfo', version: 1, identity: { durationSec: 1400 }, authorship: { author: 'Willow' },
  segments: [{ id: 'b', type: 'credits', start: 1300 }, { id: 'a', type: 'intro', start: 45, end: 90, action: 'auto', extra: 1 }],
  scenes: [{ title: 'Opening', start: 0 }], fromTheFuture: { x: 1 },
}

test('a file is taken apart and put back together without losing anything', () => {
  const d = draftOf(file)
  const m = manifestOf(d)
  assert.deepEqual(m.identity, file.identity)
  assert.deepEqual(m.fromTheFuture, { x: 1 })
  assert.deepEqual(m.segments.map((s) => s.id), ['a', 'b'], 'in time order')
  assert.equal(m.segments[0].extra, 1, 'unknown fields of a part stay')
  assert.equal('end' in m.segments[1], false, 'an open end stays open')
  assert.equal(changed(d, draftOf(file)), false)
  d.segments[0].start = 1299
  assert.equal(changed(d, draftOf(file)), true)
  // Empty fields aren't written.
  d.metadata.title = ''
  d.notes = '  '
  assert.equal(manifestOf(d).metadata, undefined)
  assert.equal(manifestOf(d).notes, undefined)
})

test('a new file starts from what is in force now', () => {
  const d = draftOf(null, { segments: [{ kind: 'intro', startSec: 30, endSec: 90, action: 'button', source: 'chapters' }] })
  assert.deepEqual(d.segments, [{ type: 'intro', start: 30, end: 90 }])
})

test('the server’s refusals are said before saving, in words', () => {
  const d = draftOf({ format: 'wittyinfo', version: 1, segments: [{ type: 'intro', start: 90, end: 45 }, { type: 'custom', start: 1, end: 2, label: 'x'.repeat(201) }], scenes: [{ title: 'a', start: -1 }] })
  const p = check(d)
  assert.deepEqual(p.map((x) => x.where), ['segments[0].label', 'segments[1].end', 'scenes[0].start'])
  assert.equal(problemWords(p[1], d), 'Part 2 (Intro), end: has to come after the start')
  assert.equal(check(draftOf({ format: 'wittyinfo', version: 1, scenes: [{ title: 'a', start: 1 }] }), 'show')[0].what, 'scenes belong to one video, not to a show')
})

test('ranges move inside the video and never turn inside out', () => {
  const r = { start: 10, end: 20 }
  assert.deepEqual(moveRange(r, 'start', 5, 100), { start: 15, end: 20 })
  assert.deepEqual(moveRange(r, 'start', 50, 100), { start: 19.9, end: 20 })
  assert.deepEqual(moveRange(r, 'end', -50, 100), { start: 10, end: 10.1 })
  assert.deepEqual(moveRange(r, 'both', -50, 100), { start: 0, end: 10 })
  assert.deepEqual(moveRange(r, 'both', 500, 100), { start: 90, end: 100 })
  assert.deepEqual(moveRange({ start: 10, end: null }, 'end', -5, 100), { start: 10, end: 95 })
  assert.deepEqual(markAt(r, 'end', 5), { start: 5, end: 10 }, 'an end before the start swaps them')
  assert.deepEqual(markAt(r, 'start', 12.34567), { start: 12.346, end: 20 })
})

test('new parts are a sensible length, inside the video', () => {
  assert.deepEqual(newSegment('intro', 30, 1400), { type: 'intro', start: 30, end: 90 })
  assert.deepEqual(newSegment('credits', 1350, 1400), { type: 'credits', start: 1350, end: null })
  assert.deepEqual(newSegment('custom', 1399.5, 1400), { type: 'custom', start: 1399, end: 1400 })
  assert.equal(timeAt(0.5, 1400), 700)
})

test('content filters follow the profile’s own choices', () => {
  const filters = [{ category: 'language', severity: 1, start: 10, end: 11, action: 'mute' }, { category: 'violence', severity: 3, start: 10, end: 20 }]
  assert.equal(filterAt(filters, filterPrefs(null), 10.5), null, 'nothing chosen, nothing done')
  const p = filterPrefs({ language: 'mute', violence: 'skip', nudity: 'sometimes' })
  assert.deepEqual(Object.keys(p).sort(), ['language', 'violence'])
  assert.equal(filterAt(filters, p, 10.5).act, 'skip', 'the strongest wins')
  assert.equal(filterAt(filters, { language: 'mute' }, 10.5).act, 'mute')
  assert.equal(filterAt(filters, { language: 'mute', minSeverity: 2 }, 10.5), null, 'milder than cared about')
  assert.equal(filterAt(filters, p, 20), null)
})
