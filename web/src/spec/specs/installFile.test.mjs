// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { installChoose, installFound } from './installFile.ts'
import { problems, withIds } from '../model.ts'

test('choose: three file fields and a key; hints name what was chosen; uploading disables them', () => {
  const p = installChoose({ archive: 'a.zip', sums: '', sig: '', key: 'K', error: '', uploading: '' })
  const f = p.sections.flatMap((s) => (s.kind === 'fields' ? s.items : []))
  assert.deepEqual(f.map((x) => x.id), ['archive', 'sums', 'sig', 'key'])
  assert.equal(f[0].hint, 'Chosen: a.zip')
  assert.match(f[1].hint, /SHA256SUMS/)
  assert.ok(f.every((x) => !x.disabled))
  const u = installChoose({ archive: 'a.zip', sums: '', sig: '', key: '', error: 'Too big', uploading: 'Uploading a.zip: 40%' })
  assert.ok(u.sections.flatMap((s) => (s.kind === 'fields' ? s.items : [])).every((x) => x.disabled))
  assert.deepEqual(u.sections.slice(-2).map((s) => s.text), ['Uploading a.zip: 40%', 'Too big'])
  assert.deepEqual(problems(withIds(p)), [])
})

test('found: facts, signature tone, permissions or none, problems', () => {
  const f = { title: 'Weather 1.0', facts: [{ label: 'Version', value: '1.0' }], signature: { words: 'Signed by a key you trust.', tone: 'ok' }, fingerprint: 'ab12', permissions: [], problems: ['Old API'], overrideNote: true, keptUntil: '5 PM', statusNote: 'Hm' }
  const p = installFound(f)
  assert.equal(p.title, 'Weather 1.0')
  assert.equal(p.sections.find((s) => s.h === 'Signature').tone, false)
  assert.match(p.sections.find((s) => s.h === 'Signature').text, /ab12/)
  assert.equal(p.sections.find((s) => s.h === 'What it asks for').kind, 'note')
  assert.equal(p.sections.find((s) => s.h === 'What’s wrong with it').items[0].title, 'Old API')
  assert.match(p.foot, /5 PM/)
  assert.equal(installFound({ ...f, permissions: ['network'], signature: { words: 'x', tone: 'warn' } }).sections.find((s) => s.h === 'Signature').tone, true)
  assert.deepEqual(problems(withIds(p)), [])
})
