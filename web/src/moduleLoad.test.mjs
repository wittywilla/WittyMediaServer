// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { bundleCache, pageOf, pickFrom } from './moduleLoad.ts'

const page = () => null
const module = { pages: { overview: page } }
const isComponent = (c) => typeof c === 'function'

test('a page a module does not have is not found, and is never reported', async () => {
  const reports = []
  let loads = 0
  const load = bundleCache((b) => b.id, async () => { loads++; return module }, (b, why) => reports.push([b.id, String(why)]))
  const b = { id: 'wittystats' }
  // A mistyped address first: the module is fine, the page isn't there.
  const first = pickFrom(await load(b), (m) => pageOf(m.pages, 'nope'), isComponent)
  assert.equal(first.found, false)
  // The real page afterwards still works: the cache holds the module, not the first verdict.
  const second = pickFrom(await load(b), (m) => pageOf(m.pages, 'overview'), isComponent)
  assert.equal(second.found, true)
  assert.equal(second.value, page)
  assert.equal(loads, 1)
  await new Promise((r) => setTimeout(r, 0))
  assert.deepEqual(reports, [])
})

test('only a bundle that will not load or run is reported, once', async () => {
  const reports = []
  const load = bundleCache((b) => b.id, async () => { throw new Error('SyntaxError: Unexpected token') }, (b, why) => reports.push(b.id + ': ' + why.message))
  const b = { id: 'broken' }
  await assert.rejects(load(b))
  await assert.rejects(load(b))
  await new Promise((r) => setTimeout(r, 0))
  assert.deepEqual(reports, ['broken: SyntaxError: Unexpected token'])
})

test('an address reaches only the pages a module names', () => {
  for (const id of ['constructor', '__proto__', 'toString', 'hasOwnProperty']) {
    assert.equal(pageOf({ overview: page }, id), undefined, id)
    assert.equal(pickFrom(module, (m) => pageOf(m.pages, id), isComponent).found, false, id)
  }
  assert.equal(pageOf(undefined, 'overview'), undefined)
  assert.equal(pageOf('pages', 'length'), undefined)
})

test('a pick that throws is nothing found, not a crash', () => {
  assert.equal(pickFrom({}, () => { throw new Error('x') }, isComponent).found, false)
})
