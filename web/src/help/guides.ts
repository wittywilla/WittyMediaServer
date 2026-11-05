// The guides, built into the web app from docs/ so they work with no network and no module.
// Each comes as its own small script, fetched from this server only when it's opened or searched.
// Every docs/*.md is listed in GROUPS or in notInHelp: internal/docscheck holds us to that.

import { settingsHelp } from './settingsHelp'

const files = import.meta.glob([
  '../../../docs/*.md',
  '../../../CHANGELOG.md',
  '../../../SECURITY.md',
  '!../../../docs/FFMPEG-BENCHMARK.md',
  '!../../../docs/LEGAL-NOTICES-CHECKLIST.md',
  '!../../../docs/MACOS.md',
  '!../../../docs/MODULES-CUTOVER.md',
  '!../../../docs/PUBLISHING.md',
  '!../../../docs/REPOSITORIES.md',
  '!../../../docs/ROADMAP.md',
  '!../../../docs/WITTYRETROARCH.md',
], { query: '?raw', import: 'default' }) as Record<string, () => Promise<string>>

// For the people working on the project, and no use to anyone running it.
export const notInHelp = ['FFMPEG-BENCHMARK', 'LEGAL-NOTICES-CHECKLIST', 'MACOS', 'MODULES-CUTOVER', 'PUBLISHING', 'REPOSITORIES', 'ROADMAP', 'WITTYRETROARCH']

export type Guide = { file: string; title: string; about: string }
export type GuideGroup = { title: string; quiet?: boolean; guides: Guide[] }

