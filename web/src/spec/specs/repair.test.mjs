// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { copiesSpec, remapSpec, repairResultsSpec, repairTopSpec } from './repair.ts'
import { problems, withIds } from '../model.ts'

const top = { running: '', phase: '', checked: '', error: '', safeCount: 0, hasReport: false, restart: false, canRestart: true }
const chips = (p) => p.sections.filter((s) => s.kind === 'chips').flatMap((s) => s.items)

test('repair top: check first; fix-all-safe only when there is something safe; Check this server stays anchored', () => {
  const p = repairTopSpec(top)
  assert.deepEqual(chips(p).map((c) => [c.id, c.title, c.on]), [['check', 'Check this server', true]])
  assert.equal(p.sections.find((s) => s.h === 'Check this server').anchor, 'check-this-server')
  const q = repairTopSpec({ ...top, safeCount: 3, hasReport: true, checked: 'today' })
  assert.deepEqual(chips(q).map((c) => [c.id, c.title, c.on]), [['check', 'Check again', false], ['allsafe', 'Fix everything that’s safe (3)', true]])
  assert.deepEqual(problems(withIds(q)), [])
})

test('repair top: running disables, says the phase; restart notice carries its chip; last repair is a notice', () => {
  const r = repairTopSpec({ ...top, running: 'check', phase: 'Looking at modules', safeCount: 2, hasReport: true })
  assert.deepEqual(chips(r).map((c) => [c.id, c.disabled]), [['check', true]])
  assert.ok(r.sections.some((s) => s.kind === 'note' && s.text === 'Looking at modules'))
  const x = repairTopSpec({ ...top, restart: true, canRestart: false, lastRepair: { ok: false, error: 'No space.', at: 'x' } })
  assert.ok(chips(x).some((c) => c.id === 'restart'))
  assert.match(x.sections[0].h, /couldn’t take over/)
  assert.match(x.sections.find((s) => s.h === 'A restart finishes the repair').text, /can’t restart itself/)
})

test('repair results, remap, copies', () => {
  const d = repairResultsSpec([{ key: 'a', title: 'T', ok: true, done: 'ok' }, { key: 'b', title: 'U', ok: false, error: 'no' }], '/x')
  assert.deepEqual(d.sections[0].items.map((r) => [r.title, r.on]), [['Fixed: T', false], ['Not fixed: U', true]])
  const rv = { from: 'D:\\a', to: '', busy: false, twoFactor: true, password: '', code: '', error: '', plan: null }
  const m = remapSpec(rv)
  assert.equal(m.sections[0].anchor, 'moved-a-drive')
  assert.equal(chips(m)[0].disabled, true)
  const plan = { roots: 2, rows: 1200, checked: 5, found: 4, sample: ['/a'], notFound: ['/b'], canApply: true }
  const m2 = remapSpec({ ...rv, to: 'E:\\a', plan })
  const ids = m2.sections.flatMap((s) => (s.kind === 'fields' ? s.items.map((f) => f.id) : []))
  assert.deepEqual(ids, ['from', 'to', 'password', 'code'])
  assert.deepEqual(chips(m2).map((c) => [c.id, c.disabled]), [['look', false], ['move', true]])
  assert.deepEqual(problems(withIds(m2)), [])
  assert.equal(copiesSpec(['/c1']).sections[1].items[0].title, '/c1')
})
