// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { api, ApiError } from '../../../api'
import { date, num, t, tk, tn } from '../../../i18n'
import { plugAppWords } from './words'
import type { ModuleStatus, VersionInfo } from '../../../moduleStore'
import type { IntegrationSummary, OverrideRecord, UiSummary } from './overrides'

// What the module store's pages ask the server. Everything a page shows about a module comes
// from here, including its pictures: a page never fetches from an address in a catalogue, and
// all of a catalogue's words are plain text, shown as text.

// The store's files that are tested on their own speak through words.ts: give them the app's language.
plugAppWords(t, tn)

export type Failure = { kind: string; title: string; message: string; try: string[]; detail?: string; repair?: boolean }
export type StoreInfo = {
  id: string; name: string; host: string; url: string; official?: boolean; fingerprint?: string; private?: boolean; enabled: boolean
  addedAt?: number; addedBy?: string; serial?: number; generated?: string; checkedAt?: number; modules: number; problem?: Failure; inUse?: string[]
}
export type ItemKind = 'official' | 'fork' | 'in-development' | 'refused' | 'server'
export type Fit = { ok: boolean; kind: string; why?: string }
export type StoreItem = {
  key: string; store: string; storeName: string; storeHost: string; official: boolean; kind: ItemKind; why?: string; lookalike?: string
  id: string; name: string; summary?: string; description?: string; group?: string; publisher?: { name: string; url?: string }; repo?: string; icon?: string; screenshots?: string[]
  libraries?: string[]; requires?: string[]; needs?: string[]; core?: boolean; updates?: string; stays?: string
  platforms?: string[]; delivery?: string; version?: string; released?: string; size?: number; license?: string; permissions?: string[]; changelog?: string
  minServer?: string; maxServer?: string; fit: Fit; status?: ModuleStatus; failure?: Failure
  // What sort of module it is, how finished its release is, what its SDUI and its integration.json
  // add, and its override.
  moduleKind?: string; releaseStatus?: string; statusNote?: string; ui?: UiSummary; integration?: IntegrationSummary; override?: OverrideRecord
  // The version offered was made for an older server: fit kind 'older-server', its why the warning.
  olderServer?: boolean; madeFor?: string
}
export type RunItem = { id: string; name: string; from?: string; to?: string; state: 'queued' | 'working' | 'done' | 'failed' | 'skipped' | 'undone'; why?: string; phase?: string; done?: number; total?: number; failure?: Failure; olderServer?: boolean; madeFor?: string; warning?: string }
export type Run = { id: string; kind: string; by?: string; started: number; finished?: number; items: RunItem[]; updated: number; failed: number; skipped: number; canUndo?: boolean; undone?: boolean }
// How well a module's helper is boxed in on this system, in words when it isn't fully.
export type SandboxInfo = { level: 'full' | 'partial' | 'limited' | 'unconfined'; title?: string; words?: string }

export type StoreView = {
  server: string; api: number; minApi: number; policy: string; minServer: string; platform: string; checkedAt?: number; checkError?: string
  stores: StoreInfo[]; items: StoreItem[]; updates: number; run?: Run; repairRoute: string; sandbox?: SandboxInfo
}
export type ItemDetail = { item: StoreItem; versions: VersionInfo[]; plan: RunItem[]; permissions: { name: string; words: string }[]; builtOn: { id: string; name: string; installed: boolean }[]; forkWarning?: string; fingerprint?: string; policy: string; server: string }
export type LogLine = { at: number; level: string; text: string }
export type ModuleLog = { id: string; name: string; lines: LogLine[]; text: string; failure?: Failure }
export type StorePreview = {
  url: string; host: string; name: string; serial: number; generated: string; key: string; fingerprint: string; keySource: 'pasted' | 'offered'; private?: boolean
  modules: { id: string; name: string; summary?: string; kind: string; why?: string }[]; count: number; warning: string[]
}

