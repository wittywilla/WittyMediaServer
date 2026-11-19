// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, mk, secF, secN, secP } from '../model.ts'
import type { SpecField, SpecPageData, SpecSection } from '../types.ts'
import { t, tk } from '../../sdui/words.ts'

// "Ask a parent" when screen time is up (canvas "askparent"): how much more time, then a parent's PIN or their username
// and password. Chip ids: more:15, more:30, more:60, switch. Field ids: code, name, password.
// Labels marked with tk: shown with t(label).
export const EXTRA_MINUTES = [{ value: '15', label: tk('15 minutes') }, { value: '30', label: tk('30 minutes') }, { value: '60', label: tk('1 hour') }]

export function askParentSpec(o: { more: string; byPassword: boolean; hasPin: boolean; name: string; password: string; code: string; error: string }): SpecPageData {
  const fields: SpecField[] = o.byPassword
    ? [
      { id: 'name', kind: 'text', label: t('A parent’s username'), value: o.name, autoComplete: 'off', required: true },
      { id: 'password', kind: 'password', label: t('Their password'), value: o.password, autoComplete: 'off', required: true, error: o.error || undefined },
    ]
    : [{ id: 'code', kind: 'pin', label: t('The parents’ PIN'), value: o.code, length: 12, autoComplete: 'off', required: true, error: o.error || undefined }]
  const sections: SpecSection[] = [
    secN('', t('A parent can give this profile more time today.')),
    secP(t('How much more'), EXTRA_MINUTES.map((m) => ({ ...Cc(t(m.label), m.value === o.more), id: 'more:' + m.value }))),
    secF('', fields),
  ]
  if (o.hasPin) sections.push(secP('', [{ ...Cc(o.byPassword ? t('Use the parents’ PIN instead') : t('Use a parent’s password instead')), id: 'switch' }]))
  return mk('askparent', t('Ask a parent'), t('Add more time'), '', sections)
}
