// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secR } from '../model.ts'
import type { SpecPageData, SpecRow, SpecSection } from '../types.ts'

// The channel editor's fixed parts (canvas "channeled"): the channel's name and number, who sees it, and the part with
// the logo, a new schedule, reset and earlier versions. The rules, timetable and bumper editors stay their own.
// Field ids: name, number, description, categories, mode, count, logo. Row action ids: removeLogo, reshuffle, reset,
// restore:<id>.
export interface BasicsState { name: string; number: string; description: string; categories: string }

export function basicsSpec(s: BasicsState): SpecPageData {
  return mk('channeled', 'Settings · Channels', 'The channel', '', [secF('', [
    { id: 'name', kind: 'text', label: 'Name', value: s.name, placeholder: 'Saturday Morning' },
    { id: 'number', kind: 'text', label: 'Number', value: s.number, placeholder: 'Next free' },
    { id: 'description', kind: 'text', label: 'Description', value: s.description },
    { id: 'categories', kind: 'text', label: 'Categories in the guide', value: s.categories, placeholder: 'Movies, Kids, Late night', hint: 'Separated by commas. Leave it empty and the guide works some out, such as Movies and Kids.' },
  ])])
}

export interface AudienceState { mode: '' | 'adults' | 'only'; countProgress: boolean }

export function audienceSpec(s: AudienceState): SpecPageData {
  return mk('channeled', 'Settings · Channels', 'Who sees it', '', [
    secF('', [
      { id: 'mode', kind: 'select', label: 'Shown to', value: s.mode, options: [{ value: '', label: 'Everyone' }, { value: 'adults', label: 'Adults only' }, { value: 'only', label: 'Chosen profiles' }],
        hint: 'A profile’s own limits always apply. Everyone on a channel shares one stream, so a channel with anything a Kids profile may not see is hidden from that profile, not filtered.' },
      { id: 'count', kind: 'switch', label: 'Count towards “watched” and Continue Watching', value: s.countProgress },
    ]),
  ])
}

export interface MoreState { hasLogo: boolean; dirty: boolean; history: { id: number; name: string; when: string }[] }

export function moreSpec(s: MoreState): SpecPageData {
  const sections: SpecSection[] = [secF('', [{ id: 'logo', kind: 'file', label: 'Upload your own logo', accept: 'image/png,image/jpeg' }])]
  const rows: SpecRow[] = []
  if (s.hasLogo) rows.push({ id: 'removeLogo', title: 'Remove the logo' })
  rows.push({ id: 'reshuffle', title: 'Make a new schedule' })
  rows.push({ id: 'reset', title: 'Reset channel', disabled: s.dirty, sub: s.dirty ? 'Save your changes first: a reset uses the saved settings.' : 'Rebuilds the schedule now, from these settings and the files that match them, instead of after what’s on now.' })
  sections.push(secR('', rows))
  sections.push(secN('', 'No logos come with the server. Use only pictures you have the right to use.'))
  if (s.history.length) sections.push(secR(`Earlier versions (${s.history.length})`, s.history.map((h) => ({ id: 'ver:' + h.id, title: h.name, sub: h.when, actions: [{ id: 'restore:' + h.id, title: 'Go back to this', label: `Go back to ${h.name}, ${h.when}` }] }))))
  return mk('channeled', 'Settings · Channels', 'More', '', sections)
}
