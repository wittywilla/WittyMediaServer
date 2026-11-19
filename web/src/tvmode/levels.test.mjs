// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { pageAddress, pathMatches, screenFor, strictest, tvClientOf } from './levels.ts'
import { messagesIn } from '../i18n/extract.ts'

const table = JSON.parse(readFileSync(new URL('../../../pkg/sdui/data/tvlevels.json', import.meta.url), 'utf8'))
const level = (p) => screenFor(table, p)?.level || 'full'

test('addresses match as the server matches them', () => {
  assert.equal(pathMatches('/settings/pages/*', '/settings/pages'), true)
  assert.equal(pathMatches('/settings/pages/*', '/settings/pages/page/home'), true)
  assert.equal(pathMatches('/tv/channels/:id', '/tv/channels'), false)
  assert.equal(pathMatches('/tv/channels/:id', '/tv/channels/4'), true)
  assert.equal(pathMatches('/markers/:item', '/markers/4/more'), false)
})

test('the editors are desktopOnly, and what a sofa needs stays full', () => {
  for (const p of ['/settings/libraries', '/settings/pages/navigation', '/settings/channels', '/tv/channels/new', '/tv/channels/7', '/manage/library/3', '/settings/logs', '/settings/modules/store/x', '/settings/backup', '/settings/remote', '/markers/9', '/settings/transcoding?tab=hw']) {
    assert.equal(level(p), 'desktopOnly', p)
  }
  for (const p of ['/settings/playback', '/settings/users', '/tv/channels']) assert.equal(level(p), 'view', p)
  for (const p of ['/', '/tv', '/tv/sets', '/search', '/you', '/settings/appearance', '/settings/games', '/settings/gaming', '/controllers', '/settings/about', '/settings/module-settings/wittyplay', '/item/4', '/library/2']) {
    assert.equal(level(p), 'full', p)
  }
})

test('every row of the table is whole', () => {
  const ids = new Set()
  for (const s of [...table.screens, ...table.parts]) {
    assert.ok(s.id && s.title && ['full', 'view', 'desktopOnly'].includes(s.level), JSON.stringify(s))
    assert.ok(!ids.has(s.id), s.id)
    ids.add(s.id)
  }
})

test('the stricter of two levels wins', () => {
  assert.equal(strictest('full', 'view'), 'view')
  assert.equal(strictest('desktopOnly', 'view'), 'desktopOnly')
  assert.equal(strictest(undefined, undefined), 'full')
  assert.equal(strictest('nonsense', 'full'), 'full')
})

test('the address given is one a phone can open', () => {
  assert.equal(pageAddress('http://192.168.1.9:9797', '/settings/libraries', []), 'http://192.168.1.9:9797/settings/libraries')
  assert.equal(pageAddress('http://localhost:9797', '/settings/libraries', ['http://192.168.1.9:9797']), 'http://192.168.1.9:9797/settings/libraries')
  assert.equal(pageAddress('http://tv.localhost:9797/', 'settings/x', []), 'http://tv.localhost:9797/settings/x')
})

test('a TV device is told to use a computer, with the address and a QR code', () => {
  // The apps say so: Android TV and Plasma Bigscreen add ?client=tv, which is remembered.
  assert.equal(tvClientOf({ client: 'tv', ua: 'Mozilla/5.0 (X11; Linux x86_64) Chrome/140', shell: true }), 'tvDevice')
  // A TV's own browser says what it is.
  for (const ua of ['Mozilla/5.0 (SMART-TV; Linux; Tizen 7.0) Chrome/94', 'Mozilla/5.0 (Web0S; Linux/SmartTV) Chrome/108', 'Mozilla/5.0 (Linux; Android 12; AFTKA) Chrome/120', 'Mozilla/5.0 (Linux; Android 11; BRAVIA 4K VH2) Chrome/112']) {
    assert.equal(tvClientOf({ ua }), 'tvDevice', ua)
  }
  assert.ok(table.note.includes('WittyMediaClient'))
})

test('a computer in TV mode is told to leave TV mode, whatever its window size', () => {
  assert.equal(tvClientOf({ ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/140' }), 'computer')
  assert.equal(tvClientOf({ ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 15_0) Safari/605' }), 'computer')
  // The desktop app's own window, started with --tv.
  assert.equal(tvClientOf({ shell: true, ua: 'Mozilla/5.0 (Windows NT 10.0) Electron/38' }), 'computer')
  assert.equal(tvClientOf({ client: 'desktop', ua: 'Mozilla/5.0 (SMART-TV; Tizen)' }), 'computer')
  assert.equal(table.noteDesktop, 'Leave TV mode to change this.')
})

test("every word in the table is written out for translation (levelWords.ts)", () => {
  const words = new Set(messagesIn(readFileSync(new URL('./levelWords.ts', import.meta.url), 'utf8')))
  const used = [table.note, table.noteDesktop, ...table.screens.map((s) => s.title), ...table.parts.map((p) => p.title)]
  for (const w of used) assert.ok(words.has(w), `levelWords.ts is missing ${JSON.stringify(w)}`)
})
