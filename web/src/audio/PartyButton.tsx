// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The full music player's way into a listening party: start one on the song that is
// playing (its code, and the songs queued after it, come with it), join one, or see the
// one this player is in. Not offered for a radio station or a disc, which have no place
// in time that a room could agree on.
import { lazy, Suspense, useState } from 'react'
import { IconUsers } from '../icons'
import { useParty } from '../remote/party'
import type { PlayerView } from './types'

const WatchParty = lazy(() => import('../remote/WatchParty'))

export default function PartyButton({ p }: { p: PlayerView }) {
  const room = useParty()
  const [open, setOpen] = useState(false)
  const it = p.current
  if (!it || p.live || p.cdActive || it.kind !== 'audio') return null
  const show = () => {
    // The dialog is drawn on the page, which a player filling the screen would cover.
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
    setOpen(true)
  }
  const label = room ? `Listening party, ${room.members.length} there` : 'Listening party'
  return (
    <>
      <button type="button" className={'mp-ibtn' + (room ? ' on' : '')} aria-haspopup="dialog" aria-label={label} title={label} onClick={show}><IconUsers /></button>
      {open && (
        <Suspense fallback={null}>
          <WatchParty onClose={() => setOpen(false)} item={{
            id: it.id, title: it.title, kind: it.kind, positionSec: p.position,
            // Songs only: a party can't hold a station.
            queue: () => { const songs = p.queue.filter((q) => q.kind === 'audio'); return { items: songs.map((q) => q.id), index: songs.findIndex((q) => q.id === it.id) } },
          }} />
        </Suspense>
      )}
    </>
  )
}
