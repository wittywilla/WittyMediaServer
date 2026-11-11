// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Thin API client. The server sets an HttpOnly cookie on login; we also keep
// the token in localStorage for native wrappers / cross-origin use.
import { reportRequestFailure } from './errorReport'
import type { CoreSupport } from './pages/games/platforms'

export type User = { id: number; name: string; displayName?: string; showUsername?: boolean; isAdmin: boolean; createdAt: number; kind: 'adult' | 'kids' | 'guest'; hasPassword: boolean; twoFactor?: boolean; avatar?: string }
// `name` is the username: unique, and what signing in uses. A profile name is what
// people are called around the app, and anyone can have the same one.
export const profileName = (u: Pick<User, 'name' | 'displayName'>) => u.displayName || u.name
// What the corner of the screen says, which is each person's choice.
export const cornerName = (u: Pick<User, 'name' | 'displayName' | 'showUsername'>) => u.showUsername ? u.name : profileName(u)
// A device signed in to an account.
export type Session = { id: string; userId: number; userName: string; device: string; createdAt: number; expiresAt: number; lastSeen: number; ip: string; where: string; via: string; streaming: boolean; current: boolean; plainAway?: boolean }
// What an account may see (null = no limits). maxAge -1 = no age limit.
export type Access = {
  maxAge: number; allowUnrated: boolean; libraries: number[]; blockGenres: string[]
  allowItems: number[]; blockItems: number[]; allowShows: number[]; blockShows: number[]
  liveTv: boolean; libraryChannels: boolean; tunerChannels: boolean
  // An age limit of its own for a kind of media: film, tv, game, music, book.
  kinds?: Record<string, KindLimit>
  onlyGenres?: string[]; blockTags?: string[]
  allowCollections?: number[]; blockCollections?: number[]
  // What the profile may not do: games, recordings, downloads, sharing, comments, syncplay, remoteControl, remote, editProfile.
  off?: string[]
  time?: TimeRules | null
}
export type KindLimit = { maxAge: number; allowUnrated: boolean }
// When a profile may play, and for how long a day. Minutes of 0 and times of '' are no limit.
export type TimeRules = { schoolDayMinutes: number; weekendMinutes: number; from: string; schoolNightUntil: string; weekendUntil: string }
// How long a profile may still play today (GET /me/screen-time). remainingSec -1 = nothing stops it.
export type ScreenTime = { allowed: boolean; reason?: string; allowanceSec: number; usedSec: number; extraSec: number; remainingSec: number; message?: string; until?: number; nextAt?: number }
// What the signed-in profile may do (GET /me/limits), for hiding what is off.
export type MyLimits = { limited: boolean; kind: string; features: Record<string, boolean>; maxAge: number; time: TimeRules | null; screenTime: ScreenTime; parentPin: boolean }
export type RatingSystem = { id: string; name: string; country: string; kinds: string[]; ratings: { label: string; age: number }[] }
export type ParentalLists = { kinds: string[]; systems: RatingSystem[]; features: string[]; avatars: string[]; avatarColours: string[]; maxExtraMinutes: number }
// A profile as the list for switching has it (GET /profiles/household).
export type HouseholdProfile = { id: number; name: string; displayName?: string; kind: 'adult' | 'kids' | 'guest'; hasPassword: boolean; avatar?: string; current: boolean }
export type UserView = User & { access: Access | null }
export type Library = {
  id: number; name: string; type: string; console?: string; paths: string[]; createdAt: number
  lastScanAt: number | null; scanning: boolean; itemCount: number; showMain?: boolean; readOnly?: boolean
}
// A commercials library stays out of the menus and Home unless switched on.
export const inMainMenus = (l: Library) => l.type !== 'commercials' || !!l.showMain
export type DiscTitle = {
  number: number; durationSec: number; chapters: number[]; angles: number
  audio: string[]; subtitles: string[]; wide: boolean; standard: string
}
export type DiscInfo = { label: string; format: string; hasMenus: boolean; mainTitle: number; titles: DiscTitle[]; scrambled: boolean
  content?: 'movie' | 'tv' | 'music'; contentManual?: boolean } // a film, or episodes of a series
