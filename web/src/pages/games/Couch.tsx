import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, Navigate, NavLink, Route, Routes, useLocation, useNavigate, useParams } from 'react-router-dom'
import { PromptBar } from '@wittywilla/wittyplay/web/prompts'
import { api, type GameConsole, type Item } from '../../api'
import { EmptyState, LoadError, Spinner } from '../../components/common'
import LegalNote from '../../components/LegalNote'
import { IconBack, IconGamepad, IconHome, IconLibrary, IconSearch, IconSettings } from '../../icons'
import { useApp } from '../../state'
import { Dialog } from '../../ui'
import { GameSettingsPage } from '../Games'
import { ConsoleMark, useConsoleLogos } from './ConsoleMark'
import { consoleMap, GameLinksContext, setCouchOn, type GameLinks } from './data'
import GameDetails from './Details'
import GamesLibrary, { GameCard } from './Library'
import { VirtualGames } from './VirtualGames'
import './games-ui.css'

// Couch mode: the game library alone, full screen and large, with nothing that needs a
// mouse. It's the same pages as the normal ones, in a frame of their own, so nothing in
// them is a second copy to keep in step.

const links: GameLinks = {
  couch: true,
  item: (id) => `/big/game/${id}`,
  library: (lib, query) => `/big/library/${lib}${query ? '?' + query : ''}`,
  play: (id, query) => `/game/${id}${query ? '?' + query : ''}`,
}

function useClock() {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    // On the minute, so it's only ever one small repaint a minute.
    let t = 0
    const next = () => { t = window.setTimeout(() => { setNow(new Date()); next() }, 60000 - (Date.now() % 60000)) }
    next()
    return () => clearTimeout(t)
  }, [])
  return now.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

function useGameLibraries() {
  const { libraries } = useApp()
  return useMemo(() => libraries.filter((l) => l.type === 'games' && l.console !== 'rig'), [libraries])
}

function CouchHome() {
  const libs = useGameLibraries()
  if (libs.length === 0) return <EmptyState icon={<IconGamepad />} title="No game libraries yet" text="Add one in Settings, Libraries, and it shows up here." />
  return <Navigate to={`/big/library/${libs[0].id}`} replace />
}

function CouchLibrary() {
  const { id } = useParams()
  const libs = useGameLibraries()
  const lib = libs.find((l) => String(l.id) === id)
  if (!lib) return <Navigate to="/big" replace />
  return <GamesLibrary key={lib.id} lib={lib} />
}

function CouchGame() {
  const { id } = useParams()
  const [it, setIt] = useState<Item | null>(null)
  const [failed, setFailed] = useState(false)
  const load = useCallback(() => api.get<Item>(`/items/${id}`).then((i) => { setIt(i); setFailed(false) }).catch(() => setFailed(true)), [id])
  useEffect(() => { void load() }, [load])
  if (failed) return <LoadError what="This game" onRetry={() => { void load() }} />
  if (!it || String(it.id) !== id) return <Spinner />
  return <GameDetails it={it} reload={() => { void load() }} />
}

function CouchConsoles() {
  const libs = useGameLibraries()
  const [lists, setLists] = useState<{ lib: number; name: string; consoles: GameConsole[] }[] | null>(null)
  useEffect(() => {
    Promise.all(libs.map((l) => api.get<GameConsole[]>(`/games/consoles?library=${l.id}`).then((c) => ({ lib: l.id, name: l.name, consoles: c.filter((x) => (x.count || 0) > 0).sort((a, b) => (b.count || 0) - (a.count || 0)) })))).then(setLists).catch(() => setLists([]))
  }, [libs])
  const logosShown = useConsoleLogos((lists || []).flatMap((l) => l.consoles))
  if (!lists) return <Spinner />
  return (
    <div className="gm-page">
      <div className="toolbar"><h1>Consoles</h1></div>
      {lists.map((l) => (
        <section key={l.lib} className="hub gm-section" data-gm-section aria-label={l.name}>
          {lists.length > 1 && <div className="hub-head"><h2>{l.name}</h2></div>}
          <div className="gm-console-grid">
            {l.consoles.map((c) => (
              <Link key={c.id} to={links.library(l.lib, 'console=' + encodeURIComponent(c.id))} className="gm-console big" aria-label={`${c.name}: ${(c.count || 0).toLocaleString()} games`}>
                <ConsoleMark console={c} />
                <small>{(c.count || 0).toLocaleString()} {c.count === 1 ? 'game' : 'games'}</small>
              </Link>
            ))}
          </div>
        </section>
      ))}
      {logosShown && <LegalNote kind="logos" />}
    </div>
  )
}

