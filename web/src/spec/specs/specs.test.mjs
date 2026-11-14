// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { problems, reachable, withIds } from '../model.ts'
import { podcastAdminSpec, AUTO_STEPS } from './podcastAdmin.ts'
import { fixGameSpec, versionLinksSpec } from './fixgame.ts'

const ok = (p) => assert.deepEqual(problems(withIds(p)), [], p.key)
const ids = (p) => p.sections.flatMap((s) => (s.items || []).map((i) => i.id)).concat(p.actions.map((a) => a.id))

test('podcastAdminSpec marks the current keep-downloaded step', () => {
  const p = podcastAdminSpec(3); ok(p)
  const keep = p.sections[1].items
  assert.equal(keep.length, AUTO_STEPS.length)
  assert.deepEqual(keep.filter((c) => c.on).map((c) => c.id), ['auto.3'])
  assert.equal(keep[0].title, 'None, stream them'); assert.equal(keep[1].title, 'Newest episode')
  assert.deepEqual([p.sections[0].items[0].id, p.sections[2].items[0].id], ['check', 'unfollow'])
})

test('fixGameSpec: how it was found, versions, archive entries, console and not-a-game', () => {
  const p = fixGameSpec({ how: 'Going by the folder it’s in', sure: 80, why: 'Because.', discs: ['A', 'B'], others: [{ id: 7, label: 'Rev 1' }, { id: 8, label: '' }], inside: [{ entry: 'a.sfc', label: 'USA', current: true }, { entry: 'b.sfc', label: 'EU', altered: true }], choosing: true, busy: false })
  ok(p)
  assert.equal(p.sections[0].text, 'Going by the folder it’s in, 80% sure. Because.')
  assert.match(p.sections[1].text, /^2 discs\./)
  const all = ids(p)
  for (const i of ['open.7', 'split.7', 'split.8', 'play.a.sfc', 'play.b.sfc', 'console', 'notgame']) assert.ok(all.includes(i), i)
  assert.equal(p.sections.find((s) => s.items?.[0]?.id === 'play.a.sfc').items[0].disabled, true)
  assert.equal(p.sections.at(-1).items[0].on, true)
  assert.equal(p.sections.find((s) => s.items?.[1]?.id === 'split.8').items[1].title, 'Make this version a game of its own')
  const q = fixGameSpec({ how: '', others: [], choosing: false, busy: true }); ok(q)
  assert.equal(q.sections.length, 1); assert.ok(q.sections[0].items.every((c) => c.disabled))
  ok(versionLinksSpec([{ id: 1, label: '' }]))
})
