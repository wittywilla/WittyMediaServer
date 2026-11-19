// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { ReactNode } from 'react'
import { UseAComputer, advancedHere } from '../ui/advanced'
import CanvasStudio from './CanvasStudio'

// A title's canvas, for an administrator: the Canvas studio (CanvasStudio.tsx), opened from a title's page, an
// album's (picker chooses which of its songs) or the full player's Look. On a phone, tablet or TV it says to use a
// computer instead (ui/advanced.tsx): the loop editor needs a precise pointer and room.
export default function CanvasManager({ itemId, title, kind, runtimeSec, picker, artist, onClose }: {
  itemId: number; title: string; kind: 'video' | 'song' | 'disc'; runtimeSec: number
  picker?: ReactNode // an album's page chooses which of its songs this is about
  artist?: string
  onClose: () => void
}) {
  if (!advancedHere()) return <UseAComputer what="A Canvas video" onClose={onClose} />
  return <CanvasStudio key={itemId} itemId={itemId} title={title} kind={kind} runtimeSec={runtimeSec} artist={artist} picker={picker} onClose={onClose} />
}
