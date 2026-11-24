// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'

const store = new Map()
globalThis.localStorage = { getItem: (k) => (store.has(k) ? store.get(k) : null), setItem: (k, v) => store.set(k, String(v)), removeItem: (k) => store.delete(k) }
const { baseLang, orderTracks, matchTrack, rememberLang, recentLangs } = await import('./trackOrder.ts')

const tr = (id, title, lang, extra = {}) => ({ id, title, lang, active: false, ...extra })

test('three-letter, old and regional codes are the same language', () => {
  assert.equal(baseLang('eng'), 'en')
  assert.equal(baseLang('ger'), 'de')
  assert.equal(baseLang('en-GB'), 'en')
  assert.equal(baseLang('pt_BR'), 'pt')
  assert.equal(baseLang('und'), '')
  assert.equal(baseLang(''), '')
})

test('Off, then the device language, then recent ones, then the rest by name', () => {
  const tracks = [tr(-1, 'Off'), tr(1, 'Spanish', 'spa'), tr(2, 'Greek', 'gre'), tr(3, 'English', 'eng'), tr(4, 'English', 'eng', { sub: 'SDH' }), tr(5, 'Danish', 'dan'), tr(6, 'Arabic', 'ara')]
  const got = orderTracks(tracks, ['en-GB', 'en'], ['es'])
  assert.deepEqual(got.map((x) => x.track.id), [-1, 3, 4, 1, 6, 5, 2])
  assert.deepEqual(got.map((x) => x.group), ['off', 'device', 'device', 'recent', 'other', 'other', 'other'])
})

test('a recent language that is also the device language is listed once, with the device', () => {
  const got = orderTracks([tr(1, 'French', 'fra'), tr(2, 'German', 'deu')], ['fr'], ['fr', 'de'])
  assert.deepEqual(got.map((x) => x.group), ['device', 'recent'])
})

test('search matches the start of any word, the code included', () => {
  const t = tr(4, 'English', 'eng', { sub: 'SDH', detail: 'Text' })
  assert.ok(matchTrack(t, 'eng'))
  assert.ok(matchTrack(t, 'sdh'))
  assert.ok(matchTrack(t, 'en sd'))
  assert.ok(!matchTrack(t, 'span'))
  assert.ok(matchTrack(t, '  '))
})

test('picks are remembered, newest first, five at most', () => {
  for (const c of ['eng', 'spa', 'fra', 'deu', 'ita', 'jpn', 'spa']) rememberLang('sub', c)
  assert.deepEqual(recentLangs('sub'), ['es', 'ja', 'it', 'de', 'fr'])
  rememberLang('sub', 'und')
  assert.equal(recentLangs('sub').length, 5)
})
