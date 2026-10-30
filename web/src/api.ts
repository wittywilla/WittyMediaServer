// Thin API client. The server sets an HttpOnly cookie on login; we also keep
// the token in localStorage for native wrappers / cross-origin use.

export type User = { id: number; name: string; isAdmin: boolean; createdAt: number; kind: 'adult' | 'kids' | 'guest'; hasPassword: boolean }
// What an account may see (null = no limits). maxAge -1 = no age limit.
export type Access = {
  maxAge: number; allowUnrated: boolean; libraries: number[]; blockGenres: string[]
  allowItems: number[]; blockItems: number[]; allowShows: number[]; blockShows: number[]
  liveTv: boolean; libraryChannels: boolean; tunerChannels: boolean
}
export type UserView = User & { access: Access | null }
export type Library = {
  id: number; name: string; type: string; paths: string[]; createdAt: number
  lastScanAt: number | null; scanning: boolean; itemCount: number
}
export type DiscTitle = {
  number: number; durationSec: number; chapters: number[]; angles: number
  audio: string[]; subtitles: string[]; wide: boolean; standard: string
}
export type DiscInfo = { label: string; format: string; hasMenus: boolean; mainTitle: number; titles: DiscTitle[]; scrambled: boolean
  content?: 'movie' | 'tv'; contentManual?: boolean } // a film, or episodes of a series
// resumeSec: where "Resume" picks up, 0 to start over (the server's rule).
export type Progress = { positionSec: number; durationSec: number; watched: boolean; discTitle?: number; updatedAt: number; resumeSec?: number }
export type Item = {
  id: number; libraryId: number; kind: 'video' | 'dvd-iso' | 'dvd-folder' | 'bluray-iso' | 'bluray-folder' | string; path: string
  title: string; sortTitle: string; year: number | null; overview: string; tagline: string; genres: string[]
  rating: number | null; contentRating: string; studio: string; runtimeSec: number
  poster: string; backdrop: string; container: string; videoCodec: string; width: number; height: number
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
}
export type Album = { artist: string; title: string; year?: number; tracks: number; durationSec: number; coverItem: number; addedAt: number }
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
export type Prefs = { dvdStart: 'menu' | 'title'; insertFx: boolean; tvdb: boolean; tmdb: boolean; episodeView: 'cards' | 'list'; maxSpeed: number; clientLogs?: boolean; insertSound?: boolean; shareLinks?: boolean; uploads?: boolean }
export type ArtCandidate = { url: string; source: 'local' | 'tmdb' | 'tvdb' | 'tvmaze' | 'anilist' | 'current' }
export type Hub = { key: string; title: string; libraryId?: number; items: Item[]; shows?: Show[] }
export type Home = { hubs: Hub[]; featured: Item[]; libraries: Library[] }
export type Track = { index: number; language: string; title?: string; codec: string; channels?: number }
export type PlayInfo = {
  type: 'direct' | 'hls'; url: string; durationSec: number; audio: Track[]; subtitles: Track[]
  audioIndex: number; subtitleIndex: number; container: string; chapters?: { start: number; title: string }[]
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
export type Presentation = { kind: 'menu' | 'title' | 'stop'; serial: number; reason?: string; media?: DvdMedia; menu?: DvdMenu; title?: DvdTitle; scrambled?: boolean; timerSec?: number; pause?: boolean; bdj?: { pollUrl: string; video?: number[] } }
export type DvdAction = { type: string; button?: number; key?: string; ttn?: number; chapter?: number; stream?: number; on?: boolean; menu?: string; positionSec?: number }

const TOKEN_KEY = 'wms.token'
const SERVER_KEY = 'wms.server'
export const getToken = () => localStorage.getItem(TOKEN_KEY) || ''
export const setToken = (t: string) => (t ? localStorage.setItem(TOKEN_KEY, t) : localStorage.removeItem(TOKEN_KEY))

// Native shells (Capacitor/Electron loading the UI from local files) point the
// client at a server with window.WMS_SERVER or the stored server URL.
export const serverBase = (): string =>
  ((window as any).WMS_SERVER as string | undefined) || localStorage.getItem(SERVER_KEY) || ''
export const setServerBase = (url: string) => (url ? localStorage.setItem(SERVER_KEY, url.replace(/\/+$/, '')) : localStorage.removeItem(SERVER_KEY))

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message) }
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers['Authorization'] = 'Bearer ' + tok
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  let res: Response
  try {
    res = await fetch(serverBase() + '/api' + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin' })
  } catch {
    // The browser's own wording ("Failed to fetch") means nothing to people.
    throw new ApiError(0, "Can't reach your server. Check that WittyMediaServer is running and this device is on the same network.")
  }
  const text = await res.text()
  let data: any = null
  try { data = text ? JSON.parse(text) : null } catch { data = { error: text } }
  if (!res.ok) throw new ApiError(res.status, data?.error || res.statusText)
  return data as T
}

export const api = {
  get: <T>(p: string) => req<T>('GET', p),
  post: <T>(p: string, b?: unknown) => req<T>('POST', p, b ?? {}),
  put: <T>(p: string, b?: unknown) => req<T>('PUT', p, b ?? {}),
  del: <T>(p: string) => req<T>('DELETE', p),
}

// Media URLs that go through <img>/<video>: append the token so native
// wrappers without cookies still work.
export function mediaUrl(path: string): string {
  const full = path.startsWith('http') ? path : serverBase() + path
  const tok = getToken()
  if (!tok) return full
  return full + (full.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(tok)
}

export const posterUrl = (it: Item) => mediaUrl(`/api/items/${it.id}/poster`) + `&v=${it.updatedAt}`
// '' when the item has no picture at all (nothing to ask the server for).
export const backdropUrl = (it: Item) => (it.backdrop || it.poster ? mediaUrl(`/api/items/${it.id}/backdrop`) + `&v=${it.updatedAt}` : '')
export const showPosterUrl = (s: Show) => mediaUrl(`/api/shows/${s.id}/art/poster`) + `&v=${s.updatedAt}`
export const showBackdropUrl = (s: Show) => mediaUrl(`/api/shows/${s.id}/art/backdrop`) + `&v=${s.updatedAt}`
export const seasonPosterUrl = (s: Show, n: number) => mediaUrl(`/api/shows/${s.id}/art/season`) + `&n=${n}&v=${s.updatedAt}`
export const artPreviewUrl = (src: string) => mediaUrl(`/api/artwork/preview?src=${encodeURIComponent(src)}`)

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
  const h = Math.floor(sec / 3600), m = Math.round((sec % 3600) / 60)
  return h > 0 ? `${h} h ${m} min` : `${m} min`
}

export const isDisc = (it: Item) => it.kind === 'dvd-iso' || it.kind === 'dvd-folder' || it.kind === 'bluray-iso' || it.kind === 'bluray-folder'
export const isBluray = (it: Item) => it.kind === 'bluray-iso' || it.kind === 'bluray-folder'
// The kind of disc, for labels: "DVD" or "Blu-ray".
export const discName = (it: Item) => (isBluray(it) ? 'Blu-ray' : 'DVD')
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
