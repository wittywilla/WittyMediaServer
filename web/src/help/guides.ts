// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The guides, built into the web app from docs/ so they work with no network and no module.
// Each comes as its own small script, fetched from this server only when it's opened or searched.
// Every docs/*.md is listed in GROUPS or in notInHelp: internal/docscheck holds us to that.

import { settingsHelp } from './settingsHelp'
import { tk } from '../i18n'

const files = import.meta.glob([
  '../../../docs/*.md',
  '../../../CHANGELOG.md',
  '../../../SECURITY.md',
  '!../../../docs/BRAND.md',
  '!../../../docs/FFMPEG-BENCHMARK.md',
  '!../../../docs/LEGAL-NOTICES-CHECKLIST.md',
  '!../../../docs/MACOS.md',
  '!../../../docs/PERFORMANCE.md',
  '!../../../docs/PUBLISHING.md',
  '!../../../docs/REPOSITORIES.md',
  '!../../../docs/ROADMAP.md',
  '!../../../docs/WITTYRETROARCH.md',
], { query: '?raw', import: 'default' }) as Record<string, () => Promise<string>>

// For the people working on the project, and no use to anyone running it.
export const notInHelp = ['BRAND', 'FFMPEG-BENCHMARK', 'LEGAL-NOTICES-CHECKLIST', 'MACOS', 'PERFORMANCE', 'PUBLISHING', 'REPOSITORIES', 'ROADMAP', 'WITTYRETROARCH']

export type Guide = { file: string; title: string; about: string }
// Titles and descriptions are marked with tk(): shown with tx(). The guides themselves are in English.
export type GuideGroup = { title: string; quiet?: boolean; guides: Guide[] }

