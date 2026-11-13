// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The profile's choice of game names (/me/ui gameTitles), as a hook. See titles.ts.
import { useUserPref } from '../../ui/userPrefs'
import { asGameTitlePref, GAME_TITLES_KEY, type GameTitlePref } from './titles'

export function useGameTitlePref(): [GameTitlePref, (p: GameTitlePref) => void] {
  const [saved, save] = useUserPref<string>(GAME_TITLES_KEY, 'romanized')
  // The default isn't written: a profile that never chose follows whatever the default becomes.
  return [asGameTitlePref(saved), (p) => { void save(p === 'romanized' ? null : p).catch(() => {}) }]
}
