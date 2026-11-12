// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { LAYERS, devicesSpec, optionsSpec } from './peerToPeer.ts'
import { problems, withIds } from '../model.ts'

const d = (id, o = {}) => ({ id, name: 'Phone', platform: 'ios', how: 'invite', created: 1, lastSeen: 2, connected: false, ...o })

test('devices: empty says how to add one; otherwise a row and a Remove chip each, then the invite chip', () => {
  const none = devicesSpec([], () => 'x')
  assert.equal(none.sections[0].kind, 'note')
  assert.deepEqual(none.sections[1].items.map((c) => c.id), ['invite'])
  const p = devicesSpec([d('abcdefghijkl', { user: 'Pip', connected: true, path: 'relayed' }), d('b')], () => 'now')
  assert.equal(p.sections[0].items[0].title, 'Phone · Pip')
  assert.equal(p.sections[0].items[0].value, 'Connected through a relay')
  assert.match(p.sections[0].items[0].sub, /added now by invite · last connected now · ID ending efghijkl/)
  assert.deepEqual(p.sections[1].items.map((c) => c.id), ['remove:abcdefghijkl', 'remove:b', 'invite'])
  assert.deepEqual(problems(withIds(p)), [])
})

test('options: a switch for each way of connecting; hole punching waits for relays', () => {
  const layers = { dht: true, relay: false, holePunch: true, portMap: false, ipv6: false }
  const p = optionsSpec({ devicesOnly: true, layers, relayKbps: 4000, relayForOthers: false })
  assert.deepEqual(p.sections[1].items.map((f) => f.id), LAYERS.map((l) => 'layer:' + l.id))
  assert.equal(p.sections[1].items.find((f) => f.id === 'layer:holePunch').disabled, true)
  assert.equal(optionsSpec({ devicesOnly: true, layers: { ...layers, relay: true }, relayKbps: 0, relayForOthers: true }).sections[1].items.find((f) => f.id === 'layer:holePunch').disabled, false)
  assert.equal(p.sections[2].items[0].value, '4000')
  assert.equal(p.sections[2].items[0].options.at(-1).label, 'No limit')
  assert.deepEqual(problems(withIds(p)), [])
})
