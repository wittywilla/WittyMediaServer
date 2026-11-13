// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What the pop-up about a module made for an older server asks. Run with:
//   node --experimental-strip-types --test web/src/pages/settings/store/olderServer.test.mjs
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { confirmedIds, notInstalledWords, olderFromStore, olderOf, olderTitle, olderWords, series, startChoices } from './olderServer.ts'

const play = { module: 'wittyplay', name: 'WittyPlay', version: '0.3.6', madeFor: '0.6.x', server: '0.8.0-alpha',
  warning: 'WittyPlay 0.3.6 was made for WittyMediaServer 0.6; this server is 0.8.0-alpha. Pages, settings and features from 0.8 will be missing, and it may not work.' }

test('a series is the first two numbers', () => {
  assert.equal(series('0.6.x'), '0.6')
  assert.equal(series('0.8.0-alpha'), '0.8')
  assert.equal(series('1.2.3+build'), '1.2')
})

test('the server’s refusal is read, and nothing else is', () => {
  const err = { status: 409, data: { code: 'older-server', older: [play] } }
  assert.deepEqual(olderOf(err), [play])
  assert.equal(olderOf({ status: 409, data: { code: 'plan', plan: [] } }), null)
  assert.equal(olderOf({ status: 409, data: { code: 'older-server', older: [] } }), null)
  assert.equal(olderOf({ status: 500, data: { code: 'older-server', older: [play] } }), null)
  assert.equal(olderOf(new Error('no')), null)
  assert.equal(olderOf(null), null)
})

test('setup finds the chosen modules whose only version is for an older server', () => {
  const items = [
    { id: 'wittyplay', name: 'WittyPlay', official: true, version: '0.3.6', madeFor: '0.6.x', fit: { kind: 'older-server', ok: false, why: 'No version of WittyPlay for this server is published yet. ' + play.warning } },
    { id: 'wittydvd', name: 'WittyDVD', official: true, version: '0.4.0', fit: { kind: 'ok', ok: true } },
    { id: 'wittycd', name: 'WittyCD', official: true, version: '0.2.0', madeFor: '0.6.x', fit: { kind: 'older-server' }, status: { installed: true } },
    { id: 'wittyplay', name: 'WittyPlay', official: false, version: '9.9.9', fit: { kind: 'older-server' } },
  ]
  const got = olderFromStore(items, ['wittyplay', 'wittydvd', 'wittycd'], '0.8.0-alpha')
  assert.equal(got.length, 1)
  assert.equal(got[0].module, 'wittyplay')
  assert.equal(got[0].version, '0.3.6')
  assert.match(got[0].warning, /^No version of WittyPlay for this server is published yet\./)
})

test('every choice starts as Don’t install, and only Install anyway is sent', () => {
  const ch = startChoices([play, { ...play, module: 'wittyebook', name: 'WittyEbook' }])
  assert.deepEqual(ch, { wittyplay: false, wittyebook: false })
  assert.deepEqual(confirmedIds(ch), [])
  assert.deepEqual(confirmedIds({ ...ch, wittyebook: true }), ['wittyebook'])
})

test('the words name the version, the server it was made for, and what may not work', () => {
  assert.equal(olderTitle([play]), 'WittyPlay 0.3.6 was made for an older server')
  assert.equal(olderTitle([play, play]), '2 modules were made for an older server')
  const words = olderWords(play).join(' ')
  assert.match(words, /made for WittyMediaServer 0\.6; this server is 0\.8\.0-alpha/)
  assert.match(words, /may not work/)
  assert.match(olderWords({ ...play, warning: '' }).join(' '), /WittyPlay 0\.3\.6 was made for WittyMediaServer 0\.6; this server is 0\.8\.0-alpha/)
  assert.match(notInstalledWords(play), /No version of WittyPlay for this server is published yet/)
})
