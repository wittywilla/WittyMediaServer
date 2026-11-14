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

test('the administrators\' pages resolve as the catalogue says (pkg/sdui/catalog_data.go)', async () => {
  const { resolve } = await import('./routes.ts')
  assert.deepEqual(resolve('/manage/libraries'), { kind: 'page', page: 'manage-libraries', params: {} })
  assert.deepEqual(resolve('/manage/library/7'), { kind: 'page', page: 'manage-library', params: { item: '7' } })
  assert.deepEqual(resolve('/manage/matches'), { kind: 'page', page: 'fix-matches', params: {} })
  assert.deepEqual(resolve('/manage/match/42'), { kind: 'page', page: 'match', params: { item: '42' } })
  // Every /manage route in the server's catalogue is in this table.
  const { readFileSync } = await import('node:fs')
  const go = readFileSync(new URL('../../../pkg/sdui/catalog_data.go', import.meta.url), 'utf8')
  for (const [, pattern, page] of go.matchAll(/\{Pattern: "(\/manage\/[^"]+)", Page: "([^"]+)"/g)) {
    const r = resolve(pattern.replace('{item}', '1'))
    assert.equal(r?.page, page, pattern)
  }
})
