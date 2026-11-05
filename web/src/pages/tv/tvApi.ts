import type { LogoKind } from '../../components/BrandLogo'
import { api, ApiError, mediaUrl, serverBase, getToken } from '../../api'

// What the TV pages talk to the server about. See docs/CHANNELS.md.

export type ClipRef = { title: number; start: number; end: number; label?: string }
export type Programme = {
  start: number; end: number; kind: 'show' | 'filler' | 'slate' | 'broadcast' | 'none' | 'group'
  title: string; sub?: string; desc?: string; genre?: string; rating?: string; year?: number
  itemId?: number; showId?: number; season?: number; episode?: number
  // Why a stretch of bumpers is there, and how much was cut to keep the timetable.
  role?: 'pre' | 'post' | 'break' | 'ad' | 'between' | 'pad'; rule?: string; mandatory?: boolean; offset?: number; trimmed?: number
  clip?: ClipRef
  // Very short programmes the grid has no room to draw one by one.
  group?: Programme[]
}
// Whether a channel is encrypted, in the server's words: every client shows the same.
export type Security = { state: 'clear' | 'encrypted' | 'unknown'; label: string; detail: string; reason?: string; helpUrl?: string }
export type Alternative = { key: string; number: string; name: string; atsc3?: boolean; canRecord: boolean }
export type GuideChannel = {
  key: string; kind: 'library' | 'tuner'; id: number; number: string; name: string; group?: string; logo?: string
  // The server's say on whether to offer Record: false for every library channel. See canOfferRecord.
  canRecord: boolean
  hd?: boolean; radio?: boolean; encrypted?: string; noRecord?: string; atsc3?: boolean
  security?: Security; alternative?: Alternative; notices?: string[]; programmes: Programme[]
  description?: string; categories?: string[]; accent?: string; favourite?: boolean; brand?: ChannelBrand
}
// The name a channel's logo goes by in the logo service.
export type ChannelBrand = { kind: 'network' | 'studio'; name: string }
export type ChannelLook = { accent?: string; logoKind?: '' | 'network' | 'studio' | 'none'; logoName?: string }
export type EncryptedHelpPage = {
  title: string; sections: { heading: string; paragraphs?: string[]; bullets?: string[] }[]
  devicesIntro: string; devices: { name: string; facts: string; source: string; url: string }[]; checked: string; note: string
}
export type GuidePage = {
  now: number; from: number; to: number; total: number; offset: number; channels: GuideChannel[]; groups: string[]; skin: Skin; layout: 'classic' | 'streaming'; theme?: string
  // What a device that hasn't chosen does with "Keep playing when this tab is in the background".
  backgroundPlay?: boolean
}
export type Skin = 'cable' | 'satellite' | 'midnight' | 'theme'
export type TuneInfo = {
  key: string; kind: 'library' | 'tuner'; number: string; name: string; logo?: string; url: string
  now: Programme | null; next: Programme | null; playing?: Programme; description?: string; accent?: string; brand?: ChannelBrand; positionSec: number; serverTime: number
  // On a channel that counts watch progress: when what is on the screen now ends,
  // a stretch of `now` or a bumper or commercial in it.
  partEnd?: number
  countProgress: boolean; canRecord: boolean; bufferSec: number; backgroundPlay?: boolean
  atsc3?: boolean; security?: Security; notices?: string[]; alternative?: Alternative
}

