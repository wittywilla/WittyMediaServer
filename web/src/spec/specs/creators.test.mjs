// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { channelSpec, topicsSpec } from './creators.ts'
import { problems, withIds } from '../model.ts'

test('topics: All is on when no tag is chosen, else the chosen tag', () => {
  assert.deepEqual(topicsSpec(['Music', 'Food'], '').sections[0].items.map((c) => [c.id, c.on]), [['tag:', true], ['tag:Music', false], ['tag:Food', false]])
  assert.deepEqual(topicsSpec(['Music'], 'Music').sections[0].items.map((c) => c.on), [false, true])
  assert.deepEqual(problems(withIds(topicsSpec(['a'], ''))), [])
})

test('channel details: four fields, the error sits on the address', () => {
  const p = channelSpec({ name: 'N', handle: '', platform: '', url: 'x' }, 'Not an address.')
  assert.deepEqual(p.sections[0].items.map((f) => f.id), ['name', 'handle', 'platform', 'url'])
  assert.equal(p.sections[0].items[3].error, 'Not an address.')
  assert.equal(p.sections[0].items[0].required, true)
  assert.deepEqual(problems(withIds(p)), [])
})
