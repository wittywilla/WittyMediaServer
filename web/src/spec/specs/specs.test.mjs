// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { problems, reachable, withIds } from '../model.ts'
import { podcastAdminSpec, AUTO_STEPS } from './podcastAdmin.ts'

const ok = (p) => assert.deepEqual(problems(withIds(p)), [], p.key)

test('podcastAdminSpec marks the current keep-downloaded step', () => {
  const p = podcastAdminSpec(3); ok(p)
  const keep = p.sections[1].items
  assert.equal(keep.length, AUTO_STEPS.length)
  assert.deepEqual(keep.filter((c) => c.on).map((c) => c.id), ['auto.3'])
  assert.equal(keep[0].title, 'None, stream them'); assert.equal(keep[1].title, 'Newest episode')
  assert.deepEqual([p.sections[0].items[0].id, p.sections[2].items[0].id], ['check', 'unfollow'])
})

