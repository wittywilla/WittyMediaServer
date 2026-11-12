// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { handledIds, problems, reachable, withIds } from '../model.ts'
import { codeSpec, signInSpec, switchSpec } from './auth.ts'
import { pairAdultsOnlySpec, pairAskSpec, pairDoneSpec, pairEnterSpec, phoneEndedSpec, phoneSignSpec } from './pair.ts'
import { DAYS, shareSpec, sharedEndedSpec, sharedHeadSpec } from './share.ts'
import { isSubmitKey } from './submit.ts'
import { uploadErrorsSpec } from './upload.ts'

const ok = (p) => assert.deepEqual(problems(withIds(p)), [], p.key)
const fields = (p) => p.sections.filter((s) => s.kind === 'fields').flatMap((s) => s.items)

const ask = { device: 'Living room TV', platform: 'tvOS', ip: '192.168.1.9', profiles: [{ id: 1, name: 'Willow', kind: 'adult' }, { id: 2, name: 'Pip', kind: 'kids' }],
  who: 1, needsPin: true, pin: '', needsPassword: true, password: '', twoFactor: true, otp: '', busy: false, pinError: '', error: '' }

test('pairing: the question names the device and asks for what the server asks for', () => {
  const p = pairAskSpec(ask)
  assert.match(p.lead, /Living room TV \(tvOS\) at 192\.168\.1\.9/)
  assert.deepEqual(fields(p).map((f) => f.id), ['who', 'pin', 'password', 'otp'])
  assert.equal(fields(p)[0].options[1].label, 'Pip (Kids)')
  assert.equal(p.actions.find((a) => a.id === 'approve').disabled, true)
  ok(p)
  const ready = pairAskSpec({ ...ask, pin: '123456', password: 'x', otp: '1' })
  assert.equal(ready.actions.find((a) => a.id === 'approve').disabled, false)
  assert.ok(['approve', 'deny'].every((id) => handledIds(withIds(ready)).has(id)))
})

test('pairing: a wrong PIN is the PIN field’s error; without a PIN the password carries it', () => {
  assert.equal(fields(pairAskSpec({ ...ask, pinError: 'Wrong PIN' })).find((f) => f.id === 'pin').error, 'Wrong PIN')
  const noPin = pairAskSpec({ ...ask, needsPin: false, pinError: 'Wrong password' })
  assert.equal(fields(noPin).find((f) => f.id === 'password').error, 'Wrong password')
  assert.equal(fields(pairAskSpec({ ...ask, profiles: [ask.profiles[0]], needsPassword: false, needsPin: false })).length, 0)
})

test('pairing: busy disables the fields and the buttons', () => {
  const p = pairAskSpec({ ...ask, busy: true })
  assert.ok(fields(p).every((f) => f.disabled))
  assert.ok(p.actions.every((a) => a.disabled))
  assert.deepEqual(reachable(withIds(p), new Set(['approve', 'deny'])).filter((i) => i.startsWith('pairing.99')), [])
})

const enter = { code: '', busy: false, error: '', invite: null, inviteError: '', choices: [{ id: 2, name: 'Pip', kind: 'kids' }], selfId: 1, selfName: 'Willow', who: 1,
  needsPassword: true, password: '', twoFactor: false, otp: '', fingerprint: 'ab12' }

test('pairing: the first page takes a code, or gets one to type on the TV', () => {
  const p = pairEnterSpec(enter)
  assert.deepEqual(fields(p).map((f) => f.id), ['code', 'who2', 'password2'])
  assert.deepEqual(p.actions.map((a) => [a.id, a.disabled]), [['continue', true], ['getcode', true]])
  assert.ok(JSON.stringify(p).includes('fingerprint is ab12'))
  ok(p)
  const typed = pairEnterSpec({ ...enter, code: 'ABCD-EFGH', password: 'pw' })
  assert.deepEqual(typed.actions.map((a) => a.disabled), [false, false])
  const got = pairEnterSpec({ ...enter, invite: { code: 'WXYZ-1234', name: 'Willow', left: '4:50' } })
  assert.equal(fields(got).find((f) => f.id === 'invite').kind, 'code')
  assert.equal(got.actions[1].title, 'Get a new code')
  assert.equal(fields(got).some((f) => f.id === 'password2'), false)
  ok(got)
  assert.ok(pairEnterSpec({ ...enter, inviteError: 'Nope' }).sections.some((s) => s.kind === 'note' && s.text === 'Nope'))
})

test('pairing: results and the adults-only page', () => {
  assert.match(pairDoneSpec({ device: 'TV', name: 'Pip' }).sections[0].text, /TV is now signed in as Pip/)
  assert.match(pairDoneSpec('denied').sections[0].text, /refused/)
  assert.equal(pairAdultsOnlySpec().actions[0].to, '/')
  ok(pairDoneSpec('denied')); ok(pairAdultsOnlySpec())
})

