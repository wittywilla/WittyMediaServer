// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
const { classifyPlayError } = await import('./errors.ts')

test('the player says what went wrong, and what can be done', () => {
  assert.equal(classifyPlayError('', 2).kind, 'network')
  assert.equal(classifyPlayError('Failed to fetch').kind, 'network')
  assert.equal(classifyPlayError('x', 3).convert, true)
  assert.equal(classifyPlayError('This file can’t be played here.', 4).kind, 'unsupported')
  assert.equal(classifyPlayError('The stream has ended on the server. Start playback again.').kind, 'ended')
  assert.equal(classifyPlayError('Subtitles failed to load').again, false)
  const other = classifyPlayError('odd')
  assert.equal(other.kind, 'other'); assert.equal(other.text, 'odd')
})
