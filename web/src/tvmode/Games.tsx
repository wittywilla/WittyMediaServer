// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { api, type GameConsole, type Item, type Library } from '../api'
import { CardRow, EmptyState, LoadError, Spinner } from '../components/common'
import { IconGamepad, IconRight } from '../icons'
import GameDetails from '../pages/games/Details'
import { playGamePath } from '../shell/playPaths'
import GamesLibrary, { GameCard } from '../pages/games/Library'
import { consoleMap, GameLinksContext, type GameLinks } from '../pages/games/data'
import '../pages/games/games-ui.css'
import { useApp } from '../state'
import { featureOff, useMyLimits } from '../myLimits'

// Games: WittyPlay's couch library, with the existing games pages until WittyPlay's own TV look replaces
// them. One games library opens straight in; several are a shelf each, as Home's rows are.

const links: GameLinks = {
  couch: true,
  item: (id) => `/games/game/${id}`,
  library: (lib, query) => `/games/library/${lib}${query ? '?' + query : ''}`,
  play: (id, query) => `/game/${id}${query ? '?' + query : ''}`,
  search: '/search',
}

function useGameLibraries() {
  const { libraries } = useApp()
  return useMemo(() => libraries.filter((l) => l.type === 'games' && l.console !== 'rig'), [libraries])
}

// One library's shelf: its name and All games, then its most played covers.
function Shelf({ lib }: { lib: Library }) {
  const [games, setGames] = useState<Item[] | null>(null)
  const [consoles, setConsoles] = useState<Map<string, GameConsole>>(new Map())
  useEffect(() => {
    api.get<{ items: Item[] }>(`/items?library=${lib.id}&kind=game&sort=popular&desc=0&limit=20`).then((r) => setGames(r.items || [])).catch(() => setGames([]))
    api.get<GameConsole[]>(`/games/consoles?library=${lib.id}`).then((c) => setConsoles(consoleMap(c))).catch(() => {})
  }, [lib.id])
  return (
    <section className="hub tvm-shelf" aria-label={lib.name}>
      <div className="hub-head">
        <h2>{lib.name}</h2>
        <Link className="btn tvm-all" to={links.library(lib.id)}>All games <IconRight /></Link>
      </div>
      {games === null ? <div className="tvm-shelf-wait"><Spinner /></div>
        : games.length === 0 ? <p className="muted">No games in this library yet.</p>
        : <CardRow>{games.map((g) => <GameCard key={g.id} item={g} console={consoles.get(g.console || '')} />)}</CardRow>}
    </section>
  )
}

export default function Games() {
  const libs = useGameLibraries()
  // Games turned off for this profile: its game libraries aren't listed, and that is why.
  const off = featureOff(useMyLimits(), 'games')
  if (libs.length === 0) {
    return (
      <div className="tvm-page">
        <h1 className="tvm-title">Games</h1>
        <EmptyState icon={<IconGamepad />} title={off ? 'Games are off for this profile' : 'No games here yet'} text={off ? 'Whoever looks after this profile can turn them on.' : 'This server has no games library. An owner adds one in Settings on the server’s website.'} />
      </div>
    )
  }
  return (
    <GameLinksContext.Provider value={links}>
      {libs.length === 1
        ? <GamesLibrary key={libs[0].id} lib={libs[0]} />
        : (
          <div className="tvm-page">
            <h1 className="tvm-title">Games</h1>
            {libs.map((l) => <Shelf key={l.id} lib={l} />)}
          </div>
        )}
    </GameLinksContext.Provider>
  )
}

// A whole games library, opened from its shelf or from Libraries.
export function GamesLibraryScreen() {
  const { id } = useParams()
  const lib = useGameLibraries().find((l) => String(l.id) === id)
  if (!lib) return <Navigate to="/games" replace />
  return <GameLinksContext.Provider value={links}><GamesLibrary key={lib.id} lib={lib} /></GameLinksContext.Provider>
}

// A game's page: Play or Resume, saves, versions.
export function GameScreen() {
  const { id } = useParams()
  const [it, setIt] = useState<Item | null>(null)
  const [failed, setFailed] = useState(false)
  const load = useCallback(() => api.get<Item>(`/items/${id}`).then((i) => { setIt(i); setFailed(false) }).catch(() => setFailed(true)), [id])
  useEffect(() => { void load() }, [load])
  if (failed) return <LoadError what="This game" onRetry={() => { void load() }} />
  if (!it || String(it.id) !== id) return <Spinner />
  // WittyPlay has every game's page, on a TV too: it opens there.
  if (it.console !== 'rig') return <Navigate to={playGamePath(it.id)} replace />
  return <GameLinksContext.Provider value={links}><GameDetails it={it} reload={() => { void load() }} /></GameLinksContext.Provider>
}
