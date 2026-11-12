// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/remote/playerStates.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { audioPlayerKind, audioState, bookState, discState, gameState, photoState, tvState } from './playerStates.ts'

const sound = { volume: 0.7, muted: false }
const music = (over = {}) => ({ queue: [{ id: 6 }, { id: 11 }, { id: 12 }], index: 1, current: { id: 11 }, playing: true, retrying: false, duration: 180, rate: 1.25, sound, live: false, cdActive: false, ...over })

test('the audio player is music, a station or a CD', () => {
  assert.equal(audioPlayerKind({ cdActive: false, live: false }), 'audio')
  assert.equal(audioPlayerKind({ cdActive: false, live: true }), 'radio')
  assert.equal(audioPlayerKind({ cdActive: true, live: false }), 'cd')
})

test('music reports its own queue, place, speed and volume', () => {
  assert.deepEqual(audioState(music(), 42.5, null), {
    status: 'playing', itemId: 11, volume: 0.7, muted: false, rate: 1.25, screen: 'music', positionSec: 42.5, durationSec: 180, queue: [6, 11, 12], queueIndex: 1,
  })
  assert.equal(audioState(music({ playing: false }), 0, null).status, 'paused')
  assert.equal(audioState(music({ playing: false, retrying: true }), 0, null).status, 'buffering')
  assert.deepEqual(audioState(music({ current: null, queue: [] }), 0, null), { status: 'idle' })
  // One song is not a queue.
  assert.equal(audioState(music({ queue: [{ id: 11 }], index: 0 }), 0, null).queue, undefined)
})

test('a station has no place, no length and one speed', () => {
  const st = audioState(music({ live: true, rate: 1.5, duration: 0 }), 999, null)
  assert.equal(st.positionSec, undefined)
  assert.equal(st.durationSec, undefined)
  assert.equal(st.rate, 1)
  assert.deepEqual(st.queue, [6, 11, 12])
})

test('a CD is the disc: the track\'s place and length, no queue, one speed', () => {
  const st = audioState(music({ cdActive: true, current: { id: 40 }, queue: [{ id: 40 }, { id: 40 }], index: 0, rate: 2 }), 12, { id: 40, trackLen: 201 })
  assert.deepEqual(st, { status: 'playing', itemId: 40, volume: 0.7, muted: false, rate: 1, screen: 'music', positionSec: 12, durationSec: 201 })
})

const v = { paused: false, currentTime: 600, playbackRate: 1, volume: 0.5, muted: true }
const label = (t) => `${t.language} ${t.n}`
const title = { durationSec: 5400, audioStream: 2, subtitleStream: -1, audio: [{ n: 1, language: 'en' }, { n: 2, language: 'nl' }], subtitles: [{ n: 1, language: 'en' }] }

test('a disc\'s title reports like a film, with the disc\'s own stream numbers', () => {
  const st = discState({ itemId: 9, stopped: false, waiting: false, pres: { kind: 'title', media: { type: 'hls' }, title } }, v, label)
  assert.deepEqual(st, {
    itemId: 9, volume: 0.5, muted: true, screen: 'player', status: 'playing', positionSec: 600, durationSec: 5400, rate: 1, audio: '2', subtitle: 'off',
    audioTracks: [{ id: '1', label: 'en 1', lang: 'en' }, { id: '2', label: 'nl 2', lang: 'nl' }], subtitleTracks: [{ id: '1', label: 'en 1', lang: 'en' }],
  })
  assert.equal(discState({ itemId: 9, stopped: true, waiting: false, pres: { kind: 'title', title } }, v, label).status, 'paused')
  assert.equal(discState({ itemId: 9, stopped: false, waiting: true, pres: { kind: 'title', title } }, v, label).status, 'buffering')
  assert.equal(discState({ itemId: 9, stopped: false, waiting: false, pres: { kind: 'title', title: { ...title, subtitleStream: 1 } } }, v, label).subtitle, '1')
})

test('a disc\'s menu has no place, no length and no tracks', () => {
  const st = discState({ itemId: 9, stopped: false, waiting: false, pres: { kind: 'menu', media: { type: 'still' } } }, { ...v, paused: true }, label)
  assert.deepEqual(st, { itemId: 9, volume: 0.5, muted: true, status: 'playing', screen: 'disc-menu' })
  // A moving menu someone paused.
  assert.equal(discState({ itemId: 9, stopped: false, waiting: false, pres: { kind: 'menu', media: { type: 'hls' } } }, { ...v, paused: true }, label).status, 'paused')
  // Before the session has answered.
  assert.equal(discState({ itemId: 9, stopped: false, waiting: false, pres: null }, null, label).status, 'buffering')
})

const tv = (over = {}) => ({ now: null, positionSec: 0, serverTime: 1000, behindSec: 0, ready: true, audio: [], subtitles: [], ...over })
const set = { paused: false, volume: 1, muted: false }

test('a tuner channel is not a library item: no item is named', () => {
  const st = tvState(tv({ now: { start: 900, end: 4500 } }), set, 1010)
  assert.deepEqual(st, { status: 'playing', volume: 1, muted: false, rate: 1, screen: 'tv' })
  assert.equal(tvState(tv(), { ...set, paused: true }, 1010).status, 'paused')
  assert.equal(tvState(tv({ ready: false }), set, 1010).status, 'buffering')
  assert.equal(tvState(tv(), null, 1010).status, 'buffering')
})

test('a library channel names the item on now, and how far in it is', () => {
  const st = tvState(tv({ now: { itemId: 3, start: 900, end: 2700 }, positionSec: 100 }), set, 1010)
  assert.equal(st.itemId, 3)
  assert.equal(st.durationSec, 1800)
  assert.equal(st.positionSec, 110)
  // Paused a while ago, and so behind live.
  assert.equal(tvState(tv({ now: { itemId: 3, start: 900, end: 2700 }, positionSec: 100, behindSec: 60 }), set, 1010).positionSec, 50)
  // Never outside the programme.
  assert.equal(tvState(tv({ now: { itemId: 3, start: 900, end: 2700 }, positionSec: 100, behindSec: 600 }), set, 1010).positionSec, 0)
  assert.equal(tvState(tv({ now: { itemId: 3, start: 900, end: 2700 }, positionSec: 100 }), set, 99999).positionSec, 1800)
})

test('live TV lists the sound and subtitle tracks the stream has', () => {
  const st = tvState(tv({ audio: [{ id: 0, label: 'English' }, { id: 1, label: 'Dutch', active: true }], subtitles: [{ id: 0, label: 'English' }] }), set, 1010)
  assert.deepEqual(st.audioTracks, [{ id: '0', label: 'English' }, { id: '1', label: 'Dutch' }])
  assert.equal(st.audio, '1')
  assert.equal(st.subtitle, 'off')
  assert.equal(tvState(tv({ subtitles: [{ id: 0, label: 'English', active: true }] }), set, 1010).subtitle, '0')
})

test('a book has a place and no clock; a picture neither; a game just plays', () => {
  assert.deepEqual(bookState(5, { position: 12, total: 240 }), { status: 'paused', itemId: 5, positionSec: 12, durationSec: 240, screen: 'reader' })
  assert.deepEqual(bookState(5, null), { status: 'paused', itemId: 5, positionSec: 0, durationSec: 0, screen: 'reader' })
  assert.deepEqual(photoState(8, true), { status: 'playing', itemId: 8, screen: 'photos' })
  assert.deepEqual(photoState(8, false), { status: 'paused', itemId: 8, screen: 'photos' })
  assert.deepEqual(gameState(6), { status: 'playing', itemId: 6, screen: 'game' })
})
