// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { chaptersSpec, autoplaySpec, photoDetailsSpec, bookActionsSpec } from './media.ts'
import { problems, withIds } from '../model.ts'

test('chapters: a row per part with a clock, nothing for one part', () => {
  const p = chaptersSpec([{ start: 0, title: 'Intro' }, { start: 3725 }], true)
  assert.equal(p.title, 'Setlist')
  assert.deepEqual(p.sections[0].items.map((r) => [r.id, r.title, r.value]), [['ch:0', 'Intro', '0:00'], ['ch:1', 'Part 2', '1:02:05']])
  assert.equal(chaptersSpec([{ start: 0 }], false).sections.length, 0)
  assert.deepEqual(problems(withIds(p)), [])
})
test('autoplay reads On or Off', () => {
  assert.equal(autoplaySpec(false).sections[0].items[0].value, 'Off')
  assert.equal(autoplaySpec(true).sections[0].items[0].on, true)
})
test('photo details leave out what is not known', () => {
  const p = photoDetailsSpec({ fileName: 'a.jpg', taken: '12 Nov, 9:00', width: 100, height: 50, size: '1 MB' })
  assert.deepEqual(p.sections[0].items.map((r) => r.title), ['Taken', 'File'])
  assert.equal(p.sections[0].items[1].sub, '100 × 50 · 1 MB')
  assert.equal(p.sections[1].items[0].id, 'original')
  assert.deepEqual(problems(withIds(p)), [])
})
test('book actions: Read, Continue, Read again; unreadable gets only Download and a reason', () => {
  const lab = (b) => bookActionsSpec(b).sections[0].items.map((c) => c.title)
  assert.deepEqual(lab({ readable: true, started: false, finished: false, formatName: 'EPUB' }), ['Read', 'Download'])
  assert.deepEqual(lab({ readable: true, started: true, finished: false, formatName: 'EPUB' }), ['Continue reading', 'Download'])
  assert.deepEqual(lab({ readable: true, started: false, finished: true, formatName: 'EPUB' }), ['Read again', 'Download'])
  const u = bookActionsSpec({ readable: false, started: false, finished: false, formatName: 'MOBI' })
  assert.deepEqual(lab({ readable: false, started: false, finished: false, formatName: 'MOBI' }), ['Download'])
  assert.match(u.sections[1].items[0].sub, /can’t be read/)
})
