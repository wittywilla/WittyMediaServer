// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which addresses are WittyPlay Online, which take over the whole window, and where it goes when put away (playPaths.test.mjs).

// Where WittyPlay goes when it is put away: the app's Home page.
export const PLAY_EXIT = '/'
// WittyPlay Online's top page: Back there puts WittyPlay away. Its other pages (controllers, privacy) step back to it.
export const PLAY_HOME = '/page/wittyplay-online'
// The design page that draws the same screen from a fixture, for administrators.
const PLAY_DESIGN = '/design/play'

// One of WittyPlay Online's pages (the module's own pages are named wittyplay-…).
export const isPlayPath = (path: string) => path === PLAY_HOME || path === '/page/wittyplay' || path.startsWith('/page/wittyplay-') || path === PLAY_DESIGN || path === PLAY_SETTINGS || path.startsWith(PLAY_SETTINGS + '/') || path.startsWith('/wittyplay/')

// WittyPlay's Settings: every games page of the app's Settings, in WittyPlay's frame (pages/PlaySettings.tsx).
export const PLAY_SETTINGS = '/wittyplay/settings'

// The status bar and dock when a page brings none (WittyPlay's Settings, the games library): the same as WittyPlay Online's own.
export const PLAY_STATUS = [{ id: 'me', kind: 'profile' as const, text: 'WittyPlay Online' }, { id: 'server', kind: 'server' as const }]
export const PLAY_DOCK = [
  { id: 'home', label: 'Home', icon: 'home', route: PLAY_HOME },
  { id: 'search', label: 'Search', icon: 'search', route: '/search' },
  { id: 'together', label: 'Together', icon: 'users', route: PLAY_HOME + '?tab=together' },
  { id: 'controllers', label: 'Controllers', icon: 'devices', route: '/page/wittyplay-controllers' },
  { id: 'memories', label: 'Memories', icon: 'photo', route: PLAY_HOME + '?tab=memories' },
  { id: 'settings', label: 'Settings', icon: 'settings', route: PLAY_SETTINGS },
]
// The dock's Settings opens WittyPlay's whole Settings (its privacy page is one of them). The dock always starts with WittyPlay's
// own Home, so every WittyPlay page (Settings, Controllers, a console) has one press back to it; leaving for the server is the
// status bar's Return To Server.
const PLAY_DOCK_HOME = PLAY_DOCK[0]
export const dockFor = <T extends { id: string; route: string }>(entries: T[] | undefined): T[] => {
  const list = (entries?.length ? entries : (PLAY_DOCK as unknown as T[])).map((e) => (e.id === 'settings' ? { ...e, route: PLAY_SETTINGS } : e))
  return list.some((e) => e.id === 'home' || e.route === PLAY_HOME) ? list : [PLAY_DOCK_HOME as unknown as T, ...list]
}

// Its top level, where Back exits to Home.
export const isPlayTop = (path: string) => path === PLAY_HOME || path === PLAY_DESIGN

// The first-time tip on how to get back to the app: shown once per profile (kept in the profile's /me/ui under this key), and
// again from the status bar's help button. The same words every app shows.
export const PLAY_TIP_KEY = 'wittyplayExitTipSeen'
export const PLAY_TIP = {
  title: 'Getting back to the app',
  intro: 'WittyPlay Online fills the whole screen. To leave it and go back to the server:',
  ways: [
    { key: 'server', text: 'Choose Return To Server, at the top right.' },
    { key: 'Esc', text: 'Press Esc (or Backspace) on a keyboard.' },
    { key: 'B', text: 'Press B on a controller, or Back on a TV remote.' },
  ],
  done: 'Got it',
}
// Whether the tip shows by itself: not once this profile has seen it.
export const tipDue = (seen: unknown) => seen !== true

