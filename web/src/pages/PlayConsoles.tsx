// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyPlay's Consoles tab: a grid of the consoles this server has games for, each a button. The one with the focus (or the pointer, or
// the one chosen) is shown off in a panel at the side: its name and mark, its era and years, how many games the library has for it,
// the core it plays on and whether it is ready (a BIOS file missing, an emulator to install), with Browse games and Settings. On a
// phone the panel is a sheet from the bottom, opened by choosing a console. A console's Settings page hosts the app's own parts for
// it: its core and BIOS files (Consoles and cores), its game settings, its controls, and the libraries. The data is the app's own
// /api/games/consoles (pages/games/consoles.tsx getConsoles).
import { lazy, Suspense, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { api, mediaUrl, type GameConsole } from '../api'
import { ConsoleMark } from '../components/ConsoleMark'
import { IconCheck, IconGamepad, IconInfo, IconSettings } from '../icons'
import { useApp } from '../state'
import { Spinner } from '../ui/Surface'
import { isBackKey } from '../ui/spatial'
import { useMedia } from '../shell/useNav'
import { LayoutSwitch, usePlayTabs } from '../shell/playPads'
import { arrangeTabs } from '../sdui/playModel'
import { libraryFor, PLAY_CONSOLES, PLAY_LIBRARY, PLAY_TABS, tabRoute, yearsWords } from '../shell/playPaths'
import { getConsoles } from './games/consoles'
import PlayLibraryView from './PlayLibraryView'
import { TileSizeSwitch } from '../shell/GamePictureArt'
import { ERA_NAMES } from './games/Shelves'
import { GameSettingsPage } from './games/consoles'
import GamingSettings from './settings/Gaming'
import './games/games-ui.css'

const ControllersScreen = lazy(() => import('./Games').then((m) => ({ default: m.ControllersScreen })))

// A console's picture of its hardware, from Wikimedia Commons by way of the server, which fetches and keeps it (GET
// /api/console-photos/{id}); none when Wikidata lookups are off, Wikimedia downloads weren't agreed to, or Commons has no free one.
type ConsolePhoto = { imageUrl: string; credit?: string; license?: string; licenseUrl?: string; pageUrl?: string; source?: string }
const photos = new Map<string, Promise<ConsolePhoto | null>>()
function useConsolePhoto(id: string): ConsolePhoto | null {
  const [p, setP] = useState<ConsolePhoto | null>(null)
  useEffect(() => {
    let on = true
    setP(null)
    if (!photos.has(id)) photos.set(id, api.get<ConsolePhoto>('/console-photos/' + encodeURIComponent(id)).catch(() => null))
    photos.get(id)!.then((x) => { if (on) setP(x) })
    return () => { on = false }
  }, [id])
  return p
}

// The console's picture, else its mark (a logo, or its name drawn).
function ConsoleArt({ c, size }: { c: GameConsole; size: 'md' | 'lg' }) {
  const photo = useConsolePhoto(c.id)
  const [broken, setBroken] = useState(false)
  if (photo && !broken) return <img className="wplay-console-photo" src={mediaUrl(photo.imageUrl)} alt="" decoding="async" loading="lazy" onError={() => setBroken(true)} />
  return <ConsoleMark console={c} size={size} on="dark" />
}

// Who took the picture, its licence and where it is: shown under it, as its licence asks.
function PhotoCredit({ id }: { id: string }) {
  const photo = useConsolePhoto(id)
  if (!photo) return null
  return (
    <p className="wplay-photo-credit">Photo: {photo.credit || 'unknown author'}{photo.license ? <>, {photo.licenseUrl ? <a href={photo.licenseUrl} target="_blank" rel="noopener noreferrer">{photo.license}</a> : photo.license}</> : null}, {photo.pageUrl ? <a href={photo.pageUrl} target="_blank" rel="noopener noreferrer">Wikimedia Commons</a> : 'Wikimedia Commons'}</p>
  )
}

const gamesOf = (c: GameConsole) => c.totalGames ?? c.count ?? 0
const gamesWords = (n: number) => `${n.toLocaleString()} ${n === 1 ? 'game' : 'games'}`

// The home's tab row, here with Consoles chosen: the same words and places as the server's page, and the layout switch at its end.
export function PlayTabsRow({ current }: { current: string }) {
  const [pref] = usePlayTabs()
  const tabs = arrangeTabs(PLAY_TABS.map((t) => ({ ...t, value: t.id })), pref.order, pref.hidden, current)
  return (
    <div className="wplay-tabs-row" data-sdui-head>
      <nav className="tabs" aria-label="Views">
        {tabs.map((t) => <Link key={t.id} to={tabRoute(t.id)} className={t.id === current ? 'active' : ''} aria-current={t.id === current ? 'page' : undefined}>{t.label}</Link>)}
      </nav>
      <div className="wplay-switches"><TileSizeSwitch /><LayoutSwitch /></div>
    </div>
  )
}

// The core (or emulator) a console plays on now, and whether it can.
function coreLine(c: GameConsole): { core: string; state: 'ready' | 'setup' | 'not-here'; note: string } {
  const emulator = c.standalone ? c.standaloneEmulators?.find((e) => e.id === c.standalone) : undefined
  const core = emulator ? `${emulator.name}${emulator.version ? ' ' + emulator.version : ''} (standalone)` : (c.installed.find((x) => x.id === (c.defaultCore || c.playsOn)) || c.installed[0])?.name || ''
  if (c.ready) return { core: core || 'Ready', state: 'ready', note: c.software ? 'Drawn by this server’s processor: no graphics card games can use.' : c.notice || '' }
  if (c.reason === 'gpu' || c.reason === 'unavailable') return { core: core || 'None', state: 'not-here', note: c.message || c.unavailable || 'This server can’t play these games.' }
  const note = c.biosMissing ? `Needs your own BIOS file: ${c.biosMissing}.` : c.reason === 'emulator' ? 'Needs its standalone emulator installed.' : c.message || 'Needs a core installed.'
  return { core: core || 'No core yet', state: 'setup', note }
}

function ConsolePanel({ c, sheet, onClose }: { c: GameConsole; sheet: boolean; onClose: () => void }) {
  const { libraries } = useApp()
  const head = useId()
  const lib = libraryFor(libraries, c.id)
  const how = coreLine(c)
  const n = gamesOf(c)
  const facts = [c.era ? ERA_NAMES[c.era] : '', yearsWords(c.launched, c.ended), c.media === 'disc' ? 'Discs' : c.media === 'cart' ? 'Cartridges' : c.media === 'pocket' ? 'Handheld' : '',
    c.capabilities?.input.ports ? (c.capabilities.input.ports > 1 ? `1 to ${c.capabilities.input.ports} players` : '1 player') : ''].filter(Boolean)
  const close = useRef<HTMLButtonElement | null>(null)
  useEffect(() => { if (sheet) close.current?.focus() }, [sheet, c.id])
  const keys = (e: KeyboardEvent<HTMLElement>) => { if (sheet && (e.key === 'Escape' || isBackKey(e.nativeEvent))) { e.preventDefault(); e.stopPropagation(); onClose() } }
  return (
    <aside key={c.id} className={'wplay-console-panel' + (sheet ? ' sheet' : '')} aria-labelledby={head} role={sheet ? 'dialog' : 'region'} aria-modal={sheet || undefined} onKeyDown={keys} aria-live={sheet ? undefined : 'polite'}>
      {sheet && <button ref={close} type="button" className="wplay-console-close" onClick={onClose}>Close</button>}
      <div className="wplay-console-mark"><ConsoleArt c={c} size="lg" /></div>
      <PhotoCredit id={c.id} />
      <h2 id={head}>{c.name}</h2>
      {facts.length > 0 && <p className="wplay-console-facts">{facts.join(' · ')}</p>}
      <dl className="wplay-console-stats">
        <div><dt>In your library</dt><dd>{gamesWords(n)}</dd></div>
        <div><dt>Plays on</dt><dd>{how.core}</dd></div>
        <div><dt>Ready</dt><dd className={'state ' + how.state}>{how.state === 'ready' ? <><IconCheck size={18} /> Plays here</> : how.state === 'setup' ? <><IconInfo size={18} /> Once set up</> : <><IconInfo size={18} /> Not on this server</>}</dd></div>
      </dl>
      {how.note && <p className="wplay-console-note">{how.note}</p>}
      <div className="wplay-console-actions">
        {lib && n > 0 && <Link className="btn primary" to={`${PLAY_LIBRARY}/${lib.id}?console=${encodeURIComponent(c.id)}`}><IconGamepad /> Browse games</Link>}
        <Link className="btn" to={`${PLAY_CONSOLES}/${encodeURIComponent(c.id)}?console=${encodeURIComponent(c.id)}`}><IconSettings /> Settings</Link>
      </div>
    </aside>
  )
}

export default function PlayConsoles() {
  const [list, setList] = useState<GameConsole[] | null>(null)
  const [failed, setFailed] = useState(false)
  useEffect(() => { getConsoles().then(setList).catch(() => setFailed(true)) }, [])
  const phone = useMedia('(max-width: 759px)')
  const shown = useMemo(() => {
    const all = list || []
    const had = all.filter((c) => gamesOf(c) > 0).sort((a, b) => gamesOf(b) - gamesOf(a) || a.short.localeCompare(b.short))
    return had.length ? had : all.filter((c) => c.ready)
  }, [list])
  const [at, setAt] = useState('')
  const [sheet, setSheet] = useState(false)
  const current = shown.find((c) => c.id === at) || shown[0]
  const head = useId()
  const pick = (id: string, open: boolean) => { setAt(id); if (open && phone) setSheet(true) }
  return (
    <div className={'wplay-consoles' + (phone ? ' phone' : '')}>
      <h1 className="sr-only">Consoles</h1>
      <PlayTabsRow current="consoles" />
      {failed ? <p className="wplay-empty">The consoles couldn’t be read. Try again in a moment.</p> : !list ? <Spinner /> : !shown.length ? (
        <p className="wplay-empty">No console has games here yet. An administrator adds a games library in Settings.</p>
      ) : (
        <div className="wplay-consoles-body">
          <section aria-labelledby={head}>
            <h2 id={head} className="wplay-consoles-h">{shown.some((c) => gamesOf(c) > 0) ? 'Consoles with games' : 'Consoles that play here'}</h2>
            <ul className="wplay-console-grid" data-nav-group>
              {shown.map((c) => (
                <li key={c.id}>
                  <button type="button" className={'wplay-console' + (current?.id === c.id ? ' on' : '')} aria-pressed={current?.id === c.id}
                    onFocus={() => !phone && setAt(c.id)} onPointerEnter={() => !phone && setAt(c.id)} onClick={() => pick(c.id, true)}>
                    <span className="wplay-console-art"><ConsoleArt c={c} size="md" /></span>
                    <b>{c.short}</b>
                    <span className="wplay-console-count">{gamesWords(gamesOf(c))}</span>
                    <i className={'wplay-console-dot ' + coreLine(c).state} aria-hidden="true" />
                  </button>
                </li>
              ))}
            </ul>
          </section>
          {current && (!phone || sheet) && <ConsolePanel c={current} sheet={phone} onClose={() => setSheet(false)} />}
          {phone && sheet && <div className="wplay-sheet-scrim" onClick={() => setSheet(false)} aria-hidden="true" />}
        </div>
      )}
    </div>
  )
}

// One console's settings: its core and BIOS files (administrators), its game settings and its controls, and the libraries.
export function PlayConsoleSettings() {
  const { id = '' } = useParams()
  const [q, setQ] = useSearchParams()
  const { user } = useApp()
  const [c, setC] = useState<GameConsole | null>(null)
  useEffect(() => { getConsoles().then((l) => setC(l.find((x) => x.id === id) || null)).catch(() => {}) }, [id])
  const parts = [
    ...(user?.isAdmin ? [{ id: 'core', label: 'Core and BIOS' }] : []),
    { id: 'games', label: 'Game settings' },
    { id: 'controls', label: 'Controls' },
    ...(user?.isAdmin ? [{ id: 'library', label: 'Library' }] : []),
  ]
  const part = parts.find((p) => p.id === q.get('part'))?.id || parts[0].id
  const go = (p: string) => { const next = new URLSearchParams(q); next.set('part', p); next.set('console', id); setQ(next, { replace: true }) }
  const name = c?.short || id.toUpperCase()
  return (
    <div className="wplay-settings-page">
      <Link className="wplay-settings-back" to={PLAY_CONSOLES}>Consoles</Link>
      <header className="wplay-console-head">{c && <ConsoleMark console={c} size="md" on="dark" />}<h1>{name} settings</h1></header>
      <div className="wplay-parts" role="tablist" aria-label={`${name} settings`}>
        {parts.map((p) => <button key={p.id} type="button" role="tab" aria-selected={p.id === part} className={p.id === part ? 'active' : ''} onClick={() => go(p.id)}>{p.label}</button>)}
      </div>
      <div role="tabpanel" className="wplay-part">
        <Suspense fallback={<Spinner />}>
          {part === 'core' && <GamingSettings only={id} />}
          {part === 'games' && <GameSettingsPage />}
          {part === 'controls' && <ControllersScreen />}
          {part === 'library' && (
            <p className="wplay-console-note">Which folders {name} games come from, a library’s console and how files are named are in <Link to="/wittyplay/settings/libraries">Settings, Libraries</Link>.</p>
          )}
        </Suspense>
      </div>
    </div>
  )
}

// A games library in WittyPlay's frame: the app's own (every game, by console, with search and filters).
export function PlayLibrary() {
  const { id } = useParams()
  const { libraries } = useApp()
  const games = libraries.filter((l) => l.type === 'games')
  const lib = (id ? games.find((l) => String(l.id) === id) : games[0])
  if (!lib) return <p className="wplay-empty">No games library here yet. An administrator adds one in Settings.</p>
  return <Suspense fallback={<Spinner />}><PlayLibraryView key={lib.id} lib={lib} /></Suspense>
}
