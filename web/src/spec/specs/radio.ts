// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { A1, Cc, R, mk, secF, secN, secP, secR } from '../model.ts'
import type { SpecChip, SpecPageData, SpecSection } from '../types.ts'

// Internet radio (canvas "radio"): the browse filters, the two ways to add by hand, and a station's Manage panel.
// The station cards, the player and the directory search stay the page's own.

// Browse by country and genre. Fields: country. Chips: genre: (all genres) and genre:<name>.
export function radioFilterSpec(countries: string[], country: string, genres: string[], genre: string): SpecPageData {
  const sections: SpecSection[] = []
  if (countries.length > 1) sections.push(secF('', [{ id: 'country', kind: 'select', label: 'Country', value: country, options: [{ value: '', label: 'All countries' }, ...countries.map((c) => ({ value: c, label: c }))] }]))
  if (genres.length > 1) {
    const chips: SpecChip[] = [{ ...Cc('All genres', !genre), id: 'genre:' }, ...genres.map((g) => ({ ...Cc(g, genre === g), id: 'genre:' + g }))]
    sections.push(secP('Genres', chips))
  }
  return mk('radio', 'Internet radio', 'Stations', '', sections)
}

export interface AddressForm { url: string; name: string; genre: string }

// Add one station by its stream address. Fields: url, name, genre. Action: add.
export function radioAddressSpec(f: AddressForm, err: string, busy: boolean): SpecPageData {
  return mk('radio', 'Internet radio', 'Stream address', '', [
    secF('', [
      { id: 'url', kind: 'text', label: 'Stream address', value: f.url, placeholder: 'https://…/stream', required: true, autoFocus: true, autoComplete: 'off', error: err || undefined },
      { id: 'name', kind: 'text', label: 'Name', value: f.name, placeholder: 'Optional' },
      { id: 'genre', kind: 'text', label: 'Genre', value: f.genre, placeholder: 'Optional' },
    ]),
    secN('', 'Only addresses on the internet work: the server won’t reach into your own network on a station’s behalf. A station’s name and details are filled in the first time it plays, if it gives them.'),
  ], [{ ...A1(busy ? 'Adding…' : 'Add station'), id: 'add', disabled: !f.url.trim() || busy }])
}

// Add stations from an M3U or PLS file (the paste box is the page's own, under the file). Field: file. Action: add.
export function radioPlaylistSpec(text: string, err: string, busy: boolean): SpecPageData {
  return mk('radio', 'Internet radio', 'Playlist', '', [
    secF('', [{ id: 'file', kind: 'file', label: 'M3U or PLS playlist file', accept: '.m3u,.m3u8,.pls,text/plain', error: err || undefined }]),
  ], [{ ...A1(busy ? 'Adding…' : 'Add stations'), id: 'add', disabled: !text.trim() || busy }])
}

export interface StationManage { fromDirectory: boolean; lastCheck?: string; address?: string; busy: string }

// A station's Manage panel for administrators. Chips: check, lookup (only for a station added by address), remove.
export function stationManageSpec(m: StationManage): SpecPageData {
  const chips: SpecChip[] = [{ ...Cc(m.busy === 'check' ? 'Trying…' : 'Try it now'), id: 'check', disabled: !!m.busy }]
  if (!m.fromDirectory) chips.push({ ...Cc(m.busy === 'lookup' ? 'Looking…' : 'Find details in the directory'), id: 'lookup', disabled: !!m.busy })
  chips.push({ ...Cc('Remove'), id: 'remove', disabled: !!m.busy })
  const rows = [R('Source', m.fromDirectory ? 'Radio Browser directory' : 'Added by its address'), R('Last tried', m.lastCheck || 'Not tried yet')]
  if (m.address) rows.push(R('Address', m.address))
  return mk('radio', 'Internet radio', 'Manage', '', [secR('', rows), secP('', chips)])
}
