// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { pageAddress, pathMatches, screenFor, strictest } from './levels.ts'

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
