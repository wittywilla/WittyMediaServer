// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What this browser can do as a Remote Protocol device, and what a remote shows for one, as
// plain rules with nothing from the page so they can be tested (capabilities.test.mjs).
// docs/REMOTE-PROTOCOL.md is the contract; docs/PLAYING.md says what each player answers to.

export type Feature = 'seek' | 'volume' | 'queue' | 'audio' | 'subtitles' | 'navigate' | 'rate'
export type CommandType = 'play' | 'pause' | 'resume' | 'stop' | 'seek' | 'next' | 'previous' | 'volume' | 'mute' | 'audio' | 'subtitle' | 'rate' | 'navigate'
export type NavKey = 'up' | 'down' | 'left' | 'right' | 'select' | 'back' | 'home' | 'menu' | 'playpause'

// The players of this app. Each plays one or more kinds of library item; live TV plays
// channels, which aren't library items, so nothing can be sent to it by `play`.
export type PlayerKind = 'video' | 'disc' | 'cd' | 'audio' | 'radio' | 'tv' | 'book' | 'photo' | 'game'

const ITEM_PLAYER: Record<string, PlayerKind> = {
  video: 'video', audio: 'audio', radio: 'radio', book: 'book', game: 'game', photo: 'photo', 'cd-image': 'cd',
  'dvd-iso': 'disc', 'dvd-folder': 'disc', 'bluray-iso': 'disc', 'bluray-folder': 'disc',
}

// playerFor: the player that opens an item of this kind, or null for a kind this app has no player for.
export const playerFor = (itemKind: string): PlayerKind | null => ITEM_PLAYER[itemKind] ?? null

// What each player answers to, beyond `play` (which the device itself carries out) and `stop`,
// which every player obeys. This is what a player can really do:
//   video  everything
//   disc   chapters for next and previous; seeking and speed in a title, where the disc allows it
//   cd     tracks for next and previous; one speed
//   audio  music, audiobooks and podcasts: their own queue
//   radio  a station is live: no seeking, no speed; next and previous step through the stations queued
//   tv     a channel is live: back and forward within what's been kept; next and previous change channel
//   book   next and previous turn the page; a page doesn't play, so nothing else
//   photo  next and previous step; pause and resume are the slideshow's
//   game   only stop: the game has the keys and the controller
const ACCEPTS: Record<PlayerKind, CommandType[]> = {
  video: ['pause', 'resume', 'stop', 'seek', 'next', 'previous', 'volume', 'mute', 'audio', 'subtitle', 'rate', 'navigate'],
  disc: ['pause', 'resume', 'stop', 'seek', 'next', 'previous', 'volume', 'mute', 'audio', 'subtitle', 'rate', 'navigate'],
  cd: ['pause', 'resume', 'stop', 'seek', 'next', 'previous', 'volume', 'mute', 'navigate'],
  audio: ['pause', 'resume', 'stop', 'seek', 'next', 'previous', 'volume', 'mute', 'rate', 'navigate'],
  radio: ['pause', 'resume', 'stop', 'next', 'previous', 'volume', 'mute', 'navigate'],
  tv: ['pause', 'resume', 'stop', 'seek', 'next', 'previous', 'volume', 'mute', 'audio', 'subtitle', 'navigate'],
  book: ['stop', 'next', 'previous', 'navigate'],
  photo: ['pause', 'resume', 'stop', 'next', 'previous', 'navigate'],
  game: ['stop'],
}

export const accepts = (player: PlayerKind, type: string): boolean => (ACCEPTS[player] as string[]).includes(type)
export const commandsOf = (player: PlayerKind): CommandType[] => ACCEPTS[player]

const NEEDS: Partial<Record<CommandType, Feature>> = {
  seek: 'seek', next: 'queue', previous: 'queue', volume: 'volume', mute: 'volume', audio: 'audio', subtitle: 'subtitles', rate: 'rate', navigate: 'navigate',
}

// featuresOf: the protocol features a player needs claimed for the commands it accepts.
export function featuresOf(player: PlayerKind): Feature[] {
  const out: Feature[] = []
  for (const c of ACCEPTS[player]) { const f = NEEDS[c]; if (f && !out.includes(f)) out.push(f) }
  return out
}

