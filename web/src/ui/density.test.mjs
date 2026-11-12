// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// node --experimental-strip-types --test web/src/ui/density.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { asDensity } from './density.ts'

test('density is one of three, and anything else is comfortable', () => {
  assert.equal(asDensity('compact'), 'compact')
  assert.equal(asDensity('spacious'), 'spacious')
  assert.equal(asDensity('comfortable'), 'comfortable')
  for (const v of [null, undefined, '', 'tiny', 3]) assert.equal(asDensity(v), 'comfortable')
})
