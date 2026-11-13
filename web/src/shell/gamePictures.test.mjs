// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { CASE_STYLES, CONSOLE_CASE, MAX_PREF_GAMES, caseFor, cleanLook, exportName, importDoc, isCaseStyle, lookFor, picturesPref, prefToDoc, readDoc, resolveLook, withLook } from './gamePictures.ts'

test('every console names a case WittyPlay draws, and the list is the format’s', () => {
  for (const [id, c] of Object.entries(CONSOLE_CASE)) assert.ok(isCaseStyle(c), id + ' → ' + c)
  const schema = JSON.parse(readFileSync(new URL('../../../docs/specs/wittyplay.schema.json', import.meta.url), 'utf8'))
  assert.deepEqual(schema.$defs.look.properties.case.examples, ['auto', ...CASE_STYLES.map((c) => c.id)])
  assert.equal(caseFor('snes'), 'cart-super')
  assert.equal(caseFor('ps2'), 'case-keep')
  assert.equal(caseFor('gc'), 'case-mini')
  assert.equal(caseFor('psp'), 'case-umd')
  assert.equal(caseFor('arcade'), 'arcade-marquee')
  // A console not listed goes by how the server says its games came.
  assert.equal(caseFor('newthing', 'jewel'), 'case-jewel')
  assert.equal(caseFor('newthing', 'box', 'pocket'), 'cart-pocket')
  assert.equal(caseFor('newthing'), 'generic-box')
})

test('no case is named after a maker or a product', () => {
  const words = CASE_STYLES.map((c) => c.label + ' ' + c.note).join(' ')
  assert.doesNotMatch(words, /nintendo|sega|sony|playstation|xbox|atari|genesis|mega drive|famicom|game ?boy|commodore|amiga|neo ?geo|snk|nec/i)
})

test('each field comes from the most particular level that sets it', () => {
  const pref = { default: { picture: 'case' }, consoles: { snes: { label: 'title' } }, games: { 7: { case: 'generic-box' } } }
  const server = { server: { default: { picture: 'filled', label: 'logo' }, consoles: { snes: { picture: 'fitted' } } }, games: { 7: { picture: 'filled' }, 8: { case: 'cart-64' } } }
  assert.deepEqual(lookFor(pref, server, '7', 'snes', 'cart-super'), { picture: 'filled', case: 'generic-box', label: 'title', caseChosen: true })
  // The person's console choice is under the server's choice for one game, over the server's console choice.
  assert.deepEqual(lookFor(pref, server, '8', 'snes', 'cart-super'), { picture: 'fitted', case: 'cart-64', label: 'title', caseChosen: true })
  assert.deepEqual(lookFor(pref, server, '9', 'nes', 'cart-nes'), { picture: 'case', case: 'cart-nes', label: 'logo', caseChosen: false })
  assert.deepEqual(lookFor(undefined, undefined, '1', 'nes', 'cart-nes'), { picture: 'fitted', case: 'cart-nes', label: 'boxart', caseChosen: false })
  // A case this version doesn't draw, or auto, is the console's own.
  assert.equal(resolveLook([{ case: 'cart-hologram' }], 'case-jewel').case, 'case-jewel')
  assert.equal(resolveLook([{ case: 'auto' }, { case: 'cart-64' }], 'case-jewel').case, 'case-jewel')
})

test('what /me/ui holds is read safely, and nonsense is left out', () => {
  assert.deepEqual(picturesPref(null), {})
  assert.deepEqual(picturesPref({ default: { picture: 'sideways', label: 'title' }, games: { 1: { picture: 7 }, 2: { picture: 'case' } }, consoles: [] }),
    { default: { label: 'title' }, consoles: {}, games: { 2: { picture: 'case' } } })
  assert.deepEqual(cleanLook({ case: '../x' }), {})
})

test('a person keeps their own look for a limited number of games, the oldest going first', () => {
  let p = {}
  for (let i = 1; i <= MAX_PREF_GAMES + 5; i++) p = withLook(p, { game: String(i) }, { picture: 'case' })
  assert.equal(Object.keys(p.games).length, MAX_PREF_GAMES)
  assert.ok(!p.games['1'] && p.games[String(MAX_PREF_GAMES + 5)])
  p = withLook(p, { game: '200' }, null)
  assert.ok(!p.games['200'])
  p = withLook(p, { console: 'nes' }, { case: 'cart-nes' })
  assert.deepEqual(p.consoles, { nes: { case: 'cart-nes' } })
  // Within the 16 KB /me/ui holds, with room for everything else.
  assert.ok(JSON.stringify(p).length < 8 * 1024, 'size ' + JSON.stringify(p).length)
})

test('a person’s choices go out as a .wittyplay file and come back in', () => {
  const pref = { default: { picture: 'case' }, consoles: { snes: { case: 'cart-super' } }, games: { 12: { label: 'title' } } }
  const doc = prefToDoc(pref, { name: 'Mine', games: { 12: { title: 'Made Up Quest', console: 'snes', file: 'Made Up Quest (USA).sfc' } } })
  assert.deepEqual(doc, { wittyplay: 1, name: 'Mine', default: { picture: 'case' }, consoles: { snes: { case: 'cart-super' } },
    games: [{ match: { item: 12, file: 'Made Up Quest (USA).sfc', title: 'Made Up Quest', console: 'snes' }, label: 'title' }] })
  const read = readDoc(JSON.stringify(doc))
  assert.ok('doc' in read)
  const back = importDoc({}, read.doc, [[12]])
  assert.equal(back.games, 1)
  assert.deepEqual(back.pref.games, { 12: { label: 'title' } })
  assert.deepEqual(back.pref.default, { picture: 'case' })
  const none = importDoc({ default: { label: 'logo' } }, read.doc, [[]])
  assert.equal(none.unmatched, 1)
  assert.deepEqual(none.pref.default, { label: 'logo', picture: 'case' })
})

test('files that aren’t .wittyplay are refused with words a person understands', () => {
  for (const [text, words] of [['nope', /isn’t JSON/], ['[1]', /isn’t a .wittyplay/], ['{"wittyplay":"1"}', /doesn’t start/], ['{"wittyplay":2}', /newer/], ['{"wittyplay":1,"x":"' + 'y'.repeat(70000) + '"}', /too big/]]) {
    const r = readDoc(text)
    assert.ok('error' in r && words.test(r.error), text.slice(0, 30))
  }
  assert.ok('doc' in readDoc('﻿{"wittyplay":1}'))
  assert.equal(exportName('Living room / TV'), 'Living room  TV.wittyplay')
  assert.equal(exportName('///'), 'game-pictures.wittyplay')
})
