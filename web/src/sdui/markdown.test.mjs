// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { inline, linkKind, parse } from './markdownModel.ts'
import { nativeInstead } from './native.ts'

test('the Markdown subset: headings, lists, paragraphs, marks and links', () => {
  const b = parse('## Friday\nPick **one** each, *quietly*.\nThe [full list](/collections/12) is `long`.\n\n- a\n- b\n\n1. first')
  assert.deepEqual(b.map((x) => x.t), ['h', 'p', 'ul', 'ol'])
  assert.equal(b[0].level, 2)
  assert.deepEqual(b[1].c.map((x) => x.t), ['text', 'bold', 'text', 'italic', 'text', 'link', 'text', 'code', 'text'])
  assert.equal(b[2].items.length, 2)
})

test('a link goes to a route in the app or an http(s) page, and nowhere else', () => {
  assert.equal(linkKind('/library/3'), 'route')
  assert.equal(linkKind('https://example.org/a'), 'external')
  assert.equal(linkKind('//elsewhere.example'), null)
  assert.equal(linkKind('javascript:alert(1)'), null)
  assert.deepEqual(inline('[x](javascript:alert(1))').some((n) => n.t === 'link'), false)
  // Raw HTML is the characters it is.
  assert.deepEqual(inline('<b>hi</b>'), [{ t: 'text', v: '<b>hi</b>' }])
})

test('the app\'s own screens stand in only for the catch-all pages of kinds the model has no page for', () => {
  const libs = [{ id: 1, type: 'movies' }, { id: 2, type: 'radio' }, { id: 3, type: 'games', console: 'rig' }]
  const lib = (id, page = 'library') => ({ page: { id: page, kind: 'library', title: '', params: { library: String(id) } }, root: { type: 'Page', id: 'page' } })
  assert.equal(nativeInstead(lib(1), libs), false)
  assert.equal(nativeInstead(lib(2), libs), true)
  assert.equal(nativeInstead(lib(3), libs), true)
  assert.equal(nativeInstead(lib(2, 'radio-night'), libs), false, 'a page someone wrote for it is drawn')
  const detail = (kind, page = 'detail') => ({ page: { id: page, kind: 'detail', title: '' }, root: { type: 'Page', id: 'page', children: [{ type: 'DetailHeader', id: 'header', props: { kind } }] } })
  assert.equal(nativeInstead(detail('movie'), libs), false)
  assert.equal(nativeInstead(detail('game'), libs), true)
  assert.equal(nativeInstead(detail('book'), libs), true)
  assert.equal(nativeInstead(detail('show', 'detail-show'), libs), false)
})
