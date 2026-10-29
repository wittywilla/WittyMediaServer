import { useEffect, useState } from 'react'
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AppProvider, ToastHost, useApp } from './state'
import { api } from './api'
import { IconDisc, IconFilm, IconHome, IconLogout, IconMenu, IconSearch, IconSettings } from './icons'
import HomePage from './pages/HomePage'
import LibraryPage from './pages/LibraryPage'
import ItemPage from './pages/ItemPage'
import SettingsPage from './pages/SettingsPage'
import SearchPage from './pages/SearchPage'
import AuthPage from './pages/AuthPage'
import PlayerPage from './player/PlayerPage'

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
  const isPlayer = loc.pathname.startsWith('/play/')
  if (isPlayer) {
    return (
      <Routes>
        <Route path="/play/:id" element={<PlayerPage />} />
      </Routes>
    )
  }
  return (
    <div className={'app' + (navOpen ? ' nav-open' : '')}>
      <Sidebar open={navOpen} onClose={() => setNavOpen(false)} />
      <div className="main">
        <TopBar onMenu={() => setNavOpen((o) => !o)} />
        <div className="content">
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/library/:id" element={<LibraryPage />} />
            <Route path="/item/:id" element={<ItemPage />} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/settings/*" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </div>
      </div>
    </div>
  )
}

function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { libraries, user, version, setUser } = useApp()
  const logout = async () => { try { await api.post('/logout') } catch { /* ignore */ } setUser(null) }
  return (
    <>
      {open && <div className="backdrop-dim" onClick={onClose} />}
      <aside className="sidebar" aria-hidden={!open}>
        <Link to="/" className="brand"><img src="/icon.svg" alt="" /><span>Witty<b>Media</b>Server</span></Link>
        <NavLink to="/" end className="nav-link"><IconHome /> Home</NavLink>
        <div className="nav-section">Libraries</div>
        {libraries.map((l) => (
          <NavLink key={l.id} to={`/library/${l.id}`} className="nav-link">
            {l.type === 'movies' ? <IconFilm /> : <IconDisc />} {l.name}
          </NavLink>
        ))}
        {libraries.length === 0 && <div className="nav-link muted" style={{ fontSize: 13 }}>No libraries yet</div>}
        <div className="nav-section">Manage</div>
        <NavLink to="/settings" className="nav-link"><IconSettings /> Settings</NavLink>
        <button className="nav-link" style={{ background: 'none', border: 0, width: '100%' }} onClick={logout}><IconLogout /> Sign out ({user?.name})</button>
        <div className="sidebar-footer">WittyMediaServer {version}</div>
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
        <input placeholder="Search movies…" value={q} onChange={(e) => setQ(e.target.value)} />
      </form>
      <div className="spacer" />
      <Link to="/settings/users" className="avatar" title={user?.name}>{user?.name?.[0]?.toUpperCase()}</Link>
    </header>
  )
}
