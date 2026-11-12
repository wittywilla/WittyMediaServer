// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The watch party dialog, as pages and the account menu open it: starting one, joining
// one with its code, and, once in one, everything about it (PartyBody.tsx). In the video
// player the same body is a panel instead (PartyPanel.tsx), which also shows while the
// player fills the screen. docs/SYNCPLAY.md is the guide people read.
import { Dialog } from '../ui'
import PartyBody, { type PartyItem } from './PartyBody'
import { useParty } from './party'

export default function WatchParty({ item, onClose }: { item?: PartyItem; onClose: () => void }) {
  const room = useParty()
  return (
    <Dialog title={room ? room.name : 'Watch party'} onClose={onClose} className="rc-dialog"
      description={room ? undefined : 'Watch or listen with other people at the same moment, each on your own screen.'}>
      <PartyBody item={item} onClose={onClose} />
    </Dialog>
  )
}