const PLAYER_WORDS: Record<PlayerKind, string> = {
  video: 'A video', disc: 'A disc', cd: 'A CD', audio: 'Music', radio: 'A radio station', tv: 'Live TV', book: 'A book', photo: 'A picture', game: 'A game',
}
const COMMAND_WORDS: Partial<Record<CommandType, string>> = {
  pause: 'be paused', resume: 'be paused', seek: 'be moved through', rate: 'change speed', audio: 'change its sound track', subtitle: 'show subtitles',
  volume: 'have its volume set from here', mute: 'have its volume set from here', next: 'step on', previous: 'step back', navigate: 'take those keys',
}

// refusal: the words the person holding the remote is shown when a player can't do something.
export function refusal(player: PlayerKind, type: string): string {
  if (player === 'game' && type === 'navigate') return 'A game is being played there: its own controller steers it.'
  const what = COMMAND_WORDS[type as CommandType]
  return what ? `${PLAYER_WORDS[player]} can’t ${what}.` : 'This device can’t do that.'
}

// A kind of library, as GET /modules/libraries lists it: whether this server has it, and the
// modules that provide it.
export type LibraryKindLike = { type: string; available: boolean; modules: { id: string; installed: boolean }[] }

// Item kinds that need a module of their own, and which. A kind is claimed only when one of
// its modules is installed: a server without WittyDVD must not have this browser say it plays DVDs.
const KIND_MODULES: Record<string, string[]> = {
  'dvd-iso': ['wittydvd'], 'dvd-folder': ['wittydvd'], 'bluray-iso': ['wittybluray'], 'bluray-folder': ['wittybluray'],
  'cd-image': ['wittycd'], book: ['wittyebook', 'wittypdf', 'wittycomics'], game: ['wittyplay'], photo: ['wittyphotos'],
}
// Item kinds that come with a kind of library, and which.
const KIND_LIBRARIES: Record<string, string[]> = {
  video: ['movies', 'shows', 'videos', 'anime', 'musicvideos', 'shorts', 'commercials', 'photos'],
  audio: ['music', 'audiobooks', 'podcasts'], radio: ['radio'],
}
// In the order the protocol lists them.
const MEDIA_KINDS = ['video', 'audio', 'dvd-iso', 'dvd-folder', 'bluray-iso', 'bluray-folder', 'cd-image', 'book', 'radio', 'game', 'photo']

// mediaKindsFor: the kinds of item this browser can really play against this server. Until
// the server has said what it has (null), only video, which every server plays.
export function mediaKindsFor(libraries: LibraryKindLike[] | null): string[] {
  if (!libraries) return ['video']
  const installed = new Set<string>()
  for (const l of libraries) for (const m of l.modules || []) if (m.installed) installed.add(m.id)
  const have = new Set(libraries.filter((l) => l.available).map((l) => l.type))
  return MEDIA_KINDS.filter((k) => {
    if (KIND_MODULES[k]) return KIND_MODULES[k].some((m) => installed.has(m))
    return k === 'video' || KIND_LIBRARIES[k].some((t) => have.has(t))
  })
}

// deviceFeatures: the union of what the players for those kinds answer to. `queue` and
// `navigate` are the device's own: it keeps the queue and it has the interface.
export function deviceFeatures(mediaKinds: string[]): Feature[] {
  const all: Feature[] = ['seek', 'volume', 'queue', 'audio', 'subtitles', 'navigate', 'rate']
  const have = new Set<Feature>(['queue', 'navigate'])
  for (const k of mediaKinds) { const p = playerFor(k); if (p) for (const f of featuresOf(p)) have.add(f) }
  return all.filter((f) => have.has(f))
}

// One queue is played by one player. Videos and discs share the video player's queue; music,
// audiobooks and podcasts the audio player's; stations theirs. Everything else opens one at a time.
export type QueueFamily = 'video' | 'audio' | 'radio' | 'single'
export const queueFamily = (player: PlayerKind): QueueFamily =>
  player === 'video' || player === 'disc' ? 'video' : player === 'audio' ? 'audio' : player === 'radio' ? 'radio' : 'single'

