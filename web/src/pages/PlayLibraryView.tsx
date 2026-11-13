// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A games library in WittyPlay (/wittyplay/library/:id): every game it holds, a row for each console, the way WittyPlay draws games (in
// their case or cartridge, or their box art, as chosen in Game pictures). Search finds any of them; a console's All shows every one of its
// games as a grid; each game opens its own page. What a scan wasn't sure about ("files need a look") is offered at the top, to an admin.
// Built for a controller first: every row is a line a d-pad walks, entered at its first game.
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, posterUrl, type GameConsole, type Item, type Library } from '../api'
import { IconLeft, IconRefresh, IconSearch } from '../icons'
import { useApp } from '../state'
import { Spinner } from '../ui/Surface'
import { GameArtContext } from '../sdui/render/glyphs'
import { playGamePath } from '../shell/playPaths'
import { GameReviewBanner } from './GameReview'
import { PlayTabsRow } from './PlayConsoles'

const ROW = 24
const PAGE = 60

// One game: drawn as WittyPlay draws it, its name under it, opening its page.
function GameTile({ it, short }: { it: Item; short: string }) {
  const art = useContext(GameArtContext)
  const image = it.poster ? posterUrl(it) : ''
  const drawn = art?.art({ itemId: String(it.id), console: short || it.console || '', image, title: it.title })
  return (
    <Link className="wpl-game" to={playGamePath(it.id)} aria-label={it.title + (it.year ? `, ${it.year}` : '')}>
      <span className="wpl-game-art">{drawn ?? (image ? <img src={image} alt="" loading="lazy" /> : <span className="wpl-game-blank">{it.title}</span>)}</span>
      <span className="wpl-game-name">{it.title}</span>
    </Link>
  )
}

// A console's row: its games sideways, and All for the rest.
function ConsoleRow({ lib, c }: { lib: Library; c: GameConsole }) {
  const [items, setItems] = useState<Item[] | null>(null)
  const [, setParams] = useSearchParams()
  useEffect(() => {
    let on = true
    api.get<{ items: Item[] }>(`/items?library=${lib.id}&kind=game&console=${encodeURIComponent(c.id)}&sort=popular&desc=0&limit=${ROW}`)
      .then((r) => { if (on) setItems(r.items || []) }).catch(() => { if (on) setItems([]) })
    return () => { on = false }
  }, [lib.id, c.id])
  if (items && items.length === 0) return null
  return (
    <section className="wpl-row" aria-labelledby={`wpl-${c.id}`}>
      <div className="wpl-row-head">
        <h2 id={`wpl-${c.id}`}>{c.short || c.name}{c.count ? <span className="wpl-count">{c.count.toLocaleString()}</span> : null}</h2>
        <button type="button" className="wpl-all" onClick={() => setParams({ console: c.id })}>All</button>
      </div>
      {items === null ? <div className="wpl-wait"><Spinner /></div> : (
        <div className="wpl-line" data-nav-group data-nav-entry=":scope > :first-child">
          {items.map((it) => <GameTile key={it.id} it={it} short={c.short} />)}
        </div>
      )}
    </section>
  )
}

// Every game of a console, or every game a search finds: a grid, more as it scrolls.
function GameGrid({ lib, consoleId, query, short }: { lib: Library; consoleId: string; query: string; short: string }) {
  const [items, setItems] = useState<Item[]>([])
  const [total, setTotal] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const end = useRef<HTMLDivElement>(null)
  const base = `/items?library=${lib.id}&kind=game&sort=title&desc=0${consoleId ? `&console=${encodeURIComponent(consoleId)}` : ''}${query ? `&search=${encodeURIComponent(query)}` : ''}`
  const more = useCallback((from: number) => {
    setBusy(true)
    api.get<{ items: Item[]; total: number }>(`${base}&limit=${PAGE}&offset=${from}`)
      .then((r) => { setItems((x) => (from === 0 ? r.items || [] : [...x, ...(r.items || [])])); setTotal(r.total ?? null) })
      .catch(() => setTotal(0)).finally(() => setBusy(false))
  }, [base])
  useEffect(() => { setItems([]); setTotal(null); more(0) }, [more])
  useEffect(() => {
    const el = end.current
    if (!el) return
    const io = new IntersectionObserver((e) => { if (e[0].isIntersecting && !busy && total !== null && items.length < total) more(items.length) }, { rootMargin: '600px' })
    io.observe(el)
    return () => io.disconnect()
  }, [busy, total, items.length, more])
  if (total === 0) return <p className="wpl-empty">{query ? `Nothing here matches “${query}”.` : 'No games for this console yet.'}</p>
  return (
    <>
      {total !== null && <p className="wpl-found" role="status">{total.toLocaleString()} {total === 1 ? 'game' : 'games'}</p>}
      <div className="wpl-grid" data-nav-group data-nav-entry=":scope > :first-child">
        {items.map((it) => <GameTile key={it.id} it={it} short={short} />)}
      </div>
      <div ref={end} aria-hidden="true" />
      {busy && <div className="wpl-wait"><Spinner /></div>}
    </>
  )
}

