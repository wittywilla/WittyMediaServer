// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Overrides, release status, kinds of module, files uploaded to install, and the module audit:
// the rules behind what the store and the Modules page show about them, kept apart from the pages
// so they can be tested on their own (overrides.test.mjs). Nothing here asks the server; every
// word the server sends is only ever shown as text.

// ------------------------------------------------------------------ what the server sends

// What has to be accepted before something the usual rules refuse can be installed.
export type NeedsOverride = {
  kinds: string[]
  // Why each kind applies, by kind or in the order of kinds.
  reasons?: Record<string, string> | string[]
  severity: number
  warning?: string
  // What the module asks for. Each can be granted, one by one; none is granted by default.
  permissions?: string[]
  version?: string
  pending?: boolean
}

// An override that was accepted, as recorded on the module.
export type OverrideRecord = {
  kinds: string[]; reasons?: Record<string, string> | string[]; by?: string; at?: number | string; version?: string
  fingerprint?: string; grants?: string[]; source?: 'catalogue' | 'upload' | 'store' | string
}

// What a module's SDUI adds to the app: from the catalogue (`ui`) or the installed copy (`uiAdds`).
export type UiSummary = { pages?: { id: string; title?: string }[]; navigation?: string[]; settings?: boolean; sources?: number; inserts?: number }

// A release downloaded and checked, held until someone accepts what it needs.
export type Pending = { version: string; needsOverride: NeedsOverride }

// The answer to an upload.
export type UploadResult = {
  token: string; expiresAt?: number | string; id?: string; name?: string; version?: string; delivery?: string; api?: number
  releaseStatus?: string; status?: string; statusNote?: string; signed?: boolean; trusted?: boolean; fingerprint?: string
  permissions?: string[]; problems?: { kind: string; text: string }[]; needsOverride?: NeedsOverride | null; refusal?: string
}

export type AuditEntry = { at: number | string; user?: string; action: string; module?: string; version?: string; detail?: string }

// ------------------------------------------------------------------ kinds of override

// Plain names for each kind, in the server's order of severity. The server's own reason for each
// is shown under its name; a kind this build has no name for is shown by its reason alone.
export const OVERRIDE_KINDS: Record<string, { title: string; severity: number }> = {
  incomplete: { title: 'Not finished', severity: 1 },
  'out-of-range': { title: 'Not made for this server’s version', severity: 2 },
  'local-file': { title: 'Installed from a file', severity: 2 },
  custom: { title: 'A module this server doesn’t know', severity: 3 },
  'untrusted-key': { title: 'Signed by a key this server doesn’t trust', severity: 3 },
  'weak-sandbox': { title: 'Less boxed in on this system', severity: 3 },
  unsigned: { title: 'Not signed', severity: 4 },
}
export const overrideTitle = (kind: string) => OVERRIDE_KINDS[kind]?.title || cap(kind.replace(/-/g, ' '))

// reasonFor finds the server's reason for one kind, whichever way the reasons came.
export function reasonFor(o: Pick<NeedsOverride, 'kinds' | 'reasons'>, kind: string): string {
  const r = o.reasons
  if (!r) return ''
  if (Array.isArray(r)) { const i = o.kinds.indexOf(kind); return i >= 0 ? String(r[i] || '') : '' }
  return String(r[kind] || '')
}

// Each bypassed kind with its words, most serious first, for a list a person reads top to bottom.
export function overrideLines(o: Pick<NeedsOverride, 'kinds' | 'reasons'>): { kind: string; title: string; reason: string }[] {
  const sev = (k: string) => OVERRIDE_KINDS[k]?.severity ?? 3
  return [...new Set(o.kinds || [])].sort((a, b) => sev(b) - sev(a)).map((kind) => ({ kind, title: overrideTitle(kind), reason: reasonFor(o, kind) }))
}

// The severity, from the server's number, or from the kinds when it sent none.
export function severityOf(o: Pick<NeedsOverride, 'kinds' | 'severity'>): number {
  if (typeof o.severity === 'number' && o.severity > 0) return Math.min(4, Math.round(o.severity))
  return Math.max(1, ...(o.kinds || []).map((k) => OVERRIDE_KINDS[k]?.severity ?? 3))
}

// ------------------------------------------------------------------ friction

// How hard it is to say yes grows with how much is being bypassed. Every override asks for each
// kind to be ticked and the password; from 3 the module's ID has to be typed as well; 4 is shown
// as the most serious there is. The tones are the design's own: warn below 3, danger from 3.
export type Friction = { level: 1 | 2 | 3 | 4; tone: 'warn' | 'danger'; typed: boolean; heading: string; button: string }
export function friction(severity: number): Friction {
  const level = (Math.min(4, Math.max(1, Math.round(severity || 1)))) as Friction['level']
  return {
    level,
    tone: level >= 3 ? 'danger' : 'warn',
    typed: level >= 3,
    heading: level >= 4 ? 'This is the riskiest kind of install' : level === 3 ? 'This could put your server at risk' : level === 2 ? 'This isn’t the usual way to install' : 'This isn’t finished',
    button: level >= 3 ? 'Install at my own risk' : 'Install anyway',
  }
}