// The store's view, made safe to draw. A catalogue is someone else's file passed on by the server, so
// a page never trusts its shape: a list that isn't a list is empty, an entry with no id or name is left
// out, and text fields that aren't text are dropped. An entry with no answer on whether it fits is
// treated as not fitting, with a reason, rather than offered for install.
const str = (v: unknown) => (typeof v === 'string' ? v : undefined)
const strs = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : undefined)
export function cleanItem(raw: unknown): StoreItem | null {
  if (!raw || typeof raw !== 'object') return null
  const it = raw as Record<string, unknown>
  const id = str(it.id), name = str(it.name) || id
  if (!id || !name) return null
  const fit = it.fit && typeof it.fit === 'object' && typeof (it.fit as Fit).kind === 'string'
    ? { ok: (it.fit as Fit).ok === true, kind: (it.fit as Fit).kind, why: str((it.fit as Fit).why) }
    : { ok: false, kind: 'unreadable', why: t('The store’s entry for it couldn’t be read.') }
  const store = str(it.store) || 'official'
  const pub = it.publisher && typeof it.publisher === 'object' ? { name: str((it.publisher as { name?: unknown }).name) || '', url: str((it.publisher as { url?: unknown }).url) } : undefined
  return {
    ...(it as unknown as StoreItem), id, name, store, key: str(it.key) || `${store}/${id}`, fit,
    storeName: str(it.storeName) || '', storeHost: str(it.storeHost) || '', official: it.official === true,
    kind: (str(it.kind) || 'official') as ItemKind, summary: str(it.summary), description: str(it.description), group: str(it.group),
    publisher: pub?.name ? pub : undefined, version: str(it.version), released: str(it.released), changelog: str(it.changelog),
    size: typeof it.size === 'number' && isFinite(it.size) ? it.size : undefined,
    libraries: strs(it.libraries), requires: strs(it.requires), needs: strs(it.needs), platforms: strs(it.platforms), permissions: strs(it.permissions), screenshots: strs(it.screenshots),
    status: it.status && typeof it.status === 'object' ? it.status as ModuleStatus : undefined,
  }
}
export function cleanView(raw: unknown): StoreView {
  const v = (raw && typeof raw === 'object' ? raw : {}) as Partial<StoreView>
  const items = (Array.isArray(v.items) ? v.items : []).map(cleanItem).filter((x): x is StoreItem => !!x)
  const stores = (Array.isArray(v.stores) ? v.stores : []).filter((s) => s && typeof s === 'object' && typeof s.id === 'string')
  return {
    ...(v as StoreView), items, stores, server: str(v.server) || '', policy: str(v.policy) || '', platform: str(v.platform) || '',
    updates: typeof v.updates === 'number' ? v.updates : items.filter((i) => i.status?.installed && i.status.latest).length, repairRoute: str(v.repairRoute) || '/settings/repair',
    run: v.run && typeof v.run === 'object' && Array.isArray(v.run.items) ? v.run : undefined,
  }
}

export const storeView = () => api.get<unknown>('/modulestore').then(cleanView)
export const itemDetail = (store: string, id: string) => api.get<ItemDetail>(`/modulestore/item/${encodeURIComponent(store)}/${encodeURIComponent(id)}`)
export const checkAll = () => api.post<unknown>('/modulestore/check').then(cleanView)
export const startUpdateAll = () => api.post<Run>('/modulestore/update-all')
export const currentRun = () => api.get<{ run?: Run }>('/modulestore/run').then((r) => r.run)
export type OwnerSecrets = { password: string; code: string }
export const undoRun = (secrets: OwnerSecrets) => api.post<Run>('/modulestore/run/undo', secrets)
export const dismissRun = () => api.del('/modulestore/run')
// more can carry an override's go-ahead, and confirmOlder: true once someone chose Install anyway
// for a version made for an older server (OlderServerDialog). Nothing else sends it.
export const installModule = (id: string, confirm: boolean, more?: object) => api.post<Run>('/modulestore/install', { id, confirm, ...more })
export const moduleLog = (id: string) => api.get<ModuleLog>(`/modulestore/modules/${encodeURIComponent(id)}/log`)
export const previewStore = (url: string, key: string, allowPrivate: boolean, secrets?: OwnerSecrets) => api.post<StorePreview>('/modulestore/stores/preview', { url, key, private: allowPrivate, ...secrets })
export const addStore = (url: string, key: string, allowPrivate: boolean, agreed: boolean, secrets: OwnerSecrets) => api.post<StoreInfo>('/modulestore/stores', { url, key, private: allowPrivate, agreed, ...secrets })
export const removeStore = (id: string) => api.del(`/modulestore/stores/${encodeURIComponent(id)}`)
export const setStoreOn = (id: string, on: boolean) => api.put<StoreInfo>(`/modulestore/stores/${encodeURIComponent(id)}/enabled`, { on })
export const checkStore = (id: string) => api.post<StoreInfo>(`/modulestore/stores/${encodeURIComponent(id)}/check`)
export const useFork = (store: string, id: string, confirm: boolean, fingerprint: string | undefined, secrets: OwnerSecrets & { override?: object }) => api.post<ItemDetail>(`/modulestore/stores/${encodeURIComponent(store)}/fork/${encodeURIComponent(id)}`, { confirm, fingerprint, ...secrets })

// The server's words for a failure, or the error's own when it has none.
export function failureOf(e: unknown, name = t('This module')): Failure {
  if (e instanceof ApiError && e.data?.failure) return e.data.failure as Failure
  const message = e instanceof Error ? e.message : String(e)
  return { kind: 'other', title: t('{name} couldn’t be changed', { name }), message, try: [t('Try it again.')] }
}

// Read when shown, so the words follow the app's language.
export const PHASE: Record<string, string> = {
  get downloading() { return t('Downloading') }, get verifying() { return t('Checking the signature') }, get installing() { return t('Installing') },
}
export const isBusy = (m?: ModuleStatus) => !!m && !!PHASE[m.phase]
// One install tells its story on its own card. Only Update All leaves a summary to read.
export const showsRun = (r?: Run) => !!r && (r.kind === 'update-all' || !r.finished)
export const anyBusy =(v?: StoreView | null) => !!v && (v.items.some((i) => isBusy(i.status)) || (!!v.run && !v.run.finished))

