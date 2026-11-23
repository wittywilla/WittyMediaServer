// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which steps first-time setup shows, and in what order. A step belongs to whatever needs it: the
// server itself, or a module. A module's step only appears once that module is on this server. Nothing here touches the
// page or the network, so it can be tested on its own (plan.test.mjs). Its words are marked with tk and translated where
// they're shown; describeOutcome is given the app's own words by the page.
import { ENGLISH, tk, type Words } from './words.ts'

export type StepId = 'welcome' | 'locale' | 'account' | 'storage' | 'name' | 'modules' | 'media' | 'tv' | 'games' | 'discs'
  | 'details' | 'logos' | 'music' | 'karaoke' | 'listings' | 'look' | 'colours' | 'tvguide' | 'people' | 'network' | 'remote' | 'done'

// The six numbered steps (docs/SETUP-FLOW.md). Their ids are the server's names for them (GET /api/setup/state).
export type DrawnId = 'welcome' | 'account' | 'modules' | 'media' | 'look' | 'ready'
export const DRAWN: { id: DrawnId; label: string }[] = [
  { id: 'welcome', label: tk('Welcome') },
  { id: 'account', label: tk('Your account') },
  { id: 'modules', label: tk('Modules') },
  { id: 'media', label: tk('Your media') },
  { id: 'look', label: tk('Look') },
  { id: 'ready', label: tk('Finish') },
]

// What the steps depend on. kinds is null until it's known, which it can't be before there's an
// account to ask with. A kind that isn't listed needs no module, as the server has the last word.
export type Facts = {
  signedIn: boolean
  store: boolean
  modules: string[]
  kinds: { type: string; available: boolean }[] | null
  // A desktop install that hasn't been told where to keep its data (GET /api/data-location, ask).
  askData?: boolean
  // Advanced setup, chosen on Welcome: every choice gets a page of its own, instead of waiting under Customize.
  advanced?: boolean
}

export type StepInfo = {
  id: StepId
  // The numbered step this page is part of.
  drawn: DrawnId
  label: string
  // Who the step belongs to: 'server', or the modules it needs, any one of which is enough.
  needs: 'server' | string[]
  // Where the same choice is in Settings. A module's page names it; the app's own pages don't repeat it.
  later?: { where: string; to: string }
  // Asked only if this is true, on top of the modules it needs.
  when?: (f: Facts) => boolean
  // A page of advanced setup only. In the quick one, the same choice is under that step's Customize, already answered.
  advanced?: boolean
}

export const canMake = (f: Facts, kind: string) => !f.kinds?.some((k) => k.type === kind && !k.available)
const music = (f: Facts) => canMake(f, 'music')
const films = (f: Facts) => canMake(f, 'movies') || canMake(f, 'shows')

// Every page, in order, each with the numbered step it belongs to (docs/SETUP-FLOW.md). Quick setup asks only what's needed
// to start watching, with the usual answer already chosen. Advanced setup adds a page for each of the rest, next to the
// step it belongs with: the server's name and the people at home with the account; what's looked up for the media with
// the media; colours and the TV guide with the look; the network and away from home last. A module's own page (Live TV,
// Games) comes with Your media, only where that module is.
export const STEPS: StepInfo[] = [
  { id: 'welcome', drawn: 'welcome', label: tk('Welcome'), needs: 'server', when: (f) => !f.signedIn },
  { id: 'locale', drawn: 'welcome', label: tk('Language and time zone'), needs: 'server', when: (f) => !f.signedIn, advanced: true },
  { id: 'account', drawn: 'account', label: tk('Your account'), needs: 'server', when: (f) => !f.signedIn },
  { id: 'storage', drawn: 'account', label: tk('Where data is kept'), needs: 'server', when: (f) => !!f.askData, later: { where: tk('Settings, Data folder'), to: '/settings/data-folder' } },
  { id: 'name', drawn: 'account', label: tk('Server name'), needs: 'server', advanced: true },
  { id: 'people', drawn: 'account', label: tk('People'), needs: 'server', advanced: true },
  { id: 'modules', drawn: 'modules', label: tk('Modules'), needs: 'server', when: (f) => f.store },
  { id: 'media', drawn: 'media', label: tk('Your media'), needs: 'server', later: { where: tk('Settings, Libraries'), to: '/settings/libraries' } },
  { id: 'tv', drawn: 'media', label: tk('Live TV'), needs: ['wittytuner'], later: { where: tk('Settings, Live TV & tuners'), to: '/settings/tuners' } },
  { id: 'games', drawn: 'media', label: tk('Games'), needs: ['wittyplay'], later: { where: tk('Settings, Game metadata'), to: '/settings/game-metadata' } },
  { id: 'discs', drawn: 'media', label: tk('Discs'), needs: ['wittydvd', 'wittybluray'], advanced: true },
  { id: 'details', drawn: 'media', label: tk('Posters and details'), needs: 'server', advanced: true },
  { id: 'logos', drawn: 'media', label: tk('Logos'), needs: 'server', advanced: true },
  { id: 'music', drawn: 'media', label: tk('Lyrics and artists'), needs: 'server', when: music, advanced: true },
  { id: 'karaoke', drawn: 'media', label: tk('Karaoke voices'), needs: 'server', when: music, advanced: true },
  { id: 'listings', drawn: 'media', label: tk('Streaming lists'), needs: 'server', when: films, advanced: true },
  { id: 'look', drawn: 'look', label: tk('Look'), needs: 'server', later: { where: tk('Settings, Appearance'), to: '/settings/appearance' } },
  { id: 'colours', drawn: 'look', label: tk('Colours'), needs: 'server', advanced: true },
  { id: 'tvguide', drawn: 'look', label: tk('TV guide'), needs: ['wittytuner'], advanced: true },
  { id: 'network', drawn: 'ready', label: tk('Home network'), needs: 'server', advanced: true },
  { id: 'remote', drawn: 'ready', label: tk('Away from home'), needs: 'server', advanced: true },
  { id: 'done', drawn: 'ready', label: tk('Ready'), needs: 'server' },
]

