// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Overrides, release status, the store's kind and status filters, uploads and the module audit.
// Run with:
//   node --experimental-strip-types --test web/src/pages/settings/store/overrides.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  auditRows, auditWords, friction, itemReleaseStatus, needsOverrideOf, overrideLines, overrideRequest, passesExtra, readFilters,
  reasonFor, refusalKind, releaseStatus, severityOf, statusWarns, stillNeeded, toDate, uiLines, uploadView, withFilter,
} from './overrides.ts'

const answers = (o = {}) => ({ accepted: [], password: '', code: '', typed: '', grants: [], ...o })

test('friction grows with severity, and the tones are the design’s own', () => {
  assert.deepEqual([1, 2, 3, 4].map((s) => friction(s).tone), ['warn', 'warn', 'danger', 'danger'])
  assert.deepEqual([1, 2, 3, 4].map((s) => friction(s).typed), [false, false, true, true])
  assert.equal(friction(0).level, 1)
  assert.equal(friction(9).level, 4)
  assert.notEqual(friction(4).heading, friction(3).heading)
})

test('severity comes from the server, or from the kinds when it sent none', () => {
  assert.equal(severityOf({ kinds: ['incomplete'], severity: 1 }), 1)
  assert.equal(severityOf({ kinds: ['incomplete', 'unsigned'], severity: 0 }), 4)
  assert.equal(severityOf({ kinds: ['something-new'] }), 3) // unknown kinds are treated as serious
})

test('reasons are found whether they came by kind or in order', () => {
  assert.equal(reasonFor({ kinds: ['a', 'b'], reasons: ['first', 'second'] }, 'b'), 'second')
  assert.equal(reasonFor({ kinds: ['a', 'b'], reasons: { a: 'first' } }, 'a'), 'first')
  assert.equal(reasonFor({ kinds: ['a'] }, 'a'), '')
  const lines = overrideLines({ kinds: ['incomplete', 'unsigned', 'unsigned'], reasons: { unsigned: 'No signature' } })
  assert.deepEqual(lines.map((l) => l.kind), ['unsigned', 'incomplete'])
  assert.equal(lines[0].reason, 'No signature')
})

test('the button waits for every warning, the password, the code and the typed ID', () => {
  const need = { kinds: ['unsigned', 'local-file'], severity: 4 }
  assert.deepEqual(stillNeeded(need, answers(), 'wittyx', true), ['tick 2 warnings', 'type the module’s ID', 'enter your password', 'enter the code from your authenticator app'])
  assert.deepEqual(stillNeeded(need, answers({ accepted: ['unsigned', 'local-file'], typed: ' wittyx ', password: 'p', code: '1' }), 'wittyx', true), [])
  assert.deepEqual(stillNeeded(need, answers({ accepted: ['unsigned', 'local-file'], typed: 'WittyX', password: 'p' }), 'wittyx', false), ['type the module’s ID'])
  // Below 3, no ID is asked for.
  assert.deepEqual(stillNeeded({ kinds: ['incomplete'], severity: 1 }, answers({ accepted: ['incomplete'], password: 'p' }), 'm', false), [])
})

test('the request sends only what was asked about, and the password at the top', () => {
  const need = { kinds: ['unsigned'], severity: 4, permissions: ['network', 'files'] }
  const body = overrideRequest(need, answers({ accepted: ['unsigned', 'made-up'], password: 'pw', code: ' 123456 ', typed: ' wittyx ', grants: ['files', 'everything'] }))
  assert.deepEqual(body, { password: 'pw', code: '123456', override: { accept: ['unsigned'], grants: ['files'], typed: 'wittyx' } })
  const low = overrideRequest({ kinds: ['incomplete'], severity: 1 }, answers({ accepted: ['incomplete'], password: 'pw', typed: 'x' }))
  assert.equal('typed' in low.override, false)
  assert.deepEqual(low.override.grants, [])
})

test('refusals are told apart', () => {
  assert.deepEqual(needsOverrideOf({ status: 409, data: { needsOverride: { kinds: ['custom'], severity: 3 } } })?.kinds, ['custom'])
  assert.equal(needsOverrideOf({ status: 409, data: { code: 'confirm' } }), null)
  assert.equal(needsOverrideOf({ status: 409, data: { needsOverride: { kinds: [] } } }), null)
  assert.equal(refusalKind({ status: 403, data: { code: 'password-needed' } }), 'password')
  assert.equal(refusalKind({ status: 400, data: { error: 'accept names a kind that is not needed' } }), 'invalid')
  assert.equal(refusalKind({ status: 409, data: { needsOverride: { kinds: ['unsigned'] } } }), 'override')
  assert.equal(refusalKind(new Error('x')), 'other')
})

