// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, Cc, mk, secF, secN, secP } from '../model.ts'
import type { SpecAction, SpecField, SpecPageData, SpecSection } from '../types.ts'

// Signing in (canvas keys "signedout", "twofactor", "pinentry"). Field ids: name, password, code. Action ids:
// signin, back, confirm.
export interface SignInState {
  brand: string; profile: string | null; kind?: 'adult' | 'kids' | 'guest'
  name: string; password: string; busy: boolean; error: string; plain: boolean; hasBack: boolean
}

export function signInSpec(s: SignInState): SpecPageData {
  const sections: SpecSection[] = []
  if (s.plain) sections.push(secN('', 'This connection isn’t encrypted. Your password can be read or changed by anyone between this device and the server. The owner allowed this. Sign in only on a network you trust.', true))
  const fields: SpecField[] = []
  if (!s.profile) fields.push({ id: 'name', kind: 'text', label: 'Username', value: s.name, autoComplete: 'username', autoFocus: true, required: true, disabled: s.busy })
  fields.push({ id: 'password', kind: 'password', label: s.profile ? (s.kind === 'adult' ? 'Password' : 'Password or PIN') : 'Password', value: s.password, autoComplete: 'current-password', autoFocus: !!s.profile, required: true, disabled: s.busy, error: s.error || undefined })
  sections.push(secF('', fields))
  sections.push(secP('', [{ ...Cc('Forgot your password?'), to: '/help/accounts#if-you-forget-your-password' }]))
  const actions: SpecAction[] = [{ ...A1(s.busy ? 'Signing in…' : 'Sign in'), id: 'signin', disabled: s.busy || !(s.profile || s.name) || !s.password }]
  if (s.hasBack) actions.push({ ...A2('Back to profiles'), id: 'back', disabled: s.busy })
  return mk('signedout', s.brand, s.profile || 'Sign in', s.profile ? 'Enter your password or PIN.' : 'Sign in to your media server.', sections, actions)
}

// The password was right and the account also wants a code.
export function codeSpec(brand: string, code: string, busy: boolean, error: string): SpecPageData {
  return mk('twofactor', brand, 'Enter your code', 'Enter the 6-digit code from your authenticator app.',
    [secF('', [{ id: 'code', kind: 'text', label: 'Code', value: code, autoComplete: 'one-time-code', autoFocus: true, required: true, disabled: busy, error: error || undefined, hint: 'Lost your phone? Enter one of your recovery codes instead.' }])],
    [{ ...A1(busy ? 'Checking…' : 'Sign in'), id: 'confirm', disabled: busy || !code.trim() }, { ...A2('Back to the password'), id: 'back', disabled: busy }])
}

// Switching profile on a signed-in device: the password or code step, inside the Dialog.
export function switchSpec(kind: string, needCode: boolean, password: string, code: string, error: string): SpecPageData {
  const f: SpecField = needCode
    ? { id: 'code', kind: 'text', label: 'Code from the authenticator app', value: code, autoComplete: 'one-time-code', autoFocus: true, required: true, error: error || undefined }
    : { id: 'password', kind: 'password', label: kind === 'adult' ? 'Password' : 'Password or PIN', value: password, autoComplete: 'current-password', autoFocus: true, required: true, error: error || undefined }
  return mk('pinentry', 'Who’s watching', 'Switch profile', '', [secF('', [f])])
}
