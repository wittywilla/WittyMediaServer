// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A listening party: the music player kept in step with a watch party that is playing a
// song (docs/SYNCPLAY.md). The room's queue is the queue; the music player holds just the
// one song the room is on, so that nothing here moves on by itself (no gapless hand-over,
// no repeat) and "next" is always the room's, for everyone at once. The sync itself is
// the video player's own (remote/party.ts), which only needs an element to keep in step.
import { useEffect, useRef } from 'react'
import { api, type Item } from '../api'
import { toast } from '../state'
import { leaveParty, partyCommand, partyStepQueue, positionAt, serverNow, useParty, usePartySync } from '../remote/party'
import { partyKind } from '../remote/partyLogic'
import type { AudioMode, PlayerView } from './types'

type Play = (items: Item[], start?: number, mode?: AudioMode, at?: number) => void

// useMusicParty takes the music player's view and gives it back as it should be while a
// party is on a song: seeking, next, previous and speed are the room's. With no party, or
// a party that is on a video, it gives back the view untouched.
export function useMusicParty(view: PlayerView, play: Play): PlayerView {
  const room = useParty()
  const playing = room && !room.blocked ? room.items[room.state.index] : undefined
  const want = room && playing && partyKind(playing.kind) === 'music' ? room.state.itemId : 0
  const latest = useRef({ view, play })
  latest.current = { view, play }
  const media = useRef<HTMLMediaElement | null>(null)
  media.current = view.media()

  // The room is on a song this player isn't: put it on, where the room is.
  const applied = useRef('')
  const key = room && want ? `${room.id}:${room.state.index}:${want}` : ''
  useEffect(() => {
    if (!key || !room) { applied.current = ''; return }
    if (applied.current === key) return
    let live = true
    const now = latest.current.view.current
    const start = (it: Item) => {
      if (!live) return
      applied.current = key
      const { view: v, play: p } = latest.current
      const same = v.current?.id === it.id
      // Already on it (the party was started from this song): it carries on from where it is.
      p([it], 0, same ? v.mode : 'music', same ? 0 : Math.max(0, positionAt(room.state, serverNow())))
      // A room that isn't playing yet holds it; without this it would sound for a moment first.
      if (room.state.paused || room.state.waiting) v.media()?.pause()
    }
    if (now?.id === want) start(now)
    else api.get<Item>(`/items/${want}`).then(start).catch((e: Error) => { if (live) toast(e.message) })
    return () => { live = false }
  }, [key]) // eslint-disable-line react-hooks/exhaustive-deps

  // Putting something else on, or stopping, is leaving the party: it can't be in step with
  // a room that is somewhere else.
  const here = !!want && view.current?.id === want
  useEffect(() => {
    if (!key || applied.current !== key || here) return
    toast('You’ve left the listening party.')
    leaveParty()
  }, [key, here])

  const sync = usePartySync(media, here ? want : 0, {
    seek: (t) => latest.current.view.seek(t),
    offset: () => { const el = latest.current.view.media(); return el ? latest.current.view.position() - el.currentTime : 0 },
    loaded: () => !!latest.current.view.media()?.getAttribute('src'),
    duration: () => latest.current.view.current?.runtimeSec || 0,
    cheapSeek: () => { const it = latest.current.view.current; return !!it && (!!it.audioDirect || it.videoCodec === 'midi') },
    blocked: () => toast('Press play to join in with the party.'),
  })

  // Repeat would start the song again under a room that has moved past its end.
  const repeat = view.repeat
  useEffect(() => { if (sync.active && repeat !== 'off') latest.current.view.cycleRepeat() }, [sync.active, repeat])

  if (!sync.active || !sync.room) return view
  const r = sync.room
  const hostOnly = () => toast('Only the host controls this watch party.')
  const fixed = () => toast('A party plays its queue in order, once through. The queue is in the party’s panel.')
  return {
    ...view,
    rate: r.state.rate,
    shuffle: false,
    repeat: 'off',
    seek: sync.seek,
    next: () => { if (sync.hasNext || !r.canControl) sync.next() },
    prev: () => {
      if (!r.canControl) { hostOnly(); return }
      if (view.position() > 4 || r.state.index === 0) sync.seek(0)
      else partyStepQueue('previous').catch((e: Error) => toast(e.message))
    },
    changeRate: (rate) => {
      if (!r.canControl) { hostOnly(); return }
      // A room plays between half and double speed.
      partyCommand('rate', { rate: Math.max(0.5, Math.min(2, rate)) }).catch((e: Error) => toast(e.message))
    },
    setShuffle: fixed,
    cycleRepeat: fixed,
  }
}
