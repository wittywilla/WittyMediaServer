import { lazy, Suspense, useEffect, useMemo, useState, useSyncExternalStore } from 'react'
import { Link, NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { api, type Library } from '../api'
import AddLibrary, { GroupingChoice, type Grouping } from '../components/AddLibrary'
import { SettingsHelp } from '../components/HelpLink'
import { BackLink, UpdateDot, VersionTag, LoadError } from '../components/common'
import { ChooseFolderButton } from '../components/FolderPicker'
import { useModulePages } from '../moduleBundles'
import { IconHelp, IconActivity, IconChannels, IconClose, IconContrast, IconDownload, IconDrive, IconEdit, IconFile, IconFilm, IconFolder, IconGamepad, IconGlobe, IconImage, IconInfo, IconLibrary, IconLock, IconPalette, IconPlay, IconPlug, IconPlus, IconRefresh, IconRemote, IconRight, IconShield, IconTag, IconTrash, IconTune, IconUsers, IconWrench, libIcon } from '../icons'
import { toast, useApp } from '../state'
import AppearanceSettings from './AppearanceSettings'
import Profiles from './Profiles'
import LicenseList from './settings/Licenses'
import SupportedFormats from './settings/SupportedFormats'
import MetadataSettings from './settings/Metadata'
import PlaybackSettings from './settings/Playback'
import SharingSettings from './settings/Sharing'
import GamingSettings from './settings/Gaming'
import { ConsolePicker, GameSettingsPage } from './games/consoles'
import { kindTitle, libraryKinds, missingKind, orList, type LibraryKind } from '../moduleStore'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './settings/ui'
import { jumpWhenReady } from './settings/jump'
import { SettingsResults, SettingsSearch } from './settings/SettingsSearch'
import TranscodeSettings from './TranscodeSettings'
import './settings/settings.css'
import LegalNote from '../components/LegalNote'
import { ask, Dialog } from '../ui/Overlay'
import { Segmented } from '../ui/Choice'
import { UpdateBanner } from '../components/UpdateBanner'
import { Updates } from './settings/Updates'

const ModulesSettings = lazy(() => import('./settings/Modules'))
const ModuleStore = lazy(() => import('./settings/store/ModuleStore'))
const ModulePage = lazy(() => import('./settings/store/ModulePage'))
const OtherStores = lazy(() => import('./settings/store/OtherStores'))
const Diagnoser = lazy(() => import('./settings/Diagnoser'))
const ActivitySettings = lazy(() => import('./settings/Activity'))
const LibraryTranscoding = lazy(() => import('./settings/LibraryTranscoding'))
const LogSettings = lazy(() => import('./LogSettings'))
const LegalSettings = lazy(() => import('./settings/Legal'))
const ResetSettings = lazy(() => import('./settings/Reset'))
const BackupSettings = lazy(() => import('./settings/Backup'))
const RepairSettings = lazy(() => import('./settings/Repair'))
const RemoteAccess = lazy(() => import('./settings/RemoteAccess'))
const CouchSettings = lazy(() => import('./settings/Couch'))
const WittyRigsSettings = lazy(() => import('./settings/WittyRigs'))
const Security = lazy(() => import('./settings/Security'))
const TvSettings = lazy(() => import('./tv/TvSettings'))

// Settings: a menu of pages grouped by what they are about, each page a column of
// section cards in the middle of the screen. On wide screens the menu sits beside the
// page. Where that would squeeze the page, the menu is a page of its own, explaining
// each item, with a way back to it from each page.
const NARROW = '(max-width: 1280px)'
const useNarrow = () => useSyncExternalStore((changed) => {
  const m = window.matchMedia(NARROW)
  m.addEventListener('change', changed)
  return () => m.removeEventListener('change', changed)
}, () => window.matchMedia(NARROW).matches)

type Page = { to: string; label: string; icon: React.ReactNode; admin?: boolean; hint: string }

// Each row's colour on a phone, where the menu is a grouped list with tinted icon tiles: the
// same colours as the apps' You page (WittyMediaClient's docs/APP-DESIGN.md, You and
// settings). The owner's tools, Server and System, are all the one quiet colour.
const QUIET = '#5d6474'
const TINTS: Record<string, string> = {
  users: '#b8500d', security: '#2e7d3c', appearance: '#0e7c6f', games: '#7f46b8', activity: '#99690a', sharing: '#0a6fb8',
  reset: 'var(--danger)', help: '#99690a',
}
const tint = (to: string) => ({ '--tint': TINTS[to] || QUIET }) as React.CSSProperties
const GROUPS: { title: string; pages: Page[] }[] = [
  { title: 'Server', pages: [
    { to: 'libraries', label: 'Libraries', icon: <IconLibrary />, admin: true, hint: 'Folders of movies, shows, music and books' },
    { to: 'metadata', label: 'Metadata agents', icon: <IconTag />, admin: true, hint: 'Where posters and plots come from' },
    { to: 'playback', label: 'Playback & discs', icon: <IconPlay />, admin: true, hint: 'Player, DVDs and Blu-rays' },
    { to: 'channels', label: 'Channels & tuners', icon: <IconChannels />, admin: true, hint: 'Tuners, IPTV, guide data and WittyTV options' },
    { to: 'gaming', label: 'Consoles & cores', icon: <IconGamepad />, admin: true, hint: 'Which consoles this server can play, their cores, and game streaming' },
    { to: 'couch', label: 'Game PCs', icon: <IconDrive />, admin: true, hint: 'Who can play on PCs running Sunshine, and which ones' },
    { to: 'transcoding', label: 'Live transcoding', icon: <IconFilm />, admin: true, hint: 'While someone watches: quality, hardware and formats' },
    { to: 'library-transcoding', label: 'Library transcoding', icon: <IconFilm />, admin: true, hint: 'Convert files ahead of time, and connect Tdarr' },
    { to: 'remote', label: 'Remote Access', icon: <IconGlobe />, admin: true, hint: 'Reaching your server away from home' },
  ] },
  { title: 'You', pages: [
    { to: 'users', label: 'Profiles', icon: <IconUsers />, hint: 'People, passwords and limits' },
    { to: 'security', label: 'Sign-in & security', icon: <IconLock />, hint: 'Your devices and two-factor sign-in' },
    { to: 'appearance', label: 'Appearance', icon: <IconPalette />, hint: 'Theme, colours, layout, and the look of WittyTV and the player' },
    { to: 'games', label: 'My game settings', icon: <IconContrast />, hint: 'Picture, speed, shortcuts and more, for your games' },
    { to: 'activity', label: 'Activity', icon: <IconActivity />, hint: 'The button showing library scans and other background work' },
    { to: 'sharing', label: 'Sharing & photos', icon: <IconImage />, hint: 'Uploads, links, the photo map' },
  ] },
  { title: 'Advanced', pages: [
    { to: 'wittyrigs', label: 'WittyRigs', icon: <IconRemote />, admin: true, hint: 'Hosting demanding games on this server’s own hardware' },
  ] },
  { title: 'System', pages: [
    { to: 'logs', label: 'WittyLog', icon: <IconFile />, admin: true, hint: 'What is recorded and for how long, and downloading logs to share' },
    { to: 'diagnoser', label: 'WittyDiagnoser', icon: <IconTune />, admin: true, hint: 'The disk diagnoser: checks your discs and your drives, and changes nothing' },
    { to: 'modules', label: 'Modules', icon: <IconPlug />, admin: true, hint: 'What this server can do: add, remove, update and choose versions' },
    { to: 'backup', label: 'Backup', icon: <IconDownload />, admin: true, hint: 'A ZIP of your data and metadata, on demand or on a schedule, and bringing one back' },
    { to: 'repair', label: 'Repair', icon: <IconWrench />, admin: true, hint: 'Finds and fixes problems with metadata, the database, modules and this install' },
    { to: 'reset', label: 'Reset', icon: <IconRefresh />, admin: true, hint: 'Back to a new install, with a copy kept if you want one' },
    { to: 'about', label: 'About', icon: <IconInfo />, hint: 'Version, modules, licenses and credits' },
    { to: 'legal', label: 'Legal', icon: <IconShield />, hint: 'Terms, who’s responsible for what, privacy and notices' },
  ] },
]

export default function SettingsPage() {
  const { user, version } = useApp()
  const loc = useLocation()
  const modulePages = useModulePages().filter((p) => p.where === 'settings')
  const groups = useMemo(() => GROUPS.map((g) => ({ ...g, pages: g.pages.filter((p) => !p.admin || user?.isAdmin) })).filter((g) => g.pages.length), [user?.isAdmin])
  const pages = useMemo(() => groups.flatMap((g) => g.pages), [groups])
  const moduleKey = modulePages.map((p) => `${p.module}/${p.id}/${p.title}`).join('|')
  const moduleResults = useMemo(() => modulePages.map((p) => ({ to: `/m/${p.module}/${p.id}`, title: p.title, where: 'From modules' })), [moduleKey])
  const [query, setQuery] = useState('')
  // A link to part of a page (#section) goes to that part once the page is drawn; any other
  // starts at the top.
  useEffect(() => {
    if (!loc.hash) { window.scrollTo({ top: 0 }); return }
    return jumpWhenReady(decodeURIComponent(loc.hash.slice(1)))
  }, [loc.pathname, loc.hash])
  const slug = loc.pathname.split('/')[2]
  const narrow = useNarrow()
  // A crash the owner hasn't looked at yet is pointed out on every settings page.
  // Asked again on leaving the logs page, where it may just have been dealt with.
  const [unseenCrashes, setUnseenCrashes] = useState<string[]>([])
  const onLogs = slug === 'logs'
  useEffect(() => {
    if (!user?.isAdmin || onLogs) return
    let gone = false
    api.get<{ crashes: { id: string; seen: boolean }[] }>('/logs/crashes').then((r) => { if (!gone) setUnseenCrashes(r.crashes.filter((c) => !c.seen).map((c) => c.id)) }).catch(() => {})
    return () => { gone = true }
  }, [user?.isAdmin, onLogs])
  const dismissCrashes = () => {
    Promise.all(unseenCrashes.map((id) => api.post(`/logs/crashes/${encodeURIComponent(id)}/seen`))).catch((e) => toast(e.message))
    setUnseenCrashes([])
  }
  if (!user?.isAdmin && GROUPS.some((g) => g.pages.some((p) => p.admin && p.to === slug))) return <Navigate to="/settings/users" replace />
  return (
    <div className={'set-shell' + (slug ? '' : ' at-menu')}>
      <nav className="set-nav" aria-label="Settings">
        <div className="set-nav-title">Settings</div>
        <SettingsSearch value={query} onChange={setQuery} />
        {query.trim() ? <SettingsResults query={query} pages={pages} extra={moduleResults} admin={!!user?.isAdmin} onPick={() => setQuery('')} /> : <>
        {groups.map((g) => (
          <div key={g.title} className="set-nav-group">
            <div className="set-nav-head">{g.title}</div>
            {g.pages.map((p) => (
              <NavLink key={p.to} to={`/settings/${p.to}`} className="set-nav-link" title={p.hint}>
                <span className="set-nav-icon" style={tint(p.to)}>{p.icon}</span><span className="set-nav-text"><span>{p.label}</span><small className="set-nav-hint">{p.hint}</small></span>{p.to === 'about' && <UpdateDot />}<IconRight className="set-nav-go" />
              </NavLink>
            ))}
          </div>
        ))}
        {modulePages.length > 0 && (
          <div className="set-nav-group">
            <div className="set-nav-head">From modules</div>
            {modulePages.map((p) => (
              <NavLink key={p.module + p.id} to={`/m/${p.module}/${p.id}`} className="set-nav-link">
                <span className="set-nav-icon" style={tint('')}><IconPlug /></span><span className="set-nav-text"><span>{p.title}</span></span><IconRight className="set-nav-go" />
              </NavLink>
            ))}
          </div>
        )}
        <div className="set-nav-group"><NavLink to="/help" className="set-nav-link" title="Guides to everything here, built in"><span className="set-nav-icon" style={tint('help')}><IconHelp /></span><span className="set-nav-text"><span>Help</span><small className="set-nav-hint">Guides to everything here, built in</small></span><IconRight className="set-nav-go" /></NavLink></div>
        <NavLink to="/settings/about#modules" className="set-nav-version" title="Version and modules">
          <VersionTag version={version} /><span>Version and modules</span>
        </NavLink>
        </>}
      </nav>
      <div className="set-main">
        <div className="set-back"><BackLink to="/settings" label="Settings" /></div>
        <SettingsHelp page={slug} />
        {unseenCrashes.length > 0 && !onLogs && (
          <div className="set-notice-row">
            <Link className="set-notice" to="/settings/logs?tab=crashes"><span>The server stopped unexpectedly. See what happened</span><IconRight /></Link>
            <button type="button" className="btn icon" aria-label="Dismiss this notice" title="Dismiss" onClick={dismissCrashes}><IconClose /></button>
          </div>
        )}
        {slug !== 'about' && <UpdateBanner />}
        <Routes>
          <Route index element={narrow ? null : <Navigate to={user?.isAdmin ? '/settings/libraries' : '/settings/users'} replace />} />
          <Route path="libraries" element={<Libraries />} />
          <Route path="metadata" element={<MetadataSettings />} />
          <Route path="playback" element={<PlaybackSettings />} />
          <Route path="channels/*" element={<Suspense fallback={<div className="center"><div className="spinner" /></div>}><TvSettings /></Suspense>} />
          <Route path="gaming" element={<GamingSettings />} />
          <Route path="games" element={<><PageHead title="My game settings" desc="How your games look, sound and behave. Set something for every game, for one console or, from a game’s own page, for one game. It follows you to every device." /><GameSettingsPage /></>} />
          <Route path="couch" element={<Suspense fallback={null}><CouchSettings /></Suspense>} />
          <Route path="wittyrigs" element={<Suspense fallback={null}><WittyRigsSettings /></Suspense>} />
          <Route path="server" element={<Navigate to="/settings/metadata" replace />} />
          <Route path="transcoding" element={<TranscodeSettings />} />
          <Route path="library-transcoding" element={<Suspense fallback={null}><LibraryTranscoding /></Suspense>} />
          <Route path="appearance" element={<><PageHead title="Appearance" desc="How this server looks for you: its theme and colours, where the menus sit, and the look of WittyTV and the player." /><AppearanceSettings /></>} />
          {/* Other names people try for it, and its parts. */}
          {['theme', 'themes', 'look', 'looks'].map((p) => <Route key={p} path={p} element={<Navigate to="/settings/appearance#theme" replace />} />)}
          {['colours', 'colors'].map((p) => <Route key={p} path={p} element={<Navigate to="/settings/appearance#colours" replace />} />)}
          {['wittytv', 'tv-look'].map((p) => <Route key={p} path={p} element={<Navigate to="/settings/appearance#wittytv" replace />} />)}
          <Route path="users" element={<Users />} />
          <Route path="sharing" element={<SharingSettings />} />
          <Route path="logs" element={<><PageHead title="WittyLog" desc="What the server records and for how long, and the logs as a ZIP to share, sanitized if you want. Nothing is ever sent anywhere." /><Suspense fallback={<div className="loading-box"><div className="spinner" /></div>}><LogSettings /></Suspense></>} />
          <Route path="remote" element={<Suspense fallback={null}><RemoteAccess /></Suspense>} />
          <Route path="security" element={<Suspense fallback={null}><Security /></Suspense>} />
          <Route path="activity" element={<Suspense fallback={null}><ActivitySettings /></Suspense>} />
          <Route path="diagnoser" element={<Suspense fallback={null}><Diagnoser /></Suspense>} />
          <Route path="metadata-files" element={<Navigate to="/settings/repair?tab=files" replace />} />
          <Route path="backup" element={<Suspense fallback={null}><BackupSettings /></Suspense>} />
          <Route path="repair" element={<Suspense fallback={null}><RepairSettings /></Suspense>} />
          <Route path="reset" element={<Suspense fallback={null}><ResetSettings /></Suspense>} />
          <Route path="modules" element={<Suspense fallback={null}><ModulesSettings /></Suspense>} />
          <Route path="modules/store" element={<Suspense fallback={null}><ModuleStore /></Suspense>} />
          <Route path="modules/store/:id" element={<Suspense fallback={null}><ModulePage /></Suspense>} />
          <Route path="modules/store/:store/:id" element={<Suspense fallback={null}><ModulePage /></Suspense>} />
          <Route path="modules/stores" element={<Suspense fallback={null}><OtherStores /></Suspense>} />
          <Route path="about" element={<About />} />
          <Route path="legal" element={<Suspense fallback={null}><LegalSettings /></Suspense>} />
          <Route path="*" element={<Navigate to={user?.isAdmin ? '/settings/libraries' : '/settings/users'} replace />} />
        </Routes>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- libraries

const TYPES: { v: string; label: string; hint: string }[] = [
  { v: 'movies', label: 'Movies', hint: 'Movie files; disc images of movies can go here too' },
  { v: 'shows', label: 'TV shows', hint: 'Series in season folders; disc images of a series too' },
  { v: 'discs', label: 'DVDs, Blu-rays & CDs', hint: 'Disc images: movies and series with their menus, audio CDs in a CD player' },
  { v: 'anime', label: 'Anime', hint: 'Series with AniList information' },
  { v: 'music', label: 'Music', hint: 'Albums and songs' },
  { v: 'audiobooks', label: 'Audiobooks', hint: 'Books to listen to, by chapter' },
  { v: 'podcasts', label: 'Podcasts', hint: 'RSS feeds you follow, plus episode files; the folder holds downloads' },
  { v: 'books', label: 'Books & comics', hint: 'EPUB, PDF, comics and manga' },
  { v: 'videos', label: 'Home videos', hint: 'Your own videos; nothing is looked up online' },
  { v: 'photos', label: 'Photos', hint: 'A timeline, albums and a map of your pictures' },
  { v: 'shorts', label: 'Vertical videos', hint: 'A feed of short videos with a For You page' },
  { v: 'channels', label: 'Creator videos', hint: 'An archive of creators’ videos, by channel' },
  { v: 'musicvideos', label: 'Music videos & concerts', hint: 'Music videos and live recordings by artist' },
  { v: 'radio', label: 'Internet radio', hint: 'Stations from a free directory or by stream address; no folder needed' },
  { v: 'commercials', label: 'Commercials', hint: 'Your own ads and bumpers for TV channels; they stay out of Home and search' },
  { v: 'games', label: 'Games', hint: 'Your own game files, by console, played in the browser' },
]
const typeLabel = (t: string) => TYPES.find((x) => x.v === t)?.label || t

// A commercials library works for channels whether or not it is on show.
function CommercialsSwitch({ library }: { library: Library }) {
  const { refreshLibraries } = useApp()
  const set = async (show: boolean) => {
    try { await api.put(`/libraries/${library.id}/main`, { show }); await refreshLibraries(); toast(show ? `${library.name} now shows in the menus and on Home` : `${library.name} is hidden, and still plays on channels`) } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="set-lib-switch">
      <span><b>Show in the main menus and on Home</b><br /><small className="muted">Off: only channels use it. Either way, it plays on the channels you choose it for.</small></span>
      <Switch checked={!!library.showMain} label={`Show ${library.name} in the main menus and on Home`} onChange={(v) => void set(v)} />
    </div>
  )
}

function Libraries() {
  const { libraries, refreshLibraries } = useApp()
  const [editing, setEditing] = useState<Partial<Library> | null>(null)
  const [adding, setAdding] = useState(false)
  const [grouping, setGrouping] = useState<Grouping>('combine')
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setGrouping(s.libraryGrouping === 'separate' ? 'separate' : 'combine')).catch(() => {}) }, [])
  const changeGrouping = async (g: Grouping) => {
    setGrouping(g)
    try { await api.put('/settings', { libraryGrouping: g }); toast(g === 'combine' ? 'Folders of one kind now join one library' : 'New folders now become libraries of their own') } catch (e: any) { toast(e.message) }
  }
  const remove = async (l: Library) => {
    if (!await ask({ title: `Remove the library “${l.name}”?`, body: 'Your files stay where they are; only the server forgets them.', confirm: 'Remove library', danger: true })) return
    await api.del(`/libraries/${l.id}`)
    await refreshLibraries()
    toast('Library removed')
  }
  const scan = async (l: Library) => { await api.post(`/libraries/${l.id}/scan`); toast(`Scanning ${l.name}`); setTimeout(refreshLibraries, 1000) }
  const add = () => setAdding(true)
  return (
    <>
      <PageHead title="Libraries" desc="Each library is one or more folders of one kind of media. The server looks through them every 6 hours, and Scan now does it straight away."
        actions={libraries.length > 0 && <button className="btn primary" onClick={add}><IconPlus /> Add library</button>} />
      {libraries.length > 0 && (
        <div className="set-lib-summary">
          <b>{libraries.length} {libraries.length === 1 ? 'library' : 'libraries'}</b>
        </div>
      )}
      <Section title="Folders of the same kind" desc="When you add a folder of a kind you already have a library for, it can join that library, or become a new library with its own name (for example “Kids’ movies” beside “Movies”). You can always choose differently when adding.">
        <GroupingChoice value={grouping} onChange={changeGrouping} />
      </Section>
      {libraries.length === 0 ? (
        <div className="set-card set-empty big">
          <IconLibrary size={34} />
          <b>No libraries yet</b>
          <p>Add a folder of movies, shows, music or books to get started.</p>
          <button className="btn primary" onClick={add}><IconPlus /> Add your first library</button>
        </div>
      ) : (
        <div className="set-libs">
          {libraries.map((l) => (
            <article key={l.id} className="set-lib">
              <div className="set-lib-top">
                <span className={'set-lib-icon t-' + l.type}>{libIcon(l.type)}</span>
                <div className="set-lib-title">
                  <b>{l.name}</b>
                  <span>{l.name === typeLabel(l.type) ? '' : `${typeLabel(l.type)} · `}{l.itemCount.toLocaleString()} {l.itemCount === 1 ? 'item' : 'items'}</span>
                </div>
                {l.scanning && <span className="set-pill live">Scanning…</span>}
              </div>
              <ul className="set-lib-paths">{l.paths.map((p) => <li key={p} title={p}><IconFolder size={14} /><span><bdi>{p}</bdi></span></li>)}</ul>
              {l.type === 'commercials' && <CommercialsSwitch library={l} />}
              <div className="set-lib-actions">
                <button className="btn sm" onClick={() => scan(l)} disabled={l.scanning}><IconRefresh /> Scan now</button>
                <button className="btn sm ghost" onClick={() => setEditing(l)}><IconEdit /> Edit</button>
                <span className="grow" />
                <button className="btn sm ghost square danger-hover" onClick={() => remove(l)} aria-label={`Remove ${l.name}`} title="Remove library"><IconTrash /></button>
              </div>
            </article>
          ))}
        </div>
      )}
      {adding && (
        <Dialog title="Add a library" size="wide" className="add-lib-modal" onClose={() => setAdding(false)}>
          <AddLibrary existing={libraries} grouping={grouping} onAdded={async (l) => { setAdding(false); await refreshLibraries(); toast(`${l.name} saved; scanning`) }} />
        </Dialog>
      )}
      {editing && <LibraryEditor lib={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await refreshLibraries(); toast('Library saved; scanning') }} />}
      <LegalNote kind="local" className="set-legal" />
    </>
  )
}