export type Rules = {
  libraries?: number[]; kinds?: string[]; genres?: string[]; excludeGenres?: string[]; networks?: string[]; studios?: string[]
  yearFrom?: number; yearTo?: number; decades?: number[]; maxAge?: number; allowUnrated?: boolean; ratings?: string[]; minScore?: number
  shows?: number[]; excludeShows?: number[]; items?: number[]; excludeItems?: number[]; collections?: number[]; clips?: DiscClip[]; seasonFrom?: number; seasonTo?: number
  includeSpecials?: boolean; watched?: '' | 'watched' | 'unwatched'; watchedBy?: number; minMinutes?: number; maxMinutes?: number; search?: string
}
export type Order = { mode: '' | 'rotate' | 'sequential' | 'aired' | 'shuffle'; keepEpisodeOrder?: boolean }
export type DiscClip = { id: string; item: number; title?: number; start?: number; end?: number; fromChapter?: number; toChapter?: number; label?: string }
export type Block = { id: string; name?: string; days?: number[]; start: string; end: string; rules?: Rules; order?: Order; fit?: '' | 'fill' | 'overrun' | 'cut'; fromDate?: string; toDate?: string; marathon?: boolean }
export type BumperWhen = 'before' | 'after' | 'between' | 'every'
export type BumperRule = { id: string; name?: string; off?: boolean; when: BumperWhen; everyMin?: number; source: Rules; count?: number; mandatory?: boolean; only?: Rules }
export type TrimChoice = '' | 'cut' | 'drop' | 'keep'
export type Trim = { on?: boolean; ads?: TrimChoice; bumpers?: TrimChoice; minKeepSec?: number; tailSec?: number }
export type BumperPlan = { rules?: BumperRule[]; trim?: Trim }
export type Definition = {
  mode: 'simple' | 'rules' | 'timetable'; rules: Rules; order: Order; blocks: Block[] | null
  filler: { rules?: Rules; between?: number; maxSeconds?: number; commercials?: CommercialPick[]; breakEveryMin?: number; breakAds?: number }; alignMin: number
  bumpers?: BumperPlan
}
export type CommercialPick = { library: number; only?: boolean; items?: number[] }
export type Visibility = { mode: '' | 'adults' | 'only'; users?: number[] }
export type Channel = {
  id: number; number: string; name: string; description: string; enabled: boolean; definition: Definition
  rev: number; logo: string; visibility: Visibility; countProgress: boolean; updatedAt: number; categories: string[]; look: ChannelLook
}
export type PoolStats = { titles: number; shows: number; seconds: number }
export type Suggestion = { key: string; group: 'network' | 'era' | 'genre' | 'studio' | 'collection' | 'show' | 'classic'; name: string; description: string; definition: Definition; stats: PoolStats; exists: boolean }
export type NetworkFact = { name: string; shows: number; episodes: number; slots: number; fromYear: number; toYear: number; decades: number[] }
export type EditorOptions = { networks: NetworkFact[]; genres: string[]; studios: string[]; ratings: string[]; decades: number[]; shows: { id: number; title: string; year?: number; network?: string; seasons: number }[]; collections: { id: number; name: string; items: number }[] }
export type Bumper = { itemId: number; title: string; durationSec: number; library: string; rating?: string; uploaded: boolean; chosen: boolean; updatedAt: number }
export type DiscOption = { id: number; title: string; year?: number; format: 'dvd' | 'bluray'; scrambled: boolean; mainTitle: number; runtimeSec: number; updatedAt: number; titles: number }
export type DiscChapter = { n: number; start: number; label: string }
export type DiscTitle = { ttn: number; label: string; durationSec: number; chapters: DiscChapter[]; wide: boolean }
export type DiscPage = { main: DiscTitle | null; extras: DiscTitle[] }
export type GuideHit = Programme & { channel: string; number: string; name: string }
export type Reminder = { id: number; channel: string; title: string; sub?: string; start: number; end: number }

