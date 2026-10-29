import { useEffect, useState } from 'react'
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AppProvider, ToastHost, useApp } from './state'
import { api } from './api'
import { IconDisc, IconFilm, IconHome, IconLogout, IconMenu, IconSearch, IconSettings, IconTv } from './icons'
import HomePage from './pages/HomePage'
import { Brand, VersionTag } from './components/common'
import LibraryPage from './pages/LibraryPage'
import ItemPage from './pages/ItemPage'
import ShowPage from './pages/ShowPage'
import SettingsPage from './pages/SettingsPage'
import SearchPage from './pages/SearchPage'
import AuthPage from './pages/AuthPage'
import { PlayerDock } from './player/PlayerPage'

export default function App() {
  return (
    <AppProvider>
      <Shell />
      <ToastHost />
    </AppProvider>
  )
}

function Shell() {
  const { user, loading, setupRequired } = useApp()
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
  if (!user) return <AuthPage setup={setupRequired} />
  // The player lives outside the page routes (see PlayerDock) so it can keep
  // playing as a mini player while the viewer browses.
  const isPlayer = loc.pathname.startsWith('/play/')
  return (
    <>
      {isPlayer ? null : <AppFrame navOpen={navOpen} setNavOpen={setNavOpen} />}
      <PlayerDock />
    </>
  )
}

function AppFrame({ navOpen, setNavOpen }: { navOpen: boolean; setNavOpen: React.Dispatch<React.SetStateAction<boolean>> }) {
  return (
    <div className={'app' + (navOpen ? ' nav-open' : '')}>
      <a className="skip-link" href="#main">Skip to content</a>
      <Sidebar open={navOpen} onClose={() => setNavOpen(false)} />
      <div className="main">
        <TopBar onMenu={() => setNavOpen((o) => !o)} />
        <main className="content" id="main" tabIndex={-1}>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/library/:id" element={<LibraryPage />} />
            <Route path="/item/:id" element={<ItemPage />} />
            <Route path="/show/:id" element={<ShowPage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/settings/*" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
    </div>
  )
}

function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { libraries, user, version, setUser, brandName } = useApp()
  const logout = async () => { try { await api.post('/logout') } catch { /* ignore */ } setUser(null) }
  return (
    <>
      {open && <div className="backdrop-dim" onClick={onClose} />}
      <aside className="sidebar" aria-label="Main navigation" aria-hidden={!open} inert={!open || undefined}>
        <Link to="/" className="brand"><img src="/icon.svg" alt="" /><Brand name={brandName} /></Link>
        <NavLink to="/" end className="nav-link"><IconHome /> Home</NavLink>
        <div className="nav-section">Libraries</div>
        {libraries.map((l) => (
          <NavLink key={l.id} to={`/library/${l.id}`} className="nav-link">
            {l.type === 'movies' ? <IconFilm /> : l.type === 'shows' ? <IconTv /> : <IconDisc />} {l.name}
          </NavLink>
        ))}
        {libraries.length === 0 && <div className="nav-link muted" style={{ fontSize: 13 }}>No libraries yet</div>}
        <div className="nav-section">Manage</div>
        <NavLink to="/settings" className="nav-link"><IconSettings /> Settings</NavLink>
        <button className="nav-link" style={{ background: 'none', border: 0, width: '100%' }} onClick={logout}><IconLogout /> Sign out ({user?.name})</button>
        <div className="sidebar-footer"><VersionTag version={version} /></div>
      </aside>
    </>
  )
}

function TopBar({ onMenu }: { onMenu: () => void }) {
  const nav = useNavigate()
  const loc = useLocation()
  const [q, setQ] = useState('')
  useEffect(() => { if (!loc.pathname.startsWith('/search')) setQ('') }, [loc.pathname])
  const { user } = useApp()
  return (
    <header className="topbar">
      <button className="btn icon menu-toggle" onClick={onMenu} aria-label="Toggle navigation" title="Toggle navigation"><IconMenu /></button>
      <form className="search" onSubmit={(e) => { e.preventDefault(); if (q.trim()) nav('/search?q=' + encodeURIComponent(q.trim())) }}>
        <IconSearch />
        <input aria-label="Search" type="search" placeholder="Search movies, shows, episodes…" value={q} onChange={(e) => setQ(e.target.value)} />
      </form>
      <div className="spacer" />
      <Link to="/settings/users" className="avatar" title={user?.name}>{user?.name?.[0]?.toUpperCase()}</Link>
    </header>
  )
}
