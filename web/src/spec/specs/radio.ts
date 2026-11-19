// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecChip, SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// Internet radio (canvas "radio"): the browse filters, the two ways to add by hand, and a station's Manage panel.
// The station cards, the player and the directory search stay the page's own.

// Browse by country and genre. Fields: country. Chips: genre: (all genres) and genre:<name>.
export function radioFilterSpec(countries: string[], country: string, genres: string[], genre: string): SpecPageData {
  const sections: SpecSection[] = []
  if (countries.length > 1) sections.push(secF('', [{ id: 'country', kind: 'select', label: t('Country'), value: country, options: [{ value: '', label: t('All countries') }, ...countries.map((c) => ({ value: c, label: c }))] }]))
  if (genres.length > 1) {
    const chips: SpecChip[] = [{ ...Cc(t('All genres'), !genre), id: 'genre:' }, ...genres.map((g) => ({ ...Cc(g, genre === g), id: 'genre:' + g }))]
    sections.push(secP(t('Genres'), chips))
  }
  return mk('radio', t('Internet radio'), t('Stations'), '', sections)
}

export interface AddressForm { url: string; name: string; genre: string }

// Add one station by its stream address. Fields: url, name, genre. Action: add.
export function radioAddressSpec(f: AddressForm, err: string, busy: boolean): SpecPageData {
  return mk('radio', t('Internet radio'), t('Stream address'), '', [
    secF('', [
      { id: 'url', kind: 'text', label: t('Stream address'), value: f.url, placeholder: 'https://…/stream', required: true, autoFocus: true, autoComplete: 'off', error: err || undefined },
      { id: 'name', kind: 'text', label: t('Name'), value: f.name, placeholder: t('Optional') },
      { id: 'genre', kind: 'text', label: t('Genre'), value: f.genre, placeholder: t('Optional') },
    ]),
    secN('', t('Only addresses on the internet work: the server won’t reach into your own network on a station’s behalf. A station’s name and details are filled in the first time it plays, if it gives them.')),
  ], [{ ...A1(busy ? t('Adding…') : t('Add station')), id: 'add', disabled: !f.url.trim() || busy }])
}

// Add stations from an M3U or PLS file (the paste box is the page's own, under the file). Field: file. Action: add.
export function radioPlaylistSpec(text: string, err: string, busy: boolean): SpecPageData {
  return mk('radio', t('Internet radio'), t('Playlist'), '', [
    secF('', [{ id: 'file', kind: 'file', label: t('M3U or PLS playlist file'), accept: '.m3u,.m3u8,.pls,text/plain', error: err || undefined }]),
  ], [{ ...A1(busy ? t('Adding…') : t('Add stations')), id: 'add', disabled: !text.trim() || busy }])
}

export interface StationManage { fromDirectory: boolean; lastCheck?: string; address?: string; busy: string }

// A station's Manage panel for administrators. Chips: check, lookup (only for a station added by address), remove.
export function stationManageSpec(m: StationManage): SpecPageData {
  const chips: SpecChip[] = [{ ...Cc(m.busy === 'check' ? t('Trying…') : t('Try it now')), id: 'check', disabled: !!m.busy }]
  if (!m.fromDirectory) chips.push({ ...Cc(m.busy === 'lookup' ? t('Looking…') : t('Find details in the directory')), id: 'lookup', disabled: !!m.busy })
  chips.push({ ...Cc(t('Remove')), id: 'remove', disabled: !!m.busy })
  const rows = [R(t('Source'), m.fromDirectory ? t('Radio Browser directory') : t('Added by its address')), R(t('Last tried'), m.lastCheck || t('Not tried yet'))]
  if (m.address) rows.push(R(t('Address'), m.address))
  return mk('radio', t('Internet radio'), t('Manage'), '', [secR('', rows), secP('', chips)])
}