export const GROUPS: GuideGroup[] = [
  { title: tk('Start here'), guides: [
    { file: 'GETTING_STARTED', title: tk('Getting started'), about: tk('From installing to your first film playing.') },
    { file: 'FIRST-TIME-SETUP', title: tk('First-time setup, step by step'), about: tk('Every page of setup, with a picture of each.') },
    { file: 'LIBRARIES', title: tk('Libraries'), about: tk('Adding your folders, and how to name what’s in them.') },
    { file: 'ACCOUNTS', title: tk('Accounts and profiles'), about: tk('People, kids and guests, passwords and two-factor sign-in.') },
    { file: 'PROFILES', title: tk('Profiles and parental controls'), about: tk('Switching on a shared screen, limits by age and kind, screen time and reports.') },
    { file: 'APPS', title: tk('Apps and devices'), about: tk('Every way to reach your server, and what works where.') },
    { file: 'PLAYING', title: tk('Watching and listening'), about: tk('The player, its shortcuts, subtitles and TV remotes.') },
    { file: 'SYNCPLAY', title: tk('Watch parties'), about: tk('Watching together, in step, from different places.') },
    { file: 'SETTINGS', title: tk('Settings tour'), about: tk('Where to change what, page by page.') },
    { file: 'PAGES', title: tk('Pages and the menu'), about: tk('Customise a page for yourself, and change Home, the libraries and the menu for everyone.') },
    { file: 'TROUBLESHOOTING', title: tk('Troubleshooting'), about: tk('Something’s wrong: find it by what you’re seeing.') },
  ] },
  { title: tk('Your media'), guides: [
    { file: 'MEDIA-TYPES', title: tk('What each media type supports'), about: tk('Films to games: what works, and what needs a module.') },
    { file: 'SUPPORTED-FORMATS', title: tk('Supported formats'), about: tk('Every file type, and how it plays.') },
    { file: 'TV', title: tk('TV shows'), about: tk('Seasons, episodes, specials and naming.') },
    { file: 'CHANNELS', title: tk('Channels and live TV'), about: tk('WittyTV: your own channels, the guide, bumpers, tuners and recordings.') },
    { file: 'MUSIC', title: tk('Music, audiobooks and podcasts'), about: tk('Listening, queues, full screen and the visualizer.') },
    { file: 'LYRICS-KARAOKE', title: tk('Lyrics and karaoke'), about: tk('Lyrics that follow the song, where they come from, and singing along.') },
    { file: 'PHOTOS', title: tk('Photos'), about: tk('Albums, the map, sharing and live photos.') },
    { file: 'BOOKS', title: tk('Books, PDFs and comics'), about: tk('The reader, PDFs, comics and your place in each.') },
    { file: 'GAMES', title: tk('Games'), about: tk('WittyPlay: adding games, saves, controllers and delay.') },
    { file: 'GAME-RIGS', title: tk('Game PCs'), about: tk('Playing on a PC at home with WittyCouch and WittyRigs.') },
    { file: 'CREATOR-VIDEOS', title: tk('Creator videos'), about: tk('WittyChannels: videos grouped by who made them.') },
    { file: 'SHORTS', title: tk('Vertical videos'), about: tk('WittyShorts: a feed of your own short videos.') },
    { file: 'COLLECTIONS', title: tk('Collections, radio and Your year'), about: tk('Hand-picked and automatic lists, internet radio and your year in review.') },
    { file: 'DVD', title: tk('DVDs'), about: tk('Playing DVDs with their menus.') },
    { file: 'BLURAY', title: tk('Blu-rays'), about: tk('Playing Blu-rays, and what Java menus need.') },
  ] },
  { title: tk('Titles, posters and details'), guides: [
    { file: 'METADATA', title: tk('Where details come from'), about: tk('Sources, keys, NFO files, artwork, categories and logos.') },
    { file: 'FIXING-METADATA', title: tk('Fixing wrong details'), about: tk('Correcting a match, a title or a poster.') },
    { file: 'METADATA-FOLDER', title: tk('The metadata folder'), about: tk('What’s in it, and what’s safe to change by hand.') },
  ] },
  { title: tk('Converting video'), guides: [
    { file: 'TRANSCODING', title: tk('Live transcoding'), about: tk('Converting while someone watches: quality and hardware.') },
    { file: 'LIBRARY-TRANSCODING', title: tk('Library transcoding'), about: tk('Converting your files ahead of time, with what can go wrong.') },
    { file: 'TDARR', title: tk('Using Tdarr'), about: tk('Connecting your own Tdarr server, and what it can and can’t do here.') },
  ] },
  { title: tk('Looking after the server'), guides: [
    { file: 'INSTALL', title: tk('Installing and updating'), about: tk('Each system’s installer, and what each program is.') },
    { file: 'HARDWARE', title: tk('What hardware it needs'), about: tk('Memory and processor for each thing it does, from measurements.') },
    { file: 'DOCKER', title: tk('Docker'), about: tk('The image, its tags and folders, graphics cards, hardening and updating.') },
    { file: 'DOCKER-UGREEN', title: tk('Docker on a UGREEN NAS'), about: tk('UGOS Pro’s Docker app, with Intel graphics.') },
    { file: 'DOCKER-SYNOLOGY', title: tk('Docker on a Synology NAS'), about: tk('Container Manager on DSM 7.2.') },
    { file: 'DOCKER-QNAP', title: tk('Docker on a QNAP NAS'), about: tk('Container Station 3.') },
    { file: 'DOCKER-UNRAID', title: tk('Docker on Unraid'), about: tk('A template, or Compose Manager.') },
    { file: 'DOCKER-TRUENAS', title: tk('Docker on TrueNAS SCALE'), about: tk('A custom app from YAML, 24.10 and later.') },
    { file: 'DOCKER-ASUSTOR', title: tk('Docker on an Asustor NAS'), about: tk('Portainer on ADM.') },
    { file: 'DOCKER-LINUX', title: tk('Docker on a Linux machine'), about: tk('Docker Engine and Compose, step by step.') },
    { file: 'UNINSTALL', title: tk('Uninstalling'), about: tk('Removing everything, or keeping your data, on each system.') },
    { file: 'BACKUP', title: tk('Backup and restore'), about: tk('A ZIP of your data, and bringing it back.') },
    { file: 'REPAIR', title: tk('Repair'), about: tk('Finding and fixing problems, in the app or on the command line.') },
    { file: 'REMOTE', title: tk('Remote access'), about: tk('Reaching your server from away, safely.') },
    { file: 'TASKS', title: tk('Background tasks'), about: tk('The activity button: scans, conversions and what’s running.') },
    { file: 'STATS', title: tk('Server statistics'), about: tk('WittyStats: what your server is doing, on a screen you arrange.') },
    { file: 'LOGGING', title: tk('Logs'), about: tk('Reading logs, and making a sanitized ZIP to share.') },
    { file: 'DIAGNOSER', title: tk('WittyDiagnoser'), about: tk('Checking your disks and drives.') },
    { file: 'DATA-FOLDER', title: tk('The data folder'), about: tk('Where settings, the database and artwork are kept, what each folder is, and moving it.') },
    { file: 'CONFIG', title: tk('Config file and folders'), about: tk('Every setting in wms.json, portable mode and where things are kept.') },
    { file: 'REFERENCE', title: tk('Command line reference'), about: tk('Commands, flags and how the parts fit together.') },
  ] },
  { title: tk('Modules and themes'), guides: [
    { file: 'MODULE-STORE', title: tk('Modules and the store'), about: tk('Adding and updating modules, other stores and what limits a module.') },
    { file: 'THEMES', title: tk('Themes'), about: tk('Choosing a look, or making your own in a few lines.') },
  ] },
  { title: tk('About this release'), guides: [
    { file: 'CHANGELOG', title: tk('What’s new'), about: tk('Every change, release by release.') },
    { file: 'SECURITY-REVIEW', title: tk('Security model'), about: tk('How this server is meant to hold up, and how that’s checked.') },
    { file: 'SECURITY', title: tk('Reporting a security problem'), about: tk('What to do if you find one.') },
  ] },
  { title: tk('For developers'), quiet: true, guides: [
    { file: 'MODULES', title: tk('Making a module'), about: tk('Manifests, helpers and publishing.') },
    { file: 'CLIENT-API', title: tk('Client API'), about: tk('Every route, for anyone writing an app.') },
    { file: 'REMOTE-PROTOCOL', title: tk('Remote Protocol'), about: tk('One app playing on and steering another: every message.') },
    { file: 'P2P', title: tk('Peer-to-peer connections'), about: tk('Reaching a server with nothing set up on the router: the design, the protocol and the threat model.') },
    { file: 'SDUI', title: tk('Server-driven screens'), about: tk('Screens the server describes and every app draws, and how to change them.') },
    { file: 'SDUI-MODULES', title: tk('A module\'s screens'), about: tk('What a module adds to every app as data: its pages, rows, menu entries, settings and colours.') },
    { file: 'MODULES-API5', title: tk('A module that owns its feature'), about: tk('Module API 5: a module\'s own routes, screens, data and setup, merged into the server so it updates from the store.') },
    { file: 'TRANSLATION', title: tk('Translation'), about: tk('The app in the server’s language, and how to add words, catalogues and a module’s own.') },
    { file: 'SECURITY-ROUTES', title: tk('Who may call what'), about: tk('Every route and the role it needs.') },
    { file: 'GAMING-API', title: tk('Games API'), about: tk('The game routes, for apps.') },
    { file: 'GAMING-STREAM', title: tk('Games: the stream'), about: tk('One stream for every device: why, and its wire protocol.') },
    { file: 'GAMING', title: tk('Games: how it’s built'), about: tk('The design of the game engine.') },
    { file: 'DVD-QUIRKS', title: tk('DVD disc quirks'), about: tk('What odd discs do, and how each is handled.') },
    { file: 'BLURAY-QUIRKS', title: tk('Blu-ray disc quirks'), about: tk('The same for Blu-rays.') },
    { file: 'DESIGN', title: tk('Design standards'), about: tk('The interface’s rules and shared parts.') },
    { file: 'UI-PLATFORMS', title: tk('One fix, every screen'), about: tk('How a change to the interface reaches every platform’s app.') },
  ] },
]

