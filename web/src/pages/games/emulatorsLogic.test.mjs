// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Standalone emulators, the parts with no page in them. Run with:
//   node --experimental-strip-types --test web/src/pages/games/emulatorsLogic.test.mjs
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { apiPath, changedValues, fmtBytes, hostOf, inheritedValue, jobProgress, mayEdit, numberFrom, placeholderOf, requiresWords, scopesFor, sinceWords, wholeSettings, withMapEntry } from './emulatorsLogic.ts'

test('a session’s addresses lose /api, which the client adds itself', () => {
  assert.equal(apiPath('/api/v1/modules/wittyplay/docker/sessions/abc/connect'), '/v1/modules/wittyplay/docker/sessions/abc/connect')
  assert.equal(apiPath('/api/v1/modules/wittyplay/docker/sessions/abc'), '/v1/modules/wittyplay/docker/sessions/abc')
  for (const bad of [undefined, '', 'https://elsewhere.example/api/v1/x', '//elsewhere.example/api/', '/apix/v1', '/other/api/', '/api/\\\\elsewhere']) assert.equal(apiPath(bad), null, String(bad))
})

test('sizes read as people say them', () => {
  assert.equal(fmtBytes(0), '0 bytes')
  assert.equal(fmtBytes(4200), '4 kB')
  assert.equal(fmtBytes(120_400_000), '120 MB')
  assert.equal(fmtBytes(2_340_000_000), '2.3 GB')
  assert.equal(fmtBytes(-1), '')
})

test('how long a game has been running', () => {
  const now = Date.parse('2026-11-11T12:00:00Z')
  assert.equal(sinceWords('2026-11-11T11:59:40Z', now), 'Just now')
  assert.equal(sinceWords('2026-11-11T11:18:00Z', now), '42 min')
  assert.equal(sinceWords('2026-11-11T10:00:00Z', now), '2 h')
  assert.equal(sinceWords('2026-11-11T09:55:00Z', now), '2 h 5 min')
  assert.equal(sinceWords('2026-11-08T09:55:00Z', now), '3 days')
  assert.equal(sinceWords('not a time', now), '')
  // A server clock a little ahead of this one isn't a time in the future.
  assert.equal(sinceWords('2026-11-11T12:00:30Z', now), 'Just now')
})

test('a download says how far along it is', () => {
  const p = jobProgress({ phase: 'downloading', got: 250_000_000, total: 1_000_000_000 })
  assert.equal(p.value, 250_000_000)
  assert.equal(p.max, 1_000_000_000)
  assert.equal(p.text, 'Downloading · 250 MB of 1.0 GB')
})

test('a download carried on from an earlier try says so', () => {
  assert.equal(jobProgress({ phase: 'downloading', got: 600_000_000, total: 1_000_000_000, resumed: 400_000_000 }).text, 'Downloading · 600 MB of 1.0 GB · carried on from 400 MB')
})

test('a download of unknown size has no bar to fill, and still says what has come', () => {
  const p = jobProgress({ phase: 'downloading', got: 5_000_000, total: -1 })
  assert.equal(p.value, undefined)
  assert.equal(p.text, 'Downloading · 5 MB')
  assert.equal(jobProgress({ phase: 'downloading', got: 0, total: -1 }).text, 'Downloading')
})

test('more arriving than was promised never overfills the bar', () => {
  const p = jobProgress({ phase: 'downloading', got: 12, total: 10 })
  assert.equal(p.value, 10)
})

test('the steps after the download have no count', () => {
  assert.deepEqual(jobProgress({ phase: 'verifying', got: 10, total: 10 }), { max: 1, text: 'Checking the download' })
  assert.equal(jobProgress({ phase: 'unpacking', got: 10, total: 10 }).text, 'Unpacking')
  assert.equal(jobProgress({ phase: 'something-new', got: 0, total: 0 }).text, 'Working')
})

test('what an emulator needs, in a sentence', () => {
  assert.equal(requiresWords(undefined), '')
  assert.equal(requiresWords({}), '')
  assert.equal(requiresWords({ memoryMB: 4096 }), 'Needs 4 GB of memory')
  assert.equal(requiresWords({ memoryMB: 512, cpus: 1 }), 'Needs 512 MB of memory and 1 processor core')
  assert.equal(requiresWords({ memoryMB: 6144, cpus: 4, gpu: true }), 'Needs 6 GB of memory, 4 processor cores and a graphics card')
})

test('where a download comes from', () => {
  assert.equal(hostOf('https://www.pcsx2.net/downloads'), 'pcsx2.net')
  assert.equal(hostOf('https://github.com/dolphin-emu/dolphin'), 'github.com')
  assert.equal(hostOf('javascript:alert(1)'), '')
  assert.equal(hostOf(''), '')
})

test('a number field: nothing typed is no value', () => {
  assert.equal(numberFrom(''), undefined)
  assert.equal(numberFrom('  '), undefined)
  assert.equal(numberFrom('2048'), 2048)
  assert.equal(numberFrom('1.5'), 1.5)
  assert.equal(numberFrom('0'), 0)
  assert.equal(numberFrom('lots'), undefined)
})

