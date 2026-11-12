// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { flipPart, makeBackupSpec, partFields, scheduleSpec, sizeWords } from './backup.ts'
import { problems, withIds } from '../model.ts'

const parts = [
  { id: 'db', label: 'Database', desc: 'Everything.' },
  { id: 'art', label: 'Artwork', desc: 'Pictures.', needs: ['db'] },
]
const ids = (p) => p.sections.flatMap((s) => (s.kind === 'fields' ? s.items.map((f) => f.id) : s.kind === 'chips' ? s.items.map((c) => c.id) : []))

test('parts: a switch each, with needs and size; flipping brings what is needed and drops what needs it', () => {
  const f = partFields(parts, new Set(['db']), { db: 2048, art: 0 })
  assert.deepEqual(f.map((x) => [x.id, x.value]), [['part:db', true], ['part:art', false]])
  assert.match(f[0].hint, /About 2 KB/)
  assert.match(f[1].hint, /Needs Database too\. Empty\./)
  assert.deepEqual([...flipPart(parts, new Set(), 'art', true)].sort(), ['art', 'db'])
  assert.deepEqual([...flipPart(parts, new Set(['db', 'art']), 'db', false)], [])
  assert.equal(sizeWords(5 << 20), '5.0 MB')
})

test('make a backup: protected shows password fields, unprotected the warning; ready gates the chip', () => {
  const v = { parts, chosen: new Set(['db']), sizes: { db: 1 << 20 }, protect: true, password: 'abc', again: 'abd', ready: false, busy: false }
  const p = makeBackupSpec(v)
  assert.deepEqual(ids(p).filter((i) => !i.startsWith('part:')), ['protect', 'password', 'again', 'make'])
  const pw = p.sections.flatMap((s) => (s.kind === 'fields' ? s.items : [])).filter((f) => f.id === 'password' || f.id === 'again')
  assert.deepEqual(pw.map((f) => f.error), ['That’s too short.', 'These aren’t the same.'])
  assert.equal(p.sections.at(-1).items[0].disabled, true)
  const q = makeBackupSpec({ ...v, protect: false, ready: true })
  assert.ok(q.sections.some((s) => s.h === 'Without a password, secrets are left out'))
  assert.equal(ids(q).includes('password'), false)
  assert.equal(q.sections.at(-1).items[0].disabled, false)
  assert.deepEqual(problems(withIds(p)), [])
  assert.deepEqual(problems(withIds(q)), [])
})

const sv = { parts, chosen: new Set(['db']), frequency: 'off', weekday: 1, hour: 3, keep: 7, fewer: false, twoFactor: false, current: '', code: '', protectedNow: false, changePw: false, password: '', again: '', lastMade: '', lastError: '', busy: false, badPw: false }
test('schedule: off is only the choice; weekly adds the day; fewer asks for the password; anchored for search', () => {
  const off = scheduleSpec(sv)
  assert.equal(off.sections[0].anchor, 'back-up-by-itself')
  assert.deepEqual(ids(off), ['freq:off', 'freq:daily', 'freq:weekly', 'save'])
  const wk = scheduleSpec({ ...sv, frequency: 'weekly', fewer: true, twoFactor: true })
  assert.deepEqual(ids(wk).filter((i) => !i.startsWith('freq') && !i.startsWith('part')), ['weekday', 'hour', 'keep', 'current', 'code', 'password', 'again', 'save'])
  assert.deepEqual(problems(withIds(wk)), [])
})
test('schedule: a protected schedule offers change or remove; save is gated', () => {
  const p = scheduleSpec({ ...sv, frequency: 'daily', protectedNow: true, chosen: new Set() })
  assert.deepEqual(ids(p).filter((i) => ['changepw', 'clearpw', 'password'].includes(i)), ['changepw', 'clearpw'])
  assert.equal(p.sections.at(-1).items[0].disabled, true)
  const q = scheduleSpec({ ...sv, frequency: 'daily', lastMade: 'today', lastError: 'disk full' })
  assert.ok(q.sections.some((s) => s.h === 'The last scheduled backup failed' && s.tone))
})
