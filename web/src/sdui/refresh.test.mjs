// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A page in progress (meta.refreshSec): how soon to ask again, and what to read out when it settles.
import test from 'node:test'
import assert from 'node:assert/strict'
import { changedRows, refreshDelay } from './refresh.ts'

test('refreshDelay follows meta.refreshSec, within bounds, and is 0 without it', () => {
  assert.equal(refreshDelay(null), 0)
  assert.equal(refreshDelay({ meta: {} }), 0)
  assert.equal(refreshDelay({ meta: { refreshSec: 0 } }), 0)
  assert.equal(refreshDelay({ meta: { refreshSec: -3 } }), 0)
  assert.equal(refreshDelay({ meta: { refreshSec: 2 } }), 2000)
  assert.equal(refreshDelay({ meta: { refreshSec: 0.2 } }), 1000, 'never sooner than a second')
  assert.equal(refreshDelay({ meta: { refreshSec: 9999 } }), 300000)
})

const page = (rows) => ({ type: 'Page', id: 'page', children: [{ type: 'List', id: 'choices', children: rows.map(([id, title, trailing]) => ({ type: 'ListItem', id, props: { title, trailing } })) }] })

test('changedRows names the rows whose words changed, in page order', () => {
  const busy = page([['choices:state', 'Full rescan: 1,681 of 12,708 files'], ['choices:changes', 'Scan for changes', 'Scanning'], ['choices:full', 'Full rescan', 'Scanning']])
  const done = page([['choices:state', 'Last scanned 14 Nov 2026 10:00'], ['choices:changes', 'Scan for changes'], ['choices:full', 'Full rescan']])
  assert.deepEqual(changedRows(busy, done)[0], 'Last scanned 14 Nov 2026 10:00')
  assert.deepEqual(changedRows(busy, busy), [])
  assert.deepEqual(changedRows(null, done), [])
  // A row that is new isn't "changed": the page's own heading says what it is.
  assert.deepEqual(changedRows(page([]), done), [])
  // The libraries' list: the row keeps its name, its end changes.
  assert.deepEqual(changedRows(page([['all:lib-1', 'TV', '1,681 of 12,708 files']]), page([['all:lib-1', 'TV', 'Scan']])), ['TV, Scan'])
})

test('a section that failed is asked again after 1, 2, 5, then every 15 seconds, never in a tight loop', async () => {
  const { sectionRetryDelay } = await import('./refresh.ts')
  assert.deepEqual([1, 2, 3, 4, 5, 9, 100].map(sectionRetryDelay), [1000, 2000, 5000, 15000, 15000, 15000, 15000])
  assert.equal(sectionRetryDelay(0), 1000)
})
