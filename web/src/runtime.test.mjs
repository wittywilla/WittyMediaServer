// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { fmtRuntime } from './runtime.ts'

test('runtimes never read 60 minutes', () => {
  for (const [sec, want] of [
    [0, ''], [30, '30 s'], [59.6, '60 s'], [60, '1 min'], [89, '1 min'], [2700, '45 min'],
    [3570, '1 h'], [3599, '1 h'], [3600, '1 h'], [6720, '1 h 52 min'],
    [8 * 3600 + 59 * 60 + 40, '9 h'], [8 * 3600 + 59 * 60 + 20, '8 h 59 min'],
  ]) assert.equal(fmtRuntime(sec), want, `${sec} s`)
})
