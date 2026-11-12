// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Standalone emulators (WittyPlay's Docker backend, the module's docs/DOCKER-BACKEND.md): what
// the server says about them, and the parts of the pages with no page in them, so they can be
// tested on their own (emulatorsLogic.test.mjs). The requests themselves are in emulatorsApi.ts.

// ---- what the server says ----

export type FieldChoice = { value: string; label: string }
// One setting, as the server describes it. The forms are drawn from these, so a setting the
// module adds later shows up with no change here.
export type Field = {
  key: string; kind: 'toggle' | 'number' | 'text' | 'choice' | 'map'
  label: string; help?: string; default: unknown
  min?: number; max?: number; step?: number
  choices?: FieldChoice[]
  scope: 'admin' | 'user'
  mapOf?: 'toggle' | 'choice' | 'text'; mapKeys?: FieldChoice[]
  // A heading the setting goes under (the section of the emulator's own file), and whether it's
  // one most people never need. A schema generated from an emulator's real settings has hundreds.
  group?: string; advanced?: boolean
}
export type Values = Record<string, unknown>

export type DockerStatus = {
  enabled: boolean; runtime: 'docker' | 'local' | 'host'
  available: boolean; host: string; apiVersion?: string; os?: string; arch?: string; rootless?: boolean; error?: string
  local: { available: boolean; why?: string }
  // An emulator on this computer's own desktop: a Mac or Windows. Missing from an older WittyPlay.
  desktop?: { available: boolean; why?: string }
  containers: number
}
export type DiagnosticCheck = { id: string; label: string; state: 'ok' | 'warn' | 'fail' | 'off'; detail: string }
export type Diagnostics = { host: { os: string; arch: string; cpus: number; memoryMB: number }; checks: DiagnosticCheck[] }
export type BackendSettings = { settings: Values; schema: Field[]; backends: string[] }

export type InstallJob = {
  id: string; state: 'running' | 'done' | 'failed' | 'cancelled'
  phase: 'downloading' | 'verifying' | 'unpacking' | 'done'
  got: number; total: number; resumed?: number; error?: string; started: string
}
export type CatalogueEntry = {
  id: string; name: string; systems: string[]; license: string; licenseUrl?: string; upstream: string
  version: string; nonCommercial?: boolean; experimental?: boolean; needsBios?: boolean
  stream: 'pipe' | 'kasmvnc'
  requires?: { memoryMB?: number; cpus?: number; gpu?: boolean }
  downloadBytes?: number
  installed?: { id: string; version: string; sha256: string; source: string; installedAt: number }
  updateAvailable: boolean
  installable: boolean; why?: string
  runnable: boolean; whyNot?: string
  job?: InstallJob
  bios?: { name: string; present: boolean; note?: string }[]
  // Never downloaded: the administrator installs it and adds it (ManualEmulator.tsx).
  manual?: { download: string; notes?: string }
}

// What the page shows about adding an emulator installed by hand.
export type ManualView = {
  download: string; notes?: string; programs?: string[]; dropFolder?: string; file?: string; system: string
  detected?: { program: string; dir: string; entry: string; version?: string }
  installed?: { id: string; version: string; source: string; installedAt: number }
}

export type SettingsScope = 'admin' | 'user' | 'game'
export type EmulatorSettingsView = {
  emulator: string; name: string; schema: Field[]
  admin: Values; user: Values; game?: Values; effective: Values
  canAdmin: boolean
}

export type RunningSession = {
  id: string; emulator: string; name: string; backend: string; userId: number; mine: boolean; itemId: number; title: string
  started: string; state: 'running' | 'paused'; viewers: number
}

// What a game's page is told: the emulators that could play it.
export type GameEmulator = { id: string; name: string; version: string; stream: string; experimental?: boolean; ready: boolean; why?: string; needsBios?: boolean }

// ---- addresses ----

// A session's addresses come whole, as the browser would call them (/api/v1/...). The app's
// own client adds the server and /api itself, so this is the rest. Anything that isn't one of
// this server's API addresses is no address at all.
export function apiPath(url: string | undefined): string | null {
  if (!url || !url.startsWith('/api/') || url.includes('://') || url.includes('\\')) return null
  return url.slice('/api'.length)
}