// ---- which buttons WittyPlay draws: the first step of the first-time tip, and the controllers page after ----
// The pad families are the app's own (ui/inputModel.ts PadFamily, padFamily), with the keyboard beside them. Kept per profile in /me/ui.
export type PlayPads = 'keyboard' | 'xbox' | 'playstation' | 'nintendo'
export const PLAY_PADS_TITLE = 'Which buttons should we show?'
// The controllers page, where the choice can be changed later.
export const PLAY_CONTROLLERS = '/page/wittyplay-controllers'
export const PLAY_PADS_KEY = 'wittyplayButtons'
export const PLAY_PADS: { id: PlayPads; label: string; note: string }[] = [
  { id: 'keyboard', label: 'Keyboard', note: 'Enter, Esc, Backspace and the arrows' },
  { id: 'xbox', label: 'Xbox / XInput', note: 'A, B, X and Y; View and Menu' },
  { id: 'playstation', label: 'PlayStation / DualSense', note: 'Cross, Circle, Square and Triangle; Create and Options' },
  // Its id is the app's pad family; the words never name the maker (a nod to its home city instead).
  { id: 'nintendo', label: 'Kyoto layout (A on the right)', note: 'A on the right confirms, B below goes back; + and −' },
]
export const isPlayPads = (v: unknown): v is PlayPads => PLAY_PADS.some((p) => p.id === v)
// The family a pad's id says (the same tests as ui/inputModel.ts padFamily); a pad with no known maker is drawn as XInput, and
// with no pad at all the keyboard's keys are shown.
export function padsFor(ids: string[]): PlayPads {
  for (const id of ids) {
    if (/054c|dualshock|dualsense|playstation/i.test(id)) return 'playstation'
    if (/nintendo|pro controller|joy-?con|vendor: 057e|^0?57e-/i.test(id)) return 'nintendo'
    if (/xbox|045e|xinput|microsoft/i.test(id)) return 'xbox'
  }
  return ids.length ? 'xbox' : 'keyboard'
}
// An SDUI glyph (a, b, x, y: by position, A at the bottom) as the keyboard's key, the same keys as the app's key legend
// (ui/inputModel.ts): A is Enter, B is Esc, X options (.), Y search (/).
export const PLAY_KEYS: Record<'a' | 'b' | 'x' | 'y', string> = { a: 'Enter', b: 'Esc', x: '.', y: '/' }
// The exit hint's words for each choice.
export const exitWords = (p: PlayPads) => ({
  keyboard: 'Press Esc (or Backspace) on the keyboard.',
  xbox: 'Press B on the controller.',
  playstation: 'Press Circle on the controller.',
  nintendo: 'Press B on the controller (the bottom button).',
}[p])
// A Nintendo choice trades the bottom and right buttons (the right one confirms); the others leave each pad to its own id.
export const swapFor = (p: PlayPads): boolean | null => (p === 'nintendo' ? true : p === 'keyboard' ? null : false)

// ---- how WittyPlay lays out its games: one row sideways with the focused game below (the console way), a grid that scrolls down, or a
// dense list for big libraries. Per profile in /me/ui, like the buttons; Settings and the tab row change it.
export type PlayLayout = 'shelf' | 'grid' | 'list'
export const PLAY_LAYOUT_KEY = 'wittyplayLayout'
export const PLAY_LAYOUTS: { id: PlayLayout; label: string; note: string }[] = [
  { id: 'shelf', label: 'Shelf', note: 'One row that scrolls sideways, the chosen game large with its details below' },
  { id: 'grid', label: 'Grid', note: 'Every game in tiles that scroll down, its details beside them' },
  { id: 'list', label: 'List', note: 'Dense rows: title, console and when it was played, for big libraries' },
]
export const isPlayLayout = (v: unknown): v is PlayLayout => PLAY_LAYOUTS.some((l) => l.id === v)
export const layoutOf = (v: unknown): PlayLayout => (isPlayLayout(v) ? v : 'shelf')

