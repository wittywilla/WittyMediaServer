// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Whether a module works here, in words (the 0.9 plan's table of store states). Run with:
//   node --experimental-strip-types --test web/src/pages/settings/store/compat.test.mjs
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { compat, permissionsLine, strictMinimum } from './compat.ts'

const ok = { name: 'WittyStats', version: '0.3.0', fit: { ok: true, kind: 'ok' } }
const NO_JARGON = /\bAPI\b|compatib|\d+\.x\b/i

test('0.9 and later take only modules built for them', () => {
  assert.equal(strictMinimum('0.8.5-alpha'), false)
  assert.equal(strictMinimum('0.9.0-alpha'), true)
  assert.equal(strictMinimum('1.0.0'), true)
})

test('a module that fits works', () => {
  const c = compat(ok, '0.9.0-alpha')
  assert.equal(c.state, 'works'); assert.ok(c.ok)
})

test('installed and made for an older server: Needs an update when one fits, else Not ready yet', () => {
  const have = { ...ok, name: 'WittyPlay', status: { installed: true, version: '0.7.20', olderServer: true, madeFor: '0.8.x', latest: { version: '0.8.0' } } }
  const c = compat(have, '0.9.0-alpha')
  assert.equal(c.tag, 'Needs an update')
  assert.match(c.line, /The version you have \(0\.7\.20\) was made for an older WittyMediaServer, so it’s turned off\. Version 0\.8\.0 is made for this server\./)
  const none = compat({ ...have, status: { ...have.status, latest: undefined } }, '0.9.0-alpha')
  assert.equal(none.tag, 'Not ready yet'); assert.match(none.line, /nothing it saved is lost/)
  // The server's own kinds say the same, on any server.
  assert.equal(compat({ ...have, fit: { ok: false, kind: 'not-ready' } }, '0.8.5-alpha').state, 'not-ready')
  // On 0.8 it still runs.
  assert.equal(compat(have, '0.8.4-alpha').state, 'older-ok')
})

test('not installed and only made for an older server: Not available yet, never "install anyway" on 0.9', () => {
  const item = { name: 'WittyPlay', fit: { ok: false, kind: 'older-server' }, madeFor: '0.8.x' }
  assert.equal(compat(item, '0.9.0-alpha').tag, 'Not available yet')
  assert.equal(compat({ ...item, fit: { ok: false, kind: 'too-old' } }, '0.9.0-alpha').line, 'There’s no version of WittyPlay for this server yet.')
  assert.equal(compat(item, '0.8.5-alpha').state, 'older-ok')
})

test('withdrawn, installed: the reason, and security turns it off', () => {
  const w = compat({ ...ok, status: { installed: true, version: '0.3.0', withdrawn: { reason: 'it deletes saves', severity: 'broken' } } }, '0.9.0-alpha')
  assert.equal(w.tag, 'Withdrawn'); assert.match(w.line, /took WittyStats 0\.3\.0 back: it deletes saves/)
  const s = compat({ ...ok, status: { installed: true, version: '0.3.0', withdrawn: { reason: 'x', severity: 'security' } } }, '0.9.0-alpha')
  assert.match(s.line, /security problem\. It has been turned off/)
})

test('every state has a badge and a sentence, and none speaks in API numbers or ranges', () => {
  for (const kind of ['ok', 'too-old', 'too-new', 'no-build', 'no-version', 'library', 'withdrawn', 'blocked', 'older-server', 'needs-update', 'not-ready', 'unreadable', 'something-new']) {
    const c = compat({ name: 'X', version: '1.0.0', minServer: '0.9.0-alpha', fit: { ok: kind === 'ok', kind } }, '0.9.0-alpha', 'Linux ARM 64-bit')
    assert.ok(c.tag && c.line, kind)
    assert.doesNotMatch(c.tag + ' ' + c.line + ' ' + (c.todo || ''), NO_JARGON, kind)
  }
})

test('library parts and empty entries are hidden from Browse', () => {
  assert.equal(compat({ name: 'X', fit: { ok: false, kind: 'library' } }, '0.9.0-alpha').hidden, true)
  assert.equal(compat({ name: 'X', fit: { ok: false, kind: 'no-version' } }, '0.9.0-alpha').hidden, true)
  assert.ok(!compat(ok, '0.9.0-alpha').hidden)
})

test('the server’s own sentence wins', () => {
  assert.equal(compat({ name: 'X', fit: { ok: false, kind: 'too-new', why: 'Its words.' } }, '0.9.0-alpha').line, 'Its words.')
})

test('permissions in a few words', () => {
  assert.equal(permissionsLine([]), 'Asks for nothing extra')
  assert.equal(permissionsLine(['network', 'data']), 'Asks for: the internet, keep its own files')
})
