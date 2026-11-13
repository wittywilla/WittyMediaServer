// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { dockCurrent } from './regions.ts'

test('the dock marks the entry whose route is the address or a parent of it', () => {
  const e = [{ id: 'a', label: 'Discover', route: '/play/discover' }, { id: 'b', label: 'Together', route: '/play/together' }]
  assert.equal(dockCurrent(e, '/play/together'), 1)
  assert.equal(dockCurrent(e, '/play/together/room'), 1)
  assert.equal(dockCurrent(e, '/play/togetherness'), -1)
  assert.equal(dockCurrent(e, '/'), -1)
})

test('a dock route with a query is current only with that query', () => {
  const e = [{ id: 'a', label: 'Search', route: '/search' }, { id: 'b', label: 'Together', route: '/page/wittyplay-online?tab=together' }]
  assert.equal(dockCurrent(e, '/page/wittyplay-online', '?tab=together'), 1)
  assert.equal(dockCurrent(e, '/page/wittyplay-online', '?x=1&tab=together'), 1)
  assert.equal(dockCurrent(e, '/page/wittyplay-online', ''), -1)
  assert.equal(dockCurrent(e, '/page/wittyplay-online', '?tab=memories'), -1)
  assert.equal(dockCurrent(e, '/search'), 0)
})
