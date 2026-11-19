// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense } from 'react'
import { IconChapters } from '../icons'
import { t } from '../i18n'
import { toast } from '../state'
import { desktopOnlyWords, editorAllowed } from './desktopOnly'

// The music pages' way into a .wittyaudio: an entry for an album's or a song's own menu, and the
// editor it opens. On a TV the entry stays, says it is for a desktop, and says where to go.

const InfoDialog = lazy(() => import('./InfoDialog'))

export function MusicInfoItem({ label, onOpen, close }: { label: string; onOpen: () => void; close: () => void }) {
  const allowed = editorAllowed()
  return (
    <button type="button" role="menuitem" aria-haspopup={allowed ? 'dialog' : undefined}
      onClick={() => { close(); if (allowed) onOpen(); else toast(desktopOnlyWords(), { ms: 9000 }) }}>
      <IconChapters size={18} /> {allowed ? label : t('{label} (desktop only)', { label })}
    </button>
  )
}

export function MusicInfoDialog({ album, id, title, onClose, onChanged }: { album: boolean; id: number; title: string; onClose: () => void; onChanged?: () => void }) {
  return (
    <Suspense fallback={null}>
      <InfoDialog target={album ? 'album' : 'item'} song={!album} id={id} title={album ? t('Album details') : title} onClose={onClose} onChanged={() => onChanged?.()} />
    </Suspense>
  )
}
