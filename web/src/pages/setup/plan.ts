// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which steps first-time setup shows, and in what order. A step belongs to whatever needs it: the
// server itself, or a module. A module's step only appears once that module is on this server, so
// adding a step means adding one line here and one file beside this one. Nothing here touches the
// page or the network, so it can be tested on its own (plan.test.mjs). Its words are marked with tk and translated where
// they're shown; describeOutcome is given the app's own words by the page.
import { ENGLISH, tk, type Words } from './words.ts'

export type StepId = 'language' | 'welcome' | 'account' | 'storage' | 'name' | 'network' | 'modules' | 'media' | 'tv' | 'games' | 'discs'
  | 'look' | 'theme' | 'wittytv' | 'people' | 'remote' | 'details' | 'logos' | 'music' | 'karaoke' | 'listings' | 'done'

// The twelve numbered steps (docs/SETUP-FLOW.md). Their ids are the server's names for them (GET /api/setup/state).
export type DrawnId = 'language' | 'welcome' | 'account' | 'server' | 'modules' | 'libraries' | 'modulesetup' | 'looks' | 'people' | 'remote' | 'extras' | 'ready'
export const DRAWN: { id: DrawnId; label: string }[] = [
  { id: 'language', label: tk('Language') },
  { id: 'welcome', label: tk('Welcome') },
  { id: 'account', label: tk('Your account') },
  { id: 'server', label: tk('This server') },
  { id: 'modules', label: tk('Modules') },
  { id: 'libraries', label: tk('Libraries') },
  { id: 'modulesetup', label: tk('Set up your modules') },
  { id: 'looks', label: tk('Looks') },
  { id: 'people', label: tk('People') },
  { id: 'remote', label: tk('Away from home') },
  { id: 'extras', label: tk('Extras') },
  { id: 'ready', label: tk('Ready') },
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
}

export type StepInfo = {
  id: StepId
  // The numbered step this page is part of.
  drawn: DrawnId
  label: string
  // Who the step belongs to: 'server', or the modules it needs, any one of which is enough.
  needs: 'server' | string[]
  // Where the same choice is, for later. Every step that asks something has one.
  later?: { where: string; to: string }
  // Asked only if this is true, on top of the modules it needs.
  when?: (f: Facts) => boolean
  // Can't be passed by, although it says where to change it later: its answer is needed now.
  noSkip?: boolean
}

export const canMake = (f: Facts, kind: string) => !f.kinds?.some((k) => k.type === kind && !k.available)

