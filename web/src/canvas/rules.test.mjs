// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { albumCanvas, canvasBlocked, canvasPlays, generateBody, parseClock, refusedWords, wantsListing } from './rules.ts'

const fine = { reducedMotion: false, saveData: false, robust: false, off: false }
const canvas = (item, v, extra = {}) => ({ available: true, url: `/api/media/${item}/canvas/file?v=${v}`, loop: true, canGenerate: false, ...extra })

test('a canvas gives way to less motion, to counted data and to a limited viewer', () => {
  assert.equal(canvasBlocked(fine), '')
  assert.equal(canvasBlocked(fine, { limited: false }), '')
  assert.equal(canvasBlocked({ ...fine, reducedMotion: true }), 'motion')
  assert.equal(canvasBlocked({ ...fine, saveData: true }), 'data')
  assert.equal(canvasBlocked({ ...fine, robust: true }), 'data')
  assert.equal(canvasBlocked(fine, { limited: true }), 'away')
  assert.equal(canvasBlocked({ ...fine, off: true }), 'off')
  // The person's own choice is the reason given when several apply.
  assert.equal(canvasBlocked({ reducedMotion: true, saveData: true, robust: true, off: true }, { limited: true }), 'off')
})

test('the listing is only asked for when it could matter', () => {
  assert.equal(wantsListing(fine, false), true)
  assert.equal(wantsListing({ ...fine, saveData: true }, false), false)
  assert.equal(wantsListing({ ...fine, reducedMotion: true }, false), false)
  // An administrator can still make or remove one from a device that won't show it.
  assert.equal(wantsListing({ ...fine, reducedMotion: true }, true), true)
})

test('the video is only loaded while it would be seen', () => {
  const on = { blocked: '', available: true, onScreen: true, tabHidden: false }
  assert.equal(canvasPlays(on), true)
  assert.equal(canvasPlays({ ...on, onScreen: false }), false)
  assert.equal(canvasPlays({ ...on, tabHidden: true }), false)
  assert.equal(canvasPlays({ ...on, available: false }), false)
  assert.equal(canvasPlays({ ...on, blocked: 'data' }), false)
  assert.equal(canvasPlays({ ...on, paused: true }), false)
  assert.equal(canvasPlays({ ...on, paused: false }), true)
})

test('an album\'s canvas is the one most of its songs share', () => {
  assert.equal(albumCanvas([]), null)
  assert.equal(albumCanvas([null, { available: false, loop: true, canGenerate: false }]), null)
  // The first song has its own; the album's covers the other two.
  const own = canvas(6, 'own'), a = canvas(11, 'album'), b = canvas(12, 'album')
  assert.equal(albumCanvas([own, a, b]), a)
  // Nothing shared: the first.
  assert.equal(albumCanvas([own, canvas(11, 'other')]), own)
  assert.equal(albumCanvas([null, own]), own)
  // One song of the album without, the rest with the album's.
  assert.equal(albumCanvas([{ available: false, loop: true, canGenerate: false }, a, b]), a)
})

test('what an administrator asks to be cut is checked before it is sent', () => {
  assert.deepEqual(generateBody('', '', 100), { body: {}, error: '' })
  assert.deepEqual(generateBody('1:30', '6', 600), { body: { startSec: 90, durationSec: 6 }, error: '' })
  assert.deepEqual(generateBody(' 45 ', '', 600), { body: { startSec: 45 }, error: '' })
  assert.match(generateBody('abc', '', 600).error, /seconds/)
  assert.match(generateBody('700', '', 600).error, /after the end/)
  assert.match(generateBody('', '1', 600).error, /2 to 15/)
  assert.match(generateBody('', '16', 600).error, /2 to 15/)
  assert.match(generateBody('', 'x', 600).error, /2 to 15/)
  // A length the server doesn't know doesn't stop a start being asked for.
  assert.deepEqual(generateBody('700', '', 0), { body: { startSec: 700 }, error: '' })
})

test('times are read as seconds or as a clock', () => {
  assert.equal(parseClock('90'), 90)
  assert.equal(parseClock('1:30'), 90)
  assert.equal(parseClock('1:02:03'), 3723)
  assert.equal(parseClock('12.5'), 12.5)
  assert.equal(parseClock('1:75'), null)
  assert.equal(parseClock('-4'), null)
  assert.equal(parseClock('1:2:3:4'), null)
  assert.equal(parseClock(''), null)
})

test('a file that is too big or too long is said to be', () => {
  assert.match(refusedWords('too-big'), /64 MB/)
  assert.match(refusedWords('too-long'), /minute/)
  assert.equal(refusedWords(undefined), '')
})