// Letters on the screen, for a controller with no keyboard. Real buttons, so the same
// arrows and OK work as everywhere else.
const KEYS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'.split('')

function CouchSearch() {
  const libs = useGameLibraries()
  const [typed, setTyped] = useState('')
  const [q, setQ] = useState('')
  const [consoles, setConsoles] = useState<GameConsole[]>([])
  const [lib, setLib] = useState(libs[0]?.id || 0)
  const first = useRef<HTMLButtonElement>(null)
  useEffect(() => { first.current?.focus({ preventScroll: true }) }, [])
  useEffect(() => { const t = setTimeout(() => setQ(typed.trim()), 250); return () => clearTimeout(t) }, [typed])
  useEffect(() => { if (lib) api.get<GameConsole[]>(`/games/consoles?library=${lib}`).then(setConsoles).catch(() => {}) }, [lib])
  const byId = useMemo(() => consoleMap(consoles), [consoles])
  const query = `library=${lib}&kind=game&sort=title&desc=0&search=${encodeURIComponent(q)}`
  return (
    <div className="gm-page">
      <div className="toolbar"><h1>Search</h1></div>
      <div className="gm-osk-wrap">
        <div className="gm-osk-side">
          <label className="gm-osk-field">Game’s name
            <input className="input" type="text" value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" spellCheck={false} placeholder="Start typing" />
          </label>
          <div className="gm-osk" role="group" aria-label="Letters">
            {KEYS.map((k, i) => <button key={k} ref={i === 0 ? first : undefined} type="button" className="gm-osk-key" onClick={() => setTyped((t) => t + (t ? k.toLowerCase() : k))}>{k}</button>)}
            <button type="button" className="gm-osk-key wide" onClick={() => setTyped((t) => t + ' ')}>Space</button>
            <button type="button" className="gm-osk-key wide" onClick={() => setTyped((t) => t.slice(0, -1))}>Delete</button>
            <button type="button" className="gm-osk-key wide" onClick={() => setTyped('')}>Clear</button>
          </div>
          {libs.length > 1 && (
            <div className="chips" role="group" aria-label="Which library">
              {libs.map((l) => <button key={l.id} type="button" className={'chip' + (l.id === lib ? ' active' : '')} aria-pressed={l.id === lib} onClick={() => setLib(l.id)}>{l.name}</button>)}
            </div>
          )}
        </div>
        <div className="gm-osk-results">
          {q
            ? <VirtualGames query={query} label="Search results" card={(it) => <GameCard item={it} console={byId.get(it.console || '')} />}
                empty={<EmptyState icon={<IconSearch />} title="Nothing found" text={`No games here match “${q}”.`} />} failed={(retry) => <LoadError what="Search" onRetry={retry} />} />
            : <EmptyState icon={<IconSearch />} title="Find a game" text="Choose letters on the left, or type on a keyboard." />}
        </div>
      </div>
    </div>
  )
}

