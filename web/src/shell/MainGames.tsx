// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Games outside WittyPlay (the app's Home, its shelves and libraries, TV mode): each game drawn the way the person chose for WittyPlay
// (in its case or cartridge, or its box art), and choosing one opens its page in WittyPlay, Play first, where every save and setting is.
// WittyPlay is how games are played here, so a game is never a dead end in the app.
import { useMemo, type ReactNode } from 'react'
import { CardRouteContext } from '../sdui/render/cardRoute'
import { GameArtContext } from '../sdui/render/glyphs'
import { useGameArtApi, useMainArt } from './GamePictureArt'
import { mainGameRoute } from './playPaths'

export function MainGames({ children }: { children: ReactNode }) {
  const api = useGameArtApi()
  const [mode] = useMainArt()
  // Only the card's picture (the app's own pages have no game panel for WittyPlay's buttons), and none at all when the person wants
  // just the box art here: the card then shows its picture as sent.
  const art = useMemo(() => (mode === 'art' ? null : { art: api.art }), [api, mode])
  return (
    <CardRouteContext.Provider value={mainGameRoute}>
      <GameArtContext.Provider value={art}>{children}</GameArtContext.Provider>
    </CardRouteContext.Provider>
  )
}
