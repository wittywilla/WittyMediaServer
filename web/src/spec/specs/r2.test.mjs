// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { problems, withIds, handledIds, reachable } from '../model.ts'
import { channelsSpec, detailsSpec, guideSpec, kindSpec, toField, trySpec } from './tunerSetup.ts'
import { guideSourcesSpec, tunersSpec } from './tuners.ts'
import { audienceSpec, basicsSpec, moreSpec } from './channelEditor.ts'

const ok = (p) => assert.deepEqual(problems(withIds(p)), [], p.title)

test('tuner setup: the kinds are a choice, unusable ones are disabled', () => {
  const p = kindSpec([{ id: 'playlist', label: 'A playlist', hint: 'M3U', usable: true }, { id: 'card', label: 'A card', hint: 'DVB', usable: false, dim: 'Not here' }], 'playlist')
  ok(p)
  const s = p.sections[0]
  assert.equal(s.choice, true)
  assert.deepEqual(s.items.map((r) => [r.id, r.on, !!r.disabled]), [['kind:playlist', true, false], ['kind:card', false, true]])
  assert.match(s.items[1].sub, /Not here/)
})

test('tuner setup: driver fields become the matching form fields', () => {
  assert.equal(toField({ key: 'a', label: 'A', kind: 'toggle' }, '1').value, true)
  assert.equal(toField({ key: 'a', label: 'A', kind: 'toggle', default: '1' }, '').value, true)
  assert.equal(toField({ key: 'p', label: 'P', kind: 'password', required: true }, 'x').label, 'P (needed)')
  assert.deepEqual(toField({ key: 's', label: 'S', kind: 'select', options: ['', 'a'] }, '').options, [{ value: '', label: 'None' }, { value: 'a', label: 'a' }])
  assert.equal(toField({ key: 'u', label: 'U', kind: 'url' }, 'http://x').kind, 'text')
})

test('tuner setup: details with found devices, a check and an error', () => {
  const d = { id: 'iptv', name: 'IPTV', available: true, description: 'A list.' }
  const p = detailsSpec({ choices: [d, { ...d, id: 'x', name: 'X', available: false, note: 'Not here' }], driver: d, fields: [{ key: 'url', label: 'Address', kind: 'url', required: true }], name: 'N', opts: { url: 'u' },
    canSearch: true, searching: false, found: [{ name: 'Box', detail: '1.2.3.4' }], missing: [], err: 'boom', check: { ok: true, text: '10 channels listed.', sample: 'A, B' } })
  ok(p)
  const handled = handledIds(p)
  assert.ok(handled.has('search'))
  assert.equal(p.sections.find((s) => s.h === 'Found').items[0].actions[0].id, 'use:0')
  assert.ok(p.sections.some((s) => s.kind === 'note' && /Such as A, B/.test(s.text)))
  assert.ok(p.sections.some((s) => s.kind === 'note' && s.text === 'boom'))
  const bad = detailsSpec({ choices: [d], driver: d, fields: [], name: '', opts: {}, canSearch: false, searching: false, found: [], missing: ['Address'], err: '', check: { ok: false, problem: { message: 'No answer', hint: 'Check it' } } })
  ok(bad)
  assert.ok(bad.sections.some((s) => s.kind === 'note' && /Still needed: Address/.test(s.text)))
  assert.ok(bad.sections.some((s) => s.kind === 'note' && /No answer Check it/.test(s.text)))
})

test('tuner setup: channels, try and guide steps', () => {
  const c = channelsSpec({ error: '', found: '12 channels found.', encrypted: true, scanNote: 'A scan', canScan: true, scanning: false, percent: 0, scanText: '', hasError: false, busy: false, zero: true })
  ok(c)
  assert.deepEqual(c.sections.at(-1).items.map((r) => r.id), ['scan', 'again'])
  assert.equal(c.sections.at(-1).items[0].on, true)
  const e = channelsSpec({ error: 'no', found: '', encrypted: false, scanNote: '', canScan: false, scanning: false, percent: 0, scanText: '', hasError: true, busy: false, zero: true })
  assert.ok(e.sections.at(-1).items.some((r) => r.id === 'change'))
  ok(trySpec({ options: [{ value: '1', label: '1 One' }], pick: '', trying: false, answer: { ok: true, title: 'One played', text: 'fast' } }))
  ok(trySpec({ options: null, pick: '', trying: true, answer: { ok: false, title: 'x', problem: { message: 'm' } } }))
  const g = guideSpec('http://g', 'err')
  ok(g)
  assert.equal(g.sections[1].items[0].id, 'guide')
})

test('tuners and guide sources: rows go to channels, buttons remove', () => {
  const p = tunersSpec([{ id: 3, name: 'Den', driverName: 'IPTV', channels: 1, clear: 1, encrypted: 0, lastError: 'late' }], [{ name: 'Box', detail: 'x', canSetUp: false }])
  ok(p)
  const row = p.sections.find((s) => s.h === 'Your tuners').items[0]
  assert.equal(row.to, 'tuner/3')
  assert.match(row.sub, /1 channel · 1 clear · late/)
  assert.equal(row.actions[0].id, 'remove:3')
  assert.equal(p.sections[0].items[0].actions, undefined)
  assert.match(tunersSpec([], null).sections[0].text, /None yet/)
  assert.match(tunersSpec([], []).sections[0].text, /No new tuners/)
  const g = guideSourcesSpec([{ id: 2, name: 'EPG', programmes: 40, read: 'today', lastError: '' }], '')
  ok(g)
  assert.deepEqual(g.sections[0].items[0].actions.map((a) => a.id), ['again:2', 'drop:2'])
  assert.equal(g.sections.at(-1).items[0].disabled, true)
  assert.ok(!reachable(withIds(g), new Set(['add'])).includes('add'))
})

test('channel editor: basics, audience and more', () => {
  ok(basicsSpec({ name: 'A', number: '', description: '', categories: '' }))
  const a = audienceSpec({ mode: 'only', countProgress: true })
  ok(a)
  assert.equal(a.sections[0].items[1].value, true)
  const m = moreSpec({ hasLogo: true, dirty: true, history: [{ id: 9, name: 'v1', when: 'today' }] })
  ok(m)
  const rows = m.sections.find((s) => s.kind === 'rows' && !s.h).items
  assert.deepEqual(rows.map((r) => [r.id, !!r.disabled]), [['removeLogo', false], ['reshuffle', false], ['reset', true]])
  assert.equal(m.sections.at(-1).items[0].actions[0].id, 'restore:9')
  assert.equal(moreSpec({ hasLogo: false, dirty: false, history: [] }).sections.filter((s) => s.kind === 'rows').length, 1)
})
