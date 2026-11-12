// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, mk, secF, secN } from '../model.ts'
import type { SpecAction, SpecField, SpecPageData, SpecSection } from '../types.ts'

// Signing a device in. Canvas keys "pairing" (the page on the phone or computer that is already signed in) and
// "phonesign" (the screen on the device being signed in). Field ids: code, who, pin, password, otp, who2, password2,
// otp2. Action ids: continue, approve, deny, getcode, home, again, back.
export const PIN_LENGTH = 6

export interface Profile { id: number; name: string; kind: string }
const label = (p: Profile) => `${p.name}${p.kind !== 'adult' ? ` (${p.kind === 'kids' ? 'Kids' : 'Guest'})` : ''}`
const options = (list: Profile[]) => list.map((p) => ({ value: String(p.id), label: label(p) }))

// Not for a child's profile.
export function pairAdultsOnlySpec(): SpecPageData {
  return mk('pairing', 'Sign in a TV', 'Sign in a TV', '', [secN('', 'Signing in a TV is for grown-ups. Ask an adult to do it from their own profile.', true)], [{ ...A1('Back to home', '/') }])
}

export function pairDoneSpec(done: { device: string; name: string } | 'denied'): SpecPageData {
  const text = done === 'denied' ? 'The sign-in was refused. The TV has been told.' : `${done.device} is now signed in as ${done.name}. It will continue by itself in a moment.`
  return mk('pairing', 'Sign in a TV', done === 'denied' ? 'Refused' : 'Signed in', '', [secN('', text, true)], [{ ...A1('Back to home', '/') }])
}

export interface AskState {
  device: string; platform: string; ip: string; profiles: Profile[]
  who: number; needsPin: boolean; pin: string; needsPassword: boolean; password: string; twoFactor: boolean; otp: string
  busy: boolean; pinError: string; error: string
}

