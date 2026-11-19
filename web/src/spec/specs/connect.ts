// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, A2, Cc, mk, secF, secN, secP } from '../model.ts'
import type { SpecAction, SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// Choosing a server, in the apps and the desktop shell (canvas key "connect"; the web has one server address,
// not a saved list, so "managesrv" and "unreachable" have no screen here). Field id: address. Action ids:
// connect, cancel.
export interface ConnectState { address: string; busy: boolean; error: string; insecure: boolean; hasCancel: boolean }

export function connectSpec(s: ConnectState): SpecPageData {
  const sections: SpecSection[] = []
  if (s.insecure) sections.push(secN('', t('This address isn’t encrypted. Don’t use it over the internet: your password could be read on the way. Use an https:// address or a VPN.'), true))
  sections.push(secF('', [{
    id: 'address', kind: 'text', label: t('Server address'), value: s.address, placeholder: 'https://media.example.com', autoComplete: 'url', autoFocus: true, required: true, disabled: s.busy,
    error: s.error || undefined,
    hint: t('At home, that’s the address shown in the server’s settings, such as http://192.168.1.10:9797. Away from home, it’s the address from Settings, Remote Access.'),
  }]))
  sections.push(secP('', [{ ...Cc(t('Where to find it')), to: '/help/apps' }]))
  const actions: SpecAction[] = [{ ...A1(s.busy ? t('Connecting…') : t('Connect')), id: 'connect', disabled: s.busy || !s.address.trim() }]
  if (s.hasCancel) actions.push({ ...A2(t('Cancel')), id: 'cancel', disabled: s.busy })
  return mk('connect', t('Welcome'), t('Choose your server'), t('Enter your server’s address.'), sections, actions)
}
