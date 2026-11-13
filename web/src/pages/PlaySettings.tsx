// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// WittyPlay Online's Settings: everything games-related, inside WittyPlay's own frame. Each page is the app's own (the same component
// as under Settings, which keeps them too), so nothing is built twice: consoles and cores, what this server plays, standalone
// emulators (installs, firmware, controllers, the manual notes), Game PCs, WittyRigs, my game settings (every game, a console or
// one game), controllers and their mapping, friends and privacy, and the WittyPlay module's own settings.
import { lazy, Suspense, type ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import { IconCheck, IconDevices, IconDisc, IconGamepad, IconImage, IconLibrary, IconPlug, IconRemote, IconServer, IconSidebar, IconTune, IconUsers, IconWrench, IconRight } from '../icons'
import { useApp } from '../state'
import { Spinner } from '../ui/Surface'
import { PageHead } from './settings/ui'
import { GameSettingsPage } from './games/consoles'
import GamingSettings from './settings/Gaming'
import { LookSetting } from '../shell/playPads'
import './settings/settings.css'

const EmulatorsSettings = lazy(() => import('./settings/Emulators'))
const GamesSupport = lazy(() => import('./settings/GamesSupport'))
const CouchSettings = lazy(() => import('./settings/Couch'))
const WittyRigsSettings = lazy(() => import('./settings/WittyRigs'))
const ModuleSettingsPage = lazy(() => import('./settings/ModuleSettings').then((m) => ({ default: m.ModuleSettingsPage })))
const LibrariesSettings = lazy(() => import('./SettingsPage').then((m) => ({ default: m.Libraries })))
const PlayPictures = lazy(() => import('./PlayPictures'))
const ControllersScreen = lazy(() => import('./Games').then((m) => ({ default: m.ControllersScreen })))

export const PLAY_SETTINGS = '/wittyplay/settings'

type Entry = { id: string; label: string; hint: string; icon: (p: { size?: number }) => ReactNode; admin?: boolean; to?: string; page?: () => ReactNode }
const ENTRIES: { title: string; entries: Entry[] }[] = [
  { title: 'For you', entries: [
    { id: 'games', icon: IconTune, label: 'My game settings', hint: 'Picture, speed, shortcuts and more: for every game, one console or one game', page: () => <><PageHead title="My game settings" desc="How your games look, sound and behave. Set something for every game, for one console or, from a game’s own page, for one game. It follows you to every device." /><GameSettingsPage /></> },
    { id: 'look', icon: IconSidebar, label: 'Layout, tabs and buttons', hint: 'Shelf, grid or list; how game titles are written; the order of the tabs and which show; the buttons WittyPlay shows', page: () => <LookSetting /> },
    { id: 'pictures', icon: IconImage, label: 'Game pictures', hint: 'Box art fitted or filled, or each game in its case or cartridge: for every game, a console or one game', page: () => <PlayPictures /> },
    { id: 'controllers', icon: IconGamepad, label: 'Controllers and buttons', hint: 'Who is on which player, and the buttons WittyPlay shows', to: '/page/wittyplay-controllers' },
    { id: 'mapping', icon: IconRemote, label: 'Controller mapping', hint: 'What each button and stick does, for each console', page: () => <ControllersScreen /> },
    { id: 'privacy', icon: IconUsers, label: 'Friends and what you play', hint: 'Who sees you online and what you are playing', to: '/page/wittyplay-privacy' },
    { id: 'module', icon: IconPlug, label: 'WittyPlay options', hint: 'What the WittyPlay module lets you set', page: () => <ModuleSettingsPage /> },
  ] },
  { title: 'This server', entries: [
    { id: 'consoles', icon: IconDisc, label: 'Consoles and cores', hint: 'Which consoles play here, each one’s core and its options, BIOS files, and game streaming', admin: true, page: () => <GamingSettings /> },
    { id: 'support', icon: IconCheck, label: 'What this server plays', hint: 'Each console, whether its games play here, and how', page: () => <GamesSupport /> },
    { id: 'libraries', icon: IconLibrary, label: 'Libraries', hint: 'The folders games come from, each library’s console and naming, and scans', admin: true, page: () => <LibrariesSettings /> },
    { id: 'emulators', icon: IconWrench, label: 'Standalone emulators', hint: 'PCSX2, Dolphin, RPCS3 and others: installs, firmware, controllers and every setting', admin: true, page: () => <EmulatorsSettings /> },
    { id: 'pcs', icon: IconDevices, label: 'Game PCs', hint: 'Who can play on PCs running Sunshine, and which ones', admin: true, page: () => <CouchSettings /> },
    { id: 'rigs', icon: IconServer, label: 'WittyRigs', hint: 'Hosting demanding games on this server’s own hardware', admin: true, page: () => <WittyRigsSettings /> },
  ] },
]

export const playSettingsEntries = (admin: boolean) => ENTRIES.map((g) => ({ ...g, entries: g.entries.filter((e) => admin || !e.admin) })).filter((g) => g.entries.length)

export default function PlaySettings() {
  const params = useParams()
  const section = params.module ? 'module' : params.section || ''
  const { user } = useApp()
  const groups = playSettingsEntries(!!user?.isAdmin)
  const entry = groups.flatMap((g) => g.entries).find((e) => e.id === section)
  if (section && entry?.page) {
    return (
      <div className="wplay-settings-page">
        <Link className="wplay-settings-back" to={PLAY_SETTINGS}>Settings</Link>
        <Suspense fallback={<Spinner />}>{entry.page()}</Suspense>
      </div>
    )
  }
  return (
    <div className="wplay-settings">
      <h1>Settings</h1>
      {groups.map((g) => (
        <section key={g.title} aria-labelledby={'wps-' + g.title}>
          <h2 id={'wps-' + g.title}>{g.title}</h2>
          <ul className="wplay-settings-list" data-nav-group>
            {g.entries.map((e) => (
              <li key={e.id}>
                <Link to={e.to || (e.id === 'module' ? `${PLAY_SETTINGS}/module/wittyplay` : `${PLAY_SETTINGS}/${e.id}`)} className="wplay-settings-link">
                  <span className="wplay-settings-icon" aria-hidden="true"><e.icon /></span><span><b>{e.label}</b><span>{e.hint}</span></span><IconRight aria-hidden="true" />
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  )
}