export type DriverField = { key: string; label: string; kind: 'text' | 'url' | 'path' | 'password' | 'select' | 'number' | 'textarea' | 'toggle'; help?: string; default?: string; required?: boolean; options?: string[] }
export type Driver = { id: string; name: string; description: string; status: 'tested' | 'mock' | 'untested' | 'planned'; platforms?: string[]; fields: DriverField[]; scans: boolean; planned?: boolean; available: boolean; note?: string }
export type Tuner = { id: number; driver: string; name: string; options: Record<string, string>; enabled: boolean; maxStreams: number; lastScan: number | null; lastError: string; detail: string; channels: number; encrypted: number; clear: number; inUse: number }
export type TunerChannel = { logoSet?: string; logoUrl?: string; brand?: { kind: LogoKind; name: string }; id: number; tunerId: number; sourceId: string; number: string; name: string; group?: string; hd?: boolean; radio?: boolean; encrypted?: string; security: Security['state']; atsc3?: boolean; videoCodec?: string; audioCodec?: string; noRecord?: string; enabled: boolean; epgId?: string; numberSet?: string; nameSet?: string; epgSet?: string }
export type ScanState = { running: boolean; percent: number; found: number; message?: string; error?: string; added: number; clear: number; encrypted: number; unknown: number }
export type Found = { driver: string; name: string; detail?: string; options: Record<string, string> }
// The guided setup's questions: what a source says before it's saved, and what one channel did when tried.
export type TunerProblem = { kind: 'settings' | 'unreachable' | 'not-a-tuner' | 'unreadable' | 'empty' | 'encrypted' | 'busy' | 'no-signal' | 'no-picture' | 'other'; message: string; hint?: string; helpUrl?: string }
export type TunerCheck = { ok: true; result: { detail?: string; maxStreams?: number; channels: number; encrypted: number; sample?: string[]; ownGuide?: boolean } } | { ok: false; problem: TunerProblem }
export type TunerTest = { ok: boolean; channel?: { id: number; number: string; name: string }; problem?: TunerProblem; seconds: number; video?: string; audio?: string; clear?: boolean }
export type EPGSource = { id: number; name: string; url: string; lastFetch: number | null; lastError: string; programmes: number; tunerId?: number }
export type Recording = { id: number; channelId: number; channel: string; number: string; title: string; sub?: string; start: number; stop: number; status: 'scheduled' | 'recording' | 'done' | 'failed' | 'cancelled'; error?: string; path?: string; sizeBytes?: number; ruleId?: number }
export type RecordingRule = { id: number; channelId: number; channel: string; title: string }

// Whose logo a channel wears when nobody chose: the one network or studio it plays.
export function brandOf(def: Definition, look?: ChannelLook): ChannelBrand | undefined {
  if (look?.logoKind === 'none') return undefined
  if (look?.logoName && (look.logoKind === 'network' || look.logoKind === 'studio')) return { kind: look.logoKind, name: look.logoName }
  const r = def.rules
  if (r.networks?.length === 1 && !r.studios?.length) return { kind: 'network', name: r.networks[0] }
  if (r.studios?.length === 1 && !r.networks?.length) return { kind: 'studio', name: r.studios[0] }
  return undefined
}

export const emptyDefinition = (): Definition => ({ mode: 'simple', rules: {}, order: { mode: 'rotate' }, blocks: [], filler: {}, alignMin: 30 })

// Logos only ever come from this server: another site would see the viewer's address.
export const logoSrc = (u?: string) => (u?.startsWith('/') ? mediaUrl(u) : '')

// How a stream address answers before the player gets it: a refused or broken
// channel says why, in words. This also waits for the first picture.
export async function preflight(url: string, signal: AbortSignal): Promise<void> {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(serverBase() + url, { headers, credentials: 'same-origin', signal })
  if (res.ok) return
  let msg = res.statusText
  try { msg = (await res.json()).error || msg } catch { /* not JSON */ }
  throw new Error(msg)
}

// A name for one player, so the server can tell when it has gone.
export const makeViewer = () => Math.random().toString(36).slice(2, 12)

// Tunes a channel. A tuner that another player has just let go of can still be
// busy for a moment, so "busy" is tried again before it is believed.
export async function tune(key: string, viewer: string, signal: AbortSignal): Promise<TuneInfo> {
  for (let i = 0; ; i++) {
    try {
      return await api.get<TuneInfo>(`/tv/${key}/tune?viewer=${viewer}`)
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 503 || i >= 3 || signal.aborted) throw e
      await new Promise((r) => setTimeout(r, 1500))
    }
  }
}

