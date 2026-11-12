// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { CanvasVideo } from './CanvasVideo'
import { useCanvas } from './useCanvas'

// Behind the full-screen music player: the song's canvas, or its album's. It runs while the
// music does and holds still while it's paused. The visualizer, when it's on, draws over it.
export function NowPlayingCanvas({ itemId, playing }: { itemId: number; playing: boolean }) {
  const c = useCanvas(itemId)
  if (!c.info?.available || c.blocked) return null
  return <CanvasVideo info={c.info} blocked={c.blocked} paused={!playing} className="np-loop" />
}
