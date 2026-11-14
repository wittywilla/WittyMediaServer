// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { problems, withIds, handledIds, reachable } from '../model.ts'
import { audienceSpec, basicsSpec, moreSpec } from './channelEditor.ts'

const ok = (p) => assert.deepEqual(problems(withIds(p)), [], p.title)

test('channel editor: basics, audience and more', () => {
  ok(basicsSpec({ name: 'A', number: '', description: '', categories: '' }))
  const a = audienceSpec({ mode: 'only', countProgress: true })
  ok(a)
  assert.equal(a.sections[0].items[1].value, true)
  const m = moreSpec({ hasLogo: true, dirty: true, history: [{ id: 9, name: 'v1', when: 'today' }] })
  ok(m)
  const rows = m.sections.find((s) => s.kind === 'rows' && !s.h).items
  assert.deepEqual(rows.map((r) => [r.id, !!r.disabled]), [['removeLogo', false], ['reshuffle', false], ['reset', true]])
  assert.equal(m.sections.at(-1).items[0].actions[0].id, 'restore:9')
  assert.equal(moreSpec({ hasLogo: false, dirty: false, history: [] }).sections.filter((s) => s.kind === 'rows').length, 1)
})
