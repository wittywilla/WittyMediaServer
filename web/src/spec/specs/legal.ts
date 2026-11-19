// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { anchored, mk, secF, secN, secR } from '../model.ts'
import type { SpecPageData } from '../types.ts'
import { t, tk } from '../../sdui/words.ts'

// Settings, Legal (canvas page "legal"): the owner's own words around the licence text, which stays the page's own prose
// (internal/legal/legal.json, with its links). Field id: contact. The notice is a multi-line box the page draws
// itself under the first section of ownerFormSpec.
export interface OwnerWords { notice: string; contact: string }

export function ownerNoticeSpec(o: OwnerWords | null): SpecPageData | null {
  if (!o || (!o.notice && !o.contact)) return null
  return mk('legal.owner', t('Settings · Legal'), t('From this server’s owner'), '', [
    anchored(secN(t('From this server’s owner'), t('Written by whoever runs this server, not by the WittyMediaServer project.')), 'this-server'),
    ...(o.notice ? [secN('', o.notice)] : []),
    ...(o.contact ? [secR('', [{ title: t('Contact'), value: o.contact }])] : []),
  ])
}

// Marked with tk: show it with t(NOTICE_HELP).
export const NOTICE_HELP = tk('For example, house rules, who may use this server, or how to reach you about something on it. Rules that apply to you depend on where you live.')
export function ownerFormSpec(contact: string): SpecPageData {
  return mk('legal.form', t('Settings · Legal'), t('Your notice for people using this server'), '', [
    anchored(secN(t('Your notice for people using this server'), t('Only you see this form. What you write appears at the top of this page for everyone, including people who aren’t signed in, so don’t write anything you want kept private. It’s shown as plain text.')), 'owner-form'),
    secF('', [{ id: 'contact', kind: 'text', label: t('Contact'), value: contact, autoComplete: 'off', hint: t('An address or other way to reach you. Optional.') }]),
  ])
}
