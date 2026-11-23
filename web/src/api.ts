// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Thin API client. The server sets an HttpOnly cookie on login; we also keep
// the token in localStorage for native wrappers / cross-origin use.
import { reportRequestFailure } from './errorReport'
import { date, language, t, tk, tx } from './i18n'

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
  lastScanAt: number | null; scanning: boolean; scanMode?: 'changes' | 'full'; // scanMode: while a scan runs or waits to carry on after a restart
  itemCount: number; showMain?: boolean; readOnly?: boolean
  foreignNfo?: boolean // reads .nfo files other programs wrote beside the media
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
  // Games: its other names, each only when known (sdui/render/titles.ts shows the one the profile prefers).
  titles?: { native?: string; english?: string; romanized?: string }
  // Internet radio: what's known about a station, from the radio routes.
  radio?: RadioMeta
  // CD images: a track says which disc item it belongs to.
  cdDisc?: number
}
export type RadioMeta = {
  source?: string; sourceId?: string; homepage?: string; country?: string; countryCode?: string; language?: string; codec?: string; bitrate?: number; hls: boolean
  status: 'ok' | 'trouble' | 'dead' | 'unknown'; reason?: string; lastOk?: number; lastCheck?: number; favorite: boolean; lastPlayed?: number
}
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
  chapters?: { start: number; title: string; thumbnailAt?: number }[]; explain?: PlaybackExplain
  videos?: { index: number; codec: string; width: number; height: number; title?: string }[]; videoIndex?: number; stereo?: string; flattened?: boolean
  streaming?: PlaybackStreaming
  quality?: QualityView; output?: PlaybackOutput
  // From the video's .wittyinfo (docs/formats/wittyinfo.md): every part with its action and source,
  // and what the file says beyond times.
  segments?: { kind: string; startSec: number; endSec: number; label?: string; action: string; source: string }[]
  info?: PlaybackInfoExtra
}
export type PlaybackInfoExtra = {
  origin: '' | 'stored' | 'file'; defaults?: boolean; startAt?: number
  match?: { result: string; usable: boolean; message?: string }
  filters?: { id?: string; category: 'violence' | 'language' | 'nudity' | 'sex' | 'drugs' | 'frightening' | 'flashing' | 'discrimination' | 'other'; severity?: number; start: number; end: number; action?: 'skip' | 'mute' | 'warn'; label?: string }[]
}
// What a quality menu may offer (see docs/CLIENT-API.md, Quality). choice is what streams are made to now.
export type QualityPreset = { name: string; height: number; kbps: number }
export type QualityLimits = { maxHeight: number; maxKbps: number }
export type QualityView = {
  choice: string; asked: string; presets: QualityPreset[]; limits: QualityLimits; away: boolean
  sourceWidth?: number; sourceHeight?: number; sourceKbps?: number; videoCodecs: string[]; audioRates: number[]
}
// The stream as it reaches the player.
export type PlaybackOutput = { width?: number; height?: number; videoCodec?: string; audioCodec?: string; kbps?: number; audioKbps?: number; audioChannels?: number; encoder?: string; work?: StreamWork }
// What converts a stream: GET /hls/{id}/work (docs/CLIENT-API.md, Quality).
export type StreamWork = { encoder: string; gpuEncode: boolean; gpuDecode: boolean; gpuToneMap: boolean; running?: boolean }
export type StreamingPrefs = { prefs: { home: string; away: string }; quality: QualityView; profile: QualityLimits; remote: QualityLimits }
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

// A date and time for people, in the app's language: Oct 31, 2026, 8:00 PM.
export const fmtDateTime = (when: number | string | Date) => date(when, { dateStyle: 'medium', timeStyle: 'short' })

