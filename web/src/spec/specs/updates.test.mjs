// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { NEWEST, updatesSpec } from './updates.ts'
import { problems, withIds } from '../model.ts'

const base = { current: '0.8.0-alpha', checkedAt: '', withdrawn: '', checkError: '', upToDate: false, latest: null, method: 'apply', hasInstallerPhase: false, active: false, checking: false, auto: true }

test('updates: installed version first, anchored for Settings search', () => {
  const p = updatesSpec(base)
  assert.equal(p.sections[0].anchor, 'updates')
  assert.equal(p.sections[0].h, 'Updates')
  assert.equal(p.sections[0].items[0].sub, 'Not checked yet')
  assert.deepEqual(p.actions.map((a) => a.id), ['check'])
  assert.deepEqual(problems(withIds(p)), [])
})

test('updates: newest version offers the one way this copy updates', () => {
  const l = { title: '', version: '0.9.0', meta: 'Version 0.9.0 · 40 MB' }
  const a = updatesSpec({ ...base, latest: l })
  assert.ok(a.sections.some((s) => s.h === NEWEST))
  assert.deepEqual(a.actions.map((x) => [x.id, x.kind]), [['apply', 'primary'], ['check', 'secondary']])
  assert.deepEqual(updatesSpec({ ...base, latest: l, method: 'installer' }).actions.map((x) => x.id), ['installer', 'check'])
  assert.deepEqual(updatesSpec({ ...base, latest: l, method: 'docker' }).actions.map((x) => x.id), ['check'])
  assert.deepEqual(updatesSpec({ ...base, latest: l, active: true }).actions.map((x) => [x.id, x.disabled]), [['check', true]])
})

test('updates: withdrawn, check errors and up to date are notes', () => {
  const p = updatesSpec({ ...base, withdrawn: 'bad', checkError: 'offline', upToDate: true, checkedAt: 'today' })
  const notes = p.sections.filter((s) => s.kind === 'note').map((s) => s.text)
  assert.equal(notes.length, 3)
  assert.match(notes[0], /withdrawn by the project: bad/)
  assert.equal(p.sections[0].items[0].sub, 'Last checked today')
})