// What's wrong with a module, from its status alone, for the installed list.
export function failureForStatus(m: ModuleStatus): Failure | null {
  const detail = m.phase === 'failed' ? m.error : m.notice
  if (!detail) return null
  return { kind: 'other', title: m.phase === 'failed' ? t('The last install didn’t work, and nothing changed') : t('This module needs a look'), message: detail, try: [t('Try it again, or look at this module’s log.')], detail, repair: true }
}

// The categories, in the order they're offered. A module in a group this build has no name for
// still shows, under Other. A catalogue chooses a group; it can't add a category's name.
// Each title is read when shown, so it follows the app's language.
export const CATEGORIES: { id: string; title: string }[] = [
  { id: 'discs', get title() { return t('Discs') } }, { id: 'games', get title() { return t('Games') } }, { id: 'tv', get title() { return t('TV') } },
  { id: 'read', get title() { return t('Reading') } }, { id: 'photos', get title() { return t('Photos') } }, { id: 'listen', get title() { return t('Listening') } },
  { id: 'watch', get title() { return t('Watching') } }, { id: 'tools', get title() { return t('Tools') } }, { id: 'stats', get title() { return t('Stats') } },
  { id: 'system', get title() { return t('System') } },
]
export const categoryTitle = (id?: string) => CATEGORIES.find((c) => c.id === id)?.title || t('Other')
export const categoryOf = (it: StoreItem) => CATEGORIES.some((c) => c.id === it.group) ? it.group! : 'other'

const PLATFORM_NAMES: Record<string, string> = {
  windows: 'Windows', darwin: 'macOS', linux: 'Linux', freebsd: 'FreeBSD',
  get amd64() { return t('64-bit') }, get arm64() { return t('ARM 64-bit') }, get arm() { return t('ARM 32-bit') }, get '386'() { return t('32-bit') }, get all() { return t('Every system') },
}
export const platformName = (p: string) => p === 'all' ? PLATFORM_NAMES.all : p.split('-').map((x) => PLATFORM_NAMES[x] || x).join(' ')

// A search matches every word, in the name, description, category or publisher, ignoring case
// and accents, so "blu ray" finds what "Blu-ray" describes.
const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().replace(/[-_]+/g, ' ')
export function matches(it: StoreItem, query: string): boolean {
  const words = fold(query).split(/\s+/).filter(Boolean)
  if (!words.length) return true
  const hay = fold([it.name, it.summary, it.description, categoryTitle(it.group), it.group, it.publisher?.name, it.id, ...(it.libraries || [])].filter(Boolean).join(' '))
  return words.every((w) => hay.includes(w))
}

// A better match comes first: the name, then the first line, then anywhere.
export function rank(it: StoreItem, query: string): number {
  const q = fold(query).trim()
  if (!q) return 0
  const name = fold(it.name)
  if (name === q) return 0
  if (name.startsWith(q)) return 1
  if (name.includes(q)) return 2
  if (fold(it.summary || '').includes(q)) return 3
  return 4
}

export type ChangelogBlock = { kind: 'p' | 'ul' | 'h'; lines: string[] }

// A changelog is plain text, often written as Markdown. Its lists and headings are kept and its
// other marks are taken off, and what comes out is only ever shown as text, never as markup.
export function changelogBlocks(text: string): ChangelogBlock[] {
  const out: ChangelogBlock[] = []
  const plain = (s: string) => s.replace(/\*\*|__|`/g, '').replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').trim()
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (!line) { out.push({ kind: 'p', lines: [] }); continue }
    const item = /^[-*+]\s+(.*)$/.exec(line)
    const head = /^#{1,6}\s+(.*)$/.exec(line)
    const last = out[out.length - 1]
    if (item) {
      if (last?.kind === 'ul') last.lines.push(plain(item[1]))
      else out.push({ kind: 'ul', lines: [plain(item[1])] })
    } else if (head) {
      out.push({ kind: 'h', lines: [plain(head[1])] })
    } else if (last?.kind === 'p' && last.lines.length) {
      last.lines.push(plain(line))
    } else {
      out.push({ kind: 'p', lines: [plain(line)] })
    }
  }
  return out.filter((b) => b.lines.length)
}

// What a person sees when a release has nothing notable to report, as on other app stores.
// Translated where it's shown, with tx().
export const NOTHING_NOTABLE = tk('Bug fixes and improvements.')

export const mb = (n = 0) => n >= 1048576
  ? t('{size} MB', { size: num(n / 1048576, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) })
  : t('{size} KB', { size: num(Math.max(1, Math.round(n / 1024))) })
export const dayOf = (s?: string) => { if (!s) return ''; const d = new Date(s + 'T12:00:00'); return isNaN(d.getTime()) ? s : date(d, { dateStyle: 'medium' }) }