export default function PlayLibraryView({ lib }: { lib: Library }) {
  const { user } = useApp()
  const [params, setParams] = useSearchParams()
  const consoleId = params.get('console') || ''
  const q = params.get('q') || ''
  const [draft, setDraft] = useState(q)
  const [consoles, setConsoles] = useState<GameConsole[] | null>(null)
  const [version, setVersion] = useState(0)
  const [scanning, setScanning] = useState(false)
  useEffect(() => { setDraft(q) }, [q])
  useEffect(() => {
    api.get<GameConsole[]>(`/games/consoles?library=${lib.id}`).then((cs) => setConsoles(cs.filter((c) => (c.count || 0) > 0).sort((a, b) => (b.count || 0) - (a.count || 0)))).catch(() => setConsoles([]))
  }, [lib.id, version])
  // Searching as someone types, a moment after they stop.
  useEffect(() => {
    if (draft === q) return
    const t = window.setTimeout(() => setParams((p) => { const n = new URLSearchParams(p); if (draft) n.set('q', draft); else n.delete('q'); return n }, { replace: true }), 300)
    return () => window.clearTimeout(t)
  }, [draft]) // eslint-disable-line react-hooks/exhaustive-deps
  const scan = async () => {
    setScanning(true)
    try { await api.post(`/libraries/${lib.id}/scan`) } catch { /* the library's own page says why */ }
    window.setTimeout(() => { setScanning(false); setVersion((v) => v + 1) }, 4000)
  }
  const total = useMemo(() => (consoles || []).reduce((n, c) => n + (c.count || 0), 0), [consoles])
  const chosen = consoles?.find((c) => c.id === consoleId)
  return (
    <div className="wpl">
      {!consoleId && !q && <PlayTabsRow current="all" />}
      <header className="wpl-head">
        {consoleId && <button type="button" className="wpl-back" onClick={() => setParams({})} aria-label="Back to every console"><IconLeft /></button>}
        <h1>{chosen ? chosen.name : lib.name}</h1>
        {!consoleId && total > 0 && <span className="wpl-count" aria-label={`${total.toLocaleString()} games`}>{total.toLocaleString()}</span>}
        <label className="wpl-search">
          <IconSearch />
          <input type="search" value={draft} onChange={(e) => setDraft(e.target.value)} placeholder={chosen ? `Search ${chosen.short || chosen.name} games` : 'Search these games'} aria-label="Search these games" />
        </label>
        {user?.isAdmin && <button type="button" className="wpl-btn" onClick={() => void scan()} disabled={scanning}><IconRefresh /> {scanning ? 'Looking…' : 'Scan'}</button>}
      </header>
      {user?.isAdmin && <div className="wpl-review"><GameReviewBanner libraryId={lib.id} consoles={consoles || []} version={version} onChanged={() => setVersion((v) => v + 1)} /></div>}
      {q || consoleId
        ? <GameGrid key={`${consoleId}|${q}`} lib={lib} consoleId={consoleId} query={q} short={chosen?.short || ''} />
        : consoles === null ? <div className="wpl-wait"><Spinner /></div>
          : consoles.length === 0 ? <p className="wpl-empty">No games found in this library yet. {user?.isAdmin ? 'Scan to look through its folders.' : ''}</p>
            : consoles.map((c) => <ConsoleRow key={c.id} lib={lib} c={c} />)}
    </div>
  )
}
