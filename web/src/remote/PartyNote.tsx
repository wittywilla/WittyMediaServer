// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What a video player says about the watch party it is in, over the picture: who the room
// is waiting for, and when the party has moved on to something this player can't follow
// it to (a title this account may not see, or a kind this app doesn't keep in step). Then
// the picture here is paused, the seat is kept, and the player opens what comes next by
// itself (RemoteHost.tsx). A party that has moved on to music hands over to the music
// player, and this player closes.
import { useEffect, useRef } from 'react'
import { useParty } from './party'
import { partyKind } from './partyLogic'
import './remote.css'
import './party.css'

// quiet: do the pausing and closing, but say nothing (the mini player has no room for it).
export default function PartyNote({ itemId, video, close, quiet }: { itemId: number; video: React.RefObject<HTMLVideoElement | null>; close: () => void; quiet?: boolean }) {
  const room = useParty()
  // Only a player that was in the party is the party's to pause or close.
  const was = useRef(false)
  const here = !!room && !room.blocked && room.state.itemId === itemId
  if (here) was.current = true
  if (!room) was.current = false
  const playing = room ? room.items[room.state.index] : undefined
  const elsewhere = !!room && was.current && !here
  const kind = room && !room.blocked ? partyKind(playing?.kind) : 'none'
  const toMusic = elsewhere && kind === 'music'
  const stuck = elsewhere && !toMusic && (room!.blocked || kind === 'none')
  useEffect(() => { if (stuck) video.current?.pause() }, [stuck, video])
  useEffect(() => { if (toMusic) close() }, [toMusic]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!room || quiet) return null
  if (stuck) {
    return (
      <div className="wp-note wp-note-big" role="status" aria-live="polite">
        <strong>{room.blocked ? 'Can’t watch this one' : 'This one isn’t kept in step here'}</strong>
        <span>{room.blocked ? 'The party has moved on to something this account can’t watch.' : 'The party has moved on to something this app can’t keep in step.'} You keep your place, and join in again when it moves on.</span>
      </div>
    )
  }
  if (!here || !room.state.waiting) return null
  const names = room.members.filter((m) => !m.you && (m.state === 'loading' || m.state === 'buffering')).map((m) => m.name)
  return <div className="wp-note" role="status" aria-live="polite">{names.length ? `Waiting for ${names.join(', ')}` : 'Waiting for everyone to be ready'}</div>
}
