// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecChip, SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t, tk, tn } from '../../sdui/words.ts'

// Settings, Sign-in and security: the devices signed in (canvas "devices"), and two-factor sign-in (canvas
// "twofactor" for the code field). Real data from GET /sessions and GET /me/2fa.
export interface SessionLine { id: string; device: string; where: string; ip: string; lastSeen: number; streaming: boolean; current: boolean }

const WHERE: Record<string, string> = { machine: tk('This computer'), home: tk('At home'), vpn: tk('Away, through your VPN'), internet: tk('Away, over the internet') }

// Ids: out:<session id> signs one device out; others signs out all but this one.
export function devicesSpec(list: SessionLine[], fmtWhen: (at: number) => string): SpecPageData {
  const rows: SpecRow[] = list.map((s) => {
    const where = WHERE[s.where] ? t(WHERE[s.where]) : t('Not used since signing in')
    return {
      ...R(s.device || t('Unknown device'), s.current ? t('This device') : s.streaming ? t('Streaming') : '', s.current,
        s.ip ? t('{where} · {address} · last used {time}', { where, address: s.ip, time: fmtWhen(s.lastSeen) }) : t('{where} · last used {time}', { where, time: fmtWhen(s.lastSeen) })),
      id: 'device:' + s.id,
    }
  })
  const others = list.filter((s) => !s.current)
  const chips: SpecChip[] = others.map((s) => ({ ...Cc(s.device ? t('Sign out {device}', { device: s.device }) : t('Sign out unknown device')), id: 'out:' + s.id }))
  if (others.length > 1) chips.push({ ...Cc(t('Sign out the others'), true), id: 'others' })
  const sections: SpecSection[] = [secR('', rows)]
  if (chips.length) sections.push(secP('', chips))
  return mk('devices', t('You · Devices'), t('Your devices'), t('Last used and sign out.'), sections)
}

// Two-factor sign-in, as one of its states. Field ids: password, code. Action ids: start, confirm, cancel, off,
// keepOn, turnOff, saved, copy.
export type TwoFactorState =
  | { mode: 'on'; recoveryLeft: number }
  | { mode: 'off' }
  | { mode: 'turningOff'; password: string; code: string }
  | { mode: 'begin'; password: string }
  | { mode: 'setup'; code: string; uri: string; secret: string }
  | { mode: 'recovery'; codes: string[] }

export function twoFactorSpec(st: TwoFactorState, error = ''): SpecPageData {
  const sections: SpecSection[] = []
  const actions: SpecPageData['actions'] = []
  if (st.mode === 'recovery') {
    sections.push(secN(t('Save these recovery codes now'), t('Each works once, in place of a code, if you lose your phone. They won’t be shown again.'), true))
    sections.push(secF('', st.codes.map((c, i) => ({ id: 'rc' + i, kind: 'code' as const, label: t('Recovery code {number}', { number: i + 1 }), value: c }))))
    actions.push({ id: 'copy', title: t('Copy them'), kind: 'secondary' }, { id: 'saved', title: t('I saved them'), kind: 'primary' })
  } else if (st.mode === 'on') {
    sections.push(secR('', [{ ...R(t('On for your account'), '', true, tn('{count} recovery code left.', '{count} recovery codes left.', st.recoveryLeft)), id: 'status' }]))
    actions.push({ id: 'off', title: t('Turn off'), kind: 'secondary' })
  } else if (st.mode === 'turningOff') {
    sections.push(secF('', [
      { id: 'password', kind: 'password', label: t('Your password'), value: st.password, autoComplete: 'current-password', required: true },
      { id: 'code', kind: 'text', label: t('A code from your app, or a recovery code'), value: st.code, autoComplete: 'one-time-code', required: true },
    ]))
    actions.push({ id: 'keepOn', title: t('Keep it on'), kind: 'secondary' }, { id: 'turnOff', title: t('Turn off'), kind: 'primary', disabled: !st.password || !st.code })
  } else if (st.mode === 'begin') {
    sections.push(secF('', [{ id: 'password', kind: 'password', label: t('Your password, to begin'), value: st.password, autoComplete: 'current-password', required: true }]))
    actions.push({ id: 'start', title: t('Set up'), kind: 'primary', disabled: !st.password })
  } else if (st.mode === 'setup') {
    sections.push(secF(t('Scan this'), [{ id: 'qr', kind: 'qr', label: t('QR code for your authenticator app'), text: st.uri, hint: t('QR code for your authenticator app') }]))
    sections.push(secF(t('Or type the key'), [{ id: 'key', kind: 'code', label: t('Key'), value: st.secret.replace(/(.{4})/g, '$1 ').trim() }]))
    sections.push(secF(t('Enter the 6 digits it shows'), [{ id: 'code', kind: 'pin', label: t('Code from your app'), value: st.code, length: 6, required: true, error: error || undefined }]))
    sections.push(secN('', t('Other devices signed in to your account are signed out when this turns on.')))
    actions.push({ id: 'cancel', title: t('Cancel'), kind: 'secondary' }, { id: 'confirm', title: t('Turn on'), kind: 'primary', disabled: st.code.replace(/\s/g, '').length !== 6 })
  }
  return mk('twofactor', t('Settings · Security'), t('Two-factor sign-in'), '', sections, actions)
}