test('phone sign-in: QR and PIN, and what to do when the address only works here', () => {
  const s = { qrText: 'https://x/pair?c=1', address: 'x/pair', local: false, host: '', code: 'ABCD-EFGH', device: 'Chrome', pin: '317 044', pinLeft: 12, codeLeft: '4:20' }
  const p = phoneSignSpec(s)
  assert.deepEqual(fields(p).map((f) => f.kind), ['qr', 'code'])
  assert.equal(fields(p)[1].value, '317 044')
  assert.match(JSON.stringify(p), /A new PIN in 12 s/)
  ok(p)
  const local = phoneSignSpec({ ...s, local: true, host: 'localhost' })
  assert.match(JSON.stringify(local), /only works here/)
  assert.equal(JSON.stringify(local).includes('Or open'), false)
  assert.deepEqual(phoneEndedSpec('Expired.', 'Try again.').actions.map((a) => a.id), ['again', 'back'])
  ok(phoneEndedSpec('Expired.', ''))
})

test('sign-in: username only without a profile; PIN allowed for kids; the error sits on the password', () => {
  const base = { brand: 'WittyMediaServer', profile: null, name: '', password: '', busy: false, error: '', plain: false, hasBack: false }
  const p = signInSpec(base)
  assert.deepEqual(fields(p).map((f) => f.id), ['name', 'password'])
  assert.equal(p.actions[0].disabled, true)
  assert.equal(signInSpec({ ...base, name: 'a', password: 'b' }).actions[0].disabled, false)
  const kid = signInSpec({ ...base, profile: 'Pip', kind: 'kids', hasBack: true, error: 'Wrong' })
  assert.deepEqual(fields(kid).map((f) => f.id), ['password'])
  assert.equal(fields(kid)[0].label, 'Password or PIN')
  assert.equal(fields(kid)[0].error, 'Wrong')
  assert.deepEqual(kid.actions.map((a) => a.id), ['signin', 'back'])
  assert.equal(kid.title, 'Pip')
  assert.ok(signInSpec({ ...base, plain: true }).sections[0].text.includes('isn’t encrypted'))
  ok(p); ok(kid)
})

test('sign-in: the code step, and switching profile', () => {
  const c = codeSpec('B', '', false, '')
  assert.equal(c.actions[0].disabled, true)
  assert.equal(codeSpec('B', '123456', true, 'x').actions[0].title, 'Checking…')
  ok(c)
  assert.equal(fields(switchSpec('adult', false, '', '', ''))[0].label, 'Password')
  assert.equal(fields(switchSpec('guest', false, '', '', ''))[0].label, 'Password or PIN')
  assert.equal(fields(switchSpec('adult', true, '', '1', 'bad'))[0].error, 'bad')
  ok(switchSpec('adult', true, '', '', ''))
})

test('share: a link inside, and one for someone without an account, shown once', () => {
  const s = { title: 'Dawn', internal: 'http://s/photo/1', allowed: true, link: '', days: 30, download: false, busy: false, error: '' }
  const p = shareSpec(s)
  assert.equal(p.title, 'Share “Dawn”')
  assert.deepEqual(fields(p).map((f) => f.id), ['days', 'download'])
  assert.equal(fields(p)[0].options.length, DAYS.length)
  assert.deepEqual(p.actions.map((a) => a.id), ['make'])
  ok(p)
  const made = shareSpec({ ...s, link: 'http://s/s/tok' })
  assert.equal(made.actions.length, 0)
  assert.ok(JSON.stringify(made).includes('only time the link is shown'))
  assert.equal(shareSpec({ ...s, allowed: false }).actions.length, 0)
  assert.equal(shareSpec({ ...s, allowed: false }).sections.some((x) => x.h === 'With someone without an account'), false)
  assert.equal(shareSpec({ ...s, busy: true }).actions[0].title, 'Making…')
  ok(made)
})

test('shared page and upload', () => {
  assert.equal(sharedHeadSpec('Dawn', '30 Nov').lead, 'Until 30 Nov')
  assert.equal(sharedHeadSpec('Dawn', null).lead, '')
  assert.match(sharedEndedSpec().title, /ended/)
  const u = uploadErrorsSpec([{ name: 'a.exe', error: 'not a photo' }, { name: 'b.txt' }])
  assert.equal(u.title, '2 files not added')
  assert.equal(u.sections[0].items[0].sub, 'not a photo')
  assert.equal(uploadErrorsSpec([{ name: 'a' }]).title, '1 file not added')
  ok(u); ok(sharedHeadSpec('x', null)); ok(sharedEndedSpec())
})

test('Enter submits from text fields only', () => {
  const key = (tagName, type, extra = {}) => ({ key: 'Enter', defaultPrevented: false, target: { tagName, type }, ...extra })
  assert.equal(isSubmitKey(key('INPUT', 'text')), true)
  assert.equal(isSubmitKey(key('INPUT', 'password')), true)
  assert.equal(isSubmitKey(key('SELECT')), false)
  assert.equal(isSubmitKey(key('INPUT', 'file')), false)
  assert.equal(isSubmitKey(key('BUTTON')), false)
  assert.equal(isSubmitKey(key('INPUT', 'text', { defaultPrevented: true })), false)
  assert.equal(isSubmitKey({ ...key('INPUT', 'text'), key: 'a' }), false)
})
