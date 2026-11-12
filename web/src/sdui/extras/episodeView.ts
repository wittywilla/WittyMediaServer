// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// How a show's episodes are laid out on its server-driven page: as cards, or as a playlist
// (one line each). The show's own choice, which an administrator keeps for everyone, else the
// server's default (Settings, Playback), else what the page sent. Shared by the switch above
// the episodes and the list itself.

import { useSyncExternalStore } from 'react'

export type EpisodeView = 'cards' | 'list'
const chosen = new Map<string, EpisodeView>()
const listeners = new Set<() => void>()
const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

export function setEpisodeView(show: string, v: EpisodeView) {
  if (chosen.get(show) === v) return
  chosen.set(show, v)
  listeners.forEach((l) => l())
}

// The view chosen for this show here, or undefined for the page's own.
export function useEpisodeView(show: string): EpisodeView | undefined {
  return useSyncExternalStore(subscribe, () => chosen.get(show))
}

// Someone pressed Cards or Playlist (not the page settling on a default): the show's page
// may keep the choice (an administrator's is kept for everyone).
const choosers = new Set<(show: string, v: EpisodeView) => void>()
export function chooseEpisodeView(show: string, v: EpisodeView) {
  setEpisodeView(show, v)
  choosers.forEach((c) => c(show, v))
}
export const onEpisodeViewChosen = (c: (show: string, v: EpisodeView) => void) => { choosers.add(c); return () => { choosers.delete(c) } }