export const GROUPS: GuideGroup[] = [
  { title: 'Start here', guides: [
    { file: 'GETTING_STARTED', title: 'Getting started', about: 'From installing to your first film playing.' },
    { file: 'FIRST-TIME-SETUP', title: 'First-time setup, step by step', about: 'Every page of setup, with a picture of each.' },
    { file: 'LIBRARIES', title: 'Libraries', about: 'Adding your folders, and how to name what’s in them.' },
    { file: 'ACCOUNTS', title: 'Accounts and profiles', about: 'People, kids and guests, passwords and two-factor sign-in.' },
    { file: 'APPS', title: 'Apps and devices', about: 'Every way to reach your server, and what works where.' },
    { file: 'PLAYING', title: 'Watching and listening', about: 'The player, its shortcuts, subtitles and TV remotes.' },
    { file: 'SETTINGS', title: 'Settings tour', about: 'Where to change what, page by page.' },
    { file: 'TROUBLESHOOTING', title: 'Troubleshooting', about: 'Something’s wrong: find it by what you’re seeing.' },
  ] },
  { title: 'Your media', guides: [
    { file: 'MEDIA-TYPES', title: 'What each media type supports', about: 'Films to games: what works, and what needs a module.' },
    { file: 'SUPPORTED-FORMATS', title: 'Supported formats', about: 'Every file type, and how it plays.' },
    { file: 'TV', title: 'TV shows', about: 'Seasons, episodes, specials and naming.' },
    { file: 'CHANNELS', title: 'Channels and live TV', about: 'WittyTV: your own channels, the guide, bumpers, tuners and recordings.' },
    { file: 'MUSIC', title: 'Music, audiobooks and podcasts', about: 'Listening, queues, full screen and the visualizer.' },
    { file: 'LYRICS-KARAOKE', title: 'Lyrics and karaoke', about: 'Lyrics that follow the song, where they come from, and singing along.' },
    { file: 'PHOTOS', title: 'Photos', about: 'Albums, the map, sharing and live photos.' },
    { file: 'BOOKS', title: 'Books, PDFs and comics', about: 'The reader, PDFs, comics and your place in each.' },
    { file: 'GAMES', title: 'Games', about: 'WittyPlay: adding games, saves, controllers and delay.' },
    { file: 'GAME-RIGS', title: 'Game PCs', about: 'Playing on a PC at home with WittyCouch and WittyRigs.' },
    { file: 'CREATOR-VIDEOS', title: 'Creator videos', about: 'WittyChannels: videos grouped by who made them.' },
    { file: 'SHORTS', title: 'Vertical videos', about: 'WittyShorts: a feed of your own short videos.' },
    { file: 'COLLECTIONS', title: 'Collections, radio and Your year', about: 'Hand-picked and automatic lists, internet radio and your year in review.' },
    { file: 'DVD', title: 'DVDs', about: 'Playing DVDs with their menus.' },
    { file: 'BLURAY', title: 'Blu-rays', about: 'Playing Blu-rays, and what Java menus need.' },
  ] },
  { title: 'Titles, posters and details', guides: [
    { file: 'METADATA', title: 'Where details come from', about: 'Sources, keys, NFO files, artwork, categories and logos.' },
    { file: 'FIXING-METADATA', title: 'Fixing wrong details', about: 'Correcting a match, a title or a poster.' },
    { file: 'METADATA-FOLDER', title: 'The metadata folder', about: 'What’s in it, and what’s safe to change by hand.' },
  ] },
  { title: 'Converting video', guides: [
    { file: 'TRANSCODING', title: 'Live transcoding', about: 'Converting while someone watches: quality and hardware.' },
    { file: 'LIBRARY-TRANSCODING', title: 'Library transcoding', about: 'Converting your files ahead of time, with what can go wrong.' },
    { file: 'TDARR', title: 'Using Tdarr', about: 'Connecting your own Tdarr server, and what it can and can’t do here.' },
  ] },
  { title: 'Looking after the server', guides: [
    { file: 'INSTALL', title: 'Installing and updating', about: 'Each system’s installer, and what each program is.' },
    { file: 'DOCKER', title: 'Docker', about: 'The image, its tags and folders, graphics cards, hardening and updating.' },
    { file: 'DOCKER-UGREEN', title: 'Docker on a UGREEN NAS', about: 'UGOS Pro’s Docker app, with Intel graphics.' },
    { file: 'DOCKER-SYNOLOGY', title: 'Docker on a Synology NAS', about: 'Container Manager on DSM 7.2.' },
    { file: 'DOCKER-QNAP', title: 'Docker on a QNAP NAS', about: 'Container Station 3.' },
    { file: 'DOCKER-UNRAID', title: 'Docker on Unraid', about: 'A template, or Compose Manager.' },
    { file: 'DOCKER-TRUENAS', title: 'Docker on TrueNAS SCALE', about: 'A custom app from YAML, 24.10 and later.' },
    { file: 'DOCKER-ASUSTOR', title: 'Docker on an Asustor NAS', about: 'Portainer on ADM.' },
    { file: 'DOCKER-LINUX', title: 'Docker on a Linux machine', about: 'Docker Engine and Compose, step by step.' },
    { file: 'UNINSTALL', title: 'Uninstalling', about: 'Removing everything, or keeping your data, on each system.' },
    { file: 'BACKUP', title: 'Backup and restore', about: 'A ZIP of your data, and bringing it back.' },
    { file: 'REPAIR', title: 'Repair', about: 'Finding and fixing problems, in the app or on the command line.' },
    { file: 'REMOTE', title: 'Remote access', about: 'Reaching your server from away, safely.' },
    { file: 'TASKS', title: 'Background tasks', about: 'The activity button: scans, conversions and what’s running.' },
    { file: 'STATS', title: 'Server statistics', about: 'WittyStats: what your server is doing, on a screen you arrange.' },
    { file: 'LOGGING', title: 'Logs', about: 'Reading logs, and making a sanitized ZIP to share.' },
    { file: 'DIAGNOSER', title: 'WittyDiagnoser', about: 'Checking your disks and drives.' },
    { file: 'CONFIG', title: 'Config file and folders', about: 'Every setting in wms.json, portable mode and where things are kept.' },
    { file: 'REFERENCE', title: 'Command line reference', about: 'Commands, flags and how the parts fit together.' },
  ] },
  { title: 'Modules and themes', guides: [
    { file: 'MODULE-STORE', title: 'Modules and the store', about: 'Adding and updating modules, other stores and what limits a module.' },
    { file: 'THEMES', title: 'Themes', about: 'Choosing a look, or making your own in a few lines.' },
  ] },
  { title: 'About this release', guides: [
    { file: 'CHANGELOG', title: 'What’s new', about: 'Every change, release by release.' },
    { file: 'SECURITY-REVIEW', title: 'Security model', about: 'How this server is meant to hold up, and how that’s checked.' },
    { file: 'SECURITY', title: 'Reporting a security problem', about: 'What to do if you find one.' },
  ] },
  { title: 'For developers', quiet: true, guides: [
    { file: 'MODULES', title: 'Making a module', about: 'Manifests, helpers and publishing.' },
    { file: 'CLIENT-API', title: 'Client API', about: 'Every route, for anyone writing an app.' },
    { file: 'SECURITY-ROUTES', title: 'Who may call what', about: 'Every route and the role it needs.' },
    { file: 'GAMING-API', title: 'Games API', about: 'The game routes, for apps.' },
    { file: 'GAMING', title: 'Games: how it’s built', about: 'The design of the game engine.' },
    { file: 'DVD-QUIRKS', title: 'DVD disc quirks', about: 'What odd discs do, and how each is handled.' },
    { file: 'BLURAY-QUIRKS', title: 'Blu-ray disc quirks', about: 'The same for Blu-rays.' },
    { file: 'DESIGN', title: 'Design standards', about: 'The interface’s rules and shared parts.' },
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
  'Metadata agents': 'metadata',
  'Playback & discs': 'playback',
  'Channels & tuners': 'channels',
  'Consoles & cores': 'gaming',
  'Game PCs': 'couch',
  'Live transcoding': 'transcoding',
  'Library transcoding': 'library-transcoding',
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
