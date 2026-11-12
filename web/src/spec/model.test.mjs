// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { A1, A2, Cc, R, current, mk, problems, reachable, secC, secF, secN, secP, secR, sectionId, withIds } from './model.ts'

const gaps = JSON.parse(readFileSync(new URL('./gaps.json', import.meta.url), 'utf8'))

test('builders mirror the canvas', () => {
  assert.deepEqual(R('Playing as', 'Direct play', true, 'sub'), { title: 'Playing as', value: 'Direct play', sub: 'sub', on: true })
  assert.deepEqual(R('A'), { title: 'A', on: false })
  assert.deepEqual(Cc('Now', true), { title: 'Now', on: true })
  assert.equal(secN('', 'text', true).tone, true)
  assert.equal(A1('Play').kind, 'primary')
  assert.equal(A2('Back').kind, 'secondary')
  const p = mk('queue', 'Now playing', 'Up next', 'lead', [secR('Next', [R('a')]), secP('', [Cc('x')]), secC('c', [{ title: 't' }])], [A1('Done')], 'foot')
  assert.equal(p.sections.length, 3)
  assert.equal(p.foot, 'foot')
})

test('withIds is stable, keeps given ids and does not change its input', () => {
  const p = mk('k', 'e', 't', '', [secR('h', [R('a'), { title: 'b', id: 'mine' }])], [A1('go')])
  const q = withIds(p)
  assert.equal(q.sections[0].items[0].id, 'k.0.0')
  assert.equal(q.sections[0].items[1].id, 'mine')
  assert.equal(q.actions[0].id, 'k.99.0')
  assert.equal(p.sections[0].items[0].id, undefined)
  assert.deepEqual(withIds(q), q)
})

test('problems finds what cannot be drawn', () => {
  assert.deepEqual(problems(withIds(mk('k', 'e', 't', '', [secR('h', [R('a')])]))), [])
  assert.ok(problems(mk('k', 'e', '', '', [])).some((m) => /no title/.test(m)))
  assert.ok(problems(mk('k', 'e', 't', '', [secR('h', [])])).some((m) => /no items/.test(m)))
  assert.ok(problems(mk('k', 'e', 't', '', [secN('', ' ')])).some((m) => /no text/.test(m)))
  assert.ok(problems(mk('k', 'e', 't', '', [], [A1('a'), A1('b')])).some((m) => /more than one primary/.test(m)))
  assert.ok(problems(withIds(mk('k', 'e', 't', '', [secR('h', [{ title: 'a', id: 'x' }, { title: 'b', id: 'x' }])]))).some((m) => /twice/.test(m)))
})

test('reachable: only what goes somewhere or is handled', () => {
  const p = withIds(mk('k', 'e', 't', '', [secR('h', [R('a'), { title: 'b', to: '/b' }, { title: 'c', to: '/c', disabled: true }, { title: 'd', id: 'd' }])], [A1('go')]))
  assert.deepEqual(reachable(p, new Set()), [p.sections[0].items[1].id])
  assert.deepEqual(reachable(p, new Set(['d', 'k.99.0'])), [p.sections[0].items[1].id, 'd', 'k.99.0'])
})

test('section ids are safe attribute values and differ by section', () => {
  assert.match(sectionId('a b/c', 2), /^spec-[a-z0-9-]+-h2$/i)
  assert.notEqual(sectionId('a', 0), sectionId('a', 1))
  assert.equal(current(secP('', [Cc('a'), Cc('b', true)])), 1)
  assert.equal(current(secN('', 'x')), -1)
})

test('the catalogue holds all 124 canvas pages, each drawable', () => {
  assert.equal(gaps.length, 124)
  assert.equal(new Set(gaps.map((p) => p.key)).size, 124)
  for (const k of ['playinfo', 'nextup', 'filters', 'collections', 'queue', 'sleep', 'gamepcs', 'readertext', 'devices', 'modstore', 'activity', 'offline', 'shorts']) assert.ok(gaps.some((p) => p.key === k), k)
  for (const p of gaps) {
    const bad = problems(withIds(p))
    assert.deepEqual(bad, [], p.key)
    assert.ok(!JSON.stringify(p).includes('undefined'))
  }
})

test('gaps.json is what the canvas gives (scripts/spec-from-canvas.mjs --check)', () => {
  execFileSync(process.execPath, [new URL('../../scripts/spec-from-canvas.mjs', import.meta.url).pathname, '--check'], { stdio: 'pipe' })
})

test('fields: ids, validation and reach', () => {
  const p = withIds(mk('f', 'e', 't', '', [secF('Account', [
    { kind: 'text', label: 'Name' }, { kind: 'pin', label: 'PIN', length: 4 }, { kind: 'select', label: 'Kind', options: [{ value: 'a', label: 'A' }] },
    { kind: 'slider', label: 'Size', min: 0, max: 10 }, { kind: 'qr', label: 'Scan', text: 'x' }, { kind: 'code', label: 'Code', value: '123456' }, { kind: 'switch', label: 'On' },
  ])]))
  assert.deepEqual(problems(p), [])
  assert.deepEqual(reachable(p, new Set()), ['f.0.0', 'f.0.1', 'f.0.2', 'f.0.3', 'f.0.6'])
  const bad = problems(withIds(mk('g', 'e', 't', '', [secF('', [{ kind: 'pin', label: 'P' }, { kind: 'select', label: 'S' }, { kind: 'slider', label: 'L' }, { kind: 'qr', label: 'Q' }, { kind: 'text', label: ' ' }])])))
  assert.equal(bad.length, 5)
})
