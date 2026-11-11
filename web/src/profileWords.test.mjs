// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { emptyHomeWords, emptyLibraryWords, isGone, lookedAfter, settingsPageHidden } from './profileWords.ts'

const owner = { isAdmin: true, kind: 'adult' }, adult = { isAdmin: false, kind: 'adult' }, kid = { isAdmin: false, kind: 'kids' }, guest = { isAdmin: false, kind: 'guest' }
const ADMIN_WORDS = /settings|folder|scan again|organi[sz]e/i

test('only the owner is told how to fill a library', () => {
  const advice = 'No videos were found in this library’s folders.'
  assert.equal(emptyLibraryWords(owner, advice), advice)
  for (const who of [adult, kid, guest, null, undefined]) {
    assert.notEqual(emptyLibraryWords(who, advice), advice)
    assert.doesNotMatch(emptyLibraryWords(who, advice), ADMIN_WORDS)
  }
  assert.match(emptyLibraryWords(kid, advice), /grown-up/)
  assert.match(emptyLibraryWords(guest, advice), /grown-up/)
  assert.doesNotMatch(emptyLibraryWords(adult, advice), /grown-up/)
})

test('an empty home page sends only the owner to Settings', () => {
  assert.match(emptyHomeWords(owner, false).text, /Settings, Libraries/)
  for (const who of [adult, kid, guest, null]) {
    const w = emptyHomeWords(who, false)
    assert.doesNotMatch(w.title + ' ' + w.text, ADMIN_WORDS)
    assert.equal(w.title, 'Nothing to watch yet')
  }
  assert.match(emptyHomeWords(kid, false).text, /grown-up/)
  // While a scan runs everyone is told to wait, and nobody is told the libraries are theirs but the owner.
  assert.equal(emptyHomeWords(owner, true).title, 'Your libraries are being scanned')
  assert.equal(emptyHomeWords(kid, true).title, 'The libraries are being scanned')
  assert.equal(emptyHomeWords(kid, true).text, emptyHomeWords(owner, true).text)
})

test('Kids and Guest profiles are looked after; adults and the owner are not', () => {
  assert.equal(lookedAfter(kid), true)
  assert.equal(lookedAfter(guest), true)
  assert.equal(lookedAfter(adult), false)
  assert.equal(lookedAfter(owner), false)
  assert.equal(lookedAfter({ isAdmin: true, kind: 'kids' }), false)
  assert.equal(lookedAfter(null), false)
})

test('Settings leaves out the pages with nothing in them for a Kids or Guest profile', () => {
  for (const page of ['activity', 'sharing']) {
    assert.equal(settingsPageHidden(page, kid), true, page)
    assert.equal(settingsPageHidden(page, guest), true, page)
    assert.equal(settingsPageHidden(page, adult), false, page)
    assert.equal(settingsPageHidden(page, owner), false, page)
  }
  for (const page of ['users', 'security', 'appearance', 'games', 'about', 'legal']) assert.equal(settingsPageHidden(page, kid), false, page)
  // Game settings go when games are turned off for the profile, whoever it is; not before that is known.
  assert.equal(settingsPageHidden('games', kid, { games: false }), true)
  assert.equal(settingsPageHidden('games', adult, { games: false }), true)
  assert.equal(settingsPageHidden('games', kid, { games: true }), false)
  assert.equal(settingsPageHidden('games', kid, null), false)
  assert.equal(settingsPageHidden('appearance', kid, { games: false }), false)
})

test('a title that is gone and one that is off limits read the same', () => {
  assert.equal(isGone('Show not found.'), true)
  assert.equal(isGone('Item not found.'), true)
  assert.equal(isGone('Can’t reach your server.'), false)
})
