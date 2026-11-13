// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/pages/games/titles.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { asGameTitlePref, gameTitle, gameTitleLabel, gameTitleParts, titleLang, GAME_TITLE_EXAMPLE } from './titles.ts'

const full = { title: 'Mega Geemu', titles: { native: 'メガゲーム', english: 'Mega Game', romanized: 'Mega Geemu' } }
const nativeOnly = { title: 'Mega Geemu', titles: { native: 'メガゲーム', romanized: 'Mega Geemu' } }
const plain = { title: 'Plain Game' }

test('the choice is one of four, and anything else is romanized', () => {
  for (const v of ['romanized', 'native', 'english', 'both']) assert.equal(asGameTitlePref(v), v)
  for (const v of [null, undefined, '', 'japanese', 3, {}]) assert.equal(asGameTitlePref(v), 'romanized')
})

test('each choice shows its name', () => {
  assert.equal(gameTitle(full, 'romanized'), 'Mega Geemu')
  assert.equal(gameTitle(full, 'native'), 'メガゲーム')
  assert.equal(gameTitle(full, 'english'), 'Mega Game')
  assert.deepEqual(gameTitleParts(full, 'both'), { main: 'メガゲーム', sub: 'Mega Game' })
  assert.equal(gameTitleLabel(full, 'both'), 'メガゲーム (Mega Game)')
})

test('a name that is not known falls back to the title', () => {
  assert.equal(gameTitle(nativeOnly, 'english'), 'Mega Geemu')
  assert.deepEqual(gameTitleParts(nativeOnly, 'both'), { main: 'メガゲーム', sub: 'Mega Geemu' })
  for (const p of ['romanized', 'native', 'english', 'both']) assert.deepEqual(gameTitleParts(plain, p), { main: 'Plain Game' })
  assert.deepEqual(gameTitleParts({ title: 'X', titles: { english: 'Ex' } }, 'both'), { main: 'Ex' })
  assert.equal(gameTitle(null, 'native'), '')
  assert.equal(gameTitle({ title: 'X', titles: null }, 'native'), 'X')
})

test('the line under is left out when it says the same', () => {
  assert.deepEqual(gameTitleParts({ title: 'メガ', titles: { native: 'メガ' } }, 'both'), { main: 'メガ' })
})

test('a name says its language when its letters do', () => {
  assert.equal(titleLang('メガゲーム'), 'ja')
  assert.equal(titleLang('A列車で行こう'), 'ja')
  assert.equal(titleLang('메가 게임'), 'ko')
  for (const s of ['超级游戏', 'Mega Game', '', null, undefined]) assert.equal(titleLang(s), undefined)
})

test('card props work as items do', () => {
  const props = { title: 'A Ressha de Ikou', kind: 'game', titles: { native: 'A列車で行こう', english: 'A-Train' } }
  assert.equal(gameTitle(props, 'native'), 'A列車で行こう')
  assert.equal(gameTitle(props, 'romanized'), 'A Ressha de Ikou')
  assert.equal(gameTitle(GAME_TITLE_EXAMPLE, 'english'), 'A-Train')
})
