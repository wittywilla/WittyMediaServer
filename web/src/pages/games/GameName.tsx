// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A game's name as the profile prefers it (/me/ui gameTitles, see titles.ts): one line, or for "Original with English
// below" the name in its own script with the English name under it, smaller. Each line says its language when its
// letters do, so it is read aloud and drawn right.
import { GAME_TITLE_EXAMPLE, gameTitleLabel, gameTitleParts, titleLang, type GameTitlePref, type Titled } from './titles'
import { useGameTitlePref } from './useGameTitles'

export function GameName({ item }: { item: Titled }) {
  const [pref] = useGameTitlePref()
  const { main, sub } = gameTitleParts(item, pref)
  return (
    <>
      <span lang={titleLang(main)}>{main}</span>
      {sub && <span className="game-title-sub" lang={titleLang(sub)}>{sub}</span>}
    </>
  )
}

// useGameName: the one line, and the whole name for a title or label attribute.
export function useGameName(item: Titled | null | undefined): { name: string; label: string } {
  const [pref] = useGameTitlePref()
  return { name: gameTitleParts(item, pref).main, label: gameTitleLabel(item, pref) }
}

// One game's name as a choice shows it.
export function GameTitleSample({ pref, className }: { pref: GameTitlePref; className?: string }) {
  const { main, sub } = gameTitleParts(GAME_TITLE_EXAMPLE, pref)
  return (
    <span className={'game-title-sample' + (className ? ' ' + className : '')} role="img" aria-label={'For example, ' + gameTitleLabel(GAME_TITLE_EXAMPLE, pref)}>
      <span lang={titleLang(main)}>{main}</span>
      {sub && <span className="game-title-sub" lang={titleLang(sub)}>{sub}</span>}
    </span>
  )
}
