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
export const isPlayPath = (path: string) => path === PLAY_HOME || path === '/page/wittyplay' || path.startsWith('/page/wittyplay-') || path === PLAY_DESIGN || path === PLAY_SETTINGS || path.startsWith(PLAY_SETTINGS + '/')

// WittyPlay's Settings: every games page of the app's Settings, in WittyPlay's frame (pages/PlaySettings.tsx).
export const PLAY_SETTINGS = '/wittyplay/settings'

// The status bar and dock when a page brings none (WittyPlay's Settings, the games library): the same as WittyPlay Online's own.
export const PLAY_STATUS = [{ id: 'me', kind: 'profile' as const, text: 'WittyPlay Online' }, { id: 'server', kind: 'server' as const }]
export const PLAY_DOCK = [
  { id: 'search', label: 'Search', icon: 'search', route: '/search' },
  { id: 'together', label: 'Together', icon: 'users', route: PLAY_HOME + '?tab=together' },
  { id: 'controllers', label: 'Controllers', icon: 'devices', route: '/page/wittyplay-controllers' },
  { id: 'memories', label: 'Memories', icon: 'photo', route: PLAY_HOME + '?tab=memories' },
  { id: 'settings', label: 'Settings', icon: 'settings', route: PLAY_SETTINGS },
]
// The dock's Settings opens WittyPlay's whole Settings (its privacy page is one of them).
export const dockFor = <T extends { id: string; route: string }>(entries: T[] | undefined): T[] =>
  (entries?.length ? entries : (PLAY_DOCK as unknown as T[])).map((e) => (e.id === 'settings' ? { ...e, route: PLAY_SETTINGS } : e))

// Its top level, where Back exits to Home.
export const isPlayTop = (path: string) => path === PLAY_HOME || path === PLAY_DESIGN

// The first-time tip on how to get back to the app: shown once per profile (kept in the profile's /me/ui under this key), and
// again from the status bar's help button. The same words every app shows.
export const PLAY_TIP_KEY = 'wittyplayExitTipSeen'
export const PLAY_TIP = {
  title: 'Getting back to the app',
  intro: 'WittyPlay Online fills the whole screen. To leave it and go back to Home:',
  ways: [
    { key: 'Home', text: 'Choose Home, at the top right.' },
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
