import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AppProvider, ToastHost, useApp } from './state'
import { api } from './api'
import { IconFilm, IconHome, IconLogout, IconMenu, IconSearch, IconSettings, IconUsers, libIcon } from './icons'
import { useNavStyle } from './navPrefs'
import { Avatar } from './pages/AuthPage'
import HomePage from './pages/HomePage'
import { Brand, BrandMark, UpdateDot, VersionTag } from './components/common'
import LibraryPage from './pages/LibraryPage'
import ItemPage from './pages/ItemPage'
import ShowPage from './pages/ShowPage'
import { AlbumPage } from './pages/AudioLibrary'
import { AudioProvider } from './audio/player'
import ReaderPage from './pages/Reader'
import { GamePage } from './pages/Games'
import SettingsPage from './pages/SettingsPage'
import SearchPage from './pages/SearchPage'
import AuthPage from './pages/AuthPage'
import SetupWizard from './pages/SetupWizard'
import { PlayerDock } from './player/PlayerPage'
import { Announcer, useDialogs } from './a11y'

export default function App() {
  useDialogs()
  return (
    <AppProvider>
      <Shell />
      <ToastHost />
      <Announcer />
    </AppProvider>
  )
}

function Shell() {
  const { user, loading, setupRequired, onboarding } = useApp()
  const loc = useLocation()
  const [navOpen, setNavOpen] = useState(() => window.matchMedia('(min-width: 901px)').matches)
  const isMobile = () => !window.matchMedia('(min-width: 901px)').matches
  // Close the drawer on navigation (mobile only).
  useEffect(() => { if (isMobile()) setNavOpen(false) }, [loc.pathname])
  // Follow viewport changes: open by default on desktop, closed on mobile.
  useEffect(() => {
    const mq = window.matchMedia('(min-width: 901px)')
    const onChange = (e: MediaQueryListEvent) => setNavOpen(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])
  if (loading) return <div className="center"><div className="spinner" /></div>
  if (!user) return setupRequired ? <SetupWizard resume={false} /> : <AuthPage />
  if (onboarding && user.isAdmin) return <SetupWizard resume />
  // The player lives outside the page routes (see PlayerDock) so it can keep
  // playing as a mini player while the viewer browses.
  const isPlayer = loc.pathname.startsWith('/play/')
  const isReader = loc.pathname.startsWith('/read/')
  const isGame = loc.pathname.startsWith('/game/')
  return (
    <AudioProvider>
      {isReader ? <Routes><Route path="/read/:id" element={<ReaderPage />} /></Routes>
        : isGame ? <Routes><Route path="/game/:id" element={<GamePage />} /></Routes>
        : isPlayer ? null : <AppFrame navOpen={navOpen} setNavOpen={setNavOpen} />}
      <PlayerDock />
    </AudioProvider>
  )
}

function AppFrame({ navOpen, setNavOpen }: { navOpen: boolean; setNavOpen: React.Dispatch<React.SetStateAction<boolean>> }) {
  const style = useNavStyle()
  const side = style !== 'top'
  return (
    <div className={'app nav-' + style + (navOpen ? ' nav-open' : '')}>
      <a className="skip-link" href="#main">Skip to content</a>
      <Sidebar open={navOpen} onClose={() => setNavOpen(false)} />
      <div className="main">
        <TopBar onMenu={() => setNavOpen((o) => !o)} showMenu={side} topNav={style !== 'side'} />
        <main className="content" id="main" tabIndex={-1}>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/library/:id" element={<LibraryPage />} />
            <Route path="/item/:id" element={<ItemPage />} />
            <Route path="/show/:id" element={<ShowPage />} />
            <Route path="/album/:lib" element={<AlbumPage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/settings/*" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
      <TabBar onLibraries={() => setNavOpen(true)} />
    </div>
  )
}


function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { libraries, version, brandName } = useApp()
  return (
    <>
      {open && <div className="backdrop-dim" onClick={onClose} />}
      <aside className="sidebar" aria-label="Main navigation" aria-hidden={!open} inert={!open || undefined}>
        <Link to="/" className="brand"><BrandMark /><Brand name={brandName} /></Link>
        <NavLink to="/" end className="nav-link"><IconHome /> Home</NavLink>
        <div className="nav-section">Libraries</div>
        {libraries.map((l) => <NavLink key={l.id} to={`/library/${l.id}`} className="nav-link">{libIcon(l.type)} {l.name}</NavLink>)}
        {libraries.length === 0 && <div className="nav-link muted" style={{ fontSize: 13 }}>No libraries yet</div>}
        <div className="nav-section">Manage</div>
        <NavLink to="/settings" className="nav-link"><IconSettings /> Settings<UpdateDot /></NavLink>
        <div className="sidebar-footer"><VersionTag version={version} /></div>
      </aside>
    </>
  )
}

function TopBar({ onMenu, showMenu, topNav }: { onMenu: () => void; showMenu: boolean; topNav: boolean }) {
  const nav = useNavigate()
  const loc = useLocation()
  const [q, setQ] = useState('')
  useEffect(() => { if (!loc.pathname.startsWith('/search')) setQ('') }, [loc.pathname])
  const { libraries, brandName } = useApp()
  return (
    <header className="topbar">
      {showMenu && <button className="btn icon menu-toggle" onClick={onMenu} aria-label="Show or hide the side menu" title="Side menu"><IconMenu /></button>}
      {topNav && !showMenu && <Link to="/" className="brand top-brand"><BrandMark /><Brand name={brandName} /></Link>}
      {topNav && (
        <nav className="topnav" aria-label="Libraries">
          <NavLink to="/" end className="topnav-link"><IconHome /> <span>Home</span></NavLink>
          {libraries.map((l) => <NavLink key={l.id} to={`/library/${l.id}`} className="topnav-link">{libIcon(l.type)} <span>{l.name}</span></NavLink>)}
        </nav>
      )}
      <form className="search" role="search" onSubmit={(e) => { e.preventDefault(); if (q.trim()) nav('/search?q=' + encodeURIComponent(q.trim())) }}>
        <IconSearch />
        <input aria-label="Search" type="search" placeholder="Search movies, shows, episodes…" value={q} onChange={(e) => setQ(e.target.value)} />
      </form>
      {!topNav && <div className="spacer" />}
      <ProfileMenu />
    </header>
  )
}

// ProfileMenu: the avatar in the corner, with settings, switching profiles
// (back to "Who's watching?") and signing out.
function ProfileMenu() {
  const { user, setUser } = useApp()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !ref.current?.contains(e.target as Node)) setOpen(false)
    }
    window.addEventListener('mousedown', close)
    window.addEventListener('keydown', close)
    return () => { window.removeEventListener('mousedown', close); window.removeEventListener('keydown', close) }
  }, [open])
  const signOut = async () => { try { await api.post('/logout') } catch { /* ignore */ } setUser(null) }
  return (
    <div className="profile-menu" ref={ref}>
      <button className="avatar-btn" aria-haspopup="menu" aria-expanded={open} aria-label={`${user?.name}: account menu`} onClick={() => setOpen((o) => !o)}>
        <span className="avatar-name">{user?.name}</span>
        <Avatar name={user?.name || '?'} />
      </button>
      {open && (
        <div className="menu" role="menu" onClick={() => setOpen(false)}>
          <div className="menu-head"><b>{user?.name}</b><span className="muted">{user?.isAdmin ? 'Administrator' : user?.kind === 'kids' ? 'Kids profile' : user?.kind === 'guest' ? 'Guest' : 'Profile'}</span></div>
          <Link role="menuitem" to="/settings/appearance">Appearance</Link>
          <Link role="menuitem" to="/settings">Settings</Link>
          <button role="menuitem" onClick={signOut}><IconUsers /> Switch profile</button>
          <button role="menuitem" onClick={signOut}><IconLogout /> Sign out</button>
        </div>
      )}
    </div>
  )
}

// TabBar: the bottom bar on phones (thumb reach), always present there.
function TabBar({ onLibraries }: { onLibraries: () => void }) {
  return (
    <nav className="tabbar" aria-label="Main">
      <NavLink to="/" end><IconHome /><span>Home</span></NavLink>
      <NavLink to="/search"><IconSearch /><span>Search</span></NavLink>
      <button type="button" onClick={onLibraries}><IconFilm /><span>Libraries</span></button>
      <NavLink to="/settings" className="tab-upd"><IconSettings /><span>You</span><UpdateDot /></NavLink>
    </nav>
  )
}
