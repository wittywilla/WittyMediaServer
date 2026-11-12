// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which steps first-time setup shows, and in what order. A step belongs to whatever needs it: the
// server itself, or a module. A module's step only appears once that module is on this server, so
// adding a step means adding one line here and one file beside this one. Nothing here touches the
// page or the network, so it can be tested on its own (plan.test.mjs).

export type StepId = 'welcome' | 'account' | 'name' | 'modules' | 'media' | 'games' | 'discs' | 'details' | 'logos' | 'music' | 'listings' | 'tv' | 'look' | 'theme' | 'people' | 'remote' | 'done'

// What the steps depend on. kinds is null until it's known, which it can't be before there's an
// account to ask with. A kind that isn't listed needs no module, as the server has the last word.
export type Facts = {
  signedIn: boolean
  store: boolean
  modules: string[]
  kinds: { type: string; available: boolean }[] | null
}

export type StepInfo = {
  id: StepId
  label: string
  // Who the step belongs to: 'server', or the modules it needs, any one of which is enough.
  needs: 'server' | string[]
  // Where the same choice is, for later. Every step that asks something has one.
  later?: { where: string; to: string }
  // Asked only if this is true, on top of the modules it needs.
  when?: (f: Facts) => boolean
}

export const canMake = (f: Facts, kind: string) => !f.kinds?.some((k) => k.type === kind && !k.available)

// Design 2 draws fourteen steps (docs/design-2, the canvas's setupData): Welcome, Account, Name and address, Look, Theme,
// Modules, Libraries, Details and artwork, Live TV, Games, Discs, People and rules, Remote access and apps, Ready. The last
// three of "Details and artwork" (logos, lyrics, streaming lists) are its pages, each with a question of its own to answer.
export const STEPS: StepInfo[] = [
  { id: 'welcome', label: 'Welcome', needs: 'server', when: (f) => !f.signedIn },
  { id: 'account', label: 'Account', needs: 'server', when: (f) => !f.signedIn },
  { id: 'name', label: 'Name and address', needs: 'server', later: { where: 'Settings, Appearance, under Name and logo', to: '/settings/appearance#name' } },
  { id: 'look', label: 'Look', needs: 'server', later: { where: 'Settings, Appearance', to: '/settings/appearance' } },
  { id: 'theme', label: 'Theme', needs: 'server', later: { where: 'Settings, Appearance', to: '/settings/appearance#theme' } },
  { id: 'modules', label: 'Modules', needs: 'server', when: (f) => f.store, later: { where: 'Settings, Modules', to: '/settings/modules' } },
  { id: 'media', label: 'Libraries', needs: 'server', later: { where: 'Settings, Libraries', to: '/settings/libraries' } },
  { id: 'details', label: 'Details and artwork', needs: 'server', later: { where: 'Settings, Metadata agents', to: '/settings/metadata' } },
  { id: 'logos', label: 'Logos', needs: 'server', later: { where: 'Settings, Metadata agents, under Logos', to: '/settings/metadata#logos' } },
  { id: 'music', label: 'Lyrics', needs: 'server', when: (f) => canMake(f, 'music'), later: { where: 'Settings, Metadata agents, under Lyrics', to: '/settings/metadata#lyrics' } },
  { id: 'listings', label: 'Streaming lists', needs: 'server', when: (f) => canMake(f, 'movies') || canMake(f, 'shows'), later: { where: 'Settings, Metadata agents, under Streaming listings', to: '/settings/metadata#listings' } },
  { id: 'tv', label: 'Live TV', needs: ['wittytuner'], later: { where: 'Settings, Channels & tuners', to: '/settings/channels' } },
  { id: 'games', label: 'Games', needs: ['wittyplay'], later: { where: 'Settings, Metadata agents', to: '/settings/metadata' } },
  { id: 'discs', label: 'Discs', needs: ['wittydvd', 'wittybluray'], later: { where: 'Settings, Playback & discs', to: '/settings/playback' } },
  { id: 'people', label: 'People and rules', needs: 'server', later: { where: 'Settings, Profiles', to: '/settings/users' } },
  { id: 'remote', label: 'Remote access and apps', needs: 'server', later: { where: 'Settings, Remote Access', to: '/settings/remote' } },
  { id: 'done', label: 'Ready', needs: 'server' },
]

