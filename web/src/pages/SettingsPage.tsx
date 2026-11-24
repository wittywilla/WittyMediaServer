// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useMemo, useState, useSyncExternalStore } from 'react'
import { Link, NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { api, type Library } from '../api'
import AddLibrary, { GroupingChoice, type Grouping } from '../components/AddLibrary'
import { SettingsHelp } from '../components/HelpLink'
import { BackLink, UpdateDot, VersionTag, LoadError } from '../components/common'
import { ScanControl, scanProgressText, useScanProgress } from '../components/ScanControl'
import { ChooseFolderButton } from '../components/FolderPicker'
import { useModulePages } from '../moduleBundles'
import { ModuleSettingsView, useModuleSettingsTabs, type ModuleSettingsTab } from '../moduleRegistry'
import { IconHelp, IconActivity, IconBell, IconChannels, IconClose, IconDownload, IconDrive, IconEdit, IconFile, IconFilm, IconFolder, IconGlobe, IconImage, IconInfo, IconLibrary, IconLock, IconNetwork, IconPalette, IconPlay, IconPlug, IconPlus, IconRefresh, IconRight, IconShield, IconSidebar, IconTag, IconTrash, IconTune, IconUsers, IconWrench, libIcon } from '../icons'
import { toast, useApp } from '../state'
import { settingsPageHidden } from '../profileWords'
import { useMyLimits } from '../myLimits'
import AppearanceSettings from './AppearanceSettings'
import Profiles from './Profiles'
import LicenseList from './settings/Licenses'
import SupportedFormats from './settings/SupportedFormats'
import MetadataSettings from './settings/Metadata'
import PlaybackSettings from './settings/Playback'
import StreamQualitySettings from './settings/StreamQuality'
import SharingSettings from './settings/Sharing'
import { ConsolePicker } from '../components/ConsolePicker'
import { kindTitle, libraryKinds, missingKind, orList, type LibraryKind } from '../moduleStore'
import { PageHead, Row, SaveBar, Section, Switch, useServerSettings } from './settings/ui'
import { jumpWhenReady } from './settings/jump'
import { commercialsSwitchText, nfoOn, nfoSwitchShown, nfoSwitchText } from './settings/librarySwitches'
import { SettingsResults, SettingsSearch } from './settings/SettingsSearch'
import TranscodeSettings from './TranscodeSettings'
import LanguageSettings from './settings/Language'
import './settings/settings.css'
import './settings/settings-page.css'
import LegalNote from '../components/LegalNote'
import { ask, Dialog } from '../ui/Overlay'
import { ForgetMetadataDialog } from './settings/ForgetMetadata'
import { Segmented } from '../ui/Choice'
import { UpdateBanner } from '../components/UpdateBanner'
import { Updates } from './settings/Updates'
import { useFeatures } from '../sdui/useSdui'
import { useTvMode } from '../tvmode/mode'
import { tvScreenFor } from '../tvmode/levelTable'
import { tvElsewhere } from '../tvmode/OnAComputer'
import { t, tk, tn, tx } from '../i18n'

const ModulesSettings = lazy(() => import('./settings/Modules'))
const WittyTvLookDialog = lazy(() => import('./tv/WittyTvLookPrompt').then((m) => ({ default: m.WittyTvLookDialog })))
const ModuleStore = lazy(() => import('./settings/store/ModuleStore'))
const ModulePage = lazy(() => import('./settings/store/ModulePage'))
const OtherStores = lazy(() => import('./settings/store/OtherStores'))
const Diagnoser = lazy(() => import('./settings/Diagnoser'))
const ActivitySettings = lazy(() => import('./settings/Activity'))
const NotificationSettings = lazy(() => import('./settings/Notifications'))
const LibraryTranscoding = lazy(() => import('./settings/LibraryTranscoding'))
const LogSettings = lazy(() => import('./LogSettings'))
const LegalSettings = lazy(() => import('./settings/Legal'))
const ResetSettings = lazy(() => import('./settings/Reset'))
const BackupSettings = lazy(() => import('./settings/Backup'))
const RepairSettings = lazy(() => import('./settings/Repair'))
const RemoteAccess = lazy(() => import('./settings/RemoteAccess'))
const NetworkSettings = lazy(() => import('./settings/Network'))
const DataFolderSettings = lazy(() => import('./settings/DataFolder'))
const ModuleSettingsList = lazy(() => import('./settings/ModuleSettings').then((m) => ({ default: m.ModuleSettingsList })))
const ModuleSettingsPage = lazy(() => import('./settings/ModuleSettings').then((m) => ({ default: m.ModuleSettingsPage })))
const Security = lazy(() => import('./settings/Security'))
const TvSettings = lazy(() => import('./tv/TvSettings'))
// The editor of server-driven pages is a program of its own: nothing of it loads until it is opened.
const PagesSettings = lazy(() => import('./settings/pages/PagesSettings'))

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

type Page = { to: string; label: string; icon: React.ReactNode; admin?: boolean; hint: string; launch?: string }

// Each row's colour on a phone, where the menu is a grouped list with tinted icon tiles: the
// same colours as the apps' You page (WittyMediaClient's docs/APP-DESIGN.md, You and
// settings). The owner's tools, Server and System, are all the one quiet colour.
const QUIET = '#5d6474'
const TINTS: Record<string, string> = {
  users: '#b8500d', security: '#2e7d3c', appearance: '#0e7c6f', games: '#7f46b8', activity: '#99690a', notifications: '#99690a', sharing: '#0a6fb8',
  reset: 'var(--danger)', help: '#99690a',
}
const tint = (to: string) => ({ '--tint': TINTS[to] || QUIET }) as React.CSSProperties
const GROUPS: { title: string; pages: Page[] }[] = [
  { title: tk('Server'), pages: [
    { to: 'libraries', label: tk('Libraries'), icon: <IconLibrary />, admin: true, hint: tk('Folders of movies, shows, music and books') },
    { to: 'pages', label: tk('Pages & menu'), icon: <IconSidebar />, admin: true, hint: tk('What Home, the libraries and the menu hold, for everyone') },
    { to: 'metadata', label: tk('Metadata agents'), icon: <IconTag />, admin: true, hint: tk('Where posters and plots come from') },
    { to: 'playback', label: tk('Playback'), icon: <IconPlay />, admin: true, hint: tk('Player, skipping and streaming') },
    { to: 'channels', label: tk('Library channels & WittyTV'), icon: <IconChannels />, admin: true, hint: tk('Library channels and WittyTV options') },
    { to: 'transcoding', label: tk('Live transcoding'), icon: <IconFilm />, admin: true, hint: tk('While someone watches: quality, hardware and formats') },
    { to: 'library-transcoding', label: tk('Library transcoding'), icon: <IconFilm />, admin: true, hint: tk('Convert files ahead of time, and connect Tdarr') },
    { to: 'network', label: tk('Network'), icon: <IconNetwork />, admin: true, hint: tk('The port this server answers on at home') },
    { to: 'data-folder', label: tk('Data folder'), icon: <IconDrive />, admin: true, hint: tk('Where settings, the database and artwork are kept, and moving them') },
    { to: 'remote', label: tk('Remote Access'), icon: <IconGlobe />, admin: true, hint: tk('Reaching your server away from home') },
  ] },
  { title: tk('You'), pages: [
    { to: 'users', label: tk('Profiles'), icon: <IconUsers />, hint: tk('People, passwords and limits') },
    { to: 'security', label: tk('Sign-in & security'), icon: <IconLock />, hint: tk('Your devices and two-factor sign-in') },
    { to: 'quality', label: tk('Video quality'), icon: <IconPlay />, hint: tk('The picture size and data videos play at, at home and away') },
    { to: 'appearance', label: tk('Appearance'), icon: <IconPalette />, hint: tk('Theme, colours, layout, and the look of WittyTV and the player') },
    { to: 'activity', label: tk('Activity'), icon: <IconActivity />, hint: tk('The button showing library scans and other background work') },
    { to: 'notifications', label: tk('Notifications'), icon: <IconBell />, hint: tk('What the bell tells you about, and email or a webhook') },
    { to: 'sharing', label: tk('Sharing & photos'), icon: <IconImage />, hint: tk('Uploads, links, the photo map') },
    { to: 'module-settings', label: tk('Module settings'), icon: <IconPlug />, hint: tk('What the modules on this server let you set') },
  ] },
  { title: tk('System'), pages: [
    { to: 'logs', label: tk('WittyLog'), icon: <IconFile />, admin: true, hint: tk('What is recorded and for how long, and downloading logs to share') },
    { to: 'diagnoser', label: tk('WittyDiagnoser'), icon: <IconTune />, admin: true, hint: tk('The disk diagnoser: checks your discs and your drives, and changes nothing') },
    { to: 'modules', label: tk('Modules'), icon: <IconPlug />, admin: true, hint: tk('What this server can do: add, remove, update and choose versions') },
    { to: 'backup', label: tk('Backup'), icon: <IconDownload />, admin: true, hint: tk('A ZIP of your data and metadata, on demand or on a schedule, and bringing one back') },
    { to: 'repair', label: tk('Repair'), icon: <IconWrench />, admin: true, hint: tk('Finds and fixes problems with metadata, the database, modules and this install') },
    { to: 'reset', label: tk('Reset'), icon: <IconRefresh />, admin: true, hint: tk('Back to a new install, with a copy kept if you want one') },
    { to: 'about', label: tk('About'), icon: <IconInfo />, hint: tk('Version, modules, licenses and credits') },
    { to: 'legal', label: tk('Legal'), icon: <IconShield />, hint: tk('Terms, who’s responsible for what, privacy and notices') },
  ] },
]

// withModuleTabs puts the Settings sections modules draw among the built-in ones.
function withModuleTabs(groups: typeof GROUPS, tabs: ModuleSettingsTab[]): typeof GROUPS {
  if (!tabs.length) return groups
  const own = new Set(groups.flatMap((g) => g.pages.map((p) => p.to)))
  const row = (t: ModuleSettingsTab, was?: Page): Page => ({ to: t.id, label: t.label, icon: was?.icon ?? <IconPlug />, admin: t.admin, hint: was?.hint || tk('Added by a module'), launch: t.launch })
  const out = groups.map((g) => ({ ...g, pages: g.pages.flatMap((p) => {
    const same = tabs.find((t) => t.id === p.to)
    if (same?.hidden) return [] // the module's own place has it now (launch)
    const after = tabs.filter((t) => t.after === p.to && !own.has(t.id))
    return [same ? row(same, p) : p, ...after.map((t) => row(t))]
  }) }))
  const rest = tabs.filter((t) => !own.has(t.id) && !(t.after && own.has(t.after)))
  if (rest.length) out.push({ title: tk('From modules'), pages: rest.map((t) => row(t)) })
  return out
}

export default function SettingsPage() {
  const { user, version } = useApp()
  const loc = useLocation()
  const modulePages = useModulePages().filter((p) => p.where === 'settings')
  const features = useMyLimits()?.features
  // Module settings are for a server that has them (feature module-settings).
  const serverCan = useFeatures()
  const moduleSettings = !!serverCan?.has('module-settings')
  // Sections modules of API 5 draw (moduleRegistry.tsx): one with a built-in section's id takes
  // its place, one after a built-in section goes there, any other under From modules.
  const moduleTabs = useModuleSettingsTabs().filter((t) => !t.admin || user?.isAdmin)
  const tabsKey = moduleTabs.map((t) => `${t.module}/${t.id}/${t.label}/${t.after || ''}`).join('|')
  const groups = useMemo(() => withModuleTabs(GROUPS, moduleTabs).map((g) => ({ ...g, pages: g.pages.filter((p) => (!p.admin || user?.isAdmin) && !settingsPageHidden(p.to, user, features) && (p.to !== 'module-settings' || moduleSettings)) })).filter((g) => g.pages.length), [user?.isAdmin, user?.kind, features, moduleSettings, tabsKey]) // eslint-disable-line react-hooks/exhaustive-deps
  const moduleTab = moduleTabs.find((t) => t.id === loc.pathname.split('/')[2])
  const pages = useMemo(() => groups.flatMap((g) => g.pages), [groups])
  const moduleKey = modulePages.map((p) => `${p.module}/${p.id}/${p.title}`).join('|')
  const moduleResults = useMemo(() => modulePages.map((p) => ({ to: `/m/${p.module}/${p.id}`, title: p.title, where: tk('From modules') })), [moduleKey])
  const [query, setQuery] = useState('')
  // A link to part of a page (#section) goes to that part once the page is drawn; any other
  // starts at the top.
  useEffect(() => {
    if (!loc.hash) { window.scrollTo({ top: 0 }); return }
    return jumpWhenReady(decodeURIComponent(loc.hash.slice(1)))
  }, [loc.pathname, loc.hash])
  const slug = loc.pathname.split('/')[2]
  const narrow = useNarrow()
  const tvMode = useTvMode()
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
    <div className={'set-shell' + (slug ? '' : ' at-menu') + (tvMode && !slug ? ' tv-menu' : '')}>
      <nav className="set-nav" aria-label={t('Settings')}>
        <div className="set-nav-title">{t('Settings')}</div>
        <SettingsSearch value={query} onChange={setQuery} />
        {query.trim() ? <SettingsResults query={query} pages={pages} extra={moduleResults} admin={!!user?.isAdmin} onPick={() => setQuery('')} /> : <>
        {groups.map((g) => (
          <div key={g.title} className="set-nav-group">
            <div className="set-nav-head">{tx(g.title)}</div>
            {g.pages.map((p) => (
              <NavLink key={p.to} to={p.launch || `/settings/${p.to}`} className="set-nav-link" title={tx(p.hint)}>
                <span className="set-nav-icon" style={tint(p.to)}>{p.icon}</span><span className="set-nav-text"><span>{tx(p.label)}</span><small className="set-nav-hint">{tvMode && tvScreenFor('/settings/' + p.to)?.level === 'desktopOnly' && <span className="tvl-chip">{tvElsewhere()}</span>}{tx(p.hint)}</small></span>{p.to === 'about' && <UpdateDot />}<IconRight className="set-nav-go" />
              </NavLink>
            ))}
          </div>
        ))}
        {modulePages.length > 0 && (
          <div className="set-nav-group">
            <div className="set-nav-head">{t('From modules')}</div>
            {modulePages.map((p) => (
              <NavLink key={p.module + p.id} to={`/m/${p.module}/${p.id}`} className="set-nav-link">
                <span className="set-nav-icon" style={tint('')}><IconPlug /></span><span className="set-nav-text"><span>{p.title}</span><small className="set-nav-hint">{t('Added by a module')}</small></span><IconRight className="set-nav-go" />
              </NavLink>
            ))}
          </div>
        )}
        <div className="set-nav-group"><NavLink to="/help" className="set-nav-link" title={t('Guides to everything here, built in')}><span className="set-nav-icon" style={tint('help')}><IconHelp /></span><span className="set-nav-text"><span>{t('Help')}</span><small className="set-nav-hint">{t('Guides to everything here, built in')}</small></span><IconRight className="set-nav-go" /></NavLink></div>
        <NavLink to="/settings/about#modules" className="set-nav-version" title={t('Version and modules')}>
          <VersionTag version={version} /><span>{t('Version and modules')}</span>
        </NavLink>
        </>}
      </nav>
      <div className="set-main">
        <div className="set-back"><BackLink to="/settings" label={t('Settings')} /></div>
        <SettingsHelp page={slug} />
        {unseenCrashes.length > 0 && !onLogs && (
          <div className="set-notice-row">
            <Link className="set-notice" to="/settings/logs?tab=crashes"><span>{t('The server stopped unexpectedly. See what happened')}</span><IconRight /></Link>
            <button type="button" className="btn icon" aria-label={t('Dismiss this notice')} title={t('Dismiss')} onClick={dismissCrashes}><IconClose /></button>
          </div>
        )}
        {slug !== 'about' && <UpdateBanner />}
        {moduleTab?.launch ? <Navigate to={moduleTab.launch} replace /> : moduleTab ? <ModuleSettingsView tab={moduleTab} /> : <Routes>
          {/* On a television the menu is the page: its first choice may be one a computer changes (TV mode's guide). */}
          <Route index element={narrow || tvMode ? null : <Navigate to={user?.isAdmin ? '/settings/libraries' : '/settings/users'} replace />} />
          <Route path="libraries" element={<Libraries />} />
          <Route path="pages/*" element={<Suspense fallback={<div className="loading-box"><div className="spinner" /></div>}><PagesSettings /></Suspense>} />
          <Route path="metadata" element={<MetadataSettings />} />
          <Route path="playback" element={<PlaybackSettings />} />
          <Route path="quality" element={<StreamQualitySettings />} />
          <Route path="channels/*" element={<Suspense fallback={<div className="center"><div className="spinner" /></div>}><TvSettings /></Suspense>} />
          <Route path="server" element={<Navigate to="/settings/metadata" replace />} />
          <Route path="transcoding" element={<TranscodeSettings />} />
          <Route path="library-transcoding" element={<Suspense fallback={null}><LibraryTranscoding /></Suspense>} />
          <Route path="appearance" element={<><PageHead title={tk('Appearance')} desc={t('How this server looks for you: its theme and colours, where the menus sit, and the look of WittyTV and the player.')} />{user?.isAdmin && <LanguageSettings />}<AppearanceSettings /></>} />
          {/* Other names people try for it, and its parts. */}
          {['theme', 'themes', 'look', 'looks'].map((p) => <Route key={p} path={p} element={<Navigate to="/settings/appearance#theme" replace />} />)}
          {['colours', 'colors'].map((p) => <Route key={p} path={p} element={<Navigate to="/settings/appearance#colours" replace />} />)}
          {['wittytv', 'tv-look'].map((p) => <Route key={p} path={p} element={<Navigate to="/settings/appearance#wittytv" replace />} />)}
          <Route path="users" element={<Users />} />
          <Route path="sharing" element={<SharingSettings />} />
          <Route path="logs" element={<><PageHead title={tk('WittyLog')} desc={t('What the server records and for how long, and the logs as a ZIP to share, sanitized if you want. Nothing is ever sent anywhere.')} /><Suspense fallback={<div className="loading-box"><div className="spinner" /></div>}><LogSettings /></Suspense></>} />
          <Route path="network" element={<Suspense fallback={null}><NetworkSettings /></Suspense>} />
          <Route path="data-folder" element={<Suspense fallback={null}><DataFolderSettings /></Suspense>} />
          <Route path="remote" element={<Suspense fallback={null}><RemoteAccess /></Suspense>} />
          <Route path="security" element={<Suspense fallback={null}><Security /></Suspense>} />
          <Route path="activity" element={<Suspense fallback={null}><ActivitySettings /></Suspense>} />
          <Route path="notifications" element={<Suspense fallback={null}><NotificationSettings /></Suspense>} />
          <Route path="diagnoser" element={<Suspense fallback={null}><Diagnoser /></Suspense>} />
          <Route path="metadata-files" element={<Navigate to="/settings/repair?tab=files" replace />} />
          <Route path="backup" element={<Suspense fallback={null}><BackupSettings /></Suspense>} />
          <Route path="repair" element={<Suspense fallback={null}><RepairSettings /></Suspense>} />
          <Route path="reset" element={<Suspense fallback={null}><ResetSettings /></Suspense>} />
          <Route path="modules" element={<Suspense fallback={null}><ModulesSettings /><WittyTvLookDialog /></Suspense>} />
          <Route path="module-settings" element={<Suspense fallback={null}><ModuleSettingsList /></Suspense>} />
          <Route path="module-settings/:module" element={<Suspense fallback={null}><ModuleSettingsPage /></Suspense>} />
          <Route path="modules/store" element={<Suspense fallback={null}><ModuleStore /><WittyTvLookDialog /></Suspense>} />
          <Route path="modules/store/:id" element={<Suspense fallback={null}><ModulePage /><WittyTvLookDialog /></Suspense>} />
          <Route path="modules/store/:store/:id" element={<Suspense fallback={null}><ModulePage /><WittyTvLookDialog /></Suspense>} />
          <Route path="modules/stores" element={<Suspense fallback={null}><OtherStores /></Suspense>} />
          <Route path="about" element={<About />} />
          <Route path="legal" element={<Suspense fallback={null}><LegalSettings /></Suspense>} />
          <Route path="*" element={<Navigate to={tvMode ? '/settings' : user?.isAdmin ? '/settings/libraries' : '/settings/users'} replace />} />
        </Routes>}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- libraries

const TYPES: { v: string; label: string; hint: string }[] = [
  { v: 'movies', label: tk('Movies'), hint: tk('Movie files; disc images of movies can go here too') },
  { v: 'shows', label: tk('TV shows'), hint: tk('Series in season folders; disc images of a series too') },
  { v: 'discs', label: tk('DVDs, Blu-rays & CDs'), hint: tk('Disc images: movies and series with their menus, audio CDs in a CD player') },
  { v: 'anime', label: tk('Anime'), hint: tk('Series with AniList information') },
  { v: 'music', label: tk('Music'), hint: tk('Albums and songs') },
  { v: 'audiobooks', label: tk('Audiobooks'), hint: tk('Books to listen to, by chapter') },
  { v: 'podcasts', label: tk('Podcasts'), hint: tk('RSS feeds you follow, plus episode files; the folder holds downloads') },
  { v: 'books', label: tk('Books & comics'), hint: tk('EPUB, PDF, comics and manga') },
  { v: 'videos', label: tk('Home videos'), hint: tk('Your own videos; nothing is looked up online') },
  { v: 'photos', label: tk('Photos'), hint: tk('A timeline, albums and a map of your pictures') },
  { v: 'shorts', label: tk('Vertical videos'), hint: tk('A feed of short videos with a For You page') },
  { v: 'channels', label: tk('Creator videos'), hint: tk('An archive of creators’ videos, by channel') },
  { v: 'musicvideos', label: tk('Music videos & concerts'), hint: tk('Music videos and live recordings by artist') },
  { v: 'radio', label: tk('Internet radio'), hint: tk('Stations from a free directory or by stream address; no folder needed') },
  { v: 'commercials', label: tk('Commercials'), hint: tk('Your own ads and bumpers for TV channels; they stay out of Home and search') },
  { v: 'games', label: tk('Games'), hint: tk('Your own game files, by console, played in the browser') },
]
const typeLabel = (t: string) => TYPES.find((x) => x.v === t)?.label || t

// A commercials library works for channels whether or not it is on show.
function CommercialsSwitch({ library }: { library: Library }) {
  const { refreshLibraries } = useApp()
  const set = async (show: boolean) => {
    try { await api.put(`/libraries/${library.id}/main`, { show }); await refreshLibraries(); toast(show ? t('{name} now shows in the menus and on Home', { name: library.name }) : t('{name} is hidden, and still plays on channels', { name: library.name })) } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="set-lib-switch">
      <span><b>{t('Show in the main menus and on Home')}</b><br /><small className="muted">{tx(commercialsSwitchText(!!library.showMain))}</small></span>
      <Switch checked={!!library.showMain} label={t('Show {name} in the main menus and on Home', { name: library.name })} onChange={(v) => void set(v)} />
    </div>
  )
}

// Other programs' .nfo files (a downloader's, a media centre's) are read unless this is off. Some
// write a release's file name as the title and the day the file came out as the year.
function NfoSwitch({ library }: { library: Library }) {
  const { refreshLibraries } = useApp()
  const set = async (use: boolean) => {
    try { await api.put(`/libraries/${library.id}/nfo`, { use }); await refreshLibraries(); toast(use ? t('{name} reads .nfo files from other programs again, and is being scanned', { name: library.name }) : t("{name} now ignores other programs' .nfo files, and is being scanned", { name: library.name })) } catch (e: any) { toast(e.message) }
  }
  const on = nfoOn(library)
  return (
    <div className="set-lib-switch">
      <span><b>{t('Use .nfo files from other programs')}</b><br /><small className="muted">{tx(nfoSwitchText(on))}</small></span>
      <Switch checked={on} label={t('Use .nfo files from other programs in {name}', { name: library.name })} onChange={(v) => void set(v)} />
    </div>
  )
}

// Read-only: nothing the server does writes into the library's folders. Every kind of library
// with folders has it, games among them (#398).
function ReadOnlySwitch({ library }: { library: Library }) {
  const { refreshLibraries } = useApp()
  const set = async (readOnly: boolean) => {
    try { await api.put(`/libraries/${library.id}/read-only`, { readOnly }); await refreshLibraries(); toast(readOnly ? t('Nothing is written into the folders of {name} now', { name: library.name }) : t('{name} can have files and details saved in its folders again', { name: library.name })) } catch (e: any) { toast(e.message) }
  }
  const on = !!library.readOnly
  return (
    <div className="set-lib-switch">
      <span><b>{t('Read-only')}</b><br /><small className="muted">{on
        ? t('The server writes nothing into these folders: no uploads, no details or pictures saved beside the files, no conversions.')
        : t('The server may save files here: uploads, and details and pictures beside the files when that is turned on.')}</small></span>
      <Switch checked={on} label={t('Make {name} read-only', { name: library.name })} onChange={(v) => void set(v)} />
    </div>
  )
}

// Exported for WittyPlay's Settings, which shows the same list (its games libraries among them).
export function Libraries() {
  const { libraries, refreshLibraries } = useApp()
  const [editing, setEditing] = useState<Partial<Library> | null>(null)
  const [adding, setAdding] = useState(false)
  // Removing downloaded details: a library, or null for every library; undefined when closed.
  const [forgetting, setForgetting] = useState<Library | null | undefined>(undefined)
  const [grouping, setGrouping] = useState<Grouping>('combine')
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => setGrouping(s.libraryGrouping === 'separate' ? 'separate' : 'combine')).catch(() => {}) }, [])
  const changeGrouping = async (g: Grouping) => {
    setGrouping(g)
    try { await api.put('/settings', { libraryGrouping: g }); toast(g === 'combine' ? t('Folders of one kind now join one library') : t('New folders now become libraries of their own')) } catch (e: any) { toast(e.message) }
  }
  const remove = async (l: Library) => {
    if (!await ask({ title: t('Remove the library “{name}”?', { name: l.name }), body: t('Your files stay where they are; only the server forgets them.'), confirm: t('Remove library'), danger: true })) return
    await api.del(`/libraries/${l.id}`)
    await refreshLibraries()
    toast(t('Library removed'))
  }
  const purge = async (l: Library) => {
    if (!await ask({ title: t('Delete the metadata files in “{name}”?', { name: l.name }), body: t("Every .nfo and .metadata.json file in its folders is deleted, whoever wrote it, and the library is scanned again from the file names and what is found online. Your media, pictures and subtitles stay. This can't be undone."), confirm: t('Delete metadata files'), danger: true })) return
    try { const r = await api.post<{ deleted: number }>(`/libraries/${l.id}/purge-metadata`); toast(tn('{count} file deleted. Scanning {name}', '{count} files deleted. Scanning {name}', r.deleted, { name: l.name })); setTimeout(refreshLibraries, 1000) } catch (e: any) { toast(e.message) }
  }
  const add = () => setAdding(true)
  return (
    <>
      <PageHead title={tk('Libraries')} desc={t('Each library is one or more folders of one kind of media. The server looks through them for changes every 6 hours. Scan for changes does it straight away, and Full rescan reads every file and its details again from scratch.')}
        actions={libraries.length > 0 && <button className="btn primary" onClick={add}><IconPlus /> {t('Add library')}</button>} />
      {libraries.length > 0 && (
        <div className="set-lib-summary">
          <b>{tn('{count} library', '{count} libraries', libraries.length)}</b>
        </div>
      )}
      <Section title={tk('Folders of the same kind')} desc={t('When you add a folder of a kind you already have a library for, it can join that library, or become a new library with its own name (for example “Kids’ movies” beside “Movies”). You can always choose differently when adding.')}>
        <GroupingChoice value={grouping} onChange={changeGrouping} />
      </Section>
      {libraries.length === 0 ? (
        <div className="set-card set-empty big">
          <IconLibrary size={34} />
          <b>{t('No libraries yet')}</b>
          <p>{t('Add a folder of movies, shows, music or books to get started.')}</p>
          <button className="btn primary" onClick={add}><IconPlus /> {t('Add your first library')}</button>
        </div>
      ) : (
        <div className="set-libs">
          {libraries.map((l) => (
            <article key={l.id} className="set-lib">
              <div className="set-lib-top">
                <span className={'set-lib-icon t-' + l.type}>{libIcon(l.type)}</span>
                <div className="set-lib-title">
                  <b>{l.name}</b>
                  <span>{l.name === typeLabel(l.type) || l.name === tx(typeLabel(l.type)) ? '' : `${tx(typeLabel(l.type))} · `}{tn('{count} item', '{count} items', l.itemCount)}</span>
                </div>
                {l.scanning && <LibraryScanPill library={l} />}
              </div>
              <ul className="set-lib-paths">{l.paths.map((p) => <li key={p} title={p}><IconFolder size={14} /><span><bdi>{p}</bdi></span></li>)}</ul>
              {l.type === 'commercials' && <CommercialsSwitch library={l} />}
              {nfoSwitchShown(l.type) && <NfoSwitch library={l} />}
              {l.type !== 'radio' && l.paths.length > 0 && <ReadOnlySwitch library={l} />}
              <div className="set-lib-actions">
                <ScanControl lib={l} scanning={l.scanning} progress={l.scanning ? { mode: l.scanMode } : null} size="sm" variant="default" onStarted={() => setTimeout(refreshLibraries, 1000)} />
                <button className="btn sm ghost" onClick={() => setEditing(l)}><IconEdit /> {t('Edit')}</button>
                <button className="btn sm ghost" onClick={() => setForgetting(l)} disabled={l.scanning} title={t('Removes what was downloaded for this library, to try matching and downloading again')}>{t('Remove downloaded details…')}</button>
                {l.type !== 'radio' && l.type !== 'podcasts' && <button className="btn sm ghost" onClick={() => void purge(l)} disabled={l.scanning || l.readOnly} title={l.readOnly ? t('This library is read-only') : t('Deletes every .nfo and .metadata.json file in its folders')}>{t('Delete metadata files…')}</button>}
                <span className="grow" />
                <button className="btn sm ghost square danger-hover" onClick={() => remove(l)} aria-label={t('Remove {name}', { name: l.name })} title={t('Remove library')}><IconTrash /></button>
              </div>
            </article>
          ))}
        </div>
      )}
      {libraries.length > 0 && (
        <div className="set-lib-tools">
          <button className="btn sm" onClick={() => setForgetting(null)} disabled={libraries.some((l) => l.scanning)}>{t('Remove downloaded details from every library…')}</button>
          <small>{t('To try matching and downloading again from a clean start. What people watched, their lists and your own edits stay.')}</small>
        </div>
      )}
      {forgetting !== undefined && (
        <ForgetMetadataDialog library={forgetting ?? undefined} libraries={libraries} onClose={() => setForgetting(undefined)} onDone={() => { void refreshLibraries() }} />
      )}
      {adding && (
        <Dialog title={t('Add a library')} size="wide" className="add-lib-modal" onClose={() => setAdding(false)}>
          <AddLibrary existing={libraries} grouping={grouping} onAdded={async (l) => { setAdding(false); await refreshLibraries(); toast(t('{name} saved; scanning', { name: l.name })) }} />
        </Dialog>
      )}
      {editing && <LibraryEditor lib={editing} onClose={() => setEditing(null)} onSaved={async () => { setEditing(null); await refreshLibraries(); toast(t('Library saved; scanning')) }} />}
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
    <Dialog title={<>{lib.id ? t('Edit {name}', { name: lib.name }) : t('Add a library')}</>} onClose={onClose} size="wide" className="set-modal" actions={<><button className="btn ghost" onClick={onClose}>{t('Cancel')}</button>
          <button className="btn primary" onClick={save} disabled={!name.trim() || (paths.length === 0 && type !== 'podcasts' && type !== 'radio' && type !== 'anime')}>{lib.id ? t('Save and scan') : t('Add library')}</button></>}>
        <div className="set-form">
          <fieldset className="set-types">
            <legend>{t("What's in it?")}</legend>
            {TYPES.map((ty) => {
              const gone = missingKind(kinds, ty.v)
              const locked = !!gone && lib.type !== ty.v
              return (
                <label key={ty.v} className={'set-type' + (type === ty.v ? ' on' : '') + (locked ? ' off' : '')}>
                  <input type="radio" name="libtype" checked={type === ty.v} disabled={locked} onChange={() => { setType(ty.v); if (!name || TYPES.some((x) => x.label === name || tx(x.label) === name)) setName(tx(ty.label)) }} />
                  <span className="set-type-icon">{libIcon(ty.v)}</span>
                  <b>{tx(kindTitle(kinds, ty.v, ty.label))}</b>
                  {locked
                    ? <small>{t('Needs {modules}.', { modules: orList(gone.modules.map((m) => m.name)) })} <Link to={`/settings/modules/store/${encodeURIComponent(gone.modules[0]?.id || '')}`}>{t('Open the module store')}</Link></small>
                    : <small>{tx(ty.hint)}</small>}
                </label>
              )
            })}
          </fieldset>
          {type === 'games' && <ConsolePicker value={console_} onChange={(id) => setConsole(id)} />}
          <label>{t('Name')}<input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder={t('Movies')} /></label>
          <div className="set-field">
            <span className="set-field-label">{t('Folders')}</span>
            {paths.length > 0 && (
              <ul className="set-paths">
                {paths.map((p) => <li key={p}><IconFolder size={16} /><span title={p}>{p}</span><button className="btn sm ghost square" onClick={() => setPaths(paths.filter((x) => x !== p))} aria-label={t('Remove {name}', { name: p })}><IconClose /></button></li>)}
              </ul>
            )}
            <div className="set-addpath">
              <ChooseFolderButton className="btn" onPick={add} />
              <input className="input mono" placeholder={t('…or type a folder path and press Enter')} aria-label={t('Folder path')} value={manual} onChange={(e) => setManual(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') add(manual) }} />
              {manual && <button className="btn sm" onClick={() => add(manual)}>{t('Add')}</button>}
            </div>
            {type === 'podcasts' && <small className="muted">{t('A folder is only needed for episodes you download.')}</small>}
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
      <PageHead title={tk('Profiles')} desc={t('Everyone who uses this server, with their passwords and what they may watch.')} />
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
  if (!s) return st.failed ? <LoadError what={t('This page')} onRetry={st.retry} /> : null
  const profiles = (s.loginStyle || 'profiles') === 'profiles'
  return (
    <Section title={tk('Signing in')}>
      <Row label={t('Sign-in screen')} desc={t('Profiles show everyone\'s picture to pick from, like a streaming service. The classic form asks for a name and password.')}>
        <Segmented label={t('Sign-in screen')} value={profiles ? 'profiles' : 'classic'} onChange={(v) => set('loginStyle', v)} options={[{ value: 'profiles', label: t('Profiles') }, { value: 'classic', label: t('Classic') }]} />
      </Row>
      <Row label={t('Show profiles outside your home')} desc={t('Off by default: anyone who can reach the server would see everyone\'s names. Passwords are always required.')} dim={!profiles}>
        <Switch label={t('Show profiles outside your home')} disabled={!profiles} checked={on('profilesRemote', '0')} onChange={(v) => set('profilesRemote', v ? '1' : '0')} />
      </Row>
      <Row label={t('Only sign devices in with a PIN')} desc={t('A TV or app then has to show a PIN that changes every 30 seconds, typed where you approve it. Off by default, because older apps can only show a code; with this on they are told to update.')}>
        <Switch label={t('Only sign devices in with a PIN')} checked={on('pairRequirePin', '0')} onChange={(v) => set('pairRequirePin', v ? '1' : '0')} />
      </Row>
      <Row label={t('Let apps find this server')} desc={t('Apps on your home network can see this server\'s name, version and a random ID. They still need a code or a password to connect, and can always connect by typing its address.')}>
        <Switch label={t('Let apps find this server')} checked={on('lanDiscovery', '1')} onChange={(v) => set('lanDiscovery', v ? '1' : '0')} />
      </Row>
      <Row label={t('Who may manage Kids profiles')}>
        <select className="select" value={s.kidsManagedBy || 'adults'} onChange={(e) => set('kidsManagedBy', e.target.value)}>
          <option value="adults">{t('Every adult')}</option>
          <option value="admin">{t('Only the administrator')}</option>
        </select>
      </Row>
      <SaveBar {...st} />
    </Section>
  )
}

// ---------------------------------------------------------------- about

type Engine = { name: string; module: string; version: string; local?: boolean; role: string }

function About() {
  const [sys, setSys] = useState<any>(null)
  const [about, setAbout] = useState<{ version: string; modules: Engine[] } | null>(null)
  const { user, version } = useApp()
  const loc = useLocation()
  useEffect(() => {
    if (!user?.isAdmin) return
    let timer = 0
    const ask = () => api.get<any>('/system').then((s) => { setSys(s); if (!s.hwTested) timer = window.setTimeout(ask, 2000) }).catch(() => {})
    ask()
    return () => clearTimeout(timer)
  }, [user])
  useEffect(() => { api.get<{ version: string; modules: Engine[] }>('/about').then(setAbout).catch(() => {}) }, [])
  useEffect(() => { if (loc.hash && about) document.getElementById(loc.hash.slice(1))?.scrollIntoView({ block: 'start' }) }, [loc.hash, about])
  const hw = (sys?.hardware || []).filter((h: any) => h.working).map((h: any) => h.label)
  return (
    <>
      <PageHead title={tk('About')} desc={t("This server's version, the modules installed on it, its system, its license, every third-party license, and the services it relies on.")} />
      <Section title={tk('Version and modules')} desc={t('What the server can do comes from separate modules, each published on its own. These are the versions on this server.')}>
        <div id="modules" className="mod-list">
          <div className="mod-row mod-self">
            <span className="mod-name"><b>WittyMediaServer</b><span className="muted">{t('The server and this app')}</span></span>
            <span className="mod-ver">{about?.version || version || 'dev'}</span>
          </div>
          <div className="mod-group">{t('Disc engines')}</div>
          {(about?.modules || []).map((m) => (
            <a key={m.name} className="mod-row" href={`https://github.com/wittywilla/${m.name}`} target="_blank" rel="noreferrer">
              <span className="mod-name"><b>{m.name}</b><span className="muted">{tx(m.role)}</span></span>
              <span className="mod-ver">{m.version}{m.local && <span className="mod-local" title={t("Built from this server's own copy of the module")}>{t('built in')}</span>}</span>
            </a>
          ))}
        </div>
      </Section>
      {user?.isAdmin && <Updates />}
      {sys && (
        <Section title={tk('This server')}>
          <dl className="set-facts">
            <div className="wide"><dt>{t('Open it on other devices at home')}</dt><dd className="mono">{(sys.lanUrls || []).length ? sys.lanUrls.join('  ·  ') : t('This server only accepts connections from this computer')}</dd></div>
            <div><dt>{t('System')}</dt><dd>{sys.os} / {sys.arch}</dd></div>
            <div><dt>{t('Computer')}</dt><dd>{sys.hostname}</dd></div>
            <div><dt>{t('Hardware video')}</dt><dd>{hw.length ? hw.join(', ') : sys?.hwTested ? t('None found; software encoding') : t('Checking…')}</dd></div>
            <div><dt>{t('Open discs')}</dt><dd>{t('{dvd} DVD · {bluray} Blu-ray', { dvd: sys.dvdSessions, bluray: sys.blurays ?? 0 })}</dd></div>
            <div><dt>{t('Built with')}</dt><dd>{sys.go}</dd></div>
            <div className="wide"><dt>{t('Data folder')}</dt><dd className="mono">{sys.dataDir}</dd></div>
            <div className="wide"><dt>FFmpeg</dt><dd className="mono">{sys.ffmpegInfo}</dd></div>
            {sys.smartAppControl && <div className="wide"><dt>{t('Windows Smart App Control')}</dt><dd>{tx(sys.smartAppControl)}. <Link to="/help/troubleshooting#smart-app-control-on-windows">{t('What this means')}</Link></dd></div>}
          </dl>
        </Section>
      )}
      <Section title={tk('License')}>
        <div className="set-prose">
          <p><b>WittyMediaServer</b> {t('is open-source software under the')} <b>GNU Affero General Public License v3.0 (or later)</b>. {t('You may use, change and share it under that license; please keep the')} <code>NOTICE</code> {t('attribution (“This product includes WittyMediaServer, developed by WittyWilla”).')} {t('Source:')} <a href="https://github.com/wittywilla/WittyMediaServer" target="_blank" rel="noreferrer">github.com/wittywilla/WittyMediaServer</a>.</p>
        </div>
      </Section>
      <Section title={tk('Legal')}>
        <div className="set-prose">
          <p>{t('Terms of use, who’s responsible for what, trademarks, copy protection, privacy and more are on the')} <Link to="/settings/legal">{t('Legal')}</Link> {t('page.')}</p>
        </div>
      </Section>
      <div id="formats" />
      <Section title={tk('Supported formats')} desc={t('Every kind of file this server takes, how each plays, and what it needs.')}>
        <SupportedFormats />
      </Section>
      <div id="licenses" />
      <Section title={tk('Third-party licenses')} desc={t('Everything in this server that someone else made, and what it’s under.')}>
        <LicenseList />
      </Section>
      <Section title={tk('Third-party components and services')}>
        <div className="set-prose">
          <p>{t('Video is converted by {ffmpeg}, run as a separate program; the bundled builds are GPL-licensed, with sources linked in {file}.', { ffmpeg: 'FFmpeg', file: 'THIRD_PARTY_LICENSES.md' })} {t('With your own key, information comes from {tmdb}: this product uses the TMDB API but is not endorsed or certified by TMDB.', { tmdb: 'The Movie Database (TMDB)' })} {t('Without keys, TV information comes from {tvmaze} (CC BY-SA 4.0), movie facts from {wikidata} (CC0) and plot summaries from {wikipedia} (CC BY-SA 4.0).', { tvmaze: 'TVmaze', wikidata: 'Wikidata', wikipedia: 'Wikipedia' })} {t("Disc menus are run by WittyMediaServer's own WittyDVD and WittyBluray engines.")}</p>
          <p><b>{t('No warranty.')}</b> {t('The software is provided “as is”, without warranty of any kind. WittyMediaServer is not affiliated with any other media software project, studio or rights holder; all trademarks belong to their owners.')}</p>
        </div>
      </Section>
    </>
  )
}

// LibraryScanPill: how far a library's scan has got, "Scanning: 12,400 of 16,780 files", asked for while it runs.
function LibraryScanPill({ library }: { library: Library }) {
  const progress = useScanProgress(library.id, library.scanning)
  return <span className="set-pill live scan-progress" role="status">{scanProgressText(progress ?? { mode: library.scanMode })}</span>
}
