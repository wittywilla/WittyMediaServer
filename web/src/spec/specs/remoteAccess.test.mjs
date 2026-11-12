// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { advancedSpec, chooserSpec, limitsSpec, outsideSpec, protectionSpec } from './remoteAccess.ts'
import { problems, withIds } from '../model.ts'

test('chooser: one marked, home locked when the server forces access open, all busy-disabled while saving', () => {
  const p = chooserSpec('private', true, false)
  const rows = p.sections[0].items
  assert.deepEqual(rows.map((r) => [r.id, r.on, !!r.disabled]), [['choice:home', false, true], ['choice:private', true, false], ['choice:internet', false, false]])
  assert.match(rows[0].sub, /Can’t be chosen here/)
  assert.ok(chooserSpec('home', false, true).sections[0].items.every((r) => r.disabled))
  assert.deepEqual(problems(withIds(p)), [])
})

test('limits: odd saved values do not break the selects', () => {
  const f = limitsSpec(1080, '6M').sections[0].items
  assert.equal(f[0].value, '1080'); assert.equal(f[1].value, '6M')
  assert.equal(f[1].options.find((o) => o.value === '6M').label, '6 Mb/s')
  assert.equal(f[1].options[0].label, 'No limit')
})

test('outside: rows, a sign-out chip for each other device, or "Nobody yet."', () => {
  const via = (w, v) => v
  const s = (id, o = {}) => ({ id, userName: 'Pip', device: 'Phone', where: 'internet', via: 'https', ip: '1.2.3.4', lastSeen: 0, streaming: false, current: false, ...o })
  const p = outsideSpec([s('a'), s('b', { current: true, streaming: true, device: '' })], via, () => 'now')
  assert.equal(p.sections[0].items[0].sub, 'Internet · https · 1.2.3.4 · now')
  assert.equal(p.sections[0].items[1].value, 'This device · Streaming')
  assert.deepEqual(p.sections[1].items.map((c) => [c.id, c.title]), [['out:a', 'Sign Pip out on Phone']])
  const none = outsideSpec([], via, () => '')
  assert.equal(none.sections[0].kind, 'note'); assert.equal(none.sections[0].text, 'Nobody yet.')
  assert.deepEqual(problems(withIds(p)), [])
})

test('protection: weak passwords flagged, lockouts listed with a way to clear them', () => {
  const base = { failures: 3, lockouts: [], weak: [], unknown: [], adminsWithout2fa: 0, fmtTime: (t) => 't' + t }
  const calm = protectionSpec(base)
  assert.equal(calm.sections.length, 1)
  assert.equal(calm.sections[0].items[1].value, 'All fine')
  assert.equal(calm.sections[0].items[2].value, 'On for administrators')
  const p = protectionSpec({ ...base, weak: ['pip'], unknown: ['sam'], adminsWithout2fa: 1, lockouts: [{ kind: 'account', name: 'pip', until: 5, fails: 4 }, { kind: 'address', addr: '9.9.9.9', until: 6, fails: 9 }] })
  assert.equal(p.sections[0].items[1].value, 'Weak: pip'); assert.equal(p.sections[0].items[1].on, true)
  assert.equal(p.sections[0].items[2].value, 'Set it up')
  assert.match(p.sections[1].items[0].title, /“pip” can't sign in from outside your home until t5/)
  assert.deepEqual(p.sections[2].items.map((c) => c.id), ['clear'])
  assert.equal(protectionSpec({ ...base, unknown: ['sam'] }).sections[0].items[1].value, 'Not checked yet: sam')
  assert.deepEqual(problems(withIds(p)), [])
})

test('advanced: Save only when something changed', () => {
  const v = { publicUrl: '', trustedProxies: '', homeNets: '', awayNets: '' }
  assert.equal(advancedSpec(v, false, false).actions[0].disabled, true)
  assert.equal(advancedSpec(v, true, false).actions[0].disabled, false)
  assert.equal(advancedSpec(v, true, true).actions[0].disabled, true)
  assert.deepEqual(advancedSpec(v, true, false).sections[0].items.map((f) => f.id), ['publicUrl', 'trustedProxies', 'homeNets', 'awayNets'])
})