export const allGuides = GROUPS.flatMap((g) => g.guides)

export const slugOf = (file: string) => file.toLowerCase().replace(/_/g, '-')
export const guideBySlug = (slug: string) => allGuides.find((g) => slugOf(g.file) === slug)
export const guideByFile = (file: string) => allGuides.find((g) => g.file.toLowerCase() === file.toLowerCase())
export const isQuiet = (g: Guide) => !!GROUPS.find((x) => x.guides.includes(g))?.quiet

export function loadGuide(g: Guide): Promise<string> {
  const key = Object.keys(files).find((k) => k.endsWith(`/${g.file}.md`))
  return key ? files[key]() : Promise.reject(new Error('This guide isn’t in this build.'))
}

export { settingsHelp }

// "Settings → Repair" in a guide can be a link to that page, for someone signed in.
export const settingsPages: Record<string, string> = {
  Libraries: 'libraries',
  'Pages & menu': 'pages',
  'Module settings': 'module-settings',
  'What this server plays': 'games-support',
  'Metadata agents': 'metadata',
  Playback: 'playback',
  'Video quality': 'quality',
  'DVDs and Blu-rays': 'discs',
  'Channels & tuners': 'channels',
  'Games & emulators': 'gaming',
  'Consoles & cores': 'gaming',
  'Standalone emulators': 'emulators',
  'Game PCs': 'couch',
  'Live transcoding': 'transcoding',
  'Library transcoding': 'library-transcoding',
  Network: 'network',
  'Data folder': 'data-folder',
  'Remote Access': 'remote',
  Profiles: 'users',
  'Sign-in & security': 'security',
  'My game settings': 'games',
  Activity: 'activity',
  Appearance: 'appearance',
  'Sharing & photos': 'sharing',
  WittyRigs: 'wittyrigs',
  WittyLog: 'logs',
  WittyDiagnoser: 'diagnoser',
  Modules: 'modules',
  Backup: 'backup',
  Repair: 'repair',
  Reset: 'reset',
  About: 'about',
  Legal: 'legal',
}