export type OverrideAnswers = { accepted: string[]; password: string; code: string; typed: string; grants: string[] }

// What's still missing before the dialog's button can be pressed, in words, for a screen reader
// and for the line under the button. Empty when everything is there.
export function stillNeeded(need: Pick<NeedsOverride, 'kinds' | 'severity'>, a: OverrideAnswers, id: string, twoFactor: boolean): string[] {
  const out: string[] = []
  const left = (need.kinds || []).filter((k) => !a.accepted.includes(k)).length
  if (left) out.push(left === 1 ? 'tick the warning' : `tick ${left} warnings`)
  if (friction(severityOf(need)).typed && a.typed.trim() !== id) out.push('type the module’s ID')
  if (!a.password) out.push('enter your password')
  if (twoFactor && !a.code.trim()) out.push('enter the code from your authenticator app')
  return out
}

// The body for the install request: the password and code at the top, as for every other change
// to which code runs, and the override beside them. Only kinds the server asked about are sent, and
// only permissions the module asks for are granted.
export function overrideRequest(need: Pick<NeedsOverride, 'kinds' | 'severity' | 'permissions'>, a: OverrideAnswers) {
  const accept = (need.kinds || []).filter((k) => a.accepted.includes(k))
  const grants = (need.permissions || []).filter((p) => a.grants.includes(p))
  const typed = friction(severityOf(need)).typed ? a.typed.trim() : undefined
  return { password: a.password, code: a.code.trim(), override: { accept, grants, ...(typed !== undefined ? { typed } : {}) } }
}

// needsOverrideOf picks the need out of a refusal, or nothing when it isn't one.
export function needsOverrideOf(e: unknown): NeedsOverride | null {
  const x = e as { status?: number; data?: { needsOverride?: NeedsOverride } } | null
  const n = x?.data?.needsOverride
  return n && Array.isArray(n.kinds) && n.kinds.length ? n : null
}

// What kind of refusal came back from an override request, so the dialog can say it in place.
export function refusalKind(e: unknown): 'password' | 'override' | 'invalid' | 'other' {
  const x = e as { status?: number; data?: { code?: string; needsOverride?: unknown } } | null
  if (x?.status === 403 && (x.data?.code === 'password-needed' || x.data?.code === 'password-required' || x.data?.code === 'password-wrong' || x.data?.code === 'code-needed')) return 'password'
  if (x?.status === 403) return 'password'
  if (x?.status === 409 && x.data?.needsOverride) return 'override'
  if (x?.status === 400) return 'invalid'
  return 'other'
}

// ------------------------------------------------------------------ release status

export const STATUSES = ['stable', 'beta', 'experimental', 'incomplete'] as const
export type ReleaseStatus = typeof STATUSES[number]
export const STATUS_WORDS: Record<ReleaseStatus, { title: string; tone?: 'new' | 'warn' | 'bad' }> = {
  stable: { title: 'Stable' }, beta: { title: 'Beta', tone: 'new' }, experimental: { title: 'Experimental', tone: 'warn' }, incomplete: { title: 'Incomplete', tone: 'bad' },
}
// A status this build doesn't know is read as the most careful one it does.
export function releaseStatus(s?: string | null): ReleaseStatus {
  if (!s) return 'stable'
  return (STATUSES as readonly string[]).includes(s) ? s as ReleaseStatus : 'incomplete'
}
// Whether a status needs its note shown where it can't be missed.
export const statusWarns = (s: ReleaseStatus) => s === 'experimental' || s === 'incomplete'

// ------------------------------------------------------------------ kinds of module

export const MODULE_KINDS = ['helper', 'web', 'program', 'library'] as const
export type ModuleKind = typeof MODULE_KINDS[number]
export const MODULE_KIND_WORDS: Record<ModuleKind, string> = { helper: 'Helper', web: 'Web bundle', program: 'Program', library: 'Library' }
export const moduleKindTitle = (k?: string) => (k && (MODULE_KIND_WORDS as Record<string, string>)[k]) || ''

// ------------------------------------------------------------------ the store's filters

// The store's own filters for kind and status, as they sit in the page's address beside its
// search, category, publisher and platform. A value this build doesn't know is ignored, so an old
// link still opens the store.
export type ExtraFilters = { moduleKind: ModuleKind | ''; status: ReleaseStatus | '' }
export function readFilters(params: { get(k: string): string | null }): ExtraFilters {
  const k = params.get('moduleKind') || ''
  const s = params.get('status') || ''
  return {
    moduleKind: (MODULE_KINDS as readonly string[]).includes(k) ? k as ModuleKind : '',
    status: (STATUSES as readonly string[]).includes(s) ? s as ReleaseStatus : '',
  }
}
// The address with one filter changed, keeping everything else in it.
export function withFilter(params: URLSearchParams, key: string, value: string): URLSearchParams {
  const n = new URLSearchParams(params)
  if (value) n.set(key, value); else n.delete(key)
  return n
}