// This player has gone: the server stops a channel nobody is left watching.
export function leaveChannel(key: string, viewer: string) {
  const headers: Record<string, string> = {}
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  fetch(serverBase() + `/api/tv/${key}/leave?viewer=${viewer}`, { method: 'POST', headers, credentials: 'same-origin', keepalive: true }).catch(() => {})
}

export async function uploadLogo(id: number, file: File): Promise<{ logo: string }> {
  const headers: Record<string, string> = { 'Content-Type': file.type || 'application/octet-stream' }
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(serverBase() + `/api/channels/${id}/logo`, { method: 'PUT', headers, credentials: 'same-origin', body: file })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || res.statusText)
  return data
}

// Every channel's number, for dialling and CH+/CH-. Kept for a minute: the
// list hardly ever changes, and it can be long with IPTV.
let lineup: { at: number; list: Promise<GuideChannel[]> } | null = null
export function getLineup(): Promise<GuideChannel[]> {
  if (!lineup || Date.now() - lineup.at > 60000) {
    lineup = { at: Date.now(), list: api.get<GuidePage>('/guide?list=1&limit=20000').then((g) => g.channels) }
    lineup.list.catch(() => { lineup = null })
  }
  return lineup.list
}
export const forgetLineup = () => { lineup = null }

// Sends a file as the whole body of a request, as uploads of one file do here.
export async function uploadBumper(file: File): Promise<{ itemId: number; title: string; durationSec: number }> {
  const headers: Record<string, string> = { 'Content-Type': file.type || 'application/octet-stream' }
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(serverBase() + `/api/channels/bumpers/upload?name=${encodeURIComponent(file.name)}`, { method: 'POST', headers, credentials: 'same-origin', body: file })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || res.statusText)
  return data
}

export const setFavourite = (key: string, on: boolean) => (on ? api.put(`/tv/favourites/${key}`, {}) : api.del(`/tv/favourites/${key}`))

// canOfferRecord is the one test for showing Record or Record every airing, the R key included:
// the server's canRecord, and a programme from the guide that hasn't finished. The server
// refuses anything else (docs/CLIENT-API.md, "Recording").
export const canOfferRecord = (ch: { canRecord?: boolean } | null | undefined, p: Programme | null | undefined, now: number): boolean =>
  ch?.canRecord === true && p?.kind === 'broadcast' && p.end > now

export const fmtClock = (u: number) => new Date(u * 1000).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
export const fmtDay = (u: number) => new Date(u * 1000).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
export const fmtSpan = (p: Programme) => `${fmtClock(p.start)} to ${fmtClock(p.end)}`
// "1h 28m left": how long, in the fewest words that are still exact enough.
export function fmtLeft(sec: number): string {
  const m = Math.max(1, Math.round(sec / 60))
  if (m < 60) return `${m} min`
  const h = Math.floor(m / 60)
  return m % 60 ? `${h}h ${m % 60}m` : `${h}h`
}
export function fmtTimecode(sec: number): string {
  const s = Math.max(0, Math.round(sec)), h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}
// Reads 1:02:03, 2:03 or 123 as seconds; null when it isn't a time.
export function parseTimecode(text: string): number | null {
  const parts = text.trim().split(':')
  if (!text.trim() || parts.length > 3 || parts.some((p) => !/^\d+(\.\d+)?$/.test(p))) return null
  return parts.reduce((t, p) => t * 60 + Number(p), 0)
}
// "Tonight", "Tomorrow", or the day's name: as someone would say it.
export function dayWord(u: number, now: number): string {
  const day = (x: number) => { const d = new Date(x * 1000); d.setHours(0, 0, 0, 0); return d.getTime() / 1000 }
  const diff = Math.round((day(u) - day(now)) / 86400)
  if (diff === 0) return new Date(u * 1000).getHours() >= 17 ? 'Tonight' : 'Today'
  if (diff === 1) return 'Tomorrow'
  if (diff === -1) return 'Yesterday'
  return new Date(u * 1000).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
}
export const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
export const fullDay = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