// resumeSec: where "Resume" picks up, 0 to start over (the server's rule).
export type Progress = { positionSec: number; durationSec: number; watched: boolean; discTitle?: number; updatedAt: number; resumeSec?: number }
export type Item = {
  id: number; libraryId: number; kind: 'video' | 'dvd-iso' | 'dvd-folder' | 'bluray-iso' | 'bluray-folder' | string; path: string
  title: string; sortTitle: string; year: number | null; overview: string; tagline: string; genres: string[]
  rating: number | null; contentRating: string; studio: string; runtimeSec: number
  poster: string; backdrop: string; container: string; videoCodec: string; audioDirect?: boolean; reader?: 'epub' | 'cbz' | 'pdf'; width: number; height: number
  // A video copied from a DVD or a Blu-ray: it gets that disc's remote, with no menus.
  discSource?: 'dvd' | 'bluray'
  audioLangs: string[]; subLangs: string[]; disc?: DiscInfo; provider: string; providerId: string
  sizeBytes: number; addedAt: number; updatedAt: number; progress?: Progress
  showId?: number; showTitle?: string; season?: number; episode?: number; episodeEnd?: number; aired?: string; nextId?: number
  playMode: '' | 'menu' | 'title'; insertFx: '' | 'on' | 'off'
  // Discs: what those come to with the server's defaults applied.
  startMode?: 'menu' | 'title'; insertEffect?: boolean
  introStart?: number; introEnd?: number
  // Audio (music, audiobooks, podcasts). videoCodec holds the audio codec for these.
  artist?: string; album?: string; albumArtist?: string; track?: number; discNo?: number
  // Books: reading direction; runtimeSec is the length in pages (comics) or chapters (EPUB).
  reading?: '' | 'ltr' | 'rtl' | 'webtoon'
  // Games: the console it runs on.
  console?: string
  // Internet radio: what's known about a station, from the radio routes.
  radio?: RadioMeta
  // CD images: a track says which disc item it belongs to.
  cdDisc?: number
}
export type RadioMeta = {
  source?: string; sourceId?: string; homepage?: string; country?: string; countryCode?: string; language?: string; codec?: string; bitrate?: number; hls: boolean
  status: 'ok' | 'trouble' | 'dead' | 'unknown'; reason?: string; lastOk?: number; lastCheck?: number; favorite: boolean; lastPlayed?: number
}
// What's known about a game beyond what its item holds (GET /games/{id}/metadata).
export type GameScore = { source: string; kind: 'critic' | 'user' | string; value: number; votes?: number; url?: string }
export type GameDetailsInfo = {
  itemId: number; status: 'pending' | 'done' | string; locked: boolean; matchSource: string; matchMethod: string; confidence: number
  tags: string; released: string; developer: string; publisher: string; franchise: string; players: number
  scores: GameScore[]; ages: { system: string; value: string; age: number | null }[]; art: { kind: string; url: string }[]
  credits: { source: string; name: string; site: string; text: string; license: string; url?: string }[]
  crcs?: string; serial?: string
  // How its console was worked out, why it can't play if it can't, and its discs.
  detectedBy?: string; detectSure?: number; detectWhy?: string; problem?: string; problemText?: string; entry?: string
  discs?: { disc: number; name: string }[]
}
// Files in a games library that a scan wasn't sure about (GET /libraries/{id}/games/review).
export type GameReviewItem = { id: number; path: string; name: string; size: number; kind: string; reason: string; suggest: { console: string; name: string; short: string }[]; itemId?: number }
export type GameReview = { items: GameReviewItem[]; counts: Record<string, number>; total: number }
export type GameImport = {
  scanning: boolean; scan: { phase: string; current: string; found: number; done: number } | null; lookup: GameMetaStatus
  review: Record<string, number>; games: Record<string, number>
}
export type GameVersion = { id?: number; entry?: string; label: string; current?: boolean; altered?: boolean }
export type GameVersions = { versions: GameVersion[]; inside?: GameVersion[]; discs?: { disc: number; name: string }[] }
export type GameCandidate = { source: string; id: string; title: string; year?: number; console?: string; poster?: string; score: number }
export type GameFacet = { value: string; count: number }
export type GameFacets = { genres: GameFacet[]; decades: GameFacet[]; players: GameFacet[]; rated: number; series: GameFacet[]; letters: GameFacet[] }
export type GameMetaStatus = {
  running: boolean; phase: string; total: number; pending: number; matched: number; byChecksum: number; pictures: number; withPoster: number; withDetails: number
  consoles: Record<string, { total: number; matched: number; byChecksum: number }>; paused: { source: string; until: number; why: string }[]; keysSet: boolean
}
export type GameSource = {
  id: string; name: string; site: string; why: string; gives: string[]; fields: { setting: string; label: string; hint?: string }[]; keyUrl?: string; cost: string; steps?: string[]
  limit: string; sends: string; credit: string; license: string; unavailable?: string; on: boolean; ready: boolean; saved: string[]; used?: number; allowance?: number; period?: string
}
export type GameCore = { id: string; name: string; license: string; source?: string; sha256?: string; nonCommercial?: boolean; licenseSource?: string; version?: string; unapproved?: boolean }
export type GameConsole = {
  id: string; name: string; short: string; extensions: string[]; cores: GameCore[]; bios?: { file: string; for: string; required: boolean }[]
  unavailable?: string; installed: GameCore[]; ready: boolean; count?: number; needsGpu?: boolean
  // When it isn't ready: why (gpu means this server can't draw its games), in words for people.
  reason?: 'unavailable' | 'core' | 'gpu'; message?: string; gpuMissing?: string
  // Ready, with the processor drawing its games: no graphics card here. notice is the speed caveat.
  software?: boolean; notice?: string
  // Where each core on offer runs, how it draws there, and here: see pages/games/platforms.tsx.
  coreSupport?: Record<string, CoreSupport>
  // What each core on offer is good at beside the console's others (docs/GAMING-API.md, Choosing a core).
  coreAbout?: Record<string, { strength?: 'accuracy' | 'speed' | 'balanced'; about: string }>
  customCores?: GameCore[]; defaultCore?: string; shared?: string[]; padLike?: string; logoName?: string
  pad: { face: { id: number; label: string }[]; shoulder?: { id: number; label: string }[]; meta: { id: number; label: string }[]; sticks?: boolean; pointer?: boolean; art?: string }
  // Its era and years, how its games come, and what it can do (docs/GAMING-API.md, Consoles).
  era?: 'first' | '8-bit' | '16-bit' | 'early-3d' | 'disc' | 'pocket'; launched?: number; ended?: number; media?: 'cart' | 'pocket' | 'disc'
  capabilities?: GameCapabilities
  // How its games stand on a shelf: its case and its cartridge or disc, as shapes to draw around a cover.
  shelf?: { case: GameShelfShape; cart: GameShelfShape }
}
export type GameShelfShape = { kind: string; aspect: number; height: number }
export type GameCapabilities = {
  screens: { count: number; touch?: number; layouts?: string[]; options?: Record<string, { layout: string; values: Record<string, [string, string]>; swap?: string; swapValues?: [string, string] }> }
  input: { ports: number; multitap?: number; sticks?: number; analogTriggers?: boolean; touch?: boolean; lightGun?: boolean; mouse?: boolean; motion?: boolean; keyboard?: boolean; mic?: boolean }
  media: { discs?: boolean; memoryCards?: number; bios: 'none' | 'optional' | 'required'; region?: Record<string, string> }
  play: { rumble?: boolean; rewindByDefault?: boolean }
  picture: { aspect: string; integer?: boolean; look: string }
}
// What this account is playing now, and where (GET /games/playing).
export type GamePlaying = { id: string; itemId: number; title: string; console: string; device?: string; startedAt?: number; seats?: { userId: number; name: string; seat: number }[] }
export type Album = { artist: string; title: string; year?: number; tracks: number; durationSec: number; coverItem: number; addedAt: number; genre?: string }
export type Artist = { name: string; albums: number; tracks: number; coverItem: number }
export type Show = {
  id: number; libraryId: number; path: string; title: string; sortTitle: string; year: number | null; overview: string
  genres: string[]; rating: number | null; contentRating: string; network: string; status: string; poster: string; backdrop: string
  provider: string; providerId: string; addedAt: number; updatedAt: number
  episodeCount: number; seasonCount: number; unwatched: number; lastAddedAt: number
  introStart?: number; introEnd?: number; view: '' | 'cards' | 'list'
}
export type Season = { number: number; title: string; overview: string; poster: string; episodeCount: number; unwatched: number }
export type ShowDetail = { show: Show; seasons: Season[]; episodes: Item[]; nextUpId: number | null }
export type Prefs = { dvdStart: 'menu' | 'title'; insertFx: boolean; tvdb: boolean; tmdb: boolean; episodeView: 'cards' | 'list'; maxSpeed: number; clientLogs?: boolean; insertSound?: boolean; shareLinks?: boolean; uploads?: boolean; visualizer?: boolean }
export type ArtCandidate = { url: string; source: 'local' | 'tmdb' | 'tvdb' | 'tvmaze' | 'anilist' | 'current' }
export type Hub = { key: string; title: string; libraryId?: number; query?: string; items: Item[]; shows?: Show[] }
export type Home = { hubs: Hub[]; featured: Item[]; libraries: Library[] }
export type Track = { index: number; language: string; title?: string; codec: string; channels?: number }
export type PlayInfo = {
  type: 'direct' | 'hls'; url: string; durationSec: number; audio: Track[]; subtitles: Track[]
  audioIndex: number; subtitleIndex: number; container: string; chapters?: { start: number; title: string }[]
}
// The answer to POST /items/{id}/playback for a video. See docs/CLIENT-API.md.
export type PlaybackSubtitle = { index: number; language: string; languageName?: string; title?: string; codec: string; forced?: boolean; sdh?: boolean; text: boolean; delivery: 'none' | 'embedded' | 'sidecar' | 'hls' | 'burn'; url?: string }
export type PlaybackExplain = { summary: string; source: string; picture: string; sound: string; subtitles?: string; reasons: string[]; load: 'none' | 'light' | 'heavy'; streaming?: string }
// How a video is streamed, and what the player should do for that (see docs/CLIENT-API.md, Robust streaming).
export type StreamingHints = { bufferSec: number; maxBufferSec: number; retries: number; firstRetryMs: number; maxRetryMs: number; segmentTimeoutMs: number; resumeOnDrop: boolean; stepDownOnStall: boolean }
export type PlaybackStreaming = { mode: 'standard' | 'robust'; from: 'default' | 'server' | 'device'; segmentSec?: number; measuredKbps?: number; ladder?: { height: number; kbps: number }[]; player: StreamingHints }
export type PlaybackAnswer = {
  decision: 'direct' | 'remux' | 'transcode' | string; reasons: string[]; protocol: string; url: string; streamId?: string; durationSec: number; container?: string
  seek: 'native' | 'restart' | 'none'; startAtSec: number; timeOffsetSec: number; resumeSec: number
  audio: Track[]; subtitles: PlaybackSubtitle[]; audioIndex: number; subtitleIndex: number; subtitleDelivery: string
  chapters?: { start: number; title: string }[]; explain?: PlaybackExplain
  videos?: { index: number; codec: string; width: number; height: number; title?: string }[]; videoIndex?: number; stereo?: string; flattened?: boolean
  streaming?: PlaybackStreaming
}
export type DvdButton = { n: number; x1: number; y1: number; x2: number; y2: number; up: number; down: number; left: number; right: number; auto: boolean; lit: boolean; cmd: string; frames?: number; fps?: number; num?: number; soundSelect?: number; soundPress?: number }
export type DvdMenu = {
  width: number; height: number; aspect: '4:3' | '16:9'; buttons: DvdButton[]; selected: number
  overlayUrl?: string; highlightUrl?: string; buttonsFromSec?: number; buttonsUntilSec?: number; hasHighlights: boolean
  forcedAction?: number; actionAtSec?: number; inTitle?: boolean; popup?: boolean; closed?: boolean
  userTimeoutSec?: number; soundUrl?: string; inEffect?: { url: string; sec: number }[]
}
export type DvdTrack = { n: number; language: string; codec?: string; channels?: number; purpose?: string }
export type DvdTitle = {
  locks?: string[]
  ttn: number; chapter: number; durationSec: number; startSec: number; chapters: number[]
  audio: DvdTrack[]; subtitles: DvdTrack[]; audioStream: number; subtitleStream: number; wide: boolean
  frameW: number; frameH: number; aspect: '4:3' | '16:9'; subtitlesUrl: string; forcedStream: number
  angle: number; angles: number
}
// The disc page: feature first, then bonus titles and shortcuts into menus.
export type DiscChapterView = { n: number; start: number; label: string }
export type DiscTitleView = { ttn: number; label: string; durationSec: number; chapters: DiscChapterView[]; audio: string[]; subtitles: string[]; angles: number; wide: boolean }
export type DiscMenuView = { kind: string; vts: number; label: string }
export type DiscView = { main: DiscTitleView | null; extras: DiscTitleView[]; menus: DiscMenuView[] }
export type DiscLabels = { titles: Record<string, string>; chapters: Record<string, Record<string, string>> }
export type SubEvent = { start: number; end: number; x: number; y: number; w: number; h: number; forced?: boolean; url: string
  text?: string; color?: string; outline?: string; size?: number; halign?: number; valign?: number; bold?: boolean; italic?: boolean }