// Before an account there is nothing past it to show.
const BEFORE_ACCOUNT: StepId[] = ['welcome', 'locale', 'account']

export const drawnId = (id: StepId): DrawnId => STEPS.find((s) => s.id === id)?.drawn || 'ready'
// The step a page is on, as "Step 3 of 5", and which page of its step it is.
export function drawnNumber(steps: StepInfo[], id: StepId): { at: number; of: number; page?: { at: number; of: number } } {
  const d = drawnId(id)
  const n = { at: Math.max(0, DRAWN.findIndex((x) => x.id === d)) + 1, of: DRAWN.length }
  // Once the account is made its page is gone from the list, but it's still the first page of its step, so the count
  // doesn't change under someone ("part 2 of 3" on the page after it, as before).
  const pages = steps.filter((s) => s.drawn === d)
  if (d === 'account' && !pages.some((s) => s.id === 'account')) pages.unshift(STEPS.find((s) => s.id === 'account')!)
  return pages.length > 1 ? { ...n, page: { at: pages.findIndex((s) => s.id === id) + 1, of: pages.length } } : n
}

const belongs = (s: StepInfo, f: Facts) => s.needs === 'server' || s.needs.some((id) => f.modules.includes(id))

// The steps this server has, in order. Before there's an account, the steps after it can't be
// known yet, so it counts the ones every server has.
export function planSteps(f: Facts): StepInfo[] {
  return STEPS.filter((s) => belongs(s, f) && (!s.advanced || !!f.advanced) && (s.when ? s.when(f) : true))
}

export const pathOf = (id: StepId) => '/setup/' + id

export function stepFromPath(path: string): string {
  return /^\/setup\/([a-z]+)\/?$/.exec(path)?.[1] || ''
}

// A page only advanced setup has, opened in the quick one (an old address, or another browser), goes to the page that
// holds its question there: the step's own Customize, or the next page.
const QUICK_HOME: Record<string, StepId> = {
  locale: 'welcome', name: 'modules', people: 'done', discs: 'media',
  details: 'done', logos: 'done', music: 'done', karaoke: 'done', listings: 'done',
  colours: 'look', tvguide: 'look', network: 'done', remote: 'done',
}

// The step to show for an address. Before there's an account, only the first two can be shown,
// and afterwards those two are gone, so an old address or the Back key can't reach them.
export function currentStep(steps: StepInfo[], wanted: string, signedIn: boolean): StepInfo {
  const old = wanted === 'theme' ? 'colours' : wanted === 'wittytv' ? 'tvguide' : wanted === 'language' ? 'locale' : wanted
  const found = steps.find((s) => s.id === old) || steps.find((s) => s.id === QUICK_HOME[old])
  if (found && (signedIn || BEFORE_ACCOUNT.includes(found.id))) return found
  return steps[0]
}