// The Consoles tab: WittyPlay's own page (pages/PlayConsoles.tsx), beside the page's tabs from the server.
export const PLAY_CONSOLES = '/wittyplay/consoles'
// A games library inside WittyPlay: the app's whole games library (by console, search, filters), in WittyPlay's frame.
export const PLAY_LIBRARY = '/wittyplay/library'
// The home's tabs as the server's page has them, for WittyPlay's own pages to show the same row.
// In the order they come unless someone arranges their own (Recent, then Consoles, then the rest).
export const PLAY_TABS = [
  { id: 'recent', label: 'Recent' }, { id: 'consoles', label: 'Consoles' }, { id: 'classics', label: 'Classics' },
  { id: 'together', label: 'Together' }, { id: 'memories', label: 'Memories' }, { id: 'discover', label: 'Discover' },
]
// Someone's own arrangement of the tabs, per profile in /me/ui: {order, hidden}, by id.
export const PLAY_TABS_KEY = 'wittyplayTabs'
export type PlayTabsPref = { order: string[]; hidden: string[] }
export function tabsPref(v: unknown): PlayTabsPref {
  const o = (v && typeof v === 'object' ? v : {}) as Partial<PlayTabsPref>
  const ids = (x: unknown) => (Array.isArray(x) ? x.filter((i): i is string => typeof i === 'string') : [])
  const order = ids(o.order)
  return { order: order.length ? order : PLAY_TABS.map((t) => t.id), hidden: ids(o.hidden).filter((h) => h !== 'recent') }
}
// One tab moved one place (-1 up, 1 down) in the arrangement.
export function moveTab(order: string[], id: string, dir: -1 | 1): string[] {
  const all = [...order, ...PLAY_TABS.map((t) => t.id).filter((t) => !order.includes(t))]
  const i = all.indexOf(id), j = i + dir
  if (i < 0 || j < 0 || j >= all.length) return all
  ;[all[i], all[j]] = [all[j], all[i]]
  return all
}
export const tabRoute = (id: string) => (id === 'consoles' ? PLAY_CONSOLES : PLAY_HOME + '?tab=' + id)

// The games library a console's games are browsed in: the one kept for that console, else the first games library (it holds many).
export function libraryFor<L extends { id: number; type: string; console?: string }>(libs: L[], consoleId: string): L | undefined {
  const games = libs.filter((l) => l.type === 'games')
  return games.find((l) => l.console === consoleId) || games.find((l) => !l.console || l.console === 'auto') || games[0]
}
// A console's years as people say them: 1990 to 1997, or since 2017.
export const yearsWords = (launched?: number, ended?: number) => (launched ? (ended && ended !== launched ? launched + ' to ' + ended : ended ? String(launched) : 'Since ' + launched) : '')

// ---- a game's own page in WittyPlay (pages/PlayGame.tsx): where a game opens first, and where it comes back to when it's closed ----
export const PLAY_GAME = '/wittyplay/game'
export const playGamePath = (id: number | string) => `${PLAY_GAME}/${id}`
// What choosing a game does: show its page first (Play is the first button there) or start it straight away. Per profile in /me/ui.
export type PlayOpen = 'info' | 'play'
export const PLAY_OPEN_KEY = 'wittyplayOpen'
export const PLAY_OPENS: { id: PlayOpen; label: string; note: string }[] = [
  { id: 'info', label: 'Show its page first', note: 'Its pictures, saves, settings and achievements, with Play as the first button' },
  { id: 'play', label: 'Start playing', note: 'Straight into the game. Its page is still there when you close it' },
]
export const openOf = (v: unknown): PlayOpen => (v === 'play' ? 'play' : 'info')
// A game card's link inside WittyPlay: to the game's page, unless this profile chose to start playing. Anything else is left alone.
export const cardRoute = (to: string, open: PlayOpen): string => {
  const m = /^\/game\/(\d+)$/.exec(to)
  return m && open === 'info' ? playGamePath(m[1]) : to
}
// Outside WittyPlay a game's card always opens its WittyPlay page: playing, saves and settings are there.
export const mainGameRoute = (to: string): string => cardRoute(to, 'info')