// Every page, in order, each with the numbered step it belongs to (docs/SETUP-FLOW.md). The language and time zone come
// first and are asked once; modules are chosen and set up before the looks, so the looks are about what's here; WittyTV's
// look is asked only where WittyTuner is; what nobody needs to start watching comes last, as extras.
export const STEPS: StepInfo[] = [
  { id: 'language', drawn: 'language', label: tk('Language and time zone'), needs: 'server', when: (f) => !f.signedIn },
  { id: 'welcome', drawn: 'welcome', label: tk('Welcome'), needs: 'server', when: (f) => !f.signedIn },
  { id: 'account', drawn: 'account', label: tk('Your account'), needs: 'server', when: (f) => !f.signedIn },
  { id: 'storage', drawn: 'server', label: tk('Where data is kept'), needs: 'server', when: (f) => !!f.askData, noSkip: true, later: { where: tk('Settings, Data folder'), to: '/settings/data-folder' } },
  { id: 'name', drawn: 'server', label: tk('Name'), needs: 'server', later: { where: tk('Settings, Appearance, under Name and logo'), to: '/settings/appearance#name' } },
  { id: 'network', drawn: 'server', label: tk('Home network'), needs: 'server', later: { where: tk('Settings, Network'), to: '/settings/network' } },
  { id: 'modules', drawn: 'modules', label: tk('Modules'), needs: 'server', when: (f) => f.store, later: { where: tk('Settings, Modules'), to: '/settings/modules' } },
  { id: 'media', drawn: 'libraries', label: tk('Libraries'), needs: 'server', later: { where: tk('Settings, Libraries'), to: '/settings/libraries' } },
  { id: 'tv', drawn: 'modulesetup', label: tk('Live TV'), needs: ['wittytuner'], later: { where: tk('Settings, Live TV & tuners'), to: '/settings/tuners' } },
  { id: 'games', drawn: 'modulesetup', label: tk('Games'), needs: ['wittyplay'], later: { where: tk('Settings, Game metadata'), to: '/settings/game-metadata' } },
  { id: 'discs', drawn: 'modulesetup', label: tk('Discs'), needs: ['wittydvd', 'wittybluray'], later: { where: tk('Settings, DVDs and Blu-rays'), to: '/settings/discs' } },
  { id: 'look', drawn: 'looks', label: tk('Layout'), needs: 'server', later: { where: tk('Settings, Appearance'), to: '/settings/appearance' } },
  { id: 'theme', drawn: 'looks', label: tk('Colours'), needs: 'server', later: { where: tk('Settings, Appearance'), to: '/settings/appearance#theme' } },
  { id: 'wittytv', drawn: 'looks', label: tk('WittyTV'), needs: ['wittytuner'], later: { where: tk('Settings, Appearance, under WittyTV'), to: '/settings/appearance#wittytv' } },
  { id: 'people', drawn: 'people', label: tk('People'), needs: 'server', later: { where: tk('Settings, Profiles'), to: '/settings/users' } },
  { id: 'remote', drawn: 'remote', label: tk('Away from home'), needs: 'server', later: { where: tk('Settings, Remote Access'), to: '/settings/remote' } },
  { id: 'details', drawn: 'extras', label: tk('Posters and details'), needs: 'server', later: { where: tk('Settings, Metadata agents'), to: '/settings/metadata' } },
  { id: 'logos', drawn: 'extras', label: tk('Logos'), needs: 'server', later: { where: tk('Settings, Metadata agents, under Logos'), to: '/settings/metadata#logos' } },
  { id: 'music', drawn: 'extras', label: tk('Lyrics and artists'), needs: 'server', when: (f) => canMake(f, 'music'), later: { where: tk('Settings, Metadata agents, under Lyrics'), to: '/settings/metadata#lyrics' } },
  { id: 'karaoke', drawn: 'extras', label: tk('Karaoke voices'), needs: 'server', when: (f) => canMake(f, 'music'), later: { where: tk('Settings, Metadata agents, under Lyrics'), to: '/settings/metadata#lyrics' } },
  { id: 'listings', drawn: 'extras', label: tk('Streaming lists'), needs: 'server', when: (f) => canMake(f, 'movies') || canMake(f, 'shows'), later: { where: tk('Settings, Metadata agents, under Streaming listings'), to: '/settings/metadata#listings' } },
  { id: 'done', drawn: 'ready', label: tk('Ready'), needs: 'server' },
]

// Before an account there is nothing past it to show.
const BEFORE_ACCOUNT: StepId[] = ['language', 'welcome', 'account']

export const drawnId = (id: StepId): DrawnId => STEPS.find((s) => s.id === id)?.drawn || 'ready'
// The twelve numbered steps, always, so the total never changes under someone: a step with nothing to do on this server
// is still counted (and shown dimmed in the list, saying why), and passed over.
export const ALL_DRAWN = DRAWN
// The numbered step a page is on, as "Step 8 of 12", and which page of its step it is.
export function drawnNumber(steps: StepInfo[], id: StepId): { at: number; of: number; page?: { at: number; of: number } } {
  const d = drawnId(id)
  const n = { at: Math.max(0, DRAWN.findIndex((x) => x.id === d)) + 1, of: DRAWN.length }
  const pages = steps.filter((s) => s.drawn === d)
  return pages.length > 1 ? { ...n, page: { at: pages.findIndex((s) => s.id === id) + 1, of: pages.length } } : n
}
// Why a numbered step is passed over, for the list of steps. Until modules are chosen, whether there's anything to set up
// for them can't be known, so it doesn't say there isn't.
export function missingWhy(d: DrawnId, steps: StepInfo[], at: StepId, { t }: Pick<Words, 't'> = ENGLISH): string {
  if (BEFORE_ACCOUNT.includes(d as StepId) || steps.some((s) => s.drawn === d)) return ''
  if (d === 'modulesetup') {
    const chose = steps.findIndex((s) => s.id === 'modules')
    return chose >= 0 && steps.findIndex((s) => s.id === at) <= chose ? t('After you choose modules') : t('Nothing to set up')
  }
  return t('Nothing to set up')
}
// The page after Extras, for "Skip the extras".
export const afterExtras = (steps: StepInfo[]): StepInfo | undefined => steps.find((s) => s.id === 'done')
// Steps with nothing to lose by passing: a step that says where to change it later.
export const canSkip = (s: StepInfo) => !!s.later && !s.noSkip

