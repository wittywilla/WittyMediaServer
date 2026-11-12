// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secF, secN, secP } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'

// "Ask a parent" when screen time is up (canvas "askparent"): how much more time, then a parent's PIN or their username
// and password. Chip ids: more:15, more:30, more:60, switch. Field ids: code, name, password.
export const EXTRA_MINUTES = [{ value: '15', label: '15 minutes' }, { value: '30', label: '30 minutes' }, { value: '60', label: '1 hour' }]

export function askParentSpec(o: { more: string; byPassword: boolean; hasPin: boolean; name: string; password: string; code: string; error: string }): SpecPageData {
  const fields: SpecField[] = o.byPassword
    ? [
      { id: 'name', kind: 'text', label: 'A parent’s username', value: o.name, autoComplete: 'off', required: true },
      { id: 'password', kind: 'password', label: 'Their password', value: o.password, autoComplete: 'off', required: true, error: o.error || undefined },
    ]
    : [{ id: 'code', kind: 'pin', label: 'The parents’ PIN', value: o.code, length: 12, autoComplete: 'off', required: true, error: o.error || undefined }]
  const sections: SpecSection[] = [
    secN('', 'A parent can give this profile more time today.'),
    secP('How much more', EXTRA_MINUTES.map((m) => ({ ...Cc(m.label, m.value === o.more), id: 'more:' + m.value }))),
    secF('', fields),
  ]
  if (o.hasPin) sections.push(secP('', [{ ...Cc(o.byPassword ? 'Use the parents’ PIN instead' : 'Use a parent’s password instead'), id: 'switch' }]))
  return mk('askparent', 'Ask a parent', 'Add more time', '', sections)
}