export default function CouchMode() {
  const nav = useNavigate()
  const loc = useLocation()
  const clock = useClock()
  const { user } = useApp()
  const libs = useGameLibraries()
  const [leaving, setLeaving] = useState(false)
  useEffect(() => {
    setCouchOn(true)
    document.documentElement.classList.add('ten-foot', 'gm-couch')
    return () => { document.documentElement.classList.remove('ten-foot', 'gm-couch') }
  }, [])
  const exit = useCallback(() => {
    setCouchOn(false)
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => {})
    nav(libs[0] ? `/library/${libs[0].id}` : '/')
  }, [nav, libs])
  // Back at the top of couch mode asks before leaving it.
  const atTop = /^\/big(\/library\/\d+)?\/?$/.test(loc.pathname) && !loc.search
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || document.querySelector('.modal, [role=menu]')) return
      if (e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack') {
        e.preventDefault()
        e.stopImmediatePropagation()
        if (atTop) setLeaving(true)
        else nav(-1)
      }
    }
    // First in line, or the page-wide Back would leave couch mode without asking.
    window.addEventListener('keydown', onKey, true)
    return () => window.removeEventListener('keydown', onKey, true)
  }, [atTop, nav])
  // Start from the first thing on a new screen, as a console does.
  const main = useRef<HTMLElement>(null)
  useEffect(() => {
    window.scrollTo(0, 0)
    const t = window.setTimeout(() => {
      if (document.activeElement && document.activeElement !== document.body && main.current?.contains(document.activeElement)) return
      main.current?.querySelector<HTMLElement>('[data-front="1"], .gm-game-actions .primary, a[href].card, a[href].gm-console, button')?.focus({ preventScroll: true })
    }, 350)
    return () => clearTimeout(t)
  }, [loc.pathname])
  return (
    <GameLinksContext.Provider value={links}>
      <div className="gm-couch-frame">
        <header className="gm-couch-bar">
          <nav className="gm-couch-tabs" aria-label="Couch mode" data-nav-group>
            {libs.length > 1
              ? libs.map((l, i) => <NavLink key={l.id} to={`/big/library/${l.id}`} className="gm-couch-tab">{i === 0 && <IconHome />} <span>{l.name}</span></NavLink>)
              : <NavLink to={libs[0] ? `/big/library/${libs[0].id}` : '/big'} className="gm-couch-tab" end><IconHome /> <span>Home</span></NavLink>}
            <NavLink to="/big/consoles" className="gm-couch-tab"><IconLibrary /> <span>Consoles</span></NavLink>
            <NavLink to="/big/search" className="gm-couch-tab"><IconSearch /> <span>Search</span></NavLink>
            <NavLink to="/big/settings" className="gm-couch-tab"><IconSettings /> <span>Settings</span></NavLink>
          </nav>
          <div className="gm-couch-right">
            <span className="gm-couch-name">{user?.name}</span>
            <time className="gm-couch-clock">{clock}</time>
            <button type="button" className="btn ghost" onClick={() => setLeaving(true)}><IconBack size={18} /> Leave</button>
          </div>
        </header>
        <main ref={main} id="main" className="content gm-couch-main" tabIndex={-1}>
          <Routes>
            <Route index element={<CouchHome />} />
            <Route path="library/:id" element={<CouchLibrary />} />
            <Route path="game/:id" element={<CouchGame />} />
            <Route path="consoles" element={<CouchConsoles />} />
            <Route path="search" element={<CouchSearch />} />
            <Route path="settings" element={<div className="gm-page"><div className="toolbar"><h1>My game settings</h1></div><GameSettingsPage /><LegalNote kind="games" /></div>} />
            <Route path="*" element={<Navigate to="/big" replace />} />
          </Routes>
        </main>
        {/* The library and a game's page say what their own buttons do. */}
        {!/^\/big\/(library|game)\//.test(loc.pathname) && (
          <footer className="gm-couch-foot">
            <PromptBar always items={[{ button: 'dpad', label: 'Move' }, { button: 'confirm', label: 'Select' }, { button: 'back', label: 'Back' }]} />
          </footer>
        )}
      </div>
      {leaving && (
        <Dialog title="Leave couch mode?" onClose={() => setLeaving(false)} size="sm" actions={<><button type="button" className="btn" onClick={() => setLeaving(false)}>Stay</button><button type="button" className="btn primary" autoFocus onClick={exit}>Leave</button></>}>
          <p>This goes back to the normal app. You can come back with the Couch mode button, or Start on a controller.</p>
        </Dialog>
      )}
    </GameLinksContext.Provider>
  )
}