// "Is this your TV?": which device is asking, who to sign it in as, the PIN on its screen, and an administrator's password.
export function pairAskSpec(s: AskState): SpecPageData {
  const fields: SpecField[] = []
  if (s.profiles.length > 1) fields.push({ id: 'who', kind: 'select', label: 'Sign it in as', value: String(s.who), options: options(s.profiles), disabled: s.busy })
  if (s.needsPin) {
    fields.push({ id: 'pin', kind: 'pin', label: 'PIN on the TV', value: s.pin, length: PIN_LENGTH, required: true, autoFocus: true, disabled: s.busy, error: s.pinError || undefined,
      hint: `The ${PIN_LENGTH} digits under the QR code. They change every 30 seconds, so type the ones shown now.` })
  }
  if (s.needsPassword) {
    fields.push({ id: 'password', kind: 'password', label: 'Your password', value: s.password, autoComplete: 'current-password', required: true, disabled: s.busy, autoFocus: !s.needsPin,
      error: !s.needsPin ? s.pinError || undefined : undefined,
      hint: 'This device would be an administrator’s, so your password is asked for again. Pick a Kids or Guest profile above to skip it.' })
    if (s.twoFactor) fields.push({ id: 'otp', kind: 'text', label: 'Code from your authenticator app', value: s.otp, autoComplete: 'one-time-code', disabled: s.busy, hint: 'Or one of your recovery codes.' })
  }
  const sections: SpecSection[] = [secN('', `Only continue if you are looking at that code on your own TV right now.`, true)]
  if (fields.length) sections.push(secF('', fields))
  if (s.error) sections.push(secN('', s.error, true))
  const ready = (!s.needsPin || s.pin.length === PIN_LENGTH) && (!s.needsPassword || (s.password !== '' && (!s.twoFactor || s.otp.trim() !== '')))
  const actions: SpecAction[] = [{ ...A1('Sign it in'), id: 'approve', disabled: s.busy || !ready }, { ...A2('Refuse'), id: 'deny', disabled: s.busy }]
  return mk('pairing', 'Sign in a TV', 'Is this your TV?', `${s.device}${s.platform ? ` (${s.platform})` : ''} at ${s.ip} wants to sign in to this server.`, sections, actions)
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
    secF('Your TV shows a code', [{ id: 'code', kind: 'text', label: 'Code from the TV', value: s.code, placeholder: 'ABCD-EFGH', autoFocus: true, autoComplete: 'off', disabled: s.busy, error: s.error || undefined,
      hint: 'Type the code from its screen here. You’ll see which device is asking before anything happens.' }]),
  ]
  const second: SpecField[] = []
  if (s.invite) {
    second.push({ id: 'invite', kind: 'code', label: 'Code to type on the TV', value: s.invite.code, hint: `Type this on the TV to sign it in as ${s.invite.name}. It works once, for another ${s.invite.left}.` })
  } else {
    if (s.choices.length) second.push({ id: 'who2', kind: 'select', label: 'Sign it in as', value: String(s.who), disabled: s.busy, options: [{ value: String(s.selfId), label: s.selfName }, ...options(s.choices)] })
    if (s.needsPassword) {
      second.push({ id: 'password2', kind: 'password', label: 'Your password', value: s.password, autoComplete: 'current-password', disabled: s.busy, hint: 'A code for an administrator’s account asks for your password again.' })
      if (s.twoFactor) second.push({ id: 'otp2', kind: 'text', label: 'Code from your authenticator app', value: s.otp, autoComplete: 'one-time-code', disabled: s.busy })
    }
  }
  const advice = 'Get a code here and type it on the TV. Anyone who sees it could use it, so get it when you’re ready.'
  if (second.length) sections.push(secF('Your TV asks for a code', second))
  if (!s.invite) sections.push(secN(second.length ? '' : 'Your TV asks for a code', advice))
  if (s.inviteError) sections.push(secN('', s.inviteError, true))
  if (s.fingerprint) sections.push(secN('', `This server’s fingerprint is ${s.fingerprint}. If the app shows one, the two should match.`))
  const actions: SpecAction[] = [
    { ...A1('Continue'), id: 'continue', disabled: s.busy || !s.code.trim() },
    { ...A2(s.invite ? 'Get a new code' : 'Get a code'), id: 'getcode', disabled: s.busy || (s.needsPassword && !s.invite && s.password === '') },
  ]
  return mk('pairing', 'Sign in a TV', 'Sign in a TV', 'Enter the code on your TV, or get a code to type on it.', sections, actions)
}

export interface PhoneState {
  qrText: string; address: string; local: boolean; host: string; code: string; device: string
  pin: string; pinLeft: number; codeLeft: string
}

// Signing this device in from a phone: a QR code and a PIN that changes every 30 seconds.
export function phoneSignSpec(s: PhoneState): SpecPageData {
  const how = s.local
    ? `On a device that’s already signed in, open Sign in a TV and type ${s.code}. A phone can’t use the picture: this device reached the server as ${s.host}, an address that only works here.`
    : `Scan this with your phone’s camera. Or open Sign in a TV on a device that’s already signed in, and type ${s.code}.`
  const sections: SpecSection[] = [
    secF('Scan this', [{ id: 'qr', kind: 'qr', label: 'QR code that opens the approval page', text: s.qrText, hint: 'QR code that opens the approval page' }]),
  ]
  if (s.address && !s.local) sections.push(secN('', `Or open ${s.address}`))
  sections.push(secN('How', how))
  sections.push(secF('PIN', [{ id: 'pin', kind: 'code', label: 'PIN', value: s.pin, hint: `Check it names this device, ${s.device}, then type this PIN there. A new PIN in ${s.pinLeft} s. This code lasts another ${s.codeLeft}.` }]))
  return mk('phonesign', 'Sign in with your phone', 'Scan this', 'A code or a QR with a PIN that changes every 30 seconds.', sections, [{ ...A2('Use a password instead'), id: 'back' }])
}

export function phoneEndedSpec(reason: string, why: string): SpecPageData {
  return mk('phonesign', 'Sign in with your phone', 'Sign in with your phone', '', [secN('', `${reason} ${why}`.trim(), true)],
    [{ ...A1('Get a new code'), id: 'again' }, { ...A2('Use a password instead'), id: 'back' }])
}
