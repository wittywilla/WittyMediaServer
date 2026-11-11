// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The part of remote control and watch parties that belongs to the whole app rather than
// to a page: this browser is announced as a device while someone is signed in, a party's
// item opens in the player when the party moves to it, and the two dialogs can be opened
// from anywhere (the account menu does).
import { lazy, Suspense, useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../state'
import { setRemoteNavigator, startDevice, stopDevice } from './device'
import { dropParty, partyPlayerItem, positionAt, resumeParty, serverNow, useParty } from './party'

const PlayOn = lazy(() => import('./PlayOn'))
const WatchParty = lazy(() => import('./WatchParty'))

export const openPlayOn = () => window.dispatchEvent(new Event('wms-play-on'))
export const openWatchParty = () => window.dispatchEvent(new Event('wms-watch-party'))

export default function RemoteHost() {
  const { user } = useApp()
  const nav = useNavigate()
  const loc = useLocation()
  const room = useParty()
  const [open, setOpen] = useState<'play-on' | 'party' | ''>('')

  useEffect(() => { setRemoteNavigator((to) => nav(to)); return () => setRemoteNavigator(null) }, [nav])
  useEffect(() => {
    if (!user) return
    startDevice()
    resumeParty()
    return () => { stopDevice(); dropParty() }
  }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  // The party is playing something this window isn't: open it. Its player then keeps step.
  const itemId = room && !room.blocked ? room.state.itemId : 0
  useEffect(() => {
    if (!itemId || partyPlayerItem() === itemId || loc.pathname === `/play/${itemId}`) return
    // Opened where the party is, so the stream is asked for once, at the right place.
    const at = Math.max(0, positionAt(room!.state, serverNow()))
    nav(`/play/${itemId}?party=${room!.state.seq}&start=${at.toFixed(1)}`, { replace: loc.pathname.startsWith('/play/') })
  }, [room?.id, itemId]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const playOn = () => setOpen('play-on'), party = () => setOpen('party')
    window.addEventListener('wms-play-on', playOn)
    window.addEventListener('wms-watch-party', party)
    return () => { window.removeEventListener('wms-play-on', playOn); window.removeEventListener('wms-watch-party', party) }
  }, [])

  if (!open) return null
  return <Suspense fallback={null}>{open === 'play-on' ? <PlayOn onClose={() => setOpen('')} /> : <WatchParty onClose={() => setOpen('')} />}</Suspense>
}
