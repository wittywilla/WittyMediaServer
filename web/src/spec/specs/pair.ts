// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, mk, secF, secN } from '../model.ts'
import type { SpecAction, SpecField, SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// Signing a device in. Canvas keys "pairing" (the page on the phone or computer that is already signed in) and
// "phonesign" (the screen on the device being signed in). Field ids: code, who, pin, password, otp, who2, password2,
// otp2. Action ids: continue, approve, deny, getcode, home, again, back.
export const PIN_LENGTH = 6

export interface Profile { id: number; name: string; kind: string }
const label = (p: Profile) => (p.kind === 'adult' ? p.name : p.kind === 'kids' ? t('{name} (Kids)', { name: p.name }) : t('{name} (Guest)', { name: p.name }))
const options = (list: Profile[]) => list.map((p) => ({ value: String(p.id), label: label(p) }))

// Not for a child's profile.
export function pairAdultsOnlySpec(): SpecPageData {
  return mk('pairing', t('Sign in a TV'), t('Sign in a TV'), '', [secN('', t('Signing in a TV is for grown-ups. Ask an adult to do it from their own profile.'), true)], [{ ...A1(t('Back to home'), '/') }])
}

export function pairDoneSpec(done: { device: string; name: string } | 'denied'): SpecPageData {
  const text = done === 'denied' ? t('The sign-in was refused. The TV has been told.') : t('{device} is now signed in as {name}. It will continue by itself in a moment.', { device: done.device, name: done.name })
  return mk('pairing', t('Sign in a TV'), done === 'denied' ? t('Refused') : t('Signed in'), '', [secN('', text, true)], [{ ...A1(t('Back to home'), '/') }])
}

export interface AskState {
  device: string; platform: string; ip: string; profiles: Profile[]
  who: number; needsPin: boolean; pin: string; needsPassword: boolean; password: string; twoFactor: boolean; otp: string
  busy: boolean; pinError: string; error: string
}

// "Is this your TV?": which device is asking, who to sign it in as, the PIN on its screen, and an administrator's password.
export function pairAskSpec(s: AskState): SpecPageData {
  const fields: SpecField[] = []
  if (s.profiles.length > 1) fields.push({ id: 'who', kind: 'select', label: t('Sign it in as'), value: String(s.who), options: options(s.profiles), disabled: s.busy })
  if (s.needsPin) {
    fields.push({ id: 'pin', kind: 'pin', label: t('PIN on the TV'), value: s.pin, length: PIN_LENGTH, required: true, autoFocus: true, disabled: s.busy, error: s.pinError || undefined,
      hint: t('The {count} digits under the QR code. They change every 30 seconds, so type the ones shown now.', { count: PIN_LENGTH }) })
  }
  if (s.needsPassword) {
    fields.push({ id: 'password', kind: 'password', label: t('Your password'), value: s.password, autoComplete: 'current-password', required: true, disabled: s.busy, autoFocus: !s.needsPin,
      error: !s.needsPin ? s.pinError || undefined : undefined,
      hint: t('This device would be an administrator’s, so your password is asked for again. Pick a Kids or Guest profile above to skip it.') })
    if (s.twoFactor) fields.push({ id: 'otp', kind: 'text', label: t('Code from your authenticator app'), value: s.otp, autoComplete: 'one-time-code', disabled: s.busy, hint: t('Or one of your recovery codes.') })
  }
  const sections: SpecSection[] = [secN('', t('Only continue if you are looking at that code on your own TV right now.'), true)]
  if (fields.length) sections.push(secF('', fields))
  if (s.error) sections.push(secN('', s.error, true))
  const ready = (!s.needsPin || s.pin.length === PIN_LENGTH) && (!s.needsPassword || (s.password !== '' && (!s.twoFactor || s.otp.trim() !== '')))
  const actions: SpecAction[] = [{ ...A1(t('Sign it in')), id: 'approve', disabled: s.busy || !ready }, { ...A2(t('Refuse')), id: 'deny', disabled: s.busy }]
  const lead = s.platform ? t('{device} ({platform}) at {address} wants to sign in to this server.', { device: s.device, platform: s.platform, address: s.ip }) : t('{device} at {address} wants to sign in to this server.', { device: s.device, address: s.ip })
  return mk('pairing', t('Sign in a TV'), t('Is this your TV?'), lead, sections, actions)
}

export interface EnterState {
  code: string; busy: boolean; error: string
  invite: { code: string; name: string; left: string } | null; inviteError: string
  choices: Profile[]; selfId: number; selfName: string; who: number
  needsPassword: boolean; password: string; twoFactor: boolean; otp: string
  fingerprint: string
}

// The page before a TV is asked about: type the code the TV shows, or get a code here to type on the TV.
export function pairEnterSpec(s: EnterState): SpecPageData {
  const sections: SpecSection[] = [
    secF(t('Your TV shows a code'), [{ id: 'code', kind: 'text', label: t('Code from the TV'), value: s.code, placeholder: 'ABCD-EFGH', autoFocus: true, autoComplete: 'off', disabled: s.busy, error: s.error || undefined,
      hint: t('Type the code from its screen here. You’ll see which device is asking before anything happens.') }]),
  ]
  const second: SpecField[] = []
  if (s.invite) {
    second.push({ id: 'invite', kind: 'code', label: t('Code to type on the TV'), value: s.invite.code, hint: t('Type this on the TV to sign it in as {name}. It works once, for another {time}.', { name: s.invite.name, time: s.invite.left }) })
  } else {
    if (s.choices.length) second.push({ id: 'who2', kind: 'select', label: t('Sign it in as'), value: String(s.who), disabled: s.busy, options: [{ value: String(s.selfId), label: s.selfName }, ...options(s.choices)] })
    if (s.needsPassword) {
      second.push({ id: 'password2', kind: 'password', label: t('Your password'), value: s.password, autoComplete: 'current-password', disabled: s.busy, hint: t('A code for an administrator’s account asks for your password again.') })
      if (s.twoFactor) second.push({ id: 'otp2', kind: 'text', label: t('Code from your authenticator app'), value: s.otp, autoComplete: 'one-time-code', disabled: s.busy })
    }
  }
  const advice = t('Get a code here and type it on the TV. Anyone who sees it could use it, so get it when you’re ready.')
  if (second.length) sections.push(secF(t('Your TV asks for a code'), second))
  if (!s.invite) sections.push(secN(second.length ? '' : t('Your TV asks for a code'), advice))
  if (s.inviteError) sections.push(secN('', s.inviteError, true))
  if (s.fingerprint) sections.push(secN('', t('This server’s fingerprint is {fingerprint}. If the app shows one, the two should match.', { fingerprint: s.fingerprint })))
  const actions: SpecAction[] = [
    { ...A1(t('Continue')), id: 'continue', disabled: s.busy || !s.code.trim() },
    { ...A2(s.invite ? t('Get a new code') : t('Get a code')), id: 'getcode', disabled: s.busy || (s.needsPassword && !s.invite && s.password === '') },
  ]
  return mk('pairing', t('Sign in a TV'), t('Sign in a TV'), t('Enter the code on your TV, or get a code to type on it.'), sections, actions)
}

export interface PhoneState {
  qrText: string; address: string; local: boolean; host: string; code: string; device: string
  pin: string; pinLeft: number; codeLeft: string
}

// Signing this device in from a phone: a QR code and a PIN that changes every 30 seconds.
export function phoneSignSpec(s: PhoneState): SpecPageData {
  const how = s.local
    ? t('On a device that’s already signed in, open Sign in a TV and type {code}. A phone can’t use the picture: this device reached the server as {host}, an address that only works here.', { code: s.code, host: s.host })
    : t('Scan this with your phone’s camera. Or open Sign in a TV on a device that’s already signed in, and type {code}.', { code: s.code })
  const sections: SpecSection[] = [
    secF(t('Scan this'), [{ id: 'qr', kind: 'qr', label: t('QR code that opens the approval page'), text: s.qrText, hint: t('QR code that opens the approval page') }]),
  ]
  if (s.address && !s.local) sections.push(secN('', t('Or open {address}', { address: s.address })))
  sections.push(secN(t('How'), how))
  sections.push(secF(t('PIN'), [{ id: 'pin', kind: 'code', label: t('PIN'), value: s.pin, hint: t('Check it names this device, {device}, then type this PIN there. A new PIN in {seconds} s. This code lasts another {time}.', { device: s.device, seconds: s.pinLeft, time: s.codeLeft }) }]))
  return mk('phonesign', t('Sign in with your phone'), t('Scan this'), t('A code or a QR with a PIN that changes every 30 seconds.'), sections, [{ ...A2(t('Use a password instead')), id: 'back' }])
}

export function phoneEndedSpec(reason: string, why: string): SpecPageData {
  return mk('phonesign', t('Sign in with your phone'), t('Sign in with your phone'), '', [secN('', `${reason} ${why}`.trim(), true)],
    [{ ...A1(t('Get a new code')), id: 'again' }, { ...A2(t('Use a password instead')), id: 'back' }])
}
