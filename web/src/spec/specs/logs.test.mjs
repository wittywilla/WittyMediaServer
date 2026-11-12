// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { logsSpec } from './logs.ts'
import { problems, withIds } from '../model.ts'

test('logs: always the two ways forward; recording and problems only when they are so', () => {
  const quiet = logsSpec({ recordingUntil: '', problems: [], flight: true, showDownload: true, saving: false })
  assert.deepEqual(quiet.sections.map((s) => s.kind), ['rows'])
  assert.deepEqual(quiet.sections[0].items.map((r) => r.title), ['Download logs as ZIP', 'Save what just happened'])
  assert.match(quiet.sections[0].items[1].sub, /10 minutes/)
  const busy = logsSpec({ recordingUntil: '3:15 PM', problems: ['a', 'b'], flight: false, showDownload: false, saving: true })
  assert.deepEqual(busy.sections.map((s) => s.kind), ['note', 'rows', 'note', 'rows'])
  assert.equal(busy.sections[0].h, 'Recording everything until 3:15 PM')
  assert.equal(busy.sections[2].text, 'a b')
  assert.equal(busy.sections[3].items.length, 1)
  assert.equal(busy.sections[3].items[0].disabled, true)
  assert.deepEqual(problems(withIds(busy)), [])
})
