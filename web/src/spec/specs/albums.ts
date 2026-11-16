// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secF, secP, secR } from '../model.ts'
import type { SpecPageData, SpecRow } from '../types.ts'
import { t, tn } from '../../sdui/words.ts'

// Photo albums (canvas "photoalbums"): the album details dialog and the Add to album picker.

export interface AlbumForm { name: string; shared: boolean }

// Name and sharing (the description box is the dialog's own, between them). Fields: name, shared.
export function albumSpec(f: AlbumForm, err: string): SpecPageData {
  return mk('photoalbums', t('Photos · Albums'), t('Album details'), '', [secF('', [
    { id: 'name', kind: 'text', label: t('Name'), value: f.name, required: true, autoFocus: true, error: err || undefined },
    { id: 'shared', kind: 'switch', label: t('Share with other profiles who can open this library'), value: f.shared },
  ])])
}

export interface AlbumLine { id: number; name: string; count: number }

// Add pictures to an album: New album, then each album you can add to. Ids: new, album:<id>.
export function addToAlbumSpec(albums: AlbumLine[], count: number): SpecPageData {
  const rows: SpecRow[] = albums.map((a) => ({ ...R(a.name, String(a.count)), id: 'album:' + a.id }))
  return mk('photoalbums', t('Photos · Albums'), t('Add to album'), tn('{count} picture chosen.', '{count} pictures chosen.', count), [
    secP('', [{ ...Cc(t('New album')), id: 'new' }]),
    ...(rows.length ? [secR(t('Albums'), rows)] : []),
  ])
}

// The same from the picture viewer: one picture; an album that already has it is shown but not offered. Ids: album:<id>.
export function viewerAlbumSpec(albums: AlbumLine[], holding: readonly number[]): SpecPageData {
  const rows: SpecRow[] = albums.map((a) => ({ ...R(a.name, holding.includes(a.id) ? t('Already in it') : '', holding.includes(a.id)), id: 'album:' + a.id, disabled: holding.includes(a.id) }))
  return mk('photoview', t('Photos · Viewer'), t('Add to album'), '', rows.length ? [secR('', rows)] : [])
}