export type DvdMedia = { type: 'hls' | 'still' | 'none'; url?: string; loop: boolean; durationSec: number; stillSec: number; audio: boolean }
export type Presentation = { kind: 'menu' | 'title' | 'stop'; serial: number; reason?: string; media?: DvdMedia; menu?: DvdMenu; title?: DvdTitle; scrambled?: boolean; timerSec?: number; pause?: boolean; bdj?: { pollUrl: string; video?: number[]; pointer?: boolean } }
export type DvdAction = { type: string; button?: number; key?: string; ttn?: number; chapter?: number; stream?: number; on?: boolean; menu?: string; positionSec?: number; x?: number; y?: number }

const TOKEN_KEY = 'wms.token'
const SERVER_KEY = 'wms.server'
export const getToken = () => localStorage.getItem(TOKEN_KEY) || ''
export const setToken = (t: string) => (t ? localStorage.setItem(TOKEN_KEY, t) : localStorage.removeItem(TOKEN_KEY))

// Native shells (Capacitor/Electron loading the UI from local files) point the
// client at a server with window.WMS_SERVER or the stored server URL.
export const serverBase = (): string =>
  ((window as any).WMS_SERVER as string | undefined) || localStorage.getItem(SERVER_KEY) || ''
export const setServerBase = (url: string) => (url ? localStorage.setItem(SERVER_KEY, url.replace(/\/+$/, '')) : localStorage.removeItem(SERVER_KEY))
// The app's pages came with the app, not from a server (the mobile app).
export const isNativeShell = () => !!(window as any).Capacitor || window.location.protocol === 'file:'
// "192.168.1.10:9797" means http://192.168.1.10:9797; a name on its own is
// taken to be an address on the internet, so https.
export function normalizeServer(input: string): string {
  const v = input.trim().replace(/\/+$/, '')
  if (!v || /^[a-z][a-z0-9+.-]*:\/\//i.test(v)) return v
  return (/^[\d.]+(:\d+)?$|^\[|^localhost\b|:\d+$/i.test(v) ? 'http://' : 'https://') + v
}

// A date and time for people, the same everywhere: Oct 31, 2026, 8:00 PM.
export const fmtDateTime = (t: number | string | Date) => new Date(t).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })

// Servers say things in lower case without a full stop, which is fine for a log. On a page it's a sentence.
const sentence = (s: string) => { const t = s.trim(); return !t ? t : t[0].toUpperCase() + t.slice(1) + (/[\w)”']$/.test(t) ? '.' : '') }

export class ApiError extends Error {
  constructor(public status: number, message: string, public data?: any) { super(sentence(message)) }
}

// The module a request has just found missing. A page that then can't load says that, rather than
// that something went wrong.
let missing: { id: string; at: number } | null = null
export const recentMissingModule = () => (missing && Date.now() - missing.at < 20000 ? missing.id : null)
export const forgetMissingModule = () => { missing = null }

const parse = (text: string): any => { try { return text ? JSON.parse(text) : null } catch { return { error: text } } }

async function send(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<{ text: string; data: any }> {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  let res: Response
  try {
    res = await fetch(serverBase() + '/api' + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin', signal })
  } catch (e) {
    if (signal?.aborted) throw e // given up on here, on purpose: nothing went wrong
    reportRequestFailure(method, path, 0, '')
    // The browser's own wording ("Failed to fetch") means nothing to people.
    throw new ApiError(0, "Can't reach your server. Check that WittyMediaServer is running and that this device can reach it: on the same network, or through Remote Access.")
  }
  const text = await res.text()
  const data = parse(text)
  if (res.status === 424 && data?.module) {
    missing = { id: String(data.module), at: Date.now() }
    throw new ApiError(res.status, data.error || res.statusText, data)
  }
  if (res.status === 403 && data?.code === 'screen-time') {
    // This profile's time is up: the screen-time watcher (shell/ScreenTime.tsx) says so.
    window.dispatchEvent(new CustomEvent('wms:screen-time', { detail: data.screenTime }))
    throw new ApiError(res.status, data.error || res.statusText, data)
  }
  if (!res.ok) {
    reportRequestFailure(method, path, res.status, data?.error || res.statusText, res.headers.get('X-Request-Id'))
    throw new ApiError(res.status, data?.error || res.statusText, data)
  }
  return { text, data }
}

// A file as the body of a request, for the endpoints that take one.
export async function uploadFile<T>(path: string, body: Blob, method = 'POST'): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/octet-stream' }
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok
  let res: Response
  try {
    res = await fetch(serverBase() + '/api' + path, { method, headers, body, credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, "Can't reach your server. Check that WittyMediaServer is running and that this device can reach it: on the same network, or through Remote Access.")
  }
  const text = await res.text()
  let data: any = null
  try { data = text ? JSON.parse(text) : null } catch { data = { error: text } }
  if (!res.ok) throw new ApiError(res.status, data?.error || res.statusText, data)
  return data as T
}
// Asking for the same thing while it's already on its way waits for that answer rather than sending
// again: a page opening has several parts that each want the account's settings, three times over
// on Home, and a slow TV or phone pays for every request. Each caller gets its own copy of the
// answer, so one changing what it was given can't change another's.
const onTheWay = new Map<string, Promise<{ text: string; data: any }>>()
function sendGet(path: string): Promise<{ text: string; data: any }> {
  const key = getToken() + ' ' + serverBase() + path
  const first = onTheWay.get(key)
  if (first) return first.then((r) => ({ text: r.text, data: parse(r.text) }))
  const p = send('GET', path).finally(() => { if (onTheWay.get(key) === p) onTheWay.delete(key) })
  onTheWay.set(key, p)
  return p
}
// A change: what was on its way before it may be out of date, so asking after it asks again.
const req = <T>(method: string, path: string, body?: unknown) => { onTheWay.clear(); return send(method, path, body).then((r) => r.data as T) }

export const api = {
  // signal: a request that's no longer wanted is given up on, so it stops holding one of the few
  // connections a browser has to the server. One that can be given up is never shared with others.
  get: <T>(p: string, signal?: AbortSignal) => (signal ? send('GET', p, undefined, signal) : sendGet(p)).then((r) => r.data as T),
  text: (p: string) => sendGet(p).then((r) => r.text), // an answer that is plain text
  post: <T>(p: string, b?: unknown) => req<T>('POST', p, b ?? {}),
  put: <T>(p: string, b?: unknown) => req<T>('PUT', p, b ?? {}),
  patch: <T>(p: string, b?: unknown) => req<T>('PATCH', p, b ?? {}),
  del: <T>(p: string, b?: unknown) => req<T>('DELETE', p, b),
}

// downloadFile saves what the server sends, signed in the same way as any other
// request, so no token goes in an address. The server's own file name wins.
export async function downloadFile(path: string, name: string) {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok
  let res: Response
  try {
    res = await fetch(serverBase() + '/api' + path, { headers, credentials: 'same-origin' })
  } catch {
    throw new ApiError(0, "Can't reach your server.")
  }
  if (!res.ok) {
    const text = await res.text()
    let msg = text
    try { msg = JSON.parse(text)?.error || text } catch { /* it was plain text */ }
    throw new ApiError(res.status, msg || res.statusText)
  }
  const given = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') || '')?.[1]
  const url = URL.createObjectURL(await res.blob())
  const a = document.createElement('a')
  a.href = url
  a.download = given || name
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

// Media URLs that go through <img>/<video>: append the token so native
// wrappers without cookies still work. Over HTTPS in a browser the cookie is
// enough, and a sign-in has no business in addresses, which end up in the logs
// of any reverse proxy on the way.
export function mediaUrl(path: string): string {
  const full = path.startsWith('http') ? path : serverBase() + path
  const tok = getToken()
  if (!tok || (!serverBase() && window.location.protocol === 'https:')) return full
  return full + (full.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(tok)
}
// mediaUrl with something more in its query.
export const mediaUrlQ = (path: string, q: string) => { const u = mediaUrl(path); return u + (u.includes('?') ? '&' : '?') + q }

export const posterUrl = (it: Item) => mediaUrlQ(`/api/items/${it.id}/poster`, `v=${it.updatedAt}`)
// '' when the item has no picture at all (nothing to ask the server for).
export const backdropUrl = (it: Item) => (it.backdrop || it.poster ? mediaUrlQ(`/api/items/${it.id}/backdrop`, `v=${it.updatedAt}`) : '')
export const showPosterUrl = (s: Show) => mediaUrlQ(`/api/shows/${s.id}/art/poster`, `v=${s.updatedAt}`)
export const showBackdropUrl = (s: Show) => mediaUrlQ(`/api/shows/${s.id}/art/backdrop`, `v=${s.updatedAt}`)
export const seasonPosterUrl = (s: Show, n: number) => mediaUrlQ(`/api/shows/${s.id}/art/season`, `n=${n}&v=${s.updatedAt}`)
export const artPreviewUrl = (src: string) => mediaUrl(`/api/artwork/preview?src=${encodeURIComponent(src)}`)

// Smaller pictures. The server keeps a copy of a picture at each of these widths
// (library.ThumbWidths) once one is asked for, and sends the original where it makes none, so a
// card isn't sent a picture made for a cinema screen and a TV or phone doesn't spend megabytes of
// memory holding one.
const ART_WIDTHS = [160, 320, 480, 640, 1280, 1920]
// sized: an art address at a width. '' stays ''.
export const sized = (url: string, w: number) => url && `${url}${url.includes('?') ? '&' : '?'}w=${w}`
// artSet: a srcset of a poster at the widths a card or a list can want.
export const artSet = (url: string, widths = ART_WIDTHS.slice(0, 4)) => widths.map((w) => `${sized(url, w)} ${w}w`).join(', ')
// fitWidth: the smallest of those widths that fills css pixels on this screen.
export const fitWidth = (css: number) => ART_WIDTHS.find((w) => w >= css * (window.devicePixelRatio || 1)) ?? 1920
// What sizes says for a card's picture: the --card-w and --card-wide-w steps in styles.css, with
// room for a grid cell to grow up to twice a card.
export const CARD_SIZES = '(max-width: 520px) 34vw, (max-width: 1099px) 220px, 20vw'
export const WIDE_CARD_SIZES = '(max-width: 520px) 64vw, (max-width: 1099px) 300px, 28vw'

// Episode label, e.g. "S2 · E15–16".
export function episodeLabel(it: Item): string {
  if (it.season == null || it.episode == null || it.episode < 0) return ''
  const ep = it.episodeEnd ? `E${it.episode}–${it.episodeEnd}` : `E${it.episode}`
  return it.season === 0 ? `Special · ${ep}` : `S${it.season} · ${ep}`
}

let prefsCache: Promise<Prefs> | null = null
export const getPrefs = (fresh = false): Promise<Prefs> => {
  if (!prefsCache || fresh) prefsCache = api.get<Prefs>('/prefs').catch(() => ({ dvdStart: 'menu', insertFx: true, tvdb: false, tmdb: false, episodeView: 'cards' } as Prefs))
  return prefsCache
}
// How a disc starts: worked out by the server from the disc's own settings and
// the server's defaults.
export const dvdStartMode = (it: Item): 'menu' | 'title' => it.startMode || 'menu'
export const insertFxOn = (it: Item) => it.insertEffect !== false

// Track label with the disc's purpose, e.g. "English · Commentary · AC3 2ch".
export const dvdTrackLabel = (t: DvdTrack) =>
  [langName(t.language), t.purpose, t.codec?.toUpperCase(), t.channels ? t.channels + 'ch' : ''].filter(Boolean).join(' · ')

export function fmtTime(sec: number): string {
  if (!isFinite(sec) || sec < 0) sec = 0
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = Math.floor(sec % 60)
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}` : `${m}:${String(s).padStart(2, '0')}`
}

export function fmtRuntime(sec: number): string {
  if (!sec) return ''
  if (sec < 60) return `${Math.max(1, Math.round(sec))} s`
  const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60)
  return h > 0 ? `${h} h ${m} min` : `${m} min`
}

// An audio CD image: it opens in the CD player rather than the video player.
export const isCD = (it: Item) => it.kind === 'cd-image'
export const isDisc = (it: Item) => it.kind === 'dvd-iso' || it.kind === 'dvd-folder' || it.kind === 'bluray-iso' || it.kind === 'bluray-folder'
export const isBluray = (it: Item) => it.kind === 'bluray-iso' || it.kind === 'bluray-folder'
// The kind of disc, for labels: "DVD" or "Blu-ray".
export const discName = (it: Item) => (isCD(it) ? 'CD' : isBluray(it) ? 'Blu-ray' : 'DVD')
// Video codecs by the names people know ("mpeg2video" is MPEG-2).
const CODECS: Record<string, string> = { h264: 'H.264', hevc: 'HEVC', h265: 'HEVC', mpeg2video: 'MPEG-2', mpeg1video: 'MPEG-1', mpeg4: 'MPEG-4', vc1: 'VC-1', av1: 'AV1', vp9: 'VP9', vp8: 'VP8', prores: 'ProRes' }
export const codecName = (c?: string) => (c ? CODECS[c.toLowerCase()] || c.toUpperCase() : '')

const LANGS: Record<string, string> = {
  en: 'English', nl: 'Dutch', fr: 'French', de: 'German', es: 'Spanish', it: 'Italian', pt: 'Portuguese', ja: 'Japanese',
  zh: 'Chinese', ko: 'Korean', ru: 'Russian', sv: 'Swedish', da: 'Danish', no: 'Norwegian', fi: 'Finnish', pl: 'Polish',
  cs: 'Czech', hu: 'Hungarian', el: 'Greek', tr: 'Turkish', he: 'Hebrew', ar: 'Arabic', hi: 'Hindi', th: 'Thai',
  eng: 'English', dut: 'Dutch', nld: 'Dutch', fre: 'French', fra: 'French', ger: 'German', deu: 'German', spa: 'Spanish',
  ita: 'Italian', por: 'Portuguese', jpn: 'Japanese', chi: 'Chinese', zho: 'Chinese', kor: 'Korean', rus: 'Russian', und: 'Unknown',
  // The older bibliographic codes discs use, which the browser's names don't know.
  gre: 'Greek', rum: 'Romanian', cze: 'Czech', ice: 'Icelandic', per: 'Persian', baq: 'Basque', slo: 'Slovak', mac: 'Macedonian',
  alb: 'Albanian', arm: 'Armenian', geo: 'Georgian', bur: 'Burmese', may: 'Malay', wel: 'Welsh', tib: 'Tibetan', mul: 'Several languages', zxx: 'None',
}
let names: Intl.DisplayNames | null | undefined
// Any other code is named by the browser ("cat" is Catalan), or shown as is.
export const langName = (code?: string) => {
  if (!code) return 'Unknown'
  const c = code.toLowerCase()
  if (LANGS[c]) return LANGS[c]
  if (names === undefined) try { names = new Intl.DisplayNames(['en'], { type: 'language', fallback: 'none' }) } catch { names = null }
  try { const n = names?.of(c); if (n) return n } catch { /* not a language code */ }
  return code.toUpperCase()
}

// albumsOnce: the home page's rows list recently added songs one by one; a row
// shows each album once, under the album's name.
export function albumsOnce(items: Item[]): Item[] {
  const seen = new Set<string>()
  const out: Item[] = []
  for (const it of items) {
    if (it.kind !== 'audio' || !it.album) { out.push(it); continue }
    const key = (it.albumArtist || it.artist || '') + '|' + it.album
    if (!seen.has(key)) { seen.add(key); out.push({ ...it, title: it.album }) }
  }
  return out
}