// Servers say things in lower case without a full stop, which is fine for a log. On a page it's a sentence.
const sentence = (s: string) => { const w = s.trim(); return !w ? w : w[0].toUpperCase() + w.slice(1) + (/[\w)”']$/.test(w) ? '.' : '') }
// The server's words in the app's language when the catalogue knows them (as the server wrote them, or as a sentence),
// else as a sentence in English.
const inLanguage = (s: string) => { const w = s.trim(), here = tx(w); return here !== w ? here : tx(sentence(w)) }

// message is what people read, in the app's language; english is the same words as they came, for error reports,
// logs and code that looks for particular words.
export class ApiError extends Error {
  readonly english: string
  constructor(public status: number, message: string, public data?: any) { super(inLanguage(message)); this.english = sentence(message) }
}

const NO_SERVER = tk("Can't reach your server. Check that WittyMediaServer is running and that this device can reach it: on the same network, or through Remote Access.")

// The module a request has just found missing. A page that then can't load says that, rather than
// that something went wrong.
let missing: { id: string; at: number } | null = null
export const recentMissingModule = () => (missing && Date.now() - missing.at < 20000 ? missing.id : null)
export const forgetMissingModule = () => { missing = null }

// A module whose route the server expects answered 404: usually a module older than this server. A page that then can't
// load says so, rather than that the server didn't answer.
let outdated: { id: string; at: number } | null = null
export const recentOutdatedModule = () => (outdated && Date.now() - outdated.at < 20000 ? outdated.id : null)

const parse = (text: string): any => { try { return text ? JSON.parse(text) : null } catch { return { error: text } } }

// The viewer's quality choice for requests that start a stream (player/quality.ts). Only on
// those: a header on every request would make a page on another address ask first each time.
let streamQuality: ((method: string, path: string) => string) | null = null
export const setStreamQuality = (fn: typeof streamQuality) => { streamQuality = fn }

async function send(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<{ text: string; data: any }> {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok
  const q = streamQuality?.(method, path)
  if (q) headers['X-WMS-Quality'] = q
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  let res: Response
  try {
    res = await fetch(serverBase() + '/api' + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin', signal })
  } catch (e) {
    if (signal?.aborted) throw e // given up on here, on purpose: nothing went wrong
    reportRequestFailure(method, path, 0, '')
    // The browser's own wording ("Failed to fetch") means nothing to people.
    throw new ApiError(0, NO_SERVER)
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
  // First run isn't over: the web is the only place it happens, so go there (the setup routes still answer).
  if (res.status === 503 && data?.error === 'setup_required' && !location.pathname.startsWith('/setup')) location.assign('/setup')
  if (res.status === 404) {
    const m = /^\/modules\/([a-z0-9-]+)\//.exec(path)
    if (m && m[1] !== 'web') outdated = { id: m[1], at: Date.now() }
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
    throw new ApiError(0, NO_SERVER)
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
// A change to the modules in use (installed, removed, rolled back) is told to whatever keeps
// an answer about them (the Game PCs entry, shell/useNav.tsx).
export const MODULES_CHANGED = 'wms-modules-changed'
const req = <T>(method: string, path: string, body?: unknown) => {
  onTheWay.clear()
  return send(method, path, body).then((r) => {
    if (method !== 'GET' && /^\/modules\/[a-z0-9-]+\/(add|remove|install|rollback|latest)$/.test(path)) { try { window.dispatchEvent(new Event(MODULES_CHANGED)) } catch { /* no window */ } }
    return r.data as T
  })
}

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
    throw new ApiError(0, tk("Can't reach your server."))
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
// wrappers without cookies still work. A page of the server's own has its
// sign-in in a cookie the server sets (GET /api/status), and a sign-in has no
// business in addresses, which end up in history, copied links and the logs of
// any proxy on the way. Nor is it ever added to an address on another server.
export function mediaUrl(path: string): string {
  const base = serverBase()
  const full = path.startsWith('http') ? path : base + path
  const tok = getToken()
  if (!tok || !base || !sameServer(full, base)) return full
  return full + (full.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(tok)
}

// sameServer: an address is on the server this app talks to.
export function sameServer(url: string, base: string): boolean {
  try {
    return new URL(url).origin === new URL(base).origin
  } catch {
    return false
  }
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

export { fmtRuntime } from './runtime'

// An audio CD image: it opens in the CD player rather than the video player.
export const isCD = (it: Item) => it.kind === 'cd-image'
export const isDisc = (it: Item) => it.kind === 'dvd-iso' || it.kind === 'dvd-folder' || it.kind === 'bluray-iso' || it.kind === 'bluray-folder'
export const isBluray = (it: Item) => it.kind === 'bluray-iso' || it.kind === 'bluray-folder'
// The kind of disc, for labels: "DVD" or "Blu-ray".
export const discName = (it: Item) => (isCD(it) ? 'CD' : isBluray(it) ? 'Blu-ray' : 'DVD')
// Video codecs by the names people know ("mpeg2video" is MPEG-2).
const CODECS: Record<string, string> = { h264: 'H.264', hevc: 'HEVC', h265: 'HEVC', mpeg2video: 'MPEG-2', mpeg1video: 'MPEG-1', mpeg4: 'MPEG-4', vc1: 'VC-1', av1: 'AV1', vp9: 'VP9', vp8: 'VP8', prores: 'ProRes' }
export const codecName = (c?: string) => (c ? CODECS[c.toLowerCase()] || c.toUpperCase() : '')

// The older bibliographic codes discs use, which the browser's names don't know, as the codes it does.
const BIB: Record<string, string> = {
  dut: 'nl', fre: 'fr', ger: 'de', chi: 'zh', gre: 'el', rum: 'ro', cze: 'cs', ice: 'is', per: 'fa', baq: 'eu', slo: 'sk',
  mac: 'mk', alb: 'sq', arm: 'hy', geo: 'ka', bur: 'my', may: 'ms', wel: 'cy', tib: 'bo',
}
let names: { here: string; of: Intl.DisplayNames | null } | undefined
// A language's name in the app's language ("German", "Allemand"), named by the browser, or the code as is.
export const langName = (code?: string) => {
  const c = code?.toLowerCase()
  if (!c || c === 'und') return t('Unknown')
  if (c === 'mul') return t('Several languages')
  if (c === 'zxx') return t('No language')
  const here = language()
  if (names?.here !== here) try { names = { here, of: new Intl.DisplayNames([here, 'en'], { type: 'language', fallback: 'none' }) } } catch { names = { here, of: null } }
  try { const n = names.of?.of(BIB[c] || c); if (n) return n[0].toLocaleUpperCase(here) + n.slice(1) } catch { /* not a language code */ }
  return code!.toUpperCase()
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