export function neighbours(steps: StepInfo[], id: StepId): { back?: StepInfo; next?: StepInfo; index: number } {
  const index = steps.findIndex((s) => s.id === id)
  const back = steps[index - 1]
  return { back: back && back.id !== 'account' && back.id !== 'storage' ? back : undefined, next: steps[index + 1], index }
}

// When a module has a new version: one question, two settings.
export type UpdateChoice = 'ask' | 'auto' | 'never'

export function updateSettings(c: UpdateChoice): { moduleUpdateCheck: string; moduleAutoInstall: string } {
  return { moduleUpdateCheck: c === 'never' ? '0' : '1', moduleAutoInstall: c === 'auto' ? '1' : '0' }
}

// Until it's set, the automatic check follows the one for server updates, which is on unless
// someone turned it off. Settings, Modules works it out the same way.
export function updateChoiceFrom(s: Record<string, string>): UpdateChoice {
  if ((s.moduleUpdateCheck || s.updateCheck || '1') !== '1') return 'never'
  return s.moduleAutoInstall === '1' ? 'auto' : 'ask'
}

// Who the module step offers. Only the official list: another store's modules have warnings of their
// own, on their own page. Not what's part of the server, or what comes inside something else.
export type Offerable = { id: string; official?: boolean; kind?: string; core?: boolean; fit?: { kind?: string } }

// Ticked to begin with: without it, choosing a folder from a TV or a phone means typing its path.
export const RECOMMENDED = ['wittyintegratedfilebrowser']
// A program another module brings along when it's chosen, with nothing to decide about by itself.
const COMES_ALONG = ['wittycouchhost']
// For people with a gaming PC of their own, kept out of the way of everyone else.
export const ADVANCED = ['wittycouch', 'wittyrigs']

export function offerable<T extends Offerable>(items: T[]): T[] {
  return items.filter((i) => i.official && i.kind !== 'server' && i.fit?.kind !== 'library' && !COMES_ALONG.includes(i.id) && (!i.core || RECOMMENDED.includes(i.id)))
}

// What each module does, for someone who's never heard of any of this. A catalogue's summaries are
// written for the store, where people have gone looking. One that isn't here shows its own.
export const PLAIN_WORDS: Record<string, string> = {
  wittyintegratedfilebrowser: tk('Lets you choose folders by looking through them, from any device, a TV or a phone included. Without it, you type a folder’s address.'),
  wittydvd: tk('Copies of your DVDs, with their real menus, chapters and subtitles.'),
  wittybluray: tk('Copies of your Blu-rays with their menus, 4K and 3D discs included.'),
  wittycd: tk('Copies of your audio CDs, played like a CD player, with no gaps between tracks.'),
  wittyplay: tk('Your own console games, played in a browser or the app, with any controller.'),
  wittycouch: tk('Play the games on a PC of yours, from your browser. The PC needs Sunshine, a free program.'),
  wittyrigs: tk('Run demanding PC games on a computer of your own, and play them from your other screens.'),
  wittytuner: tk('Live TV and recording, from a TV tuner, an internet TV list or a capture card. Encrypted channels are refused.'),
  wittycomics: tk('Comics and manga, a page at a time or as one long strip.'),
  wittyebook: tk('E-books, with text you can resize, highlights and notes.'),
  wittypdf: tk('PDF files, with zoom, search and a list of pages.'),
  wittyphotos: tk('A timeline and albums of your photos, including the RAW files some cameras make.'),
  wittyvisualizer: tk('Light shows that follow your music, full screen.'),
  wittystats: tk('A page of charts about your server: what’s playing, your libraries and your history.'),
}

// A share on another machine, as someone might type it: \\nas\media or //nas/media.
export const onAnotherMachine = (path: string) => path.trim().replace(/\\/g, '/').startsWith('//')

// What setup leaves behind, in words, from what the server says now rather than what was pressed: only what
// someone chose on the way. Everything left as it was is the usual, and isn't listed.
export type Outcome = {
  libraries: string[]
  modules: string[]
}

export function describeOutcome(o: Outcome, { t, list }: Words = ENGLISH): string[] {
  const out = [o.libraries.length ? t('Libraries: {names}', { names: list(o.libraries) }) : t('No libraries yet')]
  if (o.modules.length) out.push(t('Modules: {names}', { names: list(o.modules) }))
  return out
}

// The games promise, in one place: setup's Games step and the games folder panel both say it.
export const GAMES_LINE = tk('Point it at your games folder and leave it. Anything it can’t prove is listed for you to look at.')
