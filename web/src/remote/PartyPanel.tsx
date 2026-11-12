// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The watch party, inside the video player: a panel down the side (a sheet from the
// bottom on a phone) that is part of the player itself, so it is there when the player
// fills the screen and a remote's arrows reach it. It covers only its own corner: a click
// on the picture beside it is still a click on the picture, and a click in it never is.
import { useEffect, useRef } from 'react'
import { IconClose } from '../icons'
import { IconButton } from '../ui'
import PartyBody, { type PartyItem } from './PartyBody'
import { useParty } from './party'

export default function PartyPanel({ item, onClose }: { item: PartyItem; onClose: () => void }) {
  const room = useParty()
  const box = useRef<HTMLElement | null>(null)
  // Escape, or a remote's Back, closes the panel and nothing else. Heard first, on the way
  // down, so that it isn't also the player's "leave" or a page's "go back".
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape' && e.key !== 'GoBack' && e.key !== 'BrowserBack') return
      if (!box.current) return
      e.preventDefault()
      e.stopPropagation()
      onClose()
    }
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [onClose])
  const stop = (e: React.SyntheticEvent) => e.stopPropagation()
  return (
    <aside ref={box} className="party-panel" role="dialog" aria-modal="false" aria-labelledby="party-panel-h" onClick={stop} onDoubleClick={stop}>
      <header className="party-panel-head">
        <h2 id="party-panel-h">{room ? room.name : 'Watch party'}</h2>
        <IconButton label="Close the watch party panel" size="sm" onClick={onClose}><IconClose /></IconButton>
      </header>
      <div className="party-panel-body">
        {!room && <p className="muted">Watch with other people at the same moment, each on your own screen.</p>}
        <PartyBody item={item} onClose={onClose} />
      </div>
    </aside>
  )
}
