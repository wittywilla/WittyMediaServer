// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect } from 'react'
import { IconClose } from '../icons'
import { IconButton } from '../ui'
import type { Markers } from '../player/markers'
import InfoEditor, { type PlayerLink } from './InfoEditor'
import './info.css'

// The editor inside the player, beside the picture (below it on a phone): Mark start and
// Mark end at the moment playing, frame steps and nudges, and the player's own skip button
// trying the parts as they are now, saved or not (onDraft).

export default function PlayerPanel({ itemId, title, player, now, onDraft, onSaved, onClose }: {
  itemId: number; title: string; player: PlayerLink; now: number
  onDraft: (m: Markers | null) => void; onSaved: () => void; onClose: () => void
}) {
  // Escape closes the panel, not the player.
  useEffect(() => {
    const k = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.stopPropagation(); onClose() } }
    window.addEventListener('keydown', k, true)
    return () => window.removeEventListener('keydown', k, true)
  }, [onClose])
  return (
    <aside className="wi-player-panel" aria-label="Scenes and skips" onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => { if (e.key !== 'Escape') e.stopPropagation() }}>
      <header>
        <h2>Scenes and skips</h2>
        <IconButton size="sm" label="Close" onClick={onClose}><IconClose /></IconButton>
      </header>
      <p className="muted wi-help">{title}</p>
      <InfoEditor base={`/wittyinfo/item/${itemId}`} scope="item" player={player} now={now} onDraft={onDraft} onSaved={onSaved} compact />
    </aside>
  )
}
