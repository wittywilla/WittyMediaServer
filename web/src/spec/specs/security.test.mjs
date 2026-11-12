// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { devicesSpec, twoFactorSpec } from './security.ts'
import { problems, withIds } from '../model.ts'

const s = (id, o = {}) => ({ id, device: 'Laptop', where: 'home', ip: '10.0.0.2', lastSeen: 1, streaming: false, current: false, ...o })

test('devices: this one marked, others get a sign-out each and Sign out the others', () => {
  const p = devicesSpec([s('a', { current: true, device: 'Mac' }), s('b'), s('c', { streaming: true, device: '' })], () => 'now')
  assert.equal(p.sections[0].items[0].value, 'This device')
  assert.equal(p.sections[0].items[0].on, true)
  assert.equal(p.sections[0].items[2].value, 'Streaming')
  assert.equal(p.sections[0].items[2].title, 'Unknown device')
  assert.deepEqual(p.sections[1].items.map((c) => c.id), ['out:b', 'out:c', 'others'])
  assert.match(p.sections[0].items[1].sub, /At home · 10\.0\.0\.2 · last used now/)
  assert.deepEqual(problems(withIds(p)), [])
})

test('devices: one other device has no "others" chip; none has no chips', () => {
  assert.deepEqual(devicesSpec([s('a', { current: true }), s('b')], () => '').sections[1].items.map((c) => c.id), ['out:b'])
  assert.equal(devicesSpec([s('a', { current: true })], () => '').sections.length, 1)
})

test('two-factor: every state is drawable and gates its primary action', () => {
  const states = [
    { mode: 'on', recoveryLeft: 1 }, { mode: 'off' }, { mode: 'turningOff', password: '', code: '' }, { mode: 'begin', password: '' },
    { mode: 'setup', code: '12', uri: 'otpauth://x', secret: 'ABCDEFGHIJKL' }, { mode: 'recovery', codes: ['a-1', 'b-2'] },
  ]
  for (const st of states) assert.deepEqual(problems(withIds(twoFactorSpec(st))), [], st.mode)
  assert.equal(twoFactorSpec(states[2]).actions[1].disabled, true)
  assert.equal(twoFactorSpec({ ...states[3], password: 'x' }).actions[0].disabled, false)
  const setup = twoFactorSpec(states[4], 'Wrong code')
  assert.equal(setup.actions[1].disabled, true)
  assert.equal(twoFactorSpec({ ...states[4], code: '123 456' }).actions[1].disabled, false)
  assert.equal(setup.sections[1].items[0].value, 'ABCD EFGH IJKL')
  assert.equal(setup.sections[2].items[0].error, 'Wrong code')
  assert.equal(twoFactorSpec(states[0]).sections[0].items[0].sub, '1 recovery code left.')
})
