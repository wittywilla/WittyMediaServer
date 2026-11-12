// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// "Play on…" as a button, for every place something can be played from: a page's actions, a
// menu, a player's top bar. It opens the one dialog the app has (RemoteHost), which is
// loaded then and not before.
import type { ReactNode } from 'react'
import type { PlayerView } from '../audio/types'
import { IconDevices } from '../icons'
import { mirror, reported } from './queue'
import type { PlayOnSource } from './PlayOn'
import { openPlayOn } from './RemoteHost'

type Source = PlayOnSource | (() => PlayOnSource)

function open(source: Source) {
  // A dialog can't show over a player that has the whole screen to itself.
  if (document.fullscreenElement) document.exitFullscreen().catch(() => {})
  openPlayOn(typeof source === 'function' ? source() : source)
}

// queueOf: a list of items as a queue the protocol will take (queue.ts), starting at one of them.
export const queueOf = (ids: number[], index: number): { queue?: number[]; queueIndex?: number } => reported(mirror(ids, index))

// label is what the button says; with iconOnly it is the button's name instead.
export function PlayOnButton({ source, className = 'btn', label = 'Play on…', iconOnly, disabled, size, before }: { source: Source; className?: string; label?: string; iconOnly?: boolean; disabled?: boolean; size?: number; before?: () => void }) {
  return (
    <button type="button" className={className} aria-haspopup="dialog" disabled={disabled} aria-label={iconOnly ? label : undefined} title={iconOnly ? label : undefined}
      onClick={(e) => { e.stopPropagation(); before?.(); open(source) }}>
      <IconDevices size={size} />{iconOnly ? null : <> {label}</>}
    </button>
  )
}

// The same in a menu of role=menuitem rows.
export function PlayOnMenuItem({ source, onChosen, label = 'Play on…', size = 18 }: { source: Source; onChosen?: () => void; label?: ReactNode; size?: number }) {
  return <button type="button" role="menuitem" aria-haspopup="dialog" onClick={() => { onChosen?.(); open(source) }}><IconDevices size={size} /> {label}</button>
}

// What the audio player is playing, as something to hand over: the song with its queue, or a
// station. A CD in the player isn't here: its own page has the button, for the disc.
export function audioSource(p: PlayerView): PlayOnSource | null {
  const it = p.current
  if (!it || p.cdActive) return null
  return {
    kind: 'playing', itemId: it.id, itemKind: p.live ? 'radio' : 'audio', title: it.title, positionSec: () => (p.live ? 0 : p.position()), stop: p.stop,
    ...queueOf(p.queue.map((q) => q.id), p.index),
  }
}

export function AudioPlayOn({ p, className = 'mp-ibtn' }: { p: PlayerView; className?: string }) {
  const source = audioSource(p)
  if (!source) return null
  // The full player covers the page, dialogs and all: it folds down to its bar first.
  return <PlayOnButton className={className} iconOnly label="Play on another device" source={source} before={p.full ? p.closeFull : undefined} />
}
