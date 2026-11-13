// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A game's own page inside WittyPlay (/wittyplay/game/:id): where choosing a game goes first (unless the profile chose to start
// playing), and where a game always comes back to when it's closed. It is the app's whole game page (games/Details.tsx: Play and
// Resume, where you left off, Moments, every setting and option for the game, controls, cheats, notes) in WittyPlay's frame and
// look (playChrome.css, "a game's page"), with links that stay in WittyPlay, and the game's RetroAchievements.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, type Item } from '../api'
import { LoadError, Spinner } from '../components/common'
import GameDetails from './games/Details'
import { GameLinksContext, type GameLinks } from './games/data'
import { Achievements, useCheevos } from '../sdui/render/GameBio'
import { PLAY_LIBRARY, playGamePath } from '../shell/playPaths'

const links: GameLinks = {
  couch: false,
  item: (id) => playGamePath(id),
  library: (lib, query) => `${PLAY_LIBRARY}/${lib}${query ? '?' + query : ''}`,
  play: (id, query) => `/game/${id}${query ? '?' + query : ''}`,
}

function Cheevos({ id }: { id: number }) {
  const cheevos = useCheevos(id)
  return <div className="wplay-game-cheevos"><Achievements cheevos={cheevos} /></div>
}

export default function PlayGame() {
  const { id } = useParams()
  const [it, setIt] = useState<Item | null>(null)
  const [failed, setFailed] = useState(false)
  const load = useCallback(() => api.get<Item>(`/items/${id}`).then((i) => { setIt(i); setFailed(false) }).catch(() => setFailed(true)), [id])
  useEffect(() => { void load() }, [load])
  const value = useMemo(() => links, [])
  if (failed) return <LoadError what="This game" onRetry={() => { void load() }} />
  if (!it || String(it.id) !== id) return <Spinner />
  return (
    <div className="wplay-game">
      <GameLinksContext.Provider value={value}><GameDetails it={it} reload={() => { void load() }} /></GameLinksContext.Provider>
      <Cheevos id={it.id} />
    </div>
  )
}
