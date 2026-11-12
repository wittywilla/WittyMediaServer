// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'

const { ALL_KEYS, MENU_ONLY, COLOUR_KEYS, ARROW_KEYS, discForm, videoForm, remoteLabel, keyWorks, remoteNote } = await import('./remoteKeys.ts')

const working = (form, state) => ALL_KEYS.filter((k) => keyWorks(form, k, state))
const rich = { audio: 2, subtitles: 3, angles: 2, chapters: 12 }

test('each form has its remote', () => {
  assert.equal(remoteLabel(discForm({ kind: 'dvd-iso' })), 'WittyDVD')
  assert.equal(remoteLabel(discForm({ kind: 'dvd-folder' })), 'WittyDVD')
  assert.equal(remoteLabel(discForm({ kind: 'bluray-iso' })), 'WittyBluray')
  assert.equal(remoteLabel(discForm({ kind: 'bluray-folder' })), 'WittyBluray')
  assert.equal(remoteLabel(videoForm({ discSource: 'dvd' })), 'WittyDVD')
  assert.equal(remoteLabel(videoForm({ discSource: 'bluray' })), 'WittyBluray')
  assert.equal(remoteLabel(videoForm({})), 'WittyMedia')
})

test('a DVD in a menu: every key but the colour keys', () => {
  const keys = working('dvd', { ...rich, inMenu: true })
  assert.deepEqual(ALL_KEYS.filter((k) => !keys.includes(k)), COLOUR_KEYS)
  assert.equal(remoteNote('dvd', { inMenu: true }), '')
})

test('a DVD during the film: arrows wait for a menu, digits pick chapters', () => {
  const keys = working('dvd', rich)
  for (const k of ARROW_KEYS) assert.ok(!keys.includes(k), k)
  for (const k of ['topmenu', 'menu', 'return', 'digit1', 'digit9', 'prev', 'next', 'angle', 'audio', 'subtitle']) assert.ok(keys.includes(k), k)
})

test('a still picture has nothing to play, pause or seek', () => {
  for (const k of ['play', 'pause', 'stop', 'rew', 'ff']) assert.equal(keyWorks('dvd', k, { playing: false }), false, k)
})

test('a Blu-ray Java menu takes the arrows, OK and colour keys with no buttons listed', () => {
  const keys = working('bluray', { bdj: true })
  for (const k of [...ARROW_KEYS, ...COLOUR_KEYS, 'menu', 'topmenu', 'digit5']) assert.ok(keys.includes(k), k)
  assert.equal(keyWorks('bluray', 'red', {}), false, 'colour keys are for Java menus only')
  assert.equal(keyWorks('dvd', 'red', { bdj: true }), false, 'DVDs have no colour keys')
})

test('a Blu-ray film with a pop-up menu: Menu works, arrows wait for it to open', () => {
  assert.equal(keyWorks('bluray', 'menu', { popup: true }), true)
  assert.equal(keyWorks('bluray', 'up', { popup: true }), false)
  assert.equal(keyWorks('bluray', 'up', { popup: true, inMenu: true }), true)
})

test('a disc with no menus greys out Top menu, and Menu unless the film has a pop-up', () => {
  assert.equal(keyWorks('bluray', 'topmenu', { hasMenus: false }), false)
  assert.equal(keyWorks('bluray', 'menu', { hasMenus: false }), false)
  assert.equal(keyWorks('bluray', 'menu', { hasMenus: false, popup: true }), true)
  assert.equal(remoteNote('bluray', { hasMenus: false }), 'This disc has no menus')
  assert.equal(remoteNote('bluray', { hasMenus: false, popup: true }), '')
  assert.equal(keyWorks('dvd', 'topmenu', {}), true, 'unknown counts as having menus')
})

test('a copy of a disc: playback, chapters and tracks, menu keys greyed out with a line', () => {
  for (const form of ['dvd-copy', 'bluray-copy']) {
    const keys = working(form, rich)
    for (const k of MENU_ONLY) assert.ok(!keys.includes(k), `${form} ${k}`)
    for (const k of ['eject', 'audio', 'subtitle', 'prev', 'next', 'rew', 'ff', 'play', 'pause', 'stop', 'digit1', 'digit9']) assert.ok(keys.includes(k), `${form} ${k}`)
    assert.ok(!keys.includes('digit0'), 'there is no chapter 0')
    assert.equal(remoteNote(form), 'This copy has no disc menus')
  }
  assert.equal(keyWorks('dvd-copy', 'digit4', { chapters: 3 }), false, 'only chapters the copy has')
  assert.equal(keyWorks('bluray-copy', 'digit1', {}), false, 'a copy with no chapters has no chapter digits')
})

test('one sound track or no subtitles: those keys are greyed out', () => {
  for (const form of ['dvd', 'bluray', 'dvd-copy', 'video']) {
    assert.equal(keyWorks(form, 'audio', { audio: 1 }), false)
    assert.equal(keyWorks(form, 'subtitle', { subtitles: 0 }), false)
  }
})

test('any other video: the same as a copy, without chapter digits or a line', () => {
  const keys = working('video', rich)
  for (const k of [...MENU_ONLY, 'digit1']) assert.ok(!keys.includes(k), k)
  for (const k of ['prev', 'next', 'play', 'audio']) assert.ok(keys.includes(k), k)
  assert.equal(remoteNote('video'), '')
})

test('the catalogue says which keys a kind dims, and a dimmed key does nothing', async () => {
  const { setRemoteCatalogue, catalogueDims, keyWorks } = await import('./remoteKeys.ts')
  setRemoteCatalogue({ kinds: { dvd: { dimmed: ['red', 'green', 'yellow', 'blue'] }, bluray: { dimmed: [], dimmedWhen: { oneAngle: ['angle'] } }, video: { dimmed: ['topMenu', 'menu', 'angle'] } } })
  try {
    assert.equal(catalogueDims('dvd', 'red'), true)
    assert.equal(catalogueDims('dvd', 'menu'), false)
    assert.equal(catalogueDims('bluray', 'angle', { angles: 1 }), true)
    assert.equal(catalogueDims('bluray', 'angle', { angles: 3 }), false)
    assert.equal(catalogueDims('video', 'topmenu'), true) // the catalogue's topMenu is this remote's topmenu
    assert.equal(keyWorks('dvd', 'red', { bdj: true }), false)
  } finally { setRemoteCatalogue({}) }
})