test('a default shows as an empty field’s faint text', () => {
  assert.equal(placeholderOf({ kind: 'number' }, 2048), '2048')
  assert.equal(placeholderOf({ kind: 'number' }, 0), '0')
  assert.equal(placeholderOf({ kind: 'text' }, ''), '')
  assert.equal(placeholderOf({ kind: 'toggle' }, false), 'Off')
  assert.equal(placeholderOf({ kind: 'choice', choices: [{ value: 'docker+webrtc', label: 'Stream' }] }, 'docker+webrtc'), 'Stream')
  assert.equal(placeholderOf({ kind: 'choice' }, 'unknown'), 'unknown')
  assert.equal(placeholderOf({ kind: 'map' }, {}), '')
})

test('a map entry that is emptied leaves the map', () => {
  assert.deepEqual(withMapEntry({ pcsx2: 'a' }, 'dolphin', 'b'), { pcsx2: 'a', dolphin: 'b' })
  assert.deepEqual(withMapEntry({ pcsx2: 'a', dolphin: 'b' }, 'pcsx2', ''), { dolphin: 'b' })
  assert.deepEqual(withMapEntry(undefined, 'DRI', true), { DRI: true })
  // Off is a value: a graphics card switched off stays in the map as false.
  assert.deepEqual(withMapEntry({ DRI: true }, 'DRI', false), { DRI: false })
  assert.deepEqual(withMapEntry('nonsense', 'a', 'b'), { a: 'b' })
})

const backend = [
  { key: 'docker.enabled', kind: 'toggle', label: 'On', default: false, scope: 'admin' },
  { key: 'docker.memoryMB', kind: 'number', label: 'Memory', default: 2048, min: 256, scope: 'admin' },
  { key: 'docker.gpu', kind: 'map', label: 'Graphics', default: {}, mapOf: 'toggle', scope: 'admin' },
]

test('the backend’s settings are saved whole', () => {
  const saved = { 'docker.enabled': false, 'docker.memoryMB': 4096, 'docker.gpu': { DRI: true }, 'docker.later': 'kept' }
  // Nothing changed: what came is what goes back, a key this page doesn't know about included.
  assert.deepEqual(wholeSettings(backend, saved, {}), saved)
  assert.deepEqual(wholeSettings(backend, saved, { 'docker.enabled': true }), { ...saved, 'docker.enabled': true })
  // A number field emptied goes back to its default, which is what its faint text showed.
  assert.equal(wholeSettings(backend, saved, { 'docker.memoryMB': undefined })['docker.memoryMB'], 2048)
  // A setting the server described and didn't send still goes, as its default.
  assert.deepEqual(wholeSettings(backend, {}, {}), { 'docker.enabled': false, 'docker.memoryMB': 2048, 'docker.gpu': {} })
})

const perEmulator = [
  { key: 'resolution', kind: 'choice', label: 'Resolution', default: '2x', scope: 'user' },
  { key: 'widescreen', kind: 'toggle', label: 'Widescreen', default: false, scope: 'user' },
  { key: 'threads', kind: 'number', label: 'Threads', default: 2, scope: 'admin' },
]

test('what applies under each level', () => {
  const view = { admin: { resolution: '3x' }, user: { widescreen: true } }
  const [resolution, widescreen] = perEmulator
  assert.equal(inheritedValue(resolution, 'admin', view), '2x')
  assert.equal(inheritedValue(resolution, 'user', view), '3x')
  assert.equal(inheritedValue(resolution, 'game', view), '3x')
  assert.equal(inheritedValue(widescreen, 'user', view), false)
  assert.equal(inheritedValue(widescreen, 'game', view), true)
})

test('an administrator’s settings are read-only to everyone at their own levels', () => {
  const [resolution, , threads] = perEmulator
  assert.equal(mayEdit(threads, 'admin', true), true)
  assert.equal(mayEdit(threads, 'admin', false), false)
  assert.equal(mayEdit(threads, 'user', false), false)
  assert.equal(mayEdit(threads, 'user', true), false)
  assert.equal(mayEdit(threads, 'game', true), false)
  assert.equal(mayEdit(resolution, 'user', false), true)
  assert.equal(mayEdit(resolution, 'game', false), true)
  assert.equal(mayEdit(resolution, 'admin', true), true)
})

test('only what changed is sent, and a cleared setting goes as null', () => {
  const saved = { resolution: '3x', widescreen: true }
  assert.deepEqual(changedValues(perEmulator, saved, { ...saved }), {})
  assert.deepEqual(changedValues(perEmulator, saved, { resolution: '4x', widescreen: true }), { resolution: '4x' })
  assert.deepEqual(changedValues(perEmulator, saved, { widescreen: true }), { resolution: null })
  assert.deepEqual(changedValues(perEmulator, {}, { widescreen: false }), { widescreen: false })
  // Something in the draft the schema doesn't describe is never sent.
  assert.deepEqual(changedValues(perEmulator, {}, { other: 1 }), {})
})

test('the levels someone can open', () => {
  assert.deepEqual(scopesFor(false, true), ['user', 'game'])
  assert.deepEqual(scopesFor(false, false), ['user'])
  assert.deepEqual(scopesFor(true, true), ['admin', 'user', 'game'])
  assert.deepEqual(scopesFor(true, false, true), ['admin'])
  assert.deepEqual(scopesFor(false, false, true), [])
})
