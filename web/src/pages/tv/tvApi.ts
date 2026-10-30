import { api, ApiError, mediaUrl, serverBase, getToken } from '../../api'

// What the TV pages talk to the server about. See docs/CHANNELS.md.

export type Programme = {
  start: number; end: number; kind: 'show' | 'filler' | 'slate' | 'broadcast' | 'none'
  title: string; sub?: string; desc?: string; genre?: string; rating?: string; year?: number
  itemId?: number; showId?: number; season?: number; episode?: number
}
// Whether a channel is encrypted, in the server's words: every client shows the same.
export type Security = { state: 'clear' | 'encrypted' | 'unknown'; label: string; detail: string; reason?: string; helpUrl?: string }
export type Alternative = { key: string; number: string; name: string; atsc3?: boolean }
export type GuideChannel = {
  key: string; kind: 'library' | 'tuner'; id: number; number: string; name: string; group?: string; logo?: string
  hd?: boolean; radio?: boolean; encrypted?: string; canRecord?: boolean; noRecord?: string; atsc3?: boolean
  security?: Security; alternative?: Alternative; notices?: string[]; programmes: Programme[]
}
export type EncryptedHelpPage = {
  title: string; sections: { heading: string; paragraphs?: string[]; bullets?: string[] }[]
  devicesIntro: string; devices: { name: string; facts: string; source: string; url: string }[]; checked: string; note: string
}
export type GuidePage = { now: number; from: number; to: number; total: number; offset: number; channels: GuideChannel[]; groups: string[]; skin: Skin; layout: 'classic' | 'streaming' }
export type Skin = 'cable' | 'satellite' | 'theme'
export type TuneInfo = {
  key: string; kind: 'library' | 'tuner'; number: string; name: string; logo?: string; url: string
  now: Programme | null; next: Programme | null; positionSec: number; serverTime: number
  countProgress: boolean; canRecord: boolean; bufferSec: number
  atsc3?: boolean; security?: Security; notices?: string[]; alternative?: Alternative
}

export type Rules = {
  libraries?: number[]; kinds?: string[]; genres?: string[]; excludeGenres?: string[]; networks?: string[]; studios?: string[]
  yearFrom?: number; yearTo?: number; decades?: number[]; maxAge?: number; allowUnrated?: boolean; ratings?: string[]; minScore?: number
  shows?: number[]; excludeShows?: number[]; items?: number[]; excludeItems?: number[]; seasonFrom?: number; seasonTo?: number
  includeSpecials?: boolean; watched?: '' | 'watched' | 'unwatched'; watchedBy?: number; minMinutes?: number; maxMinutes?: number; search?: string
}
export type Order = { mode: '' | 'rotate' | 'sequential' | 'aired' | 'shuffle'; keepEpisodeOrder?: boolean }
export type Block = { id: string; name?: string; days?: number[]; start: string; end: string; rules?: Rules; order?: Order; fit?: '' | 'fill' | 'overrun' | 'cut' }
export type Definition = {
  mode: 'simple' | 'rules' | 'timetable'; rules: Rules; order: Order; blocks: Block[] | null
  filler: { rules?: Rules; between?: number; maxSeconds?: number; commercials?: CommercialPick[] }; alignMin: number
}
export type CommercialPick = { library: number; only?: boolean; items?: number[] }
export type Visibility = { mode: '' | 'adults' | 'only'; users?: number[] }
export type Channel = {
  id: number; number: string; name: string; description: string; enabled: boolean; definition: Definition
  rev: number; logo: string; visibility: Visibility; countProgress: boolean; updatedAt: number
}
export type PoolStats = { titles: number; shows: number; seconds: number }
export type Suggestion = { key: string; group: 'network' | 'era' | 'genre' | 'show' | 'classic'; name: string; description: string; definition: Definition; stats: PoolStats; exists: boolean }
export type NetworkFact = { name: string; shows: number; episodes: number; slots: number; fromYear: number; toYear: number; decades: number[] }
export type EditorOptions = { networks: NetworkFact[]; genres: string[]; studios: string[]; ratings: string[]; decades: number[]; shows: { id: number; title: string; year?: number; network?: string; seasons: number }[] }

export type DriverField = { key: string; label: string; kind: 'text' | 'url' | 'path' | 'password' | 'select' | 'number' | 'textarea' | 'toggle'; help?: string; default?: string; required?: boolean; options?: string[] }
export type Driver = { id: string; name: string; description: string; status: 'tested' | 'mock' | 'untested' | 'planned'; platforms?: string[]; fields: DriverField[]; scans: boolean; planned?: boolean; available: boolean; note?: string }
export type Tuner = { id: number; driver: string; name: string; options: Record<string, string>; enabled: boolean; maxStreams: number; lastScan: number | null; lastError: string; detail: string; channels: number; encrypted: number; clear: number; inUse: number }
export type TunerChannel = { id: number; tunerId: number; sourceId: string; number: string; name: string; group?: string; hd?: boolean; radio?: boolean; encrypted?: string; security: Security['state']; atsc3?: boolean; videoCodec?: string; audioCodec?: string; noRecord?: string; enabled: boolean; epgId?: string; numberSet?: string; nameSet?: string; epgSet?: string }
export type ScanState = { running: boolean; percent: number; found: number; message?: string; error?: string; added: number; clear: number; encrypted: number; unknown: number }
export type Found = { driver: string; name: string; detail?: string; options: Record<string, string> }
export type EPGSource = { id: number; name: string; url: string; lastFetch: number | null; lastError: string; programmes: number; tunerId?: number }
export type Recording = { id: number; channelId: number; channel: string; number: string; title: string; sub?: string; start: number; stop: number; status: 'scheduled' | 'recording' | 'done' | 'failed' | 'cancelled'; error?: string; path?: string; sizeBytes?: number; ruleId?: number }
export type RecordingRule = { id: number; channelId: number; channel: string; title: string }

export const emptyDefinition = (): Definition => ({ mode: 'simple', rules: {}, order: { mode: 'rotate' }, blocks: [], filler: {}, alignMin: 30 })

export const logoSrc = (u?: string) => (u ? (u.startsWith('/') ? mediaUrl(u) : u) : '')

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

export const fmtClock = (u: number) => new Date(u * 1000).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
export const fmtDay = (u: number) => new Date(u * 1000).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
export const fmtSpan = (p: Programme) => `${fmtClock(p.start)} to ${fmtClock(p.end)}`
export const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
export const fullDay = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