// checkQueue says why these kinds can't be played as one queue here, or '' when they can.
export function checkQueue(kinds: string[]): string {
  const players = kinds.map(playerFor)
  if (players.some((p) => !p)) return 'This device can’t play that kind of thing.'
  if (players.length < 2) return ''
  const fam = queueFamily(players[0]!)
  if (fam === 'single') return `${PLAYER_WORDS[players[0]!]} opens by itself here, not in a queue.`
  if (players.some((p) => queueFamily(p!) !== fam)) return 'A queue has to be all videos or all music on this device.'
  return ''
}

// What `play` opens, as the app's own routes (openItem.tsx chooses the same way).
export type PlayOptions = { positionSec?: number; audio?: string; subtitle?: string; remote?: string }
export type OpenItem = { id: number; kind: string; libraryId?: number; reader?: string }

// routeFor: the address that opens this item in its player, or null for the kinds the audio
// player takes (they have no page: the bar plays them).
export function routeFor(it: OpenItem, o: PlayOptions = {}): string | null {
  const player = playerFor(it.kind)
  const q = new URLSearchParams()
  switch (player) {
    case 'video': case 'disc':
      if (o.positionSec != null) q.set('start', String(o.positionSec))
      // A disc told where to start goes to its film, not its menu.
      if (player === 'disc' && (o.positionSec || 0) > 0) q.set('mode', 'title')
      if (o.audio) q.set('audio', o.audio)
      if (o.subtitle) q.set('subtitle', o.subtitle)
      // Named for this command, so that asking for what's already open starts it again there.
      if (o.remote) q.set('remote', o.remote)
      return `/play/${it.id}${q.size ? '?' + q : ''}`
    case 'cd': return `/cd/${it.id}?play=1`
    case 'book': return it.reader ? `/read/${it.id}` : null
    case 'game': return `/game/${it.id}`
    case 'photo': return it.libraryId ? `/library/${it.libraryId}?view=${it.id}` : null
    default: return null
  }
}

// The keyboard key a remote's key presses in this app: the same keys a TV remote or a
// controller's D-pad sends (tv.ts, tvmode/nav.ts). `home` and `playpause` aren't keys: the
// device goes home, and plays or pauses whatever is open.
const NAV_KEYS: Record<NavKey, string | null> = {
  up: 'ArrowUp', down: 'ArrowDown', left: 'ArrowLeft', right: 'ArrowRight', select: 'Enter', back: 'Escape', menu: 'ContextMenu', home: null, playpause: null,
}
export const isNavKey = (k: string | undefined): k is NavKey => !!k && k in NAV_KEYS
export const keyboardKey = (k: NavKey): string | null => NAV_KEYS[k]

// screenFor: where this browser is, in a word, for a remote that shows it when nothing plays.
export function screenFor(pathname: string): string {
  if (pathname === '/' || pathname === '') return 'home'
  const first = pathname.split('/')[1]
  if (first === 'tv') return pathname.startsWith('/tv/watch/') ? 'tv' : 'guide'
  return ({ play: 'player', read: 'reader', game: 'game', cd: 'cd', search: 'search', settings: 'settings', you: 'you' } as Record<string, string>)[first] || 'browse'
}

// ---------------------------------------------------------------- the controller's side

export type DeviceLike = {
  online: boolean; hidden?: boolean; itemKind?: string
  capabilities: { mediaKinds: string[]; features: string[] }
  state: { status: string; durationSec?: number; volume?: number; audioTracks?: unknown[]; subtitleTracks?: unknown[]; queue?: number[]; screen?: string }
}

// canPlayOn: "Play here" is offered only on a device that said it plays this kind of thing.
export const canPlayOn = (d: DeviceLike, itemKind: string): boolean => d.online && d.capabilities.mediaKinds.includes(itemKind)

// A device playing a channel that isn't a library item is listed by the server as idle, with
// only its screen: that is how a remote knows the TV is on.
export const watchingTv = (d: DeviceLike): boolean => d.state.status === 'idle' && d.state.screen === 'tv'

