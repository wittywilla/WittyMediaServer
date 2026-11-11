// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { api, ApiError } from '../../../api'
import type { ModuleStatus, VersionInfo } from '../../../moduleStore'

// What the module store's pages ask the server. Everything a page shows about a module comes
// from here, including its pictures: a page never fetches from an address in a catalogue, and
// all of a catalogue's words are plain text, shown as text.

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
}
export type RunItem = { id: string; name: string; from?: string; to?: string; state: 'queued' | 'working' | 'done' | 'failed' | 'skipped' | 'undone'; why?: string; phase?: string; done?: number; total?: number; failure?: Failure }
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

export const storeView = () => api.get<StoreView>('/modulestore')
export const itemDetail = (store: string, id: string) => api.get<ItemDetail>(`/modulestore/item/${encodeURIComponent(store)}/${encodeURIComponent(id)}`)
export const checkAll = () => api.post<StoreView>('/modulestore/check')
export const startUpdateAll = () => api.post<Run>('/modulestore/update-all')
export const currentRun = () => api.get<{ run?: Run }>('/modulestore/run').then((r) => r.run)
export type OwnerSecrets = { password: string; code: string }
export const undoRun = (secrets: OwnerSecrets) => api.post<Run>('/modulestore/run/undo', secrets)
export const dismissRun = () => api.del('/modulestore/run')
export const installModule = (id: string, confirm: boolean) => api.post<Run>('/modulestore/install', { id, confirm })
export const moduleLog = (id: string) => api.get<ModuleLog>(`/modulestore/modules/${encodeURIComponent(id)}/log`)
export const previewStore = (url: string, key: string, allowPrivate: boolean, secrets?: OwnerSecrets) => api.post<StorePreview>('/modulestore/stores/preview', { url, key, private: allowPrivate, ...secrets })
export const addStore = (url: string, key: string, allowPrivate: boolean, agreed: boolean, secrets: OwnerSecrets) => api.post<StoreInfo>('/modulestore/stores', { url, key, private: allowPrivate, agreed, ...secrets })
export const removeStore = (id: string) => api.del(`/modulestore/stores/${encodeURIComponent(id)}`)
export const setStoreOn = (id: string, on: boolean) => api.put<StoreInfo>(`/modulestore/stores/${encodeURIComponent(id)}/enabled`, { on })
export const checkStore = (id: string) => api.post<StoreInfo>(`/modulestore/stores/${encodeURIComponent(id)}/check`)
export const useFork = (store: string, id: string, confirm: boolean, fingerprint: string | undefined, secrets: OwnerSecrets) => api.post<ItemDetail>(`/modulestore/stores/${encodeURIComponent(store)}/fork/${encodeURIComponent(id)}`, { confirm, fingerprint, ...secrets })

// The server's words for a failure, or the error's own when it has none.
export function failureOf(e: unknown, name = 'This module'): Failure {
  if (e instanceof ApiError && e.data?.failure) return e.data.failure as Failure
  const message = e instanceof Error ? e.message : String(e)
  return { kind: 'other', title: `${name} couldn’t be changed`, message, try: ['Try it again.'] }
}

export const PHASE: Record<string, string> = { downloading: 'Downloading', verifying: 'Checking the signature', installing: 'Installing' }
export const isBusy = (m?: ModuleStatus) => !!m && !!PHASE[m.phase]
// One install tells its story on its own card. Only Update All leaves a summary to read.
export const showsRun = (r?: Run) => !!r && (r.kind === 'update-all' || !r.finished)
export const anyBusy =(v?: StoreView | null) => !!v && (v.items.some((i) => isBusy(i.status)) || (!!v.run && !v.run.finished))

// What's wrong with a module, from its status alone, for the installed list.
export function failureForStatus(m: ModuleStatus): Failure | null {
  const detail = m.phase === 'failed' ? m.error : m.notice
  if (!detail) return null
  return { kind: 'other', title: m.phase === 'failed' ? 'The last install didn’t work, and nothing changed' : 'This module needs a look', message: detail, try: ['Try it again, or look at this module’s log.'], detail, repair: true }
}

// The categories, in the order they're offered. A module in a group this build has no name for
// still shows, under Other. A catalogue chooses a group; it can't add a category's name.
export const CATEGORIES: { id: string; title: string }[] = [
  { id: 'discs', title: 'Discs' }, { id: 'games', title: 'Games' }, { id: 'tv', title: 'TV' }, { id: 'read', title: 'Reading' }, { id: 'photos', title: 'Photos' },
  { id: 'listen', title: 'Listening' }, { id: 'watch', title: 'Watching' }, { id: 'tools', title: 'Tools' }, { id: 'stats', title: 'Stats' }, { id: 'system', title: 'System' },
]
export const categoryTitle = (id?: string) => CATEGORIES.find((c) => c.id === id)?.title || 'Other'
export const categoryOf = (it: StoreItem) => CATEGORIES.some((c) => c.id === it.group) ? it.group! : 'other'

const PLATFORM_NAMES: Record<string, string> = { windows: 'Windows', darwin: 'macOS', linux: 'Linux', freebsd: 'FreeBSD', amd64: '64-bit', arm64: 'ARM 64-bit', arm: 'ARM 32-bit', '386': '32-bit', all: 'Every system' }
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
export const NOTHING_NOTABLE = 'Bug fixes and improvements.'

export const mb = (n = 0) => n >= 1048576 ? `${(n / 1048576).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`
export const dayOf = (s?: string) => { if (!s) return ''; const d = new Date(s + 'T12:00:00'); return isNaN(d.getTime()) ? s : d.toLocaleDateString(undefined, { dateStyle: 'medium' }) }