const belongs = (s: StepInfo, f: Facts) => s.needs === 'server' || s.needs.some((id) => f.modules.includes(id))

// The steps this server has, in order. Before there's an account, the steps after it can't be
// known yet, so it counts the ones every server has.
export function planSteps(f: Facts): StepInfo[] {
  return STEPS.filter((s) => belongs(s, f) && (s.when ? s.when(f) : true))
}

export const pathOf = (id: StepId) => '/setup/' + id

export function stepFromPath(path: string): string {
  return /^\/setup\/([a-z]+)\/?$/.exec(path)?.[1] || ''
}

// The step to show for an address. Before there's an account, only the first two can be shown,
// and afterwards those two are gone, so an old address or the Back key can't reach them.
export function currentStep(steps: StepInfo[], wanted: string, signedIn: boolean): StepInfo {
  const found = steps.find((s) => s.id === wanted)
  if (found && (signedIn || BEFORE_ACCOUNT.includes(found.id))) return found
  return steps[0]
}

export function neighbours(steps: StepInfo[], id: StepId): { back?: StepInfo; next?: StepInfo; index: number } {
  const index = steps.findIndex((s) => s.id === id)
  const back = steps[index - 1]
  return { back: back && back.id !== 'account' ? back : undefined, next: steps[index + 1], index }
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

// What setup leaves behind, in words, from what the server says now rather than what was pressed.
export type Outcome = {
  libraries: string[]
  modules: string[]
  logos: '' | 'yes' | 'no'
  lyrics: boolean
  artists: boolean
  listings: boolean
  region: string
  updates: UpdateChoice
}

export function describeOutcome(o: Outcome, steps: StepInfo[], { t, list }: Words = ENGLISH): { step: StepId; text: string; on: boolean }[] {
  const has = (id: StepId) => steps.some((s) => s.id === id)
  const out: { step: StepId; text: string; on: boolean }[] = []
  out.push({ step: 'media', on: o.libraries.length > 0, text: o.libraries.length ? t('Libraries: {names}', { names: list(o.libraries) }) : t('No libraries yet') })
  if (has('modules')) {
    out.push({ step: 'modules', on: o.modules.length > 0, text: o.modules.length ? t('Modules: {names}', { names: list(o.modules) }) : t('No modules added') })
    out.push({ step: 'modules', on: true, text: { ask: t('Module updates: it tells you, and you choose'), auto: t('Module updates: installed by themselves'), never: t('Module updates: only when you check') }[o.updates] })
  }
  out.push({ step: 'logos', on: o.logos === 'yes', text: o.logos === 'yes' ? t('Logos: downloading from Wikimedia Commons') : t('Logos: plain names, nothing downloaded') })
  if (has('music')) {
    out.push({ step: 'music', on: o.lyrics, text: o.lyrics ? t('Lyrics: looked up online when someone opens them') : t('Lyrics: only those beside your music or in its files') })
    out.push({ step: 'music', on: o.artists, text: o.artists ? t('Artist biographies and photos: from Wikipedia') : t('Artist biographies and photos: off') })
  }
  if (has('listings')) out.push({ step: 'listings', on: o.listings, text: o.listings ? t('Streaming lists: on, for {region}', { region: o.region }) : t('Streaming lists: off') })
  return out
}

// The games promise, in one place: setup's Games step and the games folder panel both say it.
export const GAMES_LINE = tk('Point it at your games folder and leave it. Anything it can’t prove is listed for you to look at.')