// What a remote shows for a device now: what it claimed, narrowed to what it is playing.
export type RemoteControls = {
  transport: boolean; seek: boolean; steps: boolean; volume: boolean; rate: boolean; audio: boolean; subtitles: boolean; queue: boolean; dpad: boolean
  // place: how the position is worded. A book has pages, not seconds; a picture has neither.
  place: 'time' | 'part' | 'none'
}

export function remoteControls(d: DeviceLike): RemoteControls {
  const has = (f: Feature) => d.capabilities.features.includes(f)
  const st = d.state
  const active = d.online && (st.status !== 'idle' || watchingTv(d))
  const kind = d.hidden ? '' : d.itemKind || ''
  const player = kind ? playerFor(kind) : null
  const timed = (st.durationSec || 0) > 0 && player !== 'book' && player !== 'photo' && player !== 'game'
  // What the player for this kind can't do isn't offered, whatever the device claims as a whole.
  const can = (c: CommandType) => !player || accepts(player, c)
  return {
    transport: active && can('pause'),
    seek: active && has('seek') && timed && can('seek'),
    steps: active && has('queue') && can('next'),
    volume: d.online && has('volume') && st.volume != null && can('volume'),
    rate: active && has('rate') && timed && can('rate'),
    audio: active && has('audio') && (st.audioTracks?.length || 0) > 1,
    subtitles: active && has('subtitles') && (st.subtitleTracks?.length || 0) > 0,
    queue: active && has('queue') && (st.queue?.length || 0) > 1,
    dpad: d.online && has('navigate'),
    place: !active || !kind ? 'none' : player === 'book' ? 'part' : timed ? 'time' : 'none',
  }
}

// What next and previous are called on a remote, for what the device is playing: a disc
// steps through chapters, a book through pages, live TV through channels.
export function stepWords(d: DeviceLike): { next: string; previous: string } {
  if (watchingTv(d)) return { next: 'Channel up', previous: 'Channel down' }
  switch (d.hidden ? null : playerFor(d.itemKind || '')) {
    case 'disc': return { next: 'Next chapter', previous: 'Previous chapter' }
    case 'cd': return { next: 'Next track', previous: 'Previous track' }
    case 'book': return { next: 'Next page', previous: 'Previous page' }
    case 'photo': return { next: 'Next picture', previous: 'Previous picture' }
    case 'radio': return { next: 'Next station', previous: 'Previous station' }
  }
  return { next: 'Next', previous: 'Previous' }
}

// What a device is doing, in a few words, for a list of devices. The title and the time are
// added by whoever shows it; a device playing something this account may not see says only that.
export function statusWords(d: DeviceLike): string {
  if (!d.online) return 'Not connected'
  if (watchingTv(d)) return 'Live TV'
  if (d.state.status === 'idle') return 'Nothing playing'
  if (d.hidden) return d.state.status === 'paused' ? 'Paused' : 'Playing something'
  return d.state.status === 'paused' ? 'Paused' : d.state.status === 'buffering' ? 'Loading' : 'Playing'
}

// Why a command didn't happen, in words for the person holding the remote. The server's own
// sentence is used where it says more than the code does (a device's reason for refusing, a
// limit); otherwise each code has its own, naming the device.
export function errorText(code: string | undefined, serverText: string, deviceName: string): string {
  const name = deviceName || 'That device'
  switch (code) {
    case 'offline': return `${name} isn’t connected right now.`
    case 'unsupported': return serverText || `${name} can’t do that.`
    case 'refused': return serverText || `${name} wouldn’t do that.`
    case 'timeout': return `${name} didn’t answer. It may still be doing it.`
    case 'busy': return `${name} has too much to do right now. Try again in a moment.`
    case 'rate': return 'That was too fast. Try again in a moment.'
    case 'no-device': return `${name} is no longer there.`
    case 'no-item': return 'That can’t be found, or isn’t yours to play.'
    case 'not-allowed-there': return serverText || `The account ${name} is signed in as isn’t allowed to play that.`
    case 'nothing-playing': return serverText || 'Nothing is playing there to hand over.'
    case 'disabled': return serverText || 'Playing on other devices is turned off on this server.'
  }
  return serverText || 'That didn’t work.'
}