// A page that belongs to another step's number: Details and artwork has four.
const PAGE_OF: Partial<Record<StepId, StepId>> = { logos: 'details', music: 'details', listings: 'details' }
export const drawnId = (id: StepId): StepId => PAGE_OF[id] || id
export const drawnSteps = (steps: StepInfo[]) => steps.filter((s) => !PAGE_OF[s.id])
// The drawn step a page is on, as "Step 8 of 14", and which page of its step it is.
export function drawnNumber(steps: StepInfo[], id: StepId, signedIn: boolean): { at: number; of: number; page?: { at: number; of: number } } {
  const list = drawnSteps(steps)
  const n = stepNumber(Math.max(0, list.findIndex((s) => s.id === drawnId(id))), list.length, signedIn)
  const pages = steps.filter((s) => drawnId(s.id) === drawnId(id))
  return pages.length > 1 ? { ...n, page: { at: pages.findIndex((s) => s.id === id) + 1, of: pages.length } } : n
}
// Steps with nothing to lose by passing: a step that says where to change it later.
export const canSkip = (s: StepInfo) => !!s.later && s.id !== 'name' && s.id !== 'look' && s.id !== 'theme'

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
  if (found && (signedIn || found.id === 'welcome' || found.id === 'account')) return found
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
  wittyintegratedfilebrowser: 'Lets you choose folders by looking through them, from any device, a TV or a phone included. Without it, you type a folder’s address.',
  wittydvd: 'Copies of your DVDs, with their real menus, chapters and subtitles.',
  wittybluray: 'Copies of your Blu-rays with their menus, 4K and 3D discs included.',
  wittycd: 'Copies of your audio CDs, played like a CD player, with no gaps between tracks.',
  wittyplay: 'Your own console games, played in a browser or the app, with any controller.',
  wittycouch: 'Play the games on a PC of yours, from your browser. The PC needs Sunshine, a free program.',
  wittyrigs: 'Run demanding PC games on a computer of your own, and play them from your other screens.',
  wittytuner: 'Live TV and recording, from a TV tuner, an internet TV list or a capture card. Encrypted channels are refused.',
  wittycomics: 'Comics and manga, a page at a time or as one long strip.',
  wittyebook: 'E-books, with text you can resize, highlights and notes.',
  wittypdf: 'PDF files, with zoom, search and a list of pages.',
  wittyphotos: 'A timeline and albums of your photos, including the RAW files some cameras make.',
  wittyvisualizer: 'Light shows that follow your music, full screen.',
  wittystats: 'A page of charts about your server: what’s playing, your libraries and your history.',
}

// Step numbers count the two pages before there was an account, so they don't go back to 1 after it.
export function stepNumber(index: number, total: number, signedIn: boolean): { at: number; of: number } {
  const before = signedIn ? 2 : 0
  return { at: index + 1 + before, of: total + before }
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

export function describeOutcome(o: Outcome, steps: StepInfo[]): { step: StepId; text: string; on: boolean }[] {
  const has = (id: StepId) => steps.some((s) => s.id === id)
  const list = (names: string[]) => (names.length < 2 ? names[0] || '' : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`)
  const out: { step: StepId; text: string; on: boolean }[] = []
  out.push({ step: 'media', on: o.libraries.length > 0, text: o.libraries.length ? `Libraries: ${list(o.libraries)}` : 'No libraries yet' })
  if (has('modules')) {
    out.push({ step: 'modules', on: o.modules.length > 0, text: o.modules.length ? `Modules: ${list(o.modules)}` : 'No modules added' })
    out.push({ step: 'modules', on: true, text: { ask: 'Module updates: it tells you, and you choose', auto: 'Module updates: installed by themselves', never: 'Module updates: only when you check' }[o.updates] })
  }
  out.push({ step: 'logos', on: o.logos === 'yes', text: o.logos === 'yes' ? 'Logos: downloading from Wikimedia Commons' : 'Logos: plain names, nothing downloaded' })
  if (has('music')) {
    out.push({ step: 'music', on: o.lyrics, text: o.lyrics ? 'Lyrics: looked up online when someone opens them' : 'Lyrics: only those beside your music or in its files' })
    out.push({ step: 'music', on: o.artists, text: o.artists ? 'Artist biographies and photos: from Wikipedia' : 'Artist biographies and photos: off' })
  }
  if (has('listings')) out.push({ step: 'listings', on: o.listings, text: o.listings ? `Streaming lists: on, for ${o.region}` : 'Streaming lists: off' })
  return out
}

// The games promise, in one place: setup's Games step and the games folder panel both say it.
export const GAMES_LINE = 'Point it at your games folder and leave it. Anything it can’t prove is listed for you to look at.'
