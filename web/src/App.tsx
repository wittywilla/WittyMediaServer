import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { Link, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { AppProvider, ToastHost, useApp } from './state'
import { api, inMainMenus } from './api'
import { IconFilm, IconGamepad, IconHome, IconLibrary, IconLogout, IconMenu, IconPalette, IconSearch, IconSettings, IconStar, IconTv, IconUsers, libIcon } from './icons'
import { useNavStyle } from './navPrefs'
import { Avatar } from './pages/AuthPage'
import HomePage from './pages/HomePage'
import { Brand, BrandMark, Spinner, UpdateDot, VersionTag } from './components/common'
import { AudioProvider } from './audio/player'
import PairPage from './pages/PairPage'
import ConnectServer from './components/ConnectServer'
import AuthPage from './pages/AuthPage'
import { PlayerDock } from './player/PlayerPage'
import { Announcer, useDialogs } from './a11y'
import LegalNote, { legalAckText } from './components/LegalNote'

const SharedPage = lazy(() => import('./pages/SharedPage'))
const LegalPublicPage = lazy(() => import('./pages/settings/Legal').then((m) => ({ default: m.LegalPublicPage })))
const GamePage = lazy(() => import('./pages/Games').then((m) => ({ default: m.GamePage })))
const RadioStationPage = lazy(() => import('./pages/RadioLibrary').then((m) => ({ default: m.RadioStationPage })))
const ControllersScreen = lazy(() => import('./pages/Games').then((m) => ({ default: m.ControllersScreen })))
const WatchPage = lazy(() => import('./pages/creators/WatchPage'))
const CollectionsPage = lazy(() => import('./pages/CollectionsPage'))
const MoonlightPage = lazy(() => import('./pages/Moonlight'))
const MoonlightHostPage = lazy(() => import('./pages/Moonlight').then((m) => ({ default: m.MoonlightHostPage })))
const MoonlightPlayPage = lazy(() => import('./pages/Moonlight').then((m) => ({ default: m.MoonlightPlayPage })))
const RigPlayPage = lazy(() => import('./pages/Moonlight').then((m) => ({ default: m.RigPlayPage })))
const CollectionPage = lazy(() => import('./pages/CollectionsPage').then((m) => ({ default: m.CollectionPage })))
const ReviewPage = lazy(() => import('./pages/ReviewPage'))
const CreatorPage = lazy(() => import('./pages/creators/ChannelsLibrary').then((m) => ({ default: m.CreatorPage })))
// Every other page comes when it is opened: the first screen a phone or an
// old TV has to load stays small. The player has its own (hls.js is large).
const LibraryPage = lazy(() => import('./pages/LibraryPage'))
const ItemPage = lazy(() => import('./pages/ItemPage'))
const ShowPage = lazy(() => import('./pages/ShowPage'))
const CdPlayer = lazy(() => import('./player/CdPlayer'))
const AlbumPage = lazy(() => import('./pages/AudioLibrary').then((m) => ({ default: m.AlbumPage })))
const ReaderPage = lazy(() => import('./pages/Reader'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const SearchPage = lazy(() => import('./pages/SearchPage'))
const SetupWizard = lazy(() => import('./pages/SetupWizard'))
// TV is its own part of the app, loaded when someone goes there.
const GuidePage = lazy(() => import('./pages/tv/GuidePage'))
const RecordingsPage = lazy(() => import('./pages/tv/RecordingsPage'))
const EncryptedHelp = lazy(() => import('./pages/tv/EncryptedHelp'))
const TvPlayer = lazy(() => import('./pages/tv/TvPlayer'))
const tvFallback = <div className="center"><div className="spinner" /></div>

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
  const { user, loading, setupRequired, onboarding, needServer } = useApp()
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
  // A link someone was sent: no account, no app around it.
  if (loc.pathname.startsWith('/s/')) return <Suspense fallback={null}><Routes><Route path="/s/:token" element={<SharedPage />} /></Routes></Suspense>
  // The Legal page, for anyone: a shared link, the setup wizard and the sign-in screen lead here.
  if (loc.pathname === '/legal') return <Suspense fallback={<Spinner />}><LegalPublicPage /></Suspense>
  if (loading) return <div className="center"><div className="spinner" /></div>
  if (needServer) return <ConnectServer />
  if (!user) return setupRequired ? <Suspense fallback={<Spinner />}><SetupWizard resume={false} /></Suspense> : <AuthPage />
  if (onboarding && user.isAdmin) return <Suspense fallback={<Spinner />}><SetupWizard resume /></Suspense>
  // The player lives outside the page routes (see PlayerDock) so it can keep
  // playing as a mini player while the viewer browses.
  const isPlayer = loc.pathname.startsWith('/play/')
  const isReader = loc.pathname.startsWith('/read/')
  const isTvPlayer = loc.pathname.startsWith('/tv/watch/')
  const isGame = loc.pathname.startsWith('/game/')
  const isStream = /^\/moonlight\/\d+\/play\//.test(loc.pathname) || loc.pathname.startsWith('/moonlight-rig/')
  return (
    <AudioProvider>
      {isTvPlayer ? <Suspense fallback={tvFallback}><Routes><Route path="/tv/watch/:key" element={<TvPlayer />} /></Routes></Suspense>
        : isReader ? <Suspense fallback={<Spinner />}><Routes><Route path="/read/:id" element={<ReaderPage />} /></Routes></Suspense>
        : isGame ? <Suspense fallback={<Spinner />}><Routes><Route path="/game/:id" element={<GamePage />} /></Routes></Suspense>
        : isStream ? <Suspense fallback={<Spinner />}><Routes><Route path="/moonlight/:id/play/:app" element={<MoonlightPlayPage />} /><Route path="/moonlight-rig/:id" element={<RigPlayPage />} /></Routes></Suspense>
        : isPlayer ? null : <AppFrame navOpen={navOpen} setNavOpen={setNavOpen} />}
      <PlayerDock />
    </AudioProvider>
  )
}

// Whether to offer Game PCs. It's asked for once, as the answer rarely changes.
function useCanUseMoonlight() {
  const { user } = useApp()
  const [can, setCan] = useState(false)
  useEffect(() => {
    if (user) api.get<{ allowed: boolean }>('/moonlight').then((r) => setCan(r.allowed)).catch(() => setCan(false))
  }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  return can
}

function AppFrame({ navOpen, setNavOpen }: { navOpen: boolean; setNavOpen: React.Dispatch<React.SetStateAction<boolean>> }) {
  const style = useNavStyle()
  const side = style !== 'top'
  const { version } = useApp()
  return (
    <div className={'app nav-' + style + (navOpen ? ' nav-open' : '')}>
      <a className="skip-link" href="#main">Skip to content</a>
      <Sidebar open={navOpen} onClose={() => setNavOpen(false)} />
      <div className="main">
        <TopBar onMenu={() => setNavOpen((o) => !o)} showMenu={side} topNav={style !== 'side'} />
        <LegalAckBar />
        <main className="content" id="main" tabIndex={-1}>
          <Suspense fallback={<Spinner />}>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/library/:id" element={<LibraryPage />} />
            <Route path="/item/:id" element={<ItemPage />} />
            <Route path="/show/:id" element={<ShowPage />} />
            <Route path="/album/:lib" element={<AlbumPage />} />
            <Route path="/radio/:id" element={<Suspense fallback={<Spinner />}><RadioStationPage /></Suspense>} />
            <Route path="/cd/:id" element={<CdPlayer />} />
            <Route path="/moonlight" element={<Suspense fallback={<Spinner />}><MoonlightPage /></Suspense>} />
            <Route path="/moonlight/:id" element={<Suspense fallback={<Spinner />}><MoonlightHostPage /></Suspense>} />
            <Route path="/collections" element={<Suspense fallback={<Spinner />}><CollectionsPage /></Suspense>} />
            <Route path="/collections/:id" element={<Suspense fallback={<Spinner />}><CollectionPage /></Suspense>} />
            <Route path="/review" element={<Suspense fallback={<Spinner />}><ReviewPage /></Suspense>} />
            <Route path="/watch/:id" element={<Suspense fallback={<Spinner />}><WatchPage /></Suspense>} />
            <Route path="/creator/:id" element={<Suspense fallback={<Spinner />}><CreatorPage /></Suspense>} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/pair" element={<PairPage />} />
            <Route path="/tv" element={<Suspense fallback={tvFallback}><GuidePage /></Suspense>} />
            <Route path="/tv/recordings" element={<Suspense fallback={tvFallback}><RecordingsPage /></Suspense>} />
            <Route path="/tv/encrypted" element={<Suspense fallback={tvFallback}><EncryptedHelp /></Suspense>} />
            <Route path="/controllers" element={<Suspense fallback={<Spinner />}><ControllersScreen /></Suspense>} />
            <Route path="/settings/*" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
          </Suspense>
          <LegalNote kind="general" className="legal-foot" />
          <footer className="page-foot"><VersionTag version={version} /></footer>
        </main>
      </div>
      <TabBar onLibraries={() => setNavOpen(true)} />
    </div>
  )
}


// LegalAckBar: the first-run notice, for an administrator who never went through
// the setup wizard's tick-box. Once, and it doesn't stop anyone doing anything.
function LegalAckBar() {
  const { legalAck, setLegalAck } = useApp()
  if (legalAck) return null
  const understood = () => { setLegalAck(true); api.post('/legal/acknowledge').catch(() => {}) }
  return (
    <div className="legal-ack-bar" role="region" aria-label="Legal notice">
      <p>{legalAckText()} <Link to="/settings/legal#your-content">Legal</Link></p>
      <button className="btn sm primary" onClick={understood}>Understood</button>
    </div>
  )
}

function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { libraries, version, brandName } = useApp()
  const canUseMoonlight = useCanUseMoonlight()
  return (
    <>
      {open && <div className="backdrop-dim" onClick={onClose} />}
      <aside className="sidebar" aria-label="Main navigation" aria-hidden={!open} inert={!open || undefined}>
        <Link to="/" className="brand"><BrandMark /><Brand name={brandName} /></Link>
        <NavLink to="/" end className="nav-link"><IconHome /> Home</NavLink>
        <NavLink to="/tv" className="nav-link"><IconTv /> TV guide</NavLink>
        <div className="nav-section">Libraries</div>
        {libraries.filter(inMainMenus).map((l) => <NavLink key={l.id} to={`/library/${l.id}`} className="nav-link">{libIcon(l.type)} {l.name}</NavLink>)}
        {libraries.length === 0 && <div className="nav-empty">No libraries yet</div>}
        {libraries.length > 0 && <NavLink to="/collections" className="nav-link"><IconLibrary /> Collections</NavLink>}
        {canUseMoonlight && <NavLink to="/moonlight" className="nav-link"><IconGamepad /> Game PCs</NavLink>}
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
  const canUseMoonlight = useCanUseMoonlight()
  return (
    <header className="topbar">
      {showMenu && <button className="btn icon menu-toggle" onClick={onMenu} aria-label="Show or hide the side menu" title="Side menu"><IconMenu /></button>}
      {topNav && !showMenu && <Link to="/" className="brand top-brand"><BrandMark /><Brand name={brandName} /></Link>}
      {topNav && (
        <nav className="topnav" aria-label="Libraries">
          <NavLink to="/" end className="topnav-link"><IconHome /> <span>Home</span></NavLink>
          <NavLink to="/tv" className="topnav-link"><IconTv /> <span>TV guide</span></NavLink>
          {libraries.filter(inMainMenus).map((l) => <NavLink key={l.id} to={`/library/${l.id}`} className="topnav-link">{libIcon(l.type)} <span>{l.name}</span></NavLink>)}
          {libraries.length > 0 && <NavLink to="/collections" className="topnav-link"><IconLibrary /> <span>Collections</span></NavLink>}
          {canUseMoonlight && <NavLink to="/moonlight" className="topnav-link"><IconGamepad /> <span>Game PCs</span></NavLink>}
        </nav>
      )}
      <form className="search" role="search" onSubmit={(e) => { e.preventDefault(); if (q.trim()) nav('/search?q=' + encodeURIComponent(q.trim())) }}>
        <IconSearch />
        <input aria-label="Search" type="search" placeholder="Search your libraries…" value={q} onChange={(e) => setQ(e.target.value)} />
      </form>
      {!topNav && <div className="spacer" />}
      <ProfileMenu />
    </header>
  )
}

// ProfileMenu: the avatar in the corner, with settings, switching profiles
// (back to "Who's watching?") and signing out.
function ProfileMenu() {
  const { user, setUser, libraries } = useApp()
  const nav = useNavigate()
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
  const signOut = async () => { try { await api.post('/logout') } catch { /* ignore */ } nav('/', { replace: true }); setUser(null) }
  return (
    <div className="profile-menu" ref={ref}>
      <button className="avatar-btn" aria-haspopup="menu" aria-expanded={open} aria-label={`${user?.name}: account menu`} onClick={() => setOpen((o) => !o)}>
        <span className="avatar-name">{user?.name}</span>
        <Avatar name={user?.name || '?'} />
      </button>
      {open && (
        <div className="menu" role="menu" onClick={() => setOpen(false)}>
          <div className="menu-head"><b>{user?.name}</b><span className="muted">{user?.isAdmin ? 'Administrator' : user?.kind === 'kids' ? 'Kids profile' : user?.kind === 'guest' ? 'Guest' : 'Profile'}</span></div>
          <Link role="menuitem" to="/review"><IconStar /> Your year</Link>
          <Link role="menuitem" to="/settings/appearance"><IconPalette /> Appearance</Link>
          {libraries.some((l) => l.type === 'games') && <Link role="menuitem" to="/controllers"><IconGamepad /> Controllers</Link>}
          <Link role="menuitem" to="/settings"><IconSettings /> Settings</Link>
          {(user?.kind === 'adult' || user?.isAdmin) && <Link role="menuitem" to="/pair"><IconTv /> Sign in a TV</Link>}
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
      <NavLink to="/tv"><IconTv /><span>TV</span></NavLink>
      <NavLink to="/search"><IconSearch /><span>Search</span></NavLink>
      <button type="button" onClick={onLibraries}><IconFilm /><span>Libraries</span></button>
      <NavLink to="/settings" className="tab-upd"><IconSettings /><span>You</span><UpdateDot /></NavLink>
    </nav>
  )
}
