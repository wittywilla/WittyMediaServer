import { lazy, Suspense, useEffect, useLayoutEffect, useState } from 'react'
import { Link, Route, Routes, useLocation } from 'react-router-dom'
import { AppProvider, ToastHost, useApp } from './state'
import { ReminderWatcher } from './pages/tv/reminders'
import { api } from './api'
import { getNavCollapsed, setNavCollapsed, useNavStyle } from './navPrefs'
import HomePage from './pages/HomePage'
import { EmptyState, Spinner, VersionTag } from './components/common'
import { LinkButton } from './ui'
import { AudioProvider } from './audio/player'
import ConnectServer from './components/ConnectServer'
import AuthPage from './pages/AuthPage'
import { PlayerDock } from './player/PlayerPage'
import { Announcer, useDialogs } from './a11y'
import LegalNote, { legalAckText } from './components/LegalNote'
import { AskHost } from './ui/Overlay'
import { loadUserPrefs, useUserPref } from './ui/userPrefs'
import { syncTilt } from './ui/tilt'
import { Sidebar } from './shell/Sidebar'
import { TabBar } from './shell/TabBar'
import { TopBar } from './shell/TopBar'
import { useMedia, useNavPrefs } from './shell/useNav'

const PairPage = lazy(() => import('./pages/PairPage'))
const SharedPage = lazy(() => import('./pages/SharedPage'))
const DesignPage = lazy(() => import('./pages/DesignPage'))
const HelpPage = lazy(() => import('./pages/HelpPage'))
const HelpPublicPage = lazy(() => import('./pages/HelpPage').then((m) => ({ default: m.HelpPublicPage })))
const LegalPublicPage = lazy(() => import('./pages/settings/Legal').then((m) => ({ default: m.LegalPublicPage })))
const GamePage = lazy(() => import('./pages/Games').then((m) => ({ default: m.GamePage })))
const CouchMode = lazy(() => import('./pages/games/Couch'))
const RadioStationPage = lazy(() => import('./pages/RadioLibrary').then((m) => ({ default: m.RadioStationPage })))
const ControllersScreen = lazy(() => import('./pages/Games').then((m) => ({ default: m.ControllersScreen })))
const WatchPage = lazy(() => import('./pages/creators/WatchPage'))
const CollectionsPage = lazy(() => import('./pages/CollectionsPage'))
const FixMatches = lazy(() => import('./pages/FixMatches'))
const BrandPage = lazy(() => import('./pages/library/BrandPage'))
const CouchPage = lazy(() => import('./pages/Couch'))
const ModulePage = lazy(() => import('./pages/ModulePage'))
const CouchHostPage = lazy(() => import('./pages/Couch').then((m) => ({ default: m.CouchHostPage })))
const CouchPlayPage = lazy(() => import('./pages/Couch').then((m) => ({ default: m.CouchPlayPage })))
const RigPlayPage = lazy(() => import('./pages/Couch').then((m) => ({ default: m.RigPlayPage })))
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
const ArtistPage = lazy(() => import('./pages/music/ArtistPage'))
const ReaderPage = lazy(() => import('./pages/Reader'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const SearchPage = lazy(() => import('./pages/SearchPage'))
const SetupWizard = lazy(() => import('./pages/SetupWizard'))
// TV is its own part of the app, loaded when someone goes there.
const GuidePage = lazy(() => import('./pages/tv/GuidePage'))
const RecordingsPage = lazy(() => import('./pages/tv/RecordingsPage'))
const ChannelsPage = lazy(() => import('./pages/tv/ChannelsPage'))
const EncryptedHelp = lazy(() => import('./pages/tv/EncryptedHelp'))
const TvPlayer = lazy(() => import('./pages/tv/TvPlayer'))
const tvFallback = <div className="center"><div className="spinner" /></div>

export default function App() {
  useDialogs()
  return (
    <AppProvider>
      <Shell />
      <ToastHost />
      <AskHost />
      <Announcer />
    </AppProvider>
  )
}

function Shell() {
  const { user, loading, setupRequired, onboarding, needServer } = useApp()
  const loc = useLocation()
  useLayoutEffect(() => { if (user) loadUserPrefs(user.id) }, [user?.id]) // eslint-disable-line react-hooks/exhaustive-deps
  // A link someone was sent: no account, no app around it.
  if (loc.pathname.startsWith('/s/')) return <Suspense fallback={null}><Routes><Route path="/s/:token" element={<SharedPage />} /></Routes></Suspense>
  // The Legal page, for anyone: a shared link, the setup wizard and the sign-in screen lead here.
  if (loc.pathname === '/legal') return <Suspense fallback={<Spinner />}><LegalPublicPage /></Suspense>
  // Help, for anyone: stuck at the sign-in page or partway through setup is when it's needed most.
  const isHelp = loc.pathname === '/help' || loc.pathname.startsWith('/help/')
  if (isHelp && (needServer || (!loading && (!user || (onboarding && user.isAdmin))))) return <Suspense fallback={<Spinner />}><Routes><Route path="/help" element={<HelpPublicPage />} /><Route path="/help/:guide" element={<HelpPublicPage />} /></Routes></Suspense>
  if (loading) return <div className="center"><div className="spinner" /></div>
  if (needServer) return <ConnectServer />
  if (!user) return setupRequired ? <Suspense fallback={<Spinner />}><SetupWizard /></Suspense> : <AuthPage />
  if (onboarding && user.isAdmin) return <Suspense fallback={<Spinner />}><SetupWizard /></Suspense>
  // The player lives outside the page routes (see PlayerDock) so it can keep
  // playing as a mini player while the viewer browses.
  const isPlayer = loc.pathname.startsWith('/play/')
  const isReader = loc.pathname.startsWith('/read/')
  const isTvPlayer = loc.pathname.startsWith('/tv/watch/')
  const isGame = loc.pathname.startsWith('/game/')
  const isCouchMode = loc.pathname === '/big' || loc.pathname.startsWith('/big/')
  const isStream = /^\/couch\/\d+\/play\//.test(loc.pathname) || loc.pathname.startsWith('/couch-rig/')
  return (
    <AudioProvider>
      {isTvPlayer ? <Suspense fallback={tvFallback}><Routes><Route path="/tv/watch/:key" element={<TvPlayer />} /></Routes></Suspense>
        : isReader ? <Suspense fallback={<Spinner />}><Routes><Route path="/read/:id" element={<ReaderPage />} /></Routes></Suspense>
        : isCouchMode ? <Suspense fallback={<Spinner />}><Routes><Route path="/big/*" element={<CouchMode />} /></Routes></Suspense>
        : isGame ? <Suspense fallback={<Spinner />}><Routes><Route path="/game/:id" element={<GamePage />} /></Routes></Suspense>
        : isStream ? <Suspense fallback={<Spinner />}><Routes><Route path="/couch/:id/play/:app" element={<CouchPlayPage />} /><Route path="/couch-rig/:id" element={<RigPlayPage />} /></Routes></Suspense>
        : isPlayer ? null : <AppFrame />}
      <PlayerDock />
      <ReminderWatcher />
    </AudioProvider>
  )
}

// The page's frame. On wide screens the side menu sits beside the page and can fold
// to a rail or away, which this device remembers; narrower, it's a drawer over the page.
function AppFrame() {
  const style = useNavStyle()
  const side = style !== 'top'
  const { version } = useApp()
  const { prefs } = useNavPrefs()
  const loc = useLocation()
  const wide = useMedia('(min-width: 901px)')
  const [folded, setFolded] = useState(getNavCollapsed)
  const [drawer, setDrawer] = useState(false)
  const [tilt] = useUserPref('posterTilt', true)
  useEffect(() => { syncTilt(tilt) }, [tilt])
  useEffect(() => { setDrawer(false) }, [loc.pathname, wide])
  const open = wide ? side && !folded : drawer
  const rail = wide && side && folded && prefs.collapse !== 'hide'
  const toggle = () => {
    if (wide) { setNavCollapsed(!folded); setFolded(!folded) } else setDrawer((o) => !o)
  }
  return (
    <div className={'app nav-' + style + (open ? ' nav-open' : '') + (rail ? ' nav-rail' : '')}>
      <a className="skip-link" data-nav-skip href="#main">Skip to content</a>
      <Sidebar open={open} rail={rail} drawer={!wide} onClose={() => setDrawer(false)} />
      <div className="main">
        <TopBar onMenu={toggle} showMenu={side || !wide} topNav={style !== 'side'} menuOpen={open} wide={wide} />
        <LegalAckBar />
        <main className="content" id="main" tabIndex={-1}>
          <Suspense fallback={<Spinner />}>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/library/:id" element={<LibraryPage />} />
            <Route path="/item/:id" element={<ItemPage />} />
            <Route path="/show/:id" element={<ShowPage />} />
            <Route path="/album/:lib" element={<AlbumPage />} />
            <Route path="/artist/:lib" element={<Suspense fallback={<Spinner />}><ArtistPage /></Suspense>} />
            <Route path="/radio/:id" element={<Suspense fallback={<Spinner />}><RadioStationPage /></Suspense>} />
            <Route path="/cd/:id" element={<CdPlayer />} />
            <Route path="/couch" element={<Suspense fallback={<Spinner />}><CouchPage /></Suspense>} />
            <Route path="/couch/:id" element={<Suspense fallback={<Spinner />}><CouchHostPage /></Suspense>} />
            <Route path="/collections" element={<Suspense fallback={<Spinner />}><CollectionsPage /></Suspense>} />
            <Route path="/collections/:id" element={<Suspense fallback={<Spinner />}><CollectionPage /></Suspense>} />
            <Route path="/review" element={<Suspense fallback={<Spinner />}><ReviewPage /></Suspense>} />
            <Route path="/watch/:id" element={<Suspense fallback={<Spinner />}><WatchPage /></Suspense>} />
            <Route path="/creator/:id" element={<Suspense fallback={<Spinner />}><CreatorPage /></Suspense>} />
            <Route path="/search" element={<SearchPage />} />
            <Route path="/fix-matches" element={<Suspense fallback={<Spinner />}><FixMatches /></Suspense>} />
            <Route path="/brand/:kind/:name" element={<Suspense fallback={<Spinner />}><BrandPage /></Suspense>} />
            <Route path="/pair" element={<PairPage />} />
            <Route path="/tv" element={<Suspense fallback={tvFallback}><GuidePage /></Suspense>} />
            <Route path="/tv/recordings" element={<Suspense fallback={tvFallback}><RecordingsPage /></Suspense>} />
            <Route path="/tv/channels/*" element={<Suspense fallback={tvFallback}><ChannelsPage /></Suspense>} />
            <Route path="/tv/encrypted" element={<Suspense fallback={tvFallback}><EncryptedHelp /></Suspense>} />
            <Route path="/controllers" element={<Suspense fallback={<Spinner />}><ControllersScreen /></Suspense>} />
            <Route path="/settings/*" element={<SettingsPage />} />
            <Route path="/m/:module/:page" element={<Suspense fallback={<Spinner />}><ModulePage /></Suspense>} />
            <Route path="/help" element={<Suspense fallback={<Spinner />}><HelpPage /></Suspense>} />
            <Route path="/help/:guide" element={<Suspense fallback={<Spinner />}><HelpPage /></Suspense>} />
            <Route path="/design" element={<DesignPage />} />
            <Route path="*" element={<NotFound />} />
          </Routes>
          </Suspense>
          <LegalNote kind="general" className="legal-foot" />
          <footer className="page-foot"><VersionTag version={version} /></footer>
        </main>
      </div>
      <TabBar onMenu={() => setDrawer(true)} />
    </div>
  )
}


// An address that isn't a page: say so, with the way home, rather than landing somewhere unexplained.
function NotFound() {
  return <><h1 className="sr-only">Page not found</h1><EmptyState title="There’s no page here" text="The address may have changed, or it may be mistyped." action={<LinkButton variant="primary" to="/">Go to Home</LinkButton>} /></>
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
