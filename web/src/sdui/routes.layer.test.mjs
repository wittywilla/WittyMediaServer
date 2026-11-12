// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { plan } from './routes.ts'

test('a navigate with as layer is shown over the page, not gone to', () => {
  assert.deepEqual(plan({ type: 'navigate', to: '/play/menu', as: 'layer' }), { do: 'layer', to: '/play/menu' })
  assert.deepEqual(plan({ type: 'navigate', to: '/play/menu' }), { do: 'go', to: '/play/menu' })
  assert.deepEqual(plan({ type: 'navigate', to: '//evil.example', as: 'layer' }), { do: 'nothing' })
})