test('release status: unknown reads as the most careful', () => {
  assert.equal(releaseStatus(undefined), 'stable')
  assert.equal(releaseStatus('beta'), 'beta')
  assert.equal(releaseStatus('alpha-ish'), 'incomplete')
  assert.equal(statusWarns('experimental'), true)
  assert.equal(statusWarns('beta'), false)
  assert.equal(itemReleaseStatus({ releaseStatus: 'experimental' }), 'experimental')
  assert.equal(itemReleaseStatus({ status: { releaseStatus: 'incomplete', installed: true } }), 'incomplete')
  assert.equal(itemReleaseStatus({ status: { installed: true } }), 'stable')
})

test('the kind and status filters live in the address, and unknown values are ignored', () => {
  const p = new URLSearchParams('q=disc&moduleKind=web&status=beta')
  assert.deepEqual(readFilters(p), { moduleKind: 'web', status: 'beta' })
  assert.deepEqual(readFilters(new URLSearchParams('moduleKind=rocket&status=gold')), { moduleKind: '', status: '' })
  const n = withFilter(p, 'status', '')
  assert.equal(n.toString(), 'q=disc&moduleKind=web')
  assert.equal(p.get('status'), 'beta') // the original is untouched
  assert.equal(withFilter(n, 'moduleKind', 'helper').get('moduleKind'), 'helper')
  assert.equal(passesExtra({ moduleKind: 'web', releaseStatus: 'beta' }, { moduleKind: 'web', status: 'beta' }), true)
  assert.equal(passesExtra({ moduleKind: 'helper' }, { moduleKind: 'web', status: '' }), false)
  assert.equal(passesExtra({ moduleKind: 'helper' }, { moduleKind: '', status: 'stable' }), true)
})

test('what a module adds', () => {
  assert.equal(uiLines(undefined), null)
  assert.deepEqual(uiLines({ pages: [{ id: 'discs', title: 'Discs' }, { id: 'raw' }], navigation: ['Discs', ' '], settings: true }),
    { pages: ['Discs', 'raw'], navigation: ['Discs'], settings: true, empty: false })
  assert.equal(uiLines({}).empty, true)
})

test('an upload maps to what the dialog shows', () => {
  const need = { kinds: ['local-file', 'unsigned'], severity: 4, permissions: ['network'] }
  const v = uploadView({ token: 't', id: 'wittyx', name: 'WittyX', version: '1.0.0', delivery: 'web', api: 4, status: 'experimental', signed: false, permissions: ['network'], problems: [{ kind: 'sdui', text: 'No sdui.json' }, { kind: 'x', text: '' }], needsOverride: need })
  assert.equal(v.state, 'override')
  assert.equal(v.signature.tone, 'danger')
  assert.deepEqual(v.facts.map((f) => f.label), ['Name', 'ID', 'Version', 'Kind', 'Module API', 'Status'])
  assert.equal(v.facts.find((f) => f.label === 'Kind').value, 'Web bundle')
  assert.equal(v.facts.find((f) => f.label === 'Status').value, 'Experimental')
  assert.equal(v.problems.length, 1)
  assert.equal(v.need, need)
  assert.equal(uploadView({ token: 't', signed: true, trusted: true }).state, 'ready')
  assert.equal(uploadView({ token: 't', signed: true, trusted: true }).signature.tone, 'ok')
  assert.equal(uploadView({ token: 't', signed: true, trusted: false }).signature.tone, 'warn')
  const refused = uploadView({ token: 't', name: 'Old', refusal: 'Module API 2 is too old', needsOverride: need })
  assert.equal(refused.state, 'refused')
  assert.match(refused.title, /can’t be installed/)
})

test('dates in any of the server’s forms', () => {
  assert.equal(toDate(1700000000)?.getTime(), 1700000000000)
  assert.equal(toDate(1700000000000)?.getTime(), 1700000000000)
  assert.equal(toDate('2026-11-11T10:00:00Z')?.toISOString(), '2026-11-11T10:00:00.000Z')
  assert.equal(toDate('nonsense'), null)
  assert.equal(toDate(undefined), null)
})

test('the audit, newest first', () => {
  const rows = auditRows([{ at: 1, action: 'upload' }, { at: 3, action: 'override-removed' }, { nope: true }, { at: 2, action: 'something-new' }])
  assert.deepEqual(rows.map((r) => r.action), ['override-removed', 'something-new', 'upload'])
  assert.equal(auditRows({ entries: [{ at: 1, action: 'a' }] }).length, 1)
  assert.equal(auditRows(null).length, 0)
  assert.equal(auditWords('override-removed'), 'Removed an override')
  assert.equal(auditWords('something_new'), 'Something new')
})