function LibraryEditor({ lib, onClose, onSaved }: { lib: Partial<Library>; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(lib.name || '')
  const [type, setType] = useState(lib.type || 'movies')
  const [console_, setConsole] = useState(lib.console || 'auto')
  const [paths, setPaths] = useState<string[]>(lib.paths || [])
  const [manual, setManual] = useState('')
  const [err, setErr] = useState('')
  // Kinds whose module isn't on this server can't be chosen, though a library that is
  // already of such a kind keeps its type.
  const [kinds, setKinds] = useState<LibraryKind[]>([])
  useEffect(() => { void libraryKinds().then(setKinds) }, [])
  const add = (p: string) => { if (p.trim()) setPaths([...new Set([...paths, p.trim()])]); setManual('') }
  const save = async () => {
    try {
      const body = { name: name.trim(), type, paths, console: type === 'games' ? console_ : undefined }
      if (lib.id) await api.put(`/libraries/${lib.id}`, body); else await api.post('/libraries', body)
      onSaved()
    } catch (e: any) { setErr(e.message) }
  }
  return (
    <Dialog title={<>{lib.id ? `Edit ${lib.name}` : 'Add a library'}</>} onClose={onClose} size="wide" className="set-modal" actions={<><button className="btn ghost" onClick={onClose}>Cancel</button>
          <button className="btn primary" onClick={save} disabled={!name.trim() || (paths.length === 0 && type !== 'podcasts' && type !== 'radio' && type !== 'anime')}>{lib.id ? 'Save and scan' : 'Add library'}</button></>}>
        <div className="set-form">
          <fieldset className="set-types">
            <legend>What's in it?</legend>
            {TYPES.map((t) => {
              const gone = missingKind(kinds, t.v)
              const locked = !!gone && lib.type !== t.v
              return (
                <label key={t.v} className={'set-type' + (type === t.v ? ' on' : '') + (locked ? ' off' : '')}>
                  <input type="radio" name="libtype" checked={type === t.v} disabled={locked} onChange={() => { setType(t.v); if (!name || TYPES.some((x) => x.label === name)) setName(t.label) }} />
                  <span className="set-type-icon">{libIcon(t.v)}</span>
                  <b>{kindTitle(kinds, t.v, t.label)}</b>
                  {locked
                    ? <small>Needs {orList(gone.modules.map((m) => m.name))}. <Link to={`/settings/modules/store/${encodeURIComponent(gone.modules[0]?.id || '')}`}>Open the module store</Link></small>
                    : <small>{t.hint}</small>}
                </label>
              )
            })}
          </fieldset>
          {type === 'games' && <ConsolePicker value={console_} onChange={(id) => setConsole(id)} />}
          <label>Name<input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Movies" /></label>
          <div className="set-field">
            <span className="set-field-label">Folders</span>
            {paths.length > 0 && (
              <ul className="set-paths">
                {paths.map((p) => <li key={p}><IconFolder size={16} /><span title={p}>{p}</span><button className="btn sm ghost square" onClick={() => setPaths(paths.filter((x) => x !== p))} aria-label={`Remove ${p}`}><IconClose /></button></li>)}
              </ul>
            )}
            <div className="set-addpath">
              <ChooseFolderButton className="btn" onPick={add} />
              <input className="input mono" placeholder="…or type a folder path and press Enter" aria-label="Folder path" value={manual} onChange={(e) => setManual(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') add(manual) }} />
              {manual && <button className="btn sm" onClick={() => add(manual)}>Add</button>}
            </div>
            {type === 'podcasts' && <small className="muted">A folder is only needed for episodes you download.</small>}
          </div>
          {err && <div className="error">{err}</div>}
        </div>
    </Dialog>
  )
}

// ---------------------------------------------------------------- profiles

function Users() {
  const { user } = useApp()
  return (
    <>
      <PageHead title="Profiles" desc="Everyone who uses this server, with their passwords and what they may watch." />
      <Profiles />
      {user?.isAdmin && <SignInScreen />}
    </>
  )
}

// SignInScreen chooses between the "Who's watching?" profile screen and the
// classic name + password form.
function SignInScreen() {
  const st = useServerSettings()
  const { s, set, on } = st
  if (!s) return st.failed ? <LoadError what="This page" onRetry={st.retry} /> : null
  const profiles = (s.loginStyle || 'profiles') === 'profiles'
  return (
    <Section title="Signing in">
      <Row label="Sign-in screen" desc="Profiles show everyone's picture to pick from, like a streaming service. The classic form asks for a name and password.">
        <Segmented label="Sign-in screen" value={profiles ? 'profiles' : 'classic'} onChange={(v) => set('loginStyle', v)} options={[{ value: 'profiles', label: 'Profiles' }, { value: 'classic', label: 'Classic' }]} />
      </Row>
      <Row label="Show profiles outside your home" desc="Off by default: anyone who can reach the server would see everyone's names. Passwords are always required." dim={!profiles}>
        <Switch label="Show profiles outside your home" disabled={!profiles} checked={on('profilesRemote', '0')} onChange={(v) => set('profilesRemote', v ? '1' : '0')} />
      </Row>
      <Row label="Let apps find this server" desc="Apps on your home network can see this server's name, version and a random ID. They still need a code or a password to connect, and can always connect by typing its address.">
        <Switch label="Let apps find this server" checked={on('lanDiscovery', '1')} onChange={(v) => set('lanDiscovery', v ? '1' : '0')} />
      </Row>
      <Row label="Who may manage Kids profiles">
        <select className="select" value={s.kidsManagedBy || 'adults'} onChange={(e) => set('kidsManagedBy', e.target.value)}>
          <option value="adults">Every adult</option>
          <option value="admin">Only the administrator</option>
        </select>
      </Row>
      <SaveBar {...st} />
    </Section>
  )
}

// ---------------------------------------------------------------- about

// The readers built into the web app (their versions are set at build time).
const READERS: Record<string, [string, string]> = {
  wittyebook: ['WittyEbook', 'EPUB reader'],
  wittypdf: ['WittyPDF', 'PDF reader'],
  wittycomics: ['WittyComics', 'Comic and manga reader'],
  wittyink: ['WittyInk', 'Highlights, notes and pen, shared by the readers'],
}
type Engine = { name: string; module: string; version: string; local?: boolean; role: string }

function About() {
  const [sys, setSys] = useState<any>(null)
  const [about, setAbout] = useState<{ version: string; modules: Engine[] } | null>(null)
  const { user, version } = useApp()
  const loc = useLocation()
  useEffect(() => {
    if (!user?.isAdmin) return
    let t = 0
    const ask = () => api.get<any>('/system').then((s) => { setSys(s); if (!s.hwTested) t = window.setTimeout(ask, 2000) }).catch(() => {})
    ask()
    return () => clearTimeout(t)
  }, [user])
  useEffect(() => { api.get<{ version: string; modules: Engine[] }>('/about').then(setAbout).catch(() => {}) }, [])
  useEffect(() => { if (loc.hash && about) document.getElementById(loc.hash.slice(1))?.scrollIntoView({ block: 'start' }) }, [loc.hash, about])
  const readers = __WEB_MODULES__.filter((m) => READERS[m.id]).sort((a, b) => Object.keys(READERS).indexOf(a.id) - Object.keys(READERS).indexOf(b.id))
  const hw = (sys?.hardware || []).filter((h: any) => h.working).map((h: any) => h.label)
  return (
    <>
      <PageHead title="About" desc="This server's version, the modules installed on it, its system, its license, every third-party license, and the services it relies on." />
      <Section title="Version and modules" desc="What the server can do comes from separate modules, each published on its own. These are the versions on this server.">
        <div id="modules" className="mod-list">
          <div className="mod-row mod-self">
            <span className="mod-name"><b>WittyMediaServer</b><span className="muted">The server and this app</span></span>
            <span className="mod-ver">{about?.version || version || 'dev'}</span>
          </div>
          <div className="mod-group">Disc engines</div>
          {(about?.modules || []).map((m) => (
            <a key={m.name} className="mod-row" href={`https://github.com/wittywilla/${m.name}`} target="_blank" rel="noreferrer">
              <span className="mod-name"><b>{m.name}</b><span className="muted">{m.role}</span></span>
              <span className="mod-ver">{m.version}{m.local && <span className="mod-local" title="Built from this server's own copy of the module">built in</span>}</span>
            </a>
          ))}
          <div className="mod-group">Readers</div>
          {readers.map((m) => (
            <a key={m.id} className="mod-row" href={`https://github.com/wittywilla/${READERS[m.id][0]}`} target="_blank" rel="noreferrer">
              <span className="mod-name"><b>{READERS[m.id][0]}</b><span className="muted">{READERS[m.id][1]}</span></span>
              <span className="mod-ver">{m.version}</span>
            </a>
          ))}
        </div>
      </Section>
      {user?.isAdmin && <Updates />}
      {sys && (
        <Section title="This server">
          <dl className="set-facts">
            <div className="wide"><dt>Open it on other devices at home</dt><dd className="mono">{(sys.lanUrls || []).length ? sys.lanUrls.join('  ·  ') : 'This server only accepts connections from this computer'}</dd></div>
            <div><dt>System</dt><dd>{sys.os} / {sys.arch}</dd></div>
            <div><dt>Computer</dt><dd>{sys.hostname}</dd></div>
            <div><dt>Hardware video</dt><dd>{hw.length ? hw.join(', ') : sys?.hwTested ? 'None found; software encoding' : 'Checking…'}</dd></div>
            <div><dt>Open discs</dt><dd>{sys.dvdSessions} DVD · {sys.blurays ?? 0} Blu-ray</dd></div>
            <div><dt>Built with</dt><dd>{sys.go}</dd></div>
            <div className="wide"><dt>Data folder</dt><dd className="mono">{sys.dataDir}</dd></div>
            <div className="wide"><dt>FFmpeg</dt><dd className="mono">{sys.ffmpegInfo}</dd></div>
            {sys.smartAppControl && <div className="wide"><dt>Windows Smart App Control</dt><dd>{sys.smartAppControl}. <Link to="/help/troubleshooting#smart-app-control-on-windows">What this means</Link></dd></div>}
          </dl>
        </Section>
      )}
      <Section title="License">
        <div className="set-prose">
          <p><b>WittyMediaServer</b> is open-source software under the <b>Apache License 2.0</b>. You may use, change and share it, keeping the license and the <code>NOTICE</code> attribution (“This product includes WittyMediaServer, developed by WittyWilla”). Source: <a href="https://github.com/wittywilla/WittyMediaServer" target="_blank" rel="noreferrer">github.com/wittywilla/WittyMediaServer</a>.</p>
        </div>
      </Section>
      <Section title="Legal">
        <div className="set-prose">
          <p>Terms of use, who’s responsible for what, trademarks, copy protection, privacy and more are on the <Link to="/settings/legal">Legal</Link> page.</p>
        </div>
      </Section>
      <div id="formats" />
      <Section title="Supported formats" desc="Every kind of file this server takes, how each plays, and what it needs.">
        <SupportedFormats />
      </Section>
      <div id="licenses" />
      <Section title="Third-party licenses" desc="Everything in this server that someone else made, and what it’s under.">
        <LicenseList />
      </Section>
      <Section title="Third-party components and services">
        <div className="set-prose">
          <p>Video is converted by <b>FFmpeg</b>, run as a separate program; the bundled builds are GPL-licensed, with sources linked in <code>THIRD_PARTY_LICENSES.md</code>. With your own key, information comes from <b>The Movie Database (TMDB)</b>: this product uses the TMDB API but is not endorsed or certified by TMDB. Without keys, TV information comes from <b>TVmaze</b> (CC BY-SA 4.0), movie facts from <b>Wikidata</b> (CC0) and plot summaries from <b>Wikipedia</b> (CC BY-SA 4.0). Disc menus are run by WittyMediaServer's own WittyDVD and WittyBluray engines.</p>
          <p><b>WittyCouch</b> is this server’s client for PCs running Sunshine. License: <b>Apache-2.0</b>. <a href="https://github.com/wittywilla/WittyCouch" target="_blank" rel="noreferrer">Source</a>. Built on: <b>WittyCouch Host</b>, a separate program it starts, under the <b>GPL-3.0</b> (<a href="https://github.com/wittywilla/WittyCouchHost" target="_blank" rel="noreferrer">source</a>), which contains <a href="https://github.com/moonlight-stream/moonlight-common-c" target="_blank" rel="noreferrer">moonlight-common-c</a> by the Moonlight project (GPL-3.0). It’s installed separately, and nothing else here contains it. Settings, Game PCs lists every piece.</p>
          <p><b>WittyRigs</b> is an advanced option for hosting games on this server. License: <b>Apache-2.0</b>. <a href="https://github.com/wittywilla/WittyRigs" target="_blank" rel="noreferrer">Source</a>. Built on: <a href="https://github.com/LizardByte/Sunshine" target="_blank" rel="noreferrer">Sunshine</a> by LizardByte (GPL-3.0), which you install yourself. The Moonlight and Sunshine projects didn’t make these, and don’t endorse them.</p>
          <p><b>No warranty.</b> The software is provided “as is”, without warranty of any kind. WittyMediaServer is not affiliated with any other media software project, studio or rights holder; all trademarks belong to their owners.</p>
        </div>
      </Section>
    </>
  )
}
