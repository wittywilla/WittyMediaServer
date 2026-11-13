// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { CASE_STYLES, CONSOLE_MEDIA, FAMILY_VARIANTS, MAX_PREF_GAMES, MEDIA, caseFor, cleanLook, exportName, familyVariant, importDoc, isCaseStyle, lookFor, mediaFor, picturesPref, prefToDoc, readDoc, resolveLook, styleDef, stylesOf, withFamilyVariant, withLook, withMediaStyle } from './gamePictures.ts'

const schema = JSON.parse(readFileSync(new URL('../../../docs/specs/wittyplay.schema.json', import.meta.url), 'utf8'))

test('every medium offers styles WittyPlay draws, every console has a medium, and the lists are the format’s', () => {
  for (const md of MEDIA) {
    assert.ok(stylesOf(md.id).length >= 2, md.id + ' has more than one way to be shown')
    for (const st of stylesOf(md.id)) assert.ok(isCaseStyle(st), md.id + ' → ' + st)
  }
  for (const [id, md] of Object.entries(CONSOLE_MEDIA)) assert.ok(MEDIA.some((x) => x.id === md), id + ' → ' + md)
  assert.deepEqual(schema.$defs.look.properties.case.examples, ['auto', ...CASE_STYLES.map((c) => c.id)])
  assert.deepEqual(schema.$defs.look.properties.cases.propertyNames.examples, MEDIA.map((x) => x.id))
  // Every drawing a style names exists for an angled one's base, and ids are unique.
  assert.equal(new Set(CASE_STYLES.map((c) => c.id)).size, CASE_STYLES.length)
  assert.equal(styleDef('cart-nes-angled').draw, 'cart-nes')
  assert.equal(styleDef('disc-gd').draw, 'disc-bare')
  // Every kind of medium the owner listed is here.
  for (const md of ['cart-8bit', 'card', 'hucard', 'cd', 'gd', 'dvd', 'bluray', 'minidisc', 'umd', 'floppy35', 'floppy525', 'tape', 'arcade', 'digital']) assert.ok(MEDIA.some((x) => x.id === md), md)
  assert.deepEqual(stylesOf('umd').slice(0, 2), ['umd-bare', 'case-umd'])
  // Every medium can be shown as a download, and offers plenty of choice.
  for (const md of MEDIA) assert.ok(stylesOf(md.id).some((x) => x.startsWith('digital-')), md.id + ' offers a download style')
  for (const md of MEDIA) assert.ok(stylesOf(md.id).length >= 8, md.id + ' has many styles')
  assert.ok(stylesOf('cd').includes('disc-bare') && stylesOf('dvd').includes('disc-bare'))
  assert.equal(caseFor('snes'), 'cart-super')
  assert.equal(caseFor('ps2'), 'case-keep')
  assert.equal(caseFor('gc'), 'case-mini')
  assert.equal(caseFor('psp'), 'umd-bare')
  assert.equal(caseFor('ps3'), 'case-blu')
  assert.equal(caseFor('arcade'), 'arcade-marquee')
  // A console not listed goes by how the server says its games came.
  assert.equal(mediaFor('newthing', 'jewel'), 'cd')
  assert.equal(mediaFor('newthing', 'box', 'pocket'), 'cart-pocket')
  assert.equal(caseFor('newthing'), 'generic-box')
})

test('no case is named after a maker or a product', () => {
  const words = [...CASE_STYLES.map((c) => c.label + ' ' + c.note), ...MEDIA.map((x) => x.label)].join(' ')
  assert.doesNotMatch(words, /nintendo|sega|sony|playstation|xbox|atari|genesis|mega drive|famicom|game ?boy|commodore|amiga|neo ?geo|snk|nec|blu-ray|umd|dreamcast/i)
})

test('each field comes from the most particular level that sets it', () => {
  const pref = { default: { picture: 'case' }, consoles: { snes: { label: 'title' } }, games: { 7: { case: 'generic-box' } } }
  const server = { server: { default: { picture: 'filled', label: 'logo' }, consoles: { snes: { picture: 'fitted' } } }, games: { 7: { picture: 'filled' }, 8: { case: 'cart-64' } } }
  assert.deepEqual(lookFor(pref, server, '7', 'snes', 'cart-16bit'), { picture: 'filled', case: 'generic-box', label: 'title', caseChosen: true, media: 'cart-16bit' })
  // The person's console choice is under the server's choice for one game, over the server's console choice.
  assert.deepEqual(lookFor(pref, server, '8', 'snes', 'cart-16bit'), { picture: 'fitted', case: 'cart-64', label: 'title', caseChosen: true, media: 'cart-16bit' })
  assert.deepEqual(lookFor(pref, server, '9', 'nes', 'cart-8bit'), { picture: 'case', case: 'cart-nes', label: 'logo', caseChosen: false, media: 'cart-8bit' })
  assert.deepEqual(lookFor(undefined, undefined, '1', 'nes', 'cart-8bit'), { picture: 'fitted', case: 'cart-nes', label: 'boxart', caseChosen: false, media: 'cart-8bit' })
  // A style this version doesn't draw is passed over; auto stops at the medium's usual one.
  assert.equal(resolveLook([{ case: 'cart-hologram' }, { case: 'cart-64' }], 'cd').case, 'cart-64')
  assert.equal(resolveLook([{ case: 'auto' }, { case: 'cart-64' }], 'cd').case, 'case-jewel')
})

test('a style per medium: every CD bare, every cartridge at an angle; a console or a game still chooses its own', () => {
  let all = withMediaStyle({}, 'cd', 'disc-bare')
  assert.equal(resolveLook([all], 'cd').case, 'disc-bare')
  assert.equal(resolveLook([all], 'dvd').case, 'case-keep')
  all = withFamilyVariant(all, 'cart', 'angled')
  assert.equal(familyVariant(all, 'cart'), 'angled')
  assert.equal(resolveLook([all], 'cart-16bit').case, 'cart-super-angled')
  // Thin cards have no angled view: they keep their usual one.
  assert.equal(resolveLook([all], 'hucard').case, 'card-hu')
  assert.equal(resolveLook([{ case: 'case-keep' }, all], 'cd').case, 'case-keep')
  assert.equal(resolveLook([{ cases: { cd: 'case-jewel' } }, all], 'cd').case, 'case-jewel')
  const discs = withFamilyVariant({}, 'disc', 'bare')
  assert.equal(resolveLook([discs], 'umd').case, 'umd-bare')
  assert.equal(resolveLook([discs], 'minidisc').case, 'disc-mini')
  assert.equal(resolveLook([discs], 'gd').case, 'disc-gd')
  assert.equal(familyVariant(withFamilyVariant(discs, 'disc', null), 'disc'), '')
  assert.deepEqual(FAMILY_VARIANTS.disc, ['case', 'bare', 'digital'])
  assert.equal(resolveLook([withFamilyVariant({}, 'cart', 'digital')], 'cart-64').case, 'digital-tile')
  assert.deepEqual(cleanLook({ cases: { cd: '../x', dvd: 'disc-bare', 'Bad Key': 'x' } }), { cases: { dvd: 'disc-bare' } })
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
