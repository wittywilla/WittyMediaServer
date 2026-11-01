import { api, getToken, mediaUrlQ, serverBase, type Item, type Show } from './api'
import type { LogoInfo } from './components/BrandLogo'

// What the edit dialog, the fix-matches page and the library pages ask the server.
// docs/METADATA.md says what each of these does.

export type Target = 'item' | 'show'
export type MetaField = { value: string; source: string; sourceLabel: string; locked: boolean; canRevert: boolean; revertTo?: string; revertFrom?: string }
export type AttachedTitle = { id: number; title: string; extraType?: string; label?: string; width?: number; height?: number; sizeBytes?: number; runtimeSec?: number; updatedAt: number }
export type MoveInfo = { can: boolean; why?: string; options: { id: number; name: string; type: string }[]; moved?: { from: number; name: string } }
export type TitleEditor = {
  target: Target; id: number; libraryId: number; name: string; file: string; kind: 'movie' | 'show' | 'episode'; updatedAt: number; order: string[]
  fields: Record<string, MetaField>
  match: { source: string; sourceLabel: string; id: string; url?: string; state: '' | 'manual' | 'none'; confidence: number; reasons: string[] }
  link?: { link: 'extra' | 'version'; extraType?: string; parentItem?: number; parentShow?: number; label?: string; parentName: string }
  extras: AttachedTitle[]; versions: AttachedTitle[]; move: MoveInfo; brands: LogoInfo[]; introAuto: boolean; runtimeSec: number; extraTypes: string[]; disc: boolean
}
export type MatchCandidate = { source: string; id: string; kind: 'movie' | 'show'; title: string; year?: number; rating?: number; overview?: string; poster?: string; url?: string; score: number; reasons?: string[] }
export type SearchResult = { results: MatchCandidate[]; warnings: string[]; sources: string[]; exact: boolean }
export type FieldChange = { field: string; label: string; current: string; incoming: string; source: string; locked: boolean; change: boolean; apply: boolean; note?: string }
export type MatchPreview = {
  source: string; id: string; kind: string; url?: string; sourceLabel: string; title: string; changes: FieldChange[]; unchanged: number
  episodes?: { total: number; known: number; unknown: string[] }
}
export type ArtKind = 'poster' | 'backdrop' | 'banner' | 'logo'
export type ArtChoice = { url: string; source: string; sourceLabel: string; selected: boolean }
export type QueueRow = {
  target: Target; id: number; libraryId: number; title: string; year?: number; file: string; kind: 'movie' | 'show'; provider: string; providerId: string; sourceLabel: string; url?: string
  poster: string; updatedAt: number; confidence: number; reasons: string[]; state: 'unmatched' | 'low' | 'ok' | 'manual' | 'none'; suggestions?: MatchCandidate[]
}
export type QueueResult = { rows: QueueRow[]; total: number; counts: Record<string, number> }
export type HistoryEntry = { batch: string; at: number; user: string; action: string; title: string; count: number; undone: boolean }
export type MatchRule = { id?: number; libraryId?: number; pattern: string; lookup: string; tags: string[]; extra: string; enabled: boolean }
export type RuleTry = { file: string; result: { title?: string; year?: number; edition?: string[]; source?: string; id?: string; lookup?: string; tags?: string[]; extra?: string } }
export type EpisodeRow = { id: number; file: string; title: string; season?: number | null; episode?: number | null; episodeEnd?: number | null; locked: boolean; known?: string }
export type EpisodeMap = { rows: EpisodeRow[]; source: string; sourceLabel: string; listed: { season: number; episode: number; title: string; aired?: string }[]; note?: string }

export type Facet = { value: string; label?: string; count: number }
export type Facets = { genres: Facet[]; studios: Facet[]; networks: Facet[]; decades: Facet[]; years: Facet[]; content: Facet[]; resolutions: Facet[]; audio: Facet[]; subtitles: Facet[]; formats: Facet[]; tags: Facet[]; editions: Facet[]; kinds: Facet[]; platforms: Facet[] }
export type BrowseRow = { type: 'item'; item: Item } | { type: 'show'; show: Show }
export type BrowseResult = { results: BrowseRow[]; total: number }
export type CategoryArt = { type: 'item' | 'show'; id: number; v: number }
export type Category = { key: string; name: string; genre: string; count: number; hidden: boolean; position: number; merged?: string[]; art: CategoryArt[] }
export type Brand = { kind: 'studio' | 'network'; name: string; key: string; count: number }
export type Platform = { key: string; name: string; count: number }
export type Shelves = { categories: Category[]; studios: Brand[]; networks: Brand[]; collections: { id: number; key?: string; name: string; count: number; art: CategoryArt[] }[]; platforms: Platform[] }
export type SavedView = { id: number; libraryId: number; name: string; query: string; pinned: boolean; position: number }
export type ListingStatus = {
  enabled: boolean; region: string; asOf: number; newest: number; hasKey: boolean; titles: number; listed: number; running: boolean; done: number; total: number
  source: string; attribution: string; acknowledgements: string[]
}

export const listOf = (v: string) => (v ? v.split('|').filter(Boolean) : [])
export const joinOf = (l: string[]) => l.join('|')

// M:SS or H:MM:SS, or plain seconds. '' for what isn't a time.
export function parseClock(s: string): number | null {
  const t = s.trim()
  if (!t) return null
  if (!/^\d+(:\d{1,2}){0,2}(\.\d+)?$/.test(t)) return NaN
  return t.split(':').reduce((a, x) => a * 60 + Number(x), 0)
}

export const fmtClock = (sec: number) => {
  const s = Math.round(sec)
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60
  return (h ? `${h}:${String(m).padStart(2, '0')}` : `${m}`) + `:${String(r).padStart(2, '0')}`
}

export const artPictureUrl = (a: CategoryArt) => mediaUrlQ(a.type === 'item' ? `/api/items/${a.id}/backdrop` : `/api/shows/${a.id}/art/backdrop`, `v=${a.v}`)

// Sends a picture as the body of a request, as Upload your own and a logo's own upload do.
export async function sendPicture<T>(path: string, f: File): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': f.type || 'application/octet-stream' }
  const tok = getToken()
  if (tok) headers.Authorization = 'Bearer ' + tok
  const res = await fetch(`${serverBase()}/api${path}`, { method: 'PUT', headers, body: f, credentials: 'same-origin' })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || 'That picture could not be saved')
  return data as T
}

export const undoBatch = (batch: string) => api.post<{ undone: number }>('/meta/undo', { batch })
