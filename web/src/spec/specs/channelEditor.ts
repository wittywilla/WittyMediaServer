// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// The channel editor's fixed parts (canvas "channeled"): the channel's name and number, who sees it, and the part with
// the logo, a new schedule, reset and earlier versions. The rules, timetable and bumper editors stay their own.
// Field ids: name, number, description, categories, mode, count, logo. Row action ids: removeLogo, reshuffle, reset,
// restore:<id>.
export interface BasicsState { name: string; number: string; description: string; categories: string }

export function basicsSpec(s: BasicsState): SpecPageData {
  return mk('channeled', t('Settings · Channels'), t('The channel'), '', [secF('', [
    { id: 'name', kind: 'text', label: t('Name'), value: s.name, placeholder: t('Saturday Morning') },
    { id: 'number', kind: 'text', label: t('Number'), value: s.number, placeholder: t('Next free') },
    { id: 'description', kind: 'text', label: t('Description'), value: s.description },
    { id: 'categories', kind: 'text', label: t('Categories in the guide'), value: s.categories, placeholder: t('Movies, Kids, Late night'), hint: t('Separated by commas. Leave it empty and the guide works some out, such as Movies and Kids.') },
  ])])
}

export interface AudienceState { mode: '' | 'adults' | 'only'; countProgress: boolean }

export function audienceSpec(s: AudienceState): SpecPageData {
  return mk('channeled', t('Settings · Channels'), t('Who sees it'), '', [
    secF('', [
      { id: 'mode', kind: 'select', label: t('Shown to'), value: s.mode, options: [{ value: '', label: t('Everyone') }, { value: 'adults', label: t('Adults only') }, { value: 'only', label: t('Chosen profiles') }],
        hint: t('A profile’s own limits always apply. Everyone on a channel shares one stream, so a channel with anything a Kids profile may not see is hidden from that profile, not filtered.') },
      { id: 'count', kind: 'switch', label: t('Count towards “watched” and Continue Watching'), value: s.countProgress },
    ]),
  ])
}

export interface MoreState { hasLogo: boolean; dirty: boolean; history: { id: number; name: string; when: string }[] }

export function moreSpec(s: MoreState): SpecPageData {
  const sections: SpecSection[] = [secF('', [{ id: 'logo', kind: 'file', label: t('Upload your own logo'), accept: 'image/png,image/jpeg' }])]
  const rows: SpecRow[] = []
  if (s.hasLogo) rows.push({ id: 'removeLogo', title: t('Remove the logo') })
  rows.push({ id: 'reshuffle', title: t('Make a new schedule') })
  rows.push({ id: 'reset', title: t('Reset channel'), disabled: s.dirty, sub: s.dirty ? t('Save your changes first: a reset uses the saved settings.') : t('Rebuilds the schedule now, from these settings and the files that match them, instead of after what’s on now.') })
  sections.push(secR('', rows))
  sections.push(secN('', t('No logos come with the server. Use only pictures you have the right to use.')))
  if (s.history.length) sections.push(secR(t('Earlier versions ({count})', { count: s.history.length }), s.history.map((h) => ({ id: 'ver:' + h.id, title: h.name, sub: h.when, actions: [{ id: 'restore:' + h.id, title: t('Go back to this'), label: t('Go back to {name}, {when}', { name: h.name, when: h.when }) }] }))))
  return mk('channeled', t('Settings · Channels'), t('More'), '', sections)
}
