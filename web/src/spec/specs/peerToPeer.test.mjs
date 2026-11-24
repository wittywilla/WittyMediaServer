// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { LAYERS, devicesSpec, optionsSpec } from './peerToPeer.ts'
import { problems, withIds } from '../model.ts'

const d = (id, o = {}) => ({ id, name: 'Phone', platform: 'ios', how: 'invite', created: 1, lastSeen: 2, connected: false, ...o })

test('devices: empty says how to add one; otherwise a row each with its own Remove button in it (new ones come in by Connect a device)', () => {
  const none = devicesSpec([], () => 'x')
  assert.equal(none.sections[0].kind, 'note')
  assert.equal(none.sections.length, 1)
  const p = devicesSpec([d('abcdefghijkl', { user: 'Pip', connected: true, path: 'relayed' }), d('b')], () => 'now')
  assert.equal(p.sections.length, 1, 'no section of buttons after the rows, where one reads as the next heading\'s')
  assert.equal(p.sections[0].items[0].title, 'Phone · Pip')
  assert.equal(p.sections[0].items[0].value, 'Connected through a relay')
  assert.match(p.sections[0].items[0].sub, /added now by invite · last connected now · ID ending efghijkl/)
  assert.deepEqual(p.sections[0].items.map((r) => r.actions.map((a) => a.id)), [['remove:abcdefghijkl'], ['remove:b']])
  assert.equal(p.sections[0].items[0].actions[0].label, 'Remove Phone')
  assert.equal(p.sections[0].items[0].actions[0].danger, true)
  assert.deepEqual(problems(withIds(p)), [])
})

test('options: a switch for each way of connecting; hole punching waits for relays', () => {
  const layers = { dht: true, relay: false, holePunch: true, portMap: false, ipv6: false }
  const p = optionsSpec({ devicesOnly: true, layers, relayKbps: 4000, relayForOthers: false, shareKbps: 8000, shareMaxHeight: 720 })
  assert.deepEqual(p.sections[1].items.map((f) => f.id), LAYERS.map((l) => 'layer:' + l.id))
  assert.equal(p.sections[1].items.find((f) => f.id === 'layer:holePunch').disabled, true)
  assert.equal(optionsSpec({ devicesOnly: true, layers: { ...layers, relay: true }, relayKbps: 0, relayForOthers: true, shareKbps: 8000, shareMaxHeight: 720 }).sections[1].items.find((f) => f.id === 'layer:holePunch').disabled, false)
  assert.equal(p.sections[2].items[0].value, '4000')
  assert.equal(p.sections[2].items[0].options.at(-1).label, 'No limit')
  assert.deepEqual(problems(withIds(p)), [])
})

test('options: what is shared as a relay shows only while sharing, and says it is told first', () => {
  const layers = { dht: true, relay: true, holePunch: true, portMap: true, ipv6: true }
  const off = optionsSpec({ devicesOnly: true, layers, relayKbps: 4000, relayForOthers: false, shareKbps: 8000, shareMaxHeight: 720 })
  assert.deepEqual(off.sections[2].items.map((f) => f.id), ['relayKbps', 'relayForOthers'])
  const on = optionsSpec({ devicesOnly: true, layers, relayKbps: 4000, relayForOthers: true, shareKbps: 8000, shareMaxHeight: 720 })
  assert.deepEqual(on.sections[2].items.map((f) => f.id), ['relayKbps', 'relayForOthers', 'shareKbps', 'shareMaxHeight'])
  assert.equal(on.sections[2].items[3].value, '720')
  assert.match(on.sections[2].items[2].hint, /told this before/)
  assert.deepEqual(problems(withIds(on)), [])
})
