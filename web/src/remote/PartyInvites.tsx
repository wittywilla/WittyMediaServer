// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// An invitation to a watch party, shown on whatever page someone is on: the same card, in
// the same corner, as a TV reminder coming due (pages/tv/reminders.tsx), since both are
// "something wants an answer, when you have a moment". It says who invited you and the
// party's name, never what is playing. Accept takes a seat; Decline puts it away on this
// device (invites.ts says why only here).
import { useState } from 'react'
import { createPortal } from 'react-dom'
import { useLocation } from 'react-router-dom'
import { toast } from '../state'
import { joinParty } from './party'
import { declineInvite, refreshInvites, useInvites } from './invites'
import './party.css'

export default function PartyInvites() {
  const invites = useInvites()
  const [busy, setBusy] = useState(false)
  // A player that fills the screen holds the card itself, where a remote's arrows go and
  // where it still shows in full screen. Looked up again when the page changes.
  useLocation()
  const inv = invites[0]
  if (!inv) return null
  const accept = async () => {
    setBusy(true)
    try { await joinParty({ roomId: inv.id }) } catch (e) {
      toast((e as Error).message, { tone: 'warn' })
      // Gone, or not for this account: it needn't be offered again.
      declineInvite(inv.id)
    } finally { setBusy(false); refreshInvites() }
  }
  const stop = (e: React.SyntheticEvent) => e.stopPropagation()
  const player = document.querySelector('.player:not(.mini):not(.inline)')
  const card = (
    <div className="party-invite" role="alert" onClick={stop} onDoubleClick={stop}>
      <div>
        <b>{inv.hostName || 'Someone'}</b> invited you to a watch party
        <small>{inv.name}{invites.length > 1 ? ` · and ${invites.length - 1} more ${invites.length === 2 ? 'invitation' : 'invitations'}` : ''}</small>
      </div>
      <div className="party-invite-actions">
        <button type="button" className="btn sm primary" disabled={busy} onClick={accept}>Accept</button>
        <button type="button" className="btn sm ghost" disabled={busy} onClick={() => declineInvite(inv.id)}>Decline</button>
      </div>
    </div>
  )
  return player ? createPortal(card, player) : card
}
