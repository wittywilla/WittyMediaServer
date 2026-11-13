// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useLayoutEffect, useState } from 'react'
import { Link, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { AppProvider, ToastHost, useApp } from './state'
import { ReminderWatcher } from './pages/tv/reminders'
import { api } from './api'
import { setThemeModule } from './theme'
import { moduleForPath } from './themeOverrides'
import { getNavCollapsed, setNavCollapsed } from './navPrefs'
import { SduiRoute } from './sdui/useSdui'
import { EmptyState, PageSkeleton, Spinner, VersionTag } from './components/common'
import { shapeFor } from './ui/Surface'
import { LinkButton } from './ui'
import { AudioProvider } from './audio/player'
import ConnectServer from './components/ConnectServer'
import Unreachable from './components/Unreachable'
import AuthPage from './pages/AuthPage'
import { usePageRegions } from './sdui/regions'
import { isPlayPath, PlayFrame, StatusBar, SystemDock } from './shell/PlayChrome'
import { useGamesInPlay } from './shell/PlayRedirect'
import { PlayerDock } from './player/PlayerPage'
import { Announcer, useDialogs } from './a11y'
import LegalNote, { legalAckText } from './components/LegalNote'
import AwayPage from './pages/AwayPage'
import { KeyHelp } from './ui/KeyHelp'
import { ShellEditorHost } from './pages/ShellEditor'
import { AskHost } from './ui/Overlay'
import ScreenTimeWatch from './shell/ScreenTime'
import { loadUserPrefs, useUserPref } from './ui/userPrefs'
import { syncTilt } from './ui/tilt'
import { Sidebar } from './shell/Sidebar'
import { TabBar } from './shell/TabBar'
import { TopBar } from './shell/TopBar'
import { Scene } from './ui/Scene'
import { frameFor } from './shell/navModel'
import { useMedia, useMenu, useNavMigration, useNavPrefs } from './shell/useNav'
import { useTvMode } from './tvmode/mode'
import RemoteHost from './remote/RemoteHost'

// The pages the app drew by hand before pages were server-driven: what an older server still gets, and
// what stands in where a server-driven page gives way to one of the app's own screens (sdui/native.ts).
const HomePage = lazy(() => import('./pages/HomePage'))
const PairPage = lazy(() => import('./pages/PairPage'))
const SharedPage = lazy(() => import('./pages/SharedPage'))
const DesignPage = lazy(() => import('./pages/DesignPage'))
const DesignPlay = lazy(() => import('./pages/DesignPlay'))
const PlaySettings = lazy(() => import('./pages/PlaySettings'))
const PlayConsoles = lazy(() => import('./pages/PlayConsoles'))
const PlayConsoleSettings = lazy(() => import('./pages/PlayConsoles').then((m) => ({ default: m.PlayConsoleSettings })))
const PlayLibrary = lazy(() => import('./pages/PlayConsoles').then((m) => ({ default: m.PlayLibrary })))
const PlayGame = lazy(() => import('./pages/PlayGame'))
const GapsReview = lazy(() => import('./spec/GapsReview'))
const HelpPage = lazy(() => import('./pages/HelpPage'))
const HelpPublicPage = lazy(() => import('./pages/HelpPage').then((m) => ({ default: m.HelpPublicPage })))
const LegalPublicPage = lazy(() => import('./pages/settings/Legal').then((m) => ({ default: m.LegalPublicPage })))
const GamePage = lazy(() => import('./pages/Games').then((m) => ({ default: m.GamePage })))
const CouchMode = lazy(() => import('./pages/games/Couch'))
const RadioStationPage = lazy(() => import('./pages/RadioLibrary').then((m) => ({ default: m.RadioStationPage })))
const ControllersScreen = lazy(() => import('./pages/Games').then((m) => ({ default: m.ControllersScreen })))
const WatchPage = lazy(() => import('./pages/creators/WatchPage'))
const FixMatches = lazy(() => import('./pages/FixMatches'))
const BrandPage = lazy(() => import('./pages/library/BrandPage'))
const CouchPage = lazy(() => import('./pages/Couch'))
const ModulePage = lazy(() => import('./pages/ModulePage'))
const CouchHostPage = lazy(() => import('./pages/Couch').then((m) => ({ default: m.CouchHostPage })))
const CouchPlayPage = lazy(() => import('./pages/Couch').then((m) => ({ default: m.CouchPlayPage })))
const RigPlayPage = lazy(() => import('./pages/Couch').then((m) => ({ default: m.RigPlayPage })))
const ReviewPage = lazy(() => import('./pages/ReviewPage'))
const CreatorPage = lazy(() => import('./pages/creators/ChannelsLibrary').then((m) => ({ default: m.CreatorPage })))
// Every other page comes when it is opened: the first screen a phone or an
// old TV has to load stays small. The player has its own (hls.js is large).
const LibraryPage = lazy(() => import('./pages/LibraryPage'))
const ItemPage = lazy(() => import('./pages/ItemPage'))
const ShowPage = lazy(() => import('./pages/ShowPage'))
const CdPlayer = lazy(() => import('./player/CdPlayer'))
const AlbumPage = lazy(() => import('./pages/AudioLibrary').then((m) => ({ default: m.AlbumPage })))
const ArtistPage = lazy(() => import('./pages/music/ArtistPage'))
const ReaderPage = lazy(() => import('./pages/Reader'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const SearchPage = lazy(() => import('./pages/SearchPage'))
const YouPage = lazy(() => import('./pages/YouPage'))
const SetupWizard = lazy(() => import('./pages/SetupWizard'))
// TV is its own part of the app, loaded when someone goes there.
const GuidePage = lazy(() => import('./pages/tv/GuidePage'))
const RecordingsPage = lazy(() => import('./pages/tv/RecordingsPage'))
const SetsPage = lazy(() => import('./pages/tv/SetsPage'))
const ChannelsPage = lazy(() => import('./pages/tv/ChannelsPage'))
const EncryptedHelp = lazy(() => import('./pages/tv/EncryptedHelp'))
const TvPlayer = lazy(() => import('./pages/tv/TvPlayer'))
// The TV section's one stream, shared by the guide's preview and the watch view.
const TvHost = lazy(() => import('./pages/tv/TvHost'))
const tvFallback = <div className="center"><div className="spinner" /></div>
// TV mode's frame, loaded only when TV mode is on.
const TvFrame = lazy(() => import('./tvmode/TvFrame'))

export default function App() {
  useDialogs()
  return (
    <AppProvider>
      <Shell />
      <ToastHost />
      <ScreenTimeWatch />
      <AskHost />
      <KeyHelp />
      <ShellEditorHost />
      <Announcer />
    </AppProvider>
  )
}

function Shell() {
  const { user, loading, setupRequired, onboarding, needServer, unreachable } = useApp()
  const loc = useLocation()
  const tvMode = useTvMode()
  // WittyPlay installed: the games pages show in its frame too (shell/PlayRedirect.tsx). Asked before any early return.
  const gamesInPlay = useGamesInPlay(loc.pathname)
  // A module's pages can be tinted differently from the rest (the theme engine's module overrides).
  const pageModule = moduleForPath(loc.pathname)
  useLayoutEffect(() => { setThemeModule(pageModule) }, [pageModule])
  useLayoutEffect(() => { if (user) loadUserPrefs(user.id) }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  useNavMigration()
  // A link someone was sent: no account, no app around it.
  if (loc.pathname.startsWith('/s/')) return <Suspense fallback={null}><Routes><Route path="/s/:token" element={<SharedPage />} /></Routes></Suspense>
  // The Legal page, for anyone: a shared link, the setup wizard and the sign-in screen lead here.
  if (loc.pathname === '/legal') return <Suspense fallback={<Spinner />}><LegalPublicPage /></Suspense>
  // Help, for anyone: stuck at the sign-in page or partway through setup is when it's needed most.
  const isHelp = loc.pathname === '/help' || loc.pathname.startsWith('/help/')
  if (isHelp && (needServer || (!loading && (!user || (onboarding && user.isAdmin))))) return <Suspense fallback={<Spinner />}><Routes><Route path="/help" element={<HelpPublicPage />} /><Route path="/help/:guide" element={<HelpPublicPage />} /></Routes></Suspense>
  // From away, for anyone: how a device away from home joins (the parts that make an invite wait for a sign-in).
  if (loc.pathname === '/away' && !loading && !user && !needServer && !setupRequired) return <div className="center-page"><AwayPage /></div>
  if (unreachable) return <Unreachable />
  if (loading) return <div className="center"><div className="spinner" /></div>
  if (needServer) return <ConnectServer />
  if (!user) return setupRequired ? <Suspense fallback={<Spinner />}><SetupWizard /></Suspense> : <AuthPage />
  if (onboarding && user.isAdmin) return <Suspense fallback={<Spinner />}><SetupWizard /></Suspense>
  // The player lives outside the page routes (see PlayerDock) so it can keep
  // playing as a mini player while the viewer browses.
  const isPlayer = loc.pathname.startsWith('/play/')
  const isReader = loc.pathname.startsWith('/read/')
  const isTvPlayer = loc.pathname.startsWith('/tv/watch/')
  const isTv = loc.pathname === '/tv' || loc.pathname.startsWith('/tv/')
  const isGame = loc.pathname.startsWith('/game/')
  const isCouchMode = loc.pathname === '/big' || loc.pathname.startsWith('/big/')
  const isStream = /^\/couch\/\d+\/play\//.test(loc.pathname) || loc.pathname.startsWith('/couch-rig/')
  // WittyPlay Online takes over the window, over the app's frame and in its own palette (shell/PlayChrome.tsx), in TV mode too.
  const isWPlay = isPlayPath(loc.pathname) || gamesInPlay
  // A games library's page, with WittyPlay installed: WittyPlay's own view of it (the app's whole games library, in its frame).
  const playLib = gamesInPlay ? /^\/library\/(\d+)$/.exec(loc.pathname)?.[1] : undefined
  const toPlayLibrary = playLib ? '/wittyplay/library/' + playLib + loc.search : ''
  return (
    <AudioProvider>
      {!isTvPlayer && !isReader && !isStream && !isGame && !isPlayer && !isWPlay && <Scene />}
      {isTvPlayer ? <Suspense fallback={tvFallback}><Routes><Route path="/tv/watch/:key" element={<TvPlayer />} /></Routes></Suspense>
        : isReader ? <Suspense fallback={<Spinner />}><Routes><Route path="/read/:id" element={<ReaderPage />} /></Routes></Suspense>
        : isCouchMode ? <Suspense fallback={<Spinner />}><Routes><Route path="/big/*" element={<CouchMode />} /></Routes></Suspense>
        : isGame ? <Suspense fallback={<Spinner />}><Routes><Route path="/game/:id" element={<GamePage />} /></Routes></Suspense>
        : isStream ? <Suspense fallback={<Spinner />}><Routes><Route path="/couch/:id/play/:app" element={<CouchPlayPage />} /><Route path="/couch-rig/:id" element={<RigPlayPage />} /></Routes></Suspense>
        : isPlayer ? null : isWPlay ? <PlayFrame>{toPlayLibrary ? <Navigate to={toPlayLibrary} replace /> : <Suspense fallback={<RouteSkeleton />}><PageRoutes /></Suspense>}</PlayFrame> : tvMode ? <Suspense fallback={<Spinner />}><TvFrame pages={<PageRoutes />} /></Suspense> : <AppFrame />}
      <PlayerDock />
      <RemoteHost />
      {isTv && <Suspense fallback={null}><TvHost /></Suspense>}
      <ReminderWatcher />
    </AudioProvider>
  )
}

// The shell's regions where the experience, the administrator and the profile have put them (layout.shell.placements):
// each as data-r-<region>="hidden" or its position, for design2.css to answer.
function regionAttrs(p: Record<string, { position?: string; visible?: boolean }> | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [id, v] of Object.entries(p || {})) { const val = v.visible === false ? 'hidden' : v.position; if (val) out['data-r-' + id] = val }
  return out
}

// The page's frame. On wide screens the side menu sits beside the page and can fold
// to a rail or away, which this device remembers; narrower, it's a drawer over the page.
function AppFrame() {
  // The server says how the frame is drawn: the experience's nav style (docs/SDUI.md, "Experiences").
  const menu = useMenu()
  const regions = usePageRegions()
  const frame = frameFor(menu.shell)
  const style = frame.style
  const side = style !== 'top'
  useEffect(() => {
    const root = document.documentElement.style
    if (frame.sidebar) root.setProperty('--sidebar', frame.sidebar + 'px'); else root.removeProperty('--sidebar')
    if (frame.railWidth) root.setProperty('--rail', frame.railWidth + 'px'); else root.removeProperty('--rail')
    if (frame.open) root.setProperty('--rail-open', frame.open + 'px'); else root.removeProperty('--rail-open')
    if (frame.drawerWidth) root.setProperty('--rail-drawer', frame.drawerWidth + 'px'); else root.removeProperty('--rail-drawer')
  }, [frame.sidebar, frame.railWidth, frame.open, frame.drawerWidth])
  const { version } = useApp()
  const { prefs } = useNavPrefs()
  const loc = useLocation()
  const wide = useMedia('(min-width: 901px)')
  const [folded, setFolded] = useState(getNavCollapsed)
  const [drawer, setDrawer] = useState(false)
  const [tilt] = useUserPref('posterTilt', true)
  useEffect(() => { syncTilt(tilt) }, [tilt])
  useEffect(() => { setDrawer(false) }, [loc.pathname, wide])
  const open = wide ? side && !folded && !frame.rail : drawer
  const rail = wide && side && (frame.rail || (folded && prefs.collapse !== 'hide'))
  const toggle = () => {
    // A rail: the button keeps its full menu open, or lets it fold again.
    if (wide && rail && frame.rail) { window.dispatchEvent(new Event('wms:rail-pin')); return }
    if (wide) { setNavCollapsed(!folded); setFolded(!folded) } else setDrawer((o) => !o)
  }
  return (
    <div className={'app nav-' + style + (open ? ' nav-open' : '') + (rail ? ' nav-rail' : '') + (frame.overlay ? ' nav-overlay' : '') + (frame.tabBar ? '' : ' no-tabbar')} data-nav-style={frame.nav} data-experience={menu.shell?.experience || undefined} {...regionAttrs(menu.shell?.placements)}>
      <a className="skip-link" data-nav-skip href="#main">Skip to content</a>
      <Sidebar open={open} rail={rail} drawer={!wide} onClose={() => setDrawer(false)} withDrawer={frame.drawer} />
      <div className="main">
        <TopBar onMenu={toggle} showMenu={side || !wide} topNav={style !== 'side'} menuOpen={open} wide={wide} allTabs={!frame.tabBar} />
        {regions?.statusBar && <StatusBar parts={regions.statusBar} />}
        <LegalAckBar />
        <PlainAwayBar />
        <main className="content" id="main" data-nav-zone="stage" tabIndex={-1}>
          <Suspense fallback={<RouteSkeleton />}><PageRoutes /></Suspense>
          <LegalNote kind="general" className="legal-foot" />
          <footer className="page-foot"><VersionTag version={version} /></footer>
        </main>
        {regions?.systemDock && <SystemDock entries={regions.systemDock} />}
      </div>
      <TabBar onMenu={() => setDrawer(true)} />
    </div>
  )
}

// While a page's code arrives: the shape of that page, not a spinner on an empty one.
function RouteSkeleton() {
  return <PageSkeleton shape={shapeFor(useLocation().pathname)} />
}

// Every page of the app, by its address. The usual frame and TV mode's frame both show them.
export function PageRoutes() {
  return (
    <Routes>
      <Route path="/" element={<SduiRoute fallback={<HomePage />} loading={<RouteSkeleton />} />} />
      <Route path="/library/:id" element={<SduiRoute fallback={<LibraryPage />} loading={<RouteSkeleton />} />} />
      <Route path="/item/:id" element={<SduiRoute fallback={<ItemPage />} loading={<RouteSkeleton />} />} />
      <Route path="/show/:id" element={<SduiRoute fallback={<ShowPage />} loading={<RouteSkeleton />} />} />
      <Route path="/album/:lib" element={<SduiRoute fallback={<AlbumPage />} loading={<RouteSkeleton />} />} />
      <Route path="/artist/:lib" element={<Suspense fallback={<Spinner />}><ArtistPage /></Suspense>} />
      <Route path="/radio/:id" element={<Suspense fallback={<Spinner />}><RadioStationPage /></Suspense>} />
      <Route path="/cd/:id" element={<CdPlayer />} />
      <Route path="/couch" element={<Suspense fallback={<Spinner />}><CouchPage /></Suspense>} />
      <Route path="/couch/:id" element={<Suspense fallback={<Spinner />}><CouchHostPage /></Suspense>} />
      <Route path="/collections" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/collections/:id" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/person/:id" element={<SduiRoute fallback={<NotFound />} loading={<RouteSkeleton />} />} />
      <Route path="/review" element={<Suspense fallback={<Spinner />}><ReviewPage /></Suspense>} />
      <Route path="/watch/:id" element={<Suspense fallback={<Spinner />}><WatchPage /></Suspense>} />
      <Route path="/creator/:id" element={<Suspense fallback={<Spinner />}><CreatorPage /></Suspense>} />
      <Route path="/search" element={<SduiRoute fallback={<SearchPage />} loading={<RouteSkeleton />} />} />
      <Route path="/you" element={<YouPage />} />
      <Route path="/fix-matches" element={<Suspense fallback={<Spinner />}><FixMatches /></Suspense>} />
      <Route path="/brand/:kind/:name" element={<Suspense fallback={<Spinner />}><BrandPage /></Suspense>} />
      <Route path="/pair" element={<PairPage />} />
      <Route path="/away" element={<AwayPage />} />
      <Route path="/tv" element={<SduiRoute fallback={<Suspense fallback={tvFallback}><GuidePage /></Suspense>} loading={tvFallback} />} />
      {/* Places only a server with server-driven pages has: every library together, a kind of library together, a page of the owner's own. */}
      <Route path="/libraries" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/music" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/photos" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/books" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/games" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/page/:id" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/manage/*" element={<SduiRoute fallback={<NotFound />} />} />
      <Route path="/tv/recordings" element={<Suspense fallback={tvFallback}><RecordingsPage /></Suspense>} />
      <Route path="/tv/channels/*" element={<Suspense fallback={tvFallback}><ChannelsPage /></Suspense>} />
      <Route path="/tv/sets/*" element={<Suspense fallback={tvFallback}><SetsPage /></Suspense>} />
      <Route path="/tv/encrypted" element={<Suspense fallback={tvFallback}><EncryptedHelp /></Suspense>} />
      <Route path="/controllers" element={<Suspense fallback={<Spinner />}><ControllersScreen /></Suspense>} />
      <Route path="/settings/*" element={<SettingsPage />} />
      <Route path="/m/:module/:page" element={<Suspense fallback={<Spinner />}><ModulePage /></Suspense>} />
      <Route path="/help" element={<Suspense fallback={<Spinner />}><HelpPage /></Suspense>} />
      <Route path="/help/:guide" element={<Suspense fallback={<Spinner />}><HelpPage /></Suspense>} />
      <Route path="/design" element={<DesignPage />} />
      <Route path="/design/play" element={<Suspense fallback={<Spinner />}><DesignPlay /></Suspense>} />
      <Route path="/wittyplay/consoles" element={<Suspense fallback={<Spinner />}><PlayConsoles /></Suspense>} />
      <Route path="/wittyplay/consoles/:id" element={<Suspense fallback={<Spinner />}><PlayConsoleSettings /></Suspense>} />
      <Route path="/wittyplay/library" element={<Suspense fallback={<Spinner />}><PlayLibrary /></Suspense>} />
      <Route path="/wittyplay/library/:id" element={<Suspense fallback={<Spinner />}><PlayLibrary /></Suspense>} />
      <Route path="/wittyplay/game/:id" element={<Suspense fallback={<Spinner />}><PlayGame /></Suspense>} />
      <Route path="/wittyplay/settings" element={<Suspense fallback={<Spinner />}><PlaySettings /></Suspense>} />
      <Route path="/wittyplay/settings/module/:module" element={<Suspense fallback={<Spinner />}><PlaySettings /></Suspense>} />
      <Route path="/wittyplay/settings/:section/*" element={<Suspense fallback={<Spinner />}><PlaySettings /></Suspense>} />
      <Route path="/design/gaps" element={<Suspense fallback={<Spinner />}><GapsReview /></Suspense>} />
      <Route path="/design/gaps/:key" element={<Suspense fallback={<Spinner />}><GapsReview /></Suspense>} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  )
}

// An address that isn't a page: say so, with the way home, rather than landing somewhere unexplained.
function NotFound() {
  return <><h1 className="sr-only">Page not found</h1><EmptyState title="There’s no page here" text="The address may have changed, or it may be mistyped." action={<LinkButton variant="primary" to="/">Go to Home</LinkButton>} /></>
}

// PlainAwayBar: while the owner lets the internet in unencrypted, administrators
// see it on every page, in the words Settings, Remote Access uses. It can't be
// dismissed: it goes when the setting does.
function PlainAwayBar() {
  const { user } = useApp()
  const [warning, setWarning] = useState('')
  useEffect(() => {
    if (!user?.isAdmin) { setWarning(''); return }
    const load = () => api.get<{ plainAwayWarning?: string }>('/status').then((s) => setWarning(s.plainAwayWarning || '')).catch(() => {})
    load()
    const t = setInterval(() => { if (!document.hidden) load() }, 60000)
    window.addEventListener('wms-remote-changed', load)
    return () => { clearInterval(t); window.removeEventListener('wms-remote-changed', load) }
  }, [user?.isAdmin])
  if (!warning) return null
  return (
    <div className="legal-ack-bar plain-away-bar" role="region" aria-label="Security warning">
      <p><b>Warning:</b> {warning} <Link to="/settings/remote">Remote Access</Link></p>
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