// A store item's kind and status, wherever the server put them.
type Filterable = { moduleKind?: string; releaseStatus?: string; status?: unknown }
export const itemModuleKind = (it: Filterable) => it.moduleKind || ''
export function itemReleaseStatus(it: Filterable): ReleaseStatus {
  if (it.releaseStatus) return releaseStatus(it.releaseStatus)
  // The installed copy's report entry says it too, when the catalogue doesn't.
  const installed = it.status as { releaseStatus?: string } | undefined
  return releaseStatus(installed && typeof installed === 'object' ? installed.releaseStatus : undefined)
}
export function passesExtra(it: Filterable, f: ExtraFilters): boolean {
  return (!f.moduleKind || itemModuleKind(it) === f.moduleKind) && (!f.status || itemReleaseStatus(it) === f.status)
}

// ------------------------------------------------------------------ what a module adds

export type UiLines = { pages: string[]; navigation: string[]; settings: boolean; empty: boolean }
export function uiLines(ui?: UiSummary | null): UiLines | null {
  if (!ui) return null
  const pages = (ui.pages || []).map((p) => (p.title || p.id || '').trim()).filter(Boolean)
  const navigation = (ui.navigation || []).map((l) => String(l).trim()).filter(Boolean)
  const settings = !!ui.settings
  return { pages, navigation, settings, empty: !pages.length && !navigation.length && !settings }
}

// ------------------------------------------------------------------ an upload

export type UploadFact = { label: string; value: string }
export type UploadView = {
  state: 'refused' | 'override' | 'ready'
  title: string
  facts: UploadFact[]
  signature: { tone: 'ok' | 'warn' | 'danger'; words: string }
  problems: { kind: string; text: string }[]
  permissions: string[]
  need: NeedsOverride | null
}

// uploadView turns the server's look at an uploaded archive into what the dialog shows: what it
// is, whether its signature can be trusted, what's wrong with it and what installing it needs.
export function uploadView(r: UploadResult): UploadView {
  const name = r.name || r.id || 'This file'
  const facts: UploadFact[] = []
  if (r.name) facts.push({ label: 'Name', value: r.name })
  if (r.id) facts.push({ label: 'ID', value: r.id })
  if (r.version) facts.push({ label: 'Version', value: r.version })
  if (r.delivery) facts.push({ label: 'Kind', value: moduleKindTitle(r.delivery) || r.delivery })
  if (r.api) facts.push({ label: 'Module API', value: String(r.api) })
  const st = releaseStatus(r.releaseStatus || r.status)
  facts.push({ label: 'Status', value: STATUS_WORDS[st].title })
  const signature: UploadView['signature'] = !r.signed
    ? { tone: 'danger', words: 'Not signed. Nothing proves who made it or that it wasn’t changed.' }
    : r.trusted
      ? { tone: 'ok', words: 'Signed by a key this server trusts for this module.' }
      : { tone: 'warn', words: 'Signed, by a key this server doesn’t trust for this module.' }
  const need = r.needsOverride && r.needsOverride.kinds?.length ? r.needsOverride : null
  return {
    state: r.refusal ? 'refused' : need ? 'override' : 'ready',
    title: r.refusal ? `${name} can’t be installed` : `${name}${r.version ? ' ' + r.version : ''}`,
    facts, signature,
    problems: (r.problems || []).filter((p) => p && p.text),
    permissions: r.permissions || [],
    need,
  }
}

// ------------------------------------------------------------------ dates and the audit

// A time from the server, which may be milliseconds, seconds or an ISO date.
export function toDate(at?: number | string | null): Date | null {
  if (at === undefined || at === null || at === '') return null
  const n = typeof at === 'number' ? (at < 1e12 ? at * 1000 : at) : Date.parse(at)
  const d = new Date(n)
  return isNaN(d.getTime()) ? null : d
}

const AUDIT_WORDS: Record<string, string> = {
  'module-override': 'Installed with an override', 'module-override-removed': 'Removed an override', 'module-upload': 'Installed from a file',
  'module-older': 'Went back to an older version', 'module-rollback': 'Took a version out of use', 'module-source': 'Changed where it comes from',
  'module-undo': 'Undid an update', 'fork-refused': 'Refused a fork', 'password-refused': 'Password refused',
  'store-added': 'Added a store', 'store-removed': 'Removed a store',
}
export const auditWords = (action: string) => AUDIT_WORDS[action] || cap(action.replace(/[-_]/g, ' '))

// The audit's rows, newest first, whatever order they came in, and only rows that make sense.
export function auditRows(raw: unknown): AuditEntry[] {
  const list = Array.isArray(raw) ? raw : (raw as { entries?: unknown[]; audit?: unknown[] } | null)?.entries || (raw as { audit?: unknown[] } | null)?.audit || []
  const rows = (list as AuditEntry[]).filter((r) => r && typeof r.action === 'string')
  return rows.map((r, i) => ({ r, i, t: toDate(r.at)?.getTime() ?? 0 })).sort((a, b) => b.t - a.t || a.i - b.i).map((x) => x.r)
}

function cap(s: string) { return s.charAt(0).toUpperCase() + s.slice(1) }
