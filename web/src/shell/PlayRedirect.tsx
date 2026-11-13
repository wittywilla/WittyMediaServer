// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// With WittyPlay Online installed it is the app's Games: the plain Games page (/games, and its eras) and a games library's own page
// show inside WittyPlay's frame, so every way in (the menu, Your libraries, Home's See all) leads to the one place and the whole
// library (every game, by console, with its filters) is still there. Without it they are as they were.
import { useApp } from '../state'
import { isPlayEntry } from './navModel'
import { useMenu } from './useNav'

export function usePlayInstalled(): boolean {
  const m = useMenu()
  return m.server && (m.all.some(isPlayEntry) || m.rows.some((r) => !!r.item && isPlayEntry(r.item)))
}

// Whether this address is one of the games pages WittyPlay takes in.
export function useGamesInPlay(path: string): boolean {
  const { libraries } = useApp()
  const play = usePlayInstalled()
  if (!play) return false
  if (path === '/games') return true
  const lib = /^\/library\/(\d+)$/.exec(path)?.[1]
  return !!lib && libraries.find((l) => String(l.id) === lib)?.type === 'games'
}
