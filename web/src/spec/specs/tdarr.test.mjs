// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { tdarrSpec } from './tdarr.ts'
import { problems, withIds } from '../model.ts'

const v = { url: 'http://x:8266', key: '', keySet: true, enabled: false, dirty: false, busy: '', libs: [{ id: 3, name: 'Films', tdarrId: 'old', from: '/a', to: '/b', canSend: true }], known: [{ id: 'k', name: 'K' }], report: null }

test('tdarr: connection fields, save only when changed, forget only when a key is saved', () => {
  const p = tdarrSpec(v)
  assert.equal(p.title, 'Connect your own Tdarr')
  assert.deepEqual(p.sections[1].items.map((f) => f.id), ['url', 'key', 'enabled'])
  assert.equal(p.actions.find((a) => a.id === 'save').disabled, true)
  assert.ok(p.actions.some((a) => a.id === 'forget'))
  assert.equal(tdarrSpec({ ...v, keySet: false, dirty: true }).actions.find((a) => a.id === 'save').disabled, false)
  assert.ok(!tdarrSpec({ ...v, keySet: false }).actions.some((a) => a.id === 'forget'))
  assert.deepEqual(problems(withIds(p)), [])
})

test('tdarr: library mappings only when in use; an unknown saved library stays selectable', () => {
  assert.equal(tdarrSpec(v).sections.length, 2)
  const p = tdarrSpec({ ...v, enabled: true })
  const sel = p.sections.flatMap((s) => (s.kind === 'fields' ? s.items : [])).find((f) => f.id === 'map:3:tdarr')
  assert.deepEqual(sel.options.map((o) => o.value), ['', 'old', 'k'])
  assert.ok(p.sections.some((s) => s.kind === 'rows' && s.items[0].id === 'send:3'))
  assert.deepEqual(problems(withIds(p)), [])
})

test('tdarr: a failed test is a toned note, a good one lists nodes', () => {
  const bad = tdarrSpec({ ...v, report: { ok: false, error: 'Refused', nodes: [], libraries: 0 } })
  assert.equal(bad.sections.at(-1).text, 'Refused')
  const ok = tdarrSpec({ ...v, report: { ok: true, version: '2.0', nodes: [{ id: 'n', name: 'Den', paused: true, doing: 'nothing to do' }], libraries: 2 } })
  const s = ok.sections.at(-1)
  assert.equal(s.h, 'Connected to Tdarr 2.0')
  assert.deepEqual(s.items.map((r) => r.title), ['Nodes', 'Den, paused', 'Libraries set up there'])
})