// ---- sizes and times ----

export function fmtBytes(n: number): string {
  if (!(n >= 0)) return ''
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)} GB`
  if (n >= 1e6) return `${Math.round(n / 1e6)} MB`
  if (n >= 1e3) return `${Math.round(n / 1e3)} kB`
  return `${Math.round(n)} bytes`
}

// How long ago something started, in a couple of words.
export function sinceWords(started: string, now: number): string {
  const at = Date.parse(started)
  if (!Number.isFinite(at)) return ''
  const min = Math.max(0, Math.floor((now - at) / 60000))
  if (min < 1) return 'Just now'
  if (min < 60) return `${min} min`
  const h = Math.floor(min / 60), m = min % 60
  if (h < 24) return m ? `${h} h ${m} min` : `${h} h`
  const d = Math.floor(h / 24)
  return d === 1 ? '1 day' : `${d} days`
}

// ---- an install ----

const PHASE_WORDS: Record<string, string> = { downloading: 'Downloading', verifying: 'Checking the download', unpacking: 'Unpacking', done: 'Finishing' }

// jobProgress is an install as a bar and a line of words. value is left out when there's no
// knowing how far along it is: the total isn't known, or the step has no count of its own.
export function jobProgress(job: Pick<InstallJob, 'phase' | 'got' | 'total' | 'resumed'>): { value?: number; max: number; text: string } {
  const phase = PHASE_WORDS[job.phase] || 'Working'
  if (job.phase !== 'downloading') return { max: 1, text: phase }
  const known = job.total > 0
  const parts = [phase, known ? `${fmtBytes(Math.min(job.got, job.total))} of ${fmtBytes(job.total)}` : job.got > 0 ? fmtBytes(job.got) : '']
  if ((job.resumed || 0) > 0) parts.push(`carried on from ${fmtBytes(job.resumed!)}`)
  return { value: known ? Math.min(job.got, job.total) : undefined, max: known ? job.total : 1, text: parts.filter(Boolean).join(' · ') }
}

export function requiresWords(r: CatalogueEntry['requires']): string {
  if (!r) return ''
  const parts: string[] = []
  if (r.memoryMB) parts.push(r.memoryMB >= 1024 ? `${+(r.memoryMB / 1024).toFixed(1)} GB of memory` : `${r.memoryMB} MB of memory`)
  if (r.cpus) parts.push(r.cpus === 1 ? '1 processor core' : `${r.cpus} processor cores`)
  if (r.gpu) parts.push('a graphics card')
  if (!parts.length) return ''
  return 'Needs ' + (parts.length < 3 ? parts.join(' and ') : `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`)
}

// The web address's own name, for saying where a download comes from.
export function hostOf(url: string): string {
  const m = /^https?:\/\/([^/?#]+)/i.exec(url || '')
  return m ? m[1].replace(/^www\./, '') : ''
}

// ---- forms drawn from a schema ----

export const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null)

// What a number field's text means: nothing typed is no value, and anything else is the number
// if it is one. The server keeps to min, max and step, and says so in words when it refuses.
export function numberFrom(text: string): number | undefined {
  const t = text.trim()
  if (!t) return undefined
  const n = Number(t)
  return Number.isFinite(n) ? n : undefined
}

// A default, as the faint text of an empty field.
export function placeholderOf(f: Pick<Field, 'kind' | 'choices'>, value: unknown): string {
  if (value === undefined || value === null || value === '') return ''
  if (f.kind === 'toggle') return value ? 'On' : 'Off'
  if (f.kind === 'choice') return f.choices?.find((c) => c.value === value)?.label || String(value)
  if (typeof value === 'object') return ''
  return String(value)
}

export const asMap = (v: unknown): Values => (v && typeof v === 'object' && !Array.isArray(v) ? v as Values : {})

// One entry of a map changed. An entry emptied is taken out of the map, not kept as "".
export function withMapEntry(map: unknown, key: string, value: unknown): Values {
  const out = { ...asMap(map) }
  if (value === undefined || value === null || value === '') delete out[key]
  else out[key] = value
  return out
}

// wholeSettings is the object the backend's settings are saved as: every key the server sent or
// describes, each with what was chosen, or its default where a field was emptied.
export function wholeSettings(schema: Field[], saved: Values, draft: Values): Values {
  const out: Values = { ...saved }
  for (const f of schema) {
    const v = f.key in draft ? draft[f.key] : saved[f.key]
    out[f.key] = v === undefined || v === null ? f.default : v
  }
  return out
}

// What applies when a setting isn't set at this level: the levels under it, then the default.
// An administrator's value is under a person's own, which is under theirs for one game.
export function inheritedValue(f: Field, scope: SettingsScope, view: Pick<EmulatorSettingsView, 'admin' | 'user'>): unknown {
  const layers = scope === 'game' ? [view.user, view.admin] : scope === 'user' ? [view.admin] : []
  for (const l of layers) if (l && l[f.key] !== undefined && l[f.key] !== null) return l[f.key]
  return f.default
}

// Who may change a field at a level. A setting marked for administrators is theirs to set, for
// everyone; nobody sets it for themselves or for one game.
export function mayEdit(f: Pick<Field, 'scope'>, scope: SettingsScope, canAdmin: boolean): boolean {
  return scope === 'admin' ? canAdmin : f.scope === 'user'
}

// changedValues is what to send to save a level: only what changed, with null for a setting
// that was cleared, which is how the server is told to forget it.
export function changedValues(schema: Field[], saved: Values, draft: Values): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of schema) {
    const before = saved[f.key], after = draft[f.key]
    if (same(before, after)) continue
    out[f.key] = after === undefined || after === null ? null : after
  }
  return out
}

export const scopeValues = (view: EmulatorSettingsView, scope: SettingsScope): Values => (scope === 'admin' ? view.admin : scope === 'user' ? view.user : view.game) || {}

// The levels someone can open, the widest first. This game's only where there is a game.
export function scopesFor(canAdmin: boolean, hasGame: boolean, adminOnly = false): SettingsScope[] {
  if (adminOnly) return canAdmin ? ['admin'] : []
  return [...(canAdmin ? ['admin' as const] : []), 'user' as const, ...(hasGame ? ['game' as const] : [])]
}

// Settings worth grouping, searching and hiding: a whole emulator's, rather than a handful.
export const LARGE_SCHEMA = 24

// The fields to show for a search and the advanced switch, in their groups, in the order the
// schema gives them. Words match a setting's label, help, key, group or a choice's label.
export function groupFields(schema: Field[], query: string, advanced: boolean): { group: string; fields: Field[] }[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  const hay = (f: Field) => [f.label, f.help || '', f.key, f.group || '', ...(f.choices || []).map((c) => c.label)].join(' ').toLowerCase()
  const out: { group: string; fields: Field[] }[] = []
  const at = new Map<string, number>()
  for (const f of schema) {
    if (f.advanced && !advanced && !words.length) continue
    if (words.length && !words.every((w) => hay(f).includes(w))) continue
    const g = f.group || 'General'
    if (!at.has(g)) { at.set(g, out.length); out.push({ group: g, fields: [] }) }
    out[at.get(g)!].fields.push(f)
  }
  return out
}

// An emulator already installed on this server, which an administrator added by its path:
// emulators/own.go in WittyPlay. Args holds {game}, {config}, {bios} and {settings}.
export type OwnProgram = { id?: string; name: string; systems: string[]; program: string; args?: string[]; configDir?: string; like?: string; added?: number }

// An own program's command line as typed, one argument per space unless quoted.
export function splitArgs(line: string): string[] {
  const out: string[] = []
  let cur = '', quote = '', any = false
  for (const ch of line) {
    if (quote) { if (ch === quote) quote = ''; else cur += ch; continue }
    if (ch === '"' || ch === "'") { quote = ch; any = true; continue }
    if (/\s/.test(ch)) { if (cur || any) out.push(cur); cur = ''; any = false; continue }
    cur += ch
  }
  if (cur || any) out.push(cur)
  return out
}

export const joinArgs = (args?: string[]) => (args || []).map((a) => (a === '' || /[\s"]/.test(a) ? `'${a}'` : a)).join(' ')
