// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The part of remote control and watch parties that belongs to the whole app rather than
// to a page: this browser is announced as a device while someone is signed in, a party's
// item opens in the player when the party moves to it, and the two dialogs can be opened
// from anywhere (the account menu does).
import { lazy, Suspense, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../state'
import { startDevice, stopDevice } from './device'
import { useRemoteOpener } from './opener'
import type { PlayOnAsk, PlayOnSource } from './PlayOn'
import { dropParty, partyPlayerItem, positionAt, resumeParty, serverNow, takeResumed, useParty } from './party'
import { watchInvites } from './invites'
import { partyKind } from './partyLogic'

const PlayOn = lazy(() => import('./PlayOn'))
const WatchParty = lazy(() => import('./WatchParty'))
const PartyInvites = lazy(() => import('./PartyInvites'))

// With a source: what to start or move there (a title's page, a player). Without: the list of devices and their remotes.
export const openPlayOn = (source?: PlayOnSource) => window.dispatchEvent(new CustomEvent<PlayOnAsk>('wms-play-on', { detail: { source: source && typeof source === 'object' && 'itemId' in source ? source : undefined } }))
export const openWatchParty = () => window.dispatchEvent(new Event('wms-watch-party'))

export default function RemoteHost() {
  const { user } = useApp()
  const nav = useNavigate()
  const loc = useLocation()
  const room = useParty()
  const [open, setOpen] = useState<'play-on' | 'party' | ''>('')
  const [ask, setAsk] = useState<PlayOnAsk>({})

  useRemoteOpener()
  useEffect(() => {
    if (!user) return
    startDevice()
    resumeParty()
    watchInvites(user.id)
    return () => { stopDevice(); dropParty(); watchInvites(0) }
  }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  // The party is playing a video this window isn't: open it. Its player then keeps step.
  // A song is the music player's to follow (audio/partyMusic.ts); anything else isn't kept in step.
  const itemId = room && !room.blocked && partyKind(room.items[room.state.index]?.kind) === 'video' ? room.state.itemId : 0
  useEffect(() => {
    // After a reload the player is already open, at the place its address remembers: open it
    // again where the party is now.
    const back = !!itemId && takeResumed()
    if (!itemId || (!back && (partyPlayerItem() === itemId || loc.pathname === `/play/${itemId}`))) return
    // Opened where the party is, so the stream is asked for once, at the right place.
    // Never past the end: a room left running after its last title has nothing there to open.
    const length = room!.items[room!.state.index]?.runtimeSec || 0
    const at = Math.max(0, Math.min(positionAt(room!.state, serverNow()), length > 2 ? length - 1 : Infinity))
    nav(`/play/${itemId}?party=${room!.state.seq}&start=${at.toFixed(1)}`, { replace: loc.pathname.startsWith('/play/') })
  }, [room?.id, itemId]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const playOn = (e: Event) => { setAsk((e as CustomEvent<PlayOnAsk | undefined>).detail || {}); setOpen('play-on') }, party = () => setOpen('party')
    window.addEventListener('wms-play-on', playOn)
    window.addEventListener('wms-watch-party', party)
    return () => { window.removeEventListener('wms-play-on', playOn); window.removeEventListener('wms-watch-party', party) }
  }, [])

  return (
    <Suspense fallback={null}>
      {user && <PartyInvites />}
      {open === 'play-on' ? <PlayOn source={ask.source} remote={ask.remote} onClose={() => setOpen('')} /> : open === 'party' ? <WatchParty onClose={() => setOpen('')} /> : null}
    </Suspense>
  )
}
