// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// One router for every place in the app (docs/SDUI.md, "Routes"): an address is a
// server-driven page asked for with some parameters, or one of the app's own screens.
// The table is the catalogue's; this copy is what the app starts with, and
// GET /sdui/catalog replaces it when a server has more.

import type { Action } from './types.ts'

export type RouteRule = { route: string; page?: string; native?: string; params?: Record<string, string> }

export const ROUTES: RouteRule[] = [
  { route: '/', page: 'home' },
  { route: '/libraries', page: 'libraries' },
  { route: '/library/{library}', page: 'library', params: { library: '{library}' } },
  { route: '/item/{item}', page: 'detail', params: { item: '{item}' } },
  { route: '/show/{show}', page: 'detail', params: { show: '{show}' } },
  { route: '/album/{library}', page: 'detail', params: { album: '?album', artist: '?artist', library: '{library}' } },
  { route: '/collections/{collection}', page: 'detail', params: { collection: '{collection}' } },
  { route: '/person/{person}', page: 'detail', params: { person: '{person}' } },
  { route: '/collections', page: 'collections' },
  { route: '/search', page: 'search', params: { q: '?q' } },
  { route: '/tv', page: 'livetv' },
  { route: '/music', page: 'music' },
  { route: '/photos', page: 'photos' },
  { route: '/books', page: 'books' },
  { route: '/games', page: 'games' },
  { route: '/page/{id}', page: '{id}' },
  { route: '/settings', native: 'settings' },
  { route: '/settings/{section}', native: 'settings' },
  { route: '/tv/guide', native: 'guide' },
  { route: '/tv/recordings', native: 'guide' },
  { route: '/tv/watch/{key}', native: 'livePlayer' },
  { route: '/watch/{item}', native: 'player' },
  { route: '/play/{item}', native: 'player' },
  { route: '/read/{item}', native: 'reader' },
  { route: '/game/{item}', native: 'game' },
  { route: '/you', native: 'profiles' },
  { route: '/pair', native: 'pair' },
]

export type Resolved =
  | { kind: 'page'; page: string; params: Record<string, string> }
  | { kind: 'native'; screen: string; vars: Record<string, string> }

// The query parameters each kind of page takes, beside what its route gives it.
const PASSED = ['tab', 'sort', 'desc', 'genre', 'decade', 'year', 'content', 'resolution', 'watched', 'studio', 'network', 'tag', 'season',
  // The rest of the app's own Filters panel and saved views (pages/library/Filters.tsx).
  'series', 'kind', 'disc', 'minRating', 'audio', 'subtitle', 'format', 'edition', 'search', 'seed', 'service']

function matchRule(rule: RouteRule, path: string): Record<string, string> | null {
  const want = rule.route.split('/'), got = path.split('/')
  if (want.length !== got.length) return null
  const vars: Record<string, string> = {}
  for (let i = 0; i < want.length; i++) {
    const w = want[i]
    if (w.startsWith('{') && w.endsWith('}')) {
      if (!got[i]) return null
      try { vars[w.slice(1, -1)] = decodeURIComponent(got[i]) } catch { return null }
    } else if (w !== got[i]) return null
  }
  return vars
}

// resolve: what shows an address. null: nothing in the table does, so it is one of the
// app's own pages the table doesn't know (Help, a module's page).
export function resolve(pathname: string, search = '', rules: RouteRule[] = ROUTES): Resolved | null {
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname
  const query = new URLSearchParams(search)
  for (const rule of rules) {
    const vars = matchRule(rule, path)
    if (!vars) continue
    if (rule.native) return { kind: 'native', screen: rule.native, vars }
    const fill = (v: string) => (v.startsWith('{') ? vars[v.slice(1, -1)] : v.startsWith('?') ? query.get(v.slice(1)) : v) || ''
    const params: Record<string, string> = {}
    for (const [k, v] of Object.entries(rule.params || {})) { const x = fill(v); if (x) params[k] = x }
    for (const k of PASSED) { const x = query.get(k); if (x && !(k in params)) params[k] = x }
    // The app's own Filters and saved views name a streaming service `platform`; a page takes it
    // as `service`, since `platform` there is the device's.
    const service = query.get('platform')
    if (service && !params.service) params.service = service
    return { kind: 'page', page: fill(rule.page || ''), params }
  }
  return null
}

// The app's own screens by name, where a screen has an address in the web app.
export const NATIVE_ROUTES: Record<string, string> = {
  settings: '/settings', guide: '/tv/guide', profiles: '/you', pair: '/pair', collections: '/collections',
  remote: '/settings/remote-control', nowPlaying: '', downloads: '',
}

// What an action comes to in this app. Pure, so it can be tested: the page carries it out.
export type Step =
  | { do: 'go'; to: string }
  | { do: 'layer'; to: string }
  | { do: 'back' }
  | { do: 'play'; item: string; resume: boolean }
  | { do: 'open'; url: string }
  | { do: 'screen'; screen: string; item?: string }
  | { do: 'tab'; tabs: string; tab: string }
  | { do: 'refresh' }
  | { do: 'request'; method: 'POST' | 'PUT' | 'DELETE'; path: string; body?: unknown }
  // A command this app answers itself, with its params: the game player's rewind and remap panels.
  | { do: 'client'; op: string; params: Record<string, unknown> }
  | { do: 'nothing' }

const safeRoute = (to: string) => to.startsWith('/') && !to.startsWith('//')

// The catalogue's commands (GET /sdui/catalog `commands`): the request each op carries out and the values besides {item}
// it needs (docs/SDUI.md, "Commands"). Set when the catalogue has been read.
export type CommandSpec = { op: string; method: string; path: string; body?: string; params?: { name: string; type: 'string' | 'int' | 'bool' | 'ints' | 'object'; required?: boolean }[] }
let commandSpecs = new Map<string, CommandSpec>()
export const setCommandSpecs = (list: CommandSpec[]) => { commandSpecs = new Map(list.map((c) => [c.op, c])) }
// The ops the app answers with a panel of its own, with no request.
const CLIENT_OPS = new Set(['rewind.open', 'controller.remap'])

const inJSON = (v: string) => JSON.stringify(v).slice(1, -1)

// commandRequest: a catalogue command as a request. {item} is the action's item as it is; each parameter goes in place
// of {name}, from the action's params: URL-escaped in the path, escaped as a JSON string in the body, and for an int
// parameter the quoted token is replaced by the number. A parameter not given is an empty string.
// objectParam: an `object` parameter from a Form's controls named `<param>.<key>` (`rule.yearFrom`): each is a key; a number field
// gives a number, an empty control gives no key, and a key ending `[]` (`rule.genres[]`) splits comma-separated text into a list.
export function objectParam(name: string, params: Record<string, unknown> | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(params || {})) {
    if (!k.startsWith(name + '.')) continue
    let key = k.slice(name.length + 1)
    if (v === undefined || v === null || v === '') continue
    if (key.endsWith('[]')) {
      key = key.slice(0, -2)
      const parts = (Array.isArray(v) ? v.map(String) : String(v).split(',')).map((x) => x.trim()).filter(Boolean)
      if (parts.length) out[key] = parts
    } else out[key] = typeof v === 'number' || typeof v === 'boolean' ? v : v
  }
  return out
}

export function commandRequest(spec: CommandSpec, a: Action): Step {
  const given = (n: string) => { const v = a.params?.[n]; return v === undefined || v === null ? '' : String(v) }
  const item = a.item ?? ''
  if (spec.params?.some((p) => p.required && p.type !== 'object' && given(p.name) === '')) return { do: 'nothing' }
  let path = spec.path.replace(/^\/api(\/v1)?/, '').replaceAll('{item}', encodeURIComponent(item))
  let body = spec.body
  for (const p of spec.params || []) {
    path = path.replaceAll(`{${p.name}}`, encodeURIComponent(given(p.name)))
    if (!body) continue
    const token = `"{${p.name}}"`
    const v = a.params?.[p.name]
    if (p.type === 'int') body = body.replaceAll(token, String(Number(given(p.name)) || 0))
    else if (p.type === 'bool') body = body.replaceAll(token, v === true || given(p.name) === 'true' ? 'true' : 'false')
    else if (p.type === 'ints') body = body.replaceAll(token, JSON.stringify((Array.isArray(v) ? v : given(p.name).split(',')).map(Number).filter((n) => Number.isFinite(n) && String(n) !== '')))
    else if (p.type === 'object') body = body.replaceAll(token, JSON.stringify(objectParam(p.name, a.params)))
    else body = body.replaceAll(`{${p.name}}`, inJSON(given(p.name)))
  }
  if (body) body = body.replaceAll('{item}', inJSON(item))
  const method = spec.method === 'PUT' ? 'PUT' : spec.method === 'DELETE' ? 'DELETE' : 'POST'
  let parsed: unknown
  if (body) { try { parsed = JSON.parse(body) } catch { return { do: 'nothing' } } }
  return { do: 'request', method, path, body: parsed }
}

export function plan(a: Action | undefined, opts: { externalLinks?: boolean } = {}): Step {
  if (!a) return { do: 'nothing' }
  switch (a.type) {
    case 'navigate': return a.to && safeRoute(a.to) ? { do: a.as === 'layer' ? 'layer' : 'go', to: a.to } : { do: 'nothing' }
    case 'back': return { do: 'back' }
    case 'play': return a.item ? { do: 'play', item: a.item, resume: a.params?.resume !== false } : { do: 'nothing' }
    case 'external': return a.url && /^https?:\/\//i.test(a.url) && opts.externalLinks !== false ? { do: 'open', url: a.url } : { do: 'nothing' }
    case 'native': {
      if (!a.to) return { do: 'nothing' }
      const item = typeof a.params?.item === 'string' ? a.params.item : undefined
      const to = NATIVE_ROUTES[a.to]
      // A screen with an address is a place to go; the others the page opens itself.
      if (to && !item) return { do: 'go', to }
      return { do: 'screen', screen: a.to, item }
    }
    case 'command':
      switch (a.op) {
        case 'watchLater.add': return a.item ? { do: 'request', method: 'POST', path: '/collections/watch-later/items', body: a.item.startsWith('show-') ? { shows: [numeric(a.item.slice(5))] } : { items: [numeric(a.item)] } } : { do: 'nothing' }
        case 'watched.set': return a.item ? { do: 'request', method: 'POST', path: `/items/${encodeURIComponent(a.item)}/progress`, body: { watched: true } } : { do: 'nothing' }
        case 'watched.clear': return a.item ? { do: 'request', method: 'DELETE', path: `/items/${encodeURIComponent(a.item)}/progress` } : { do: 'nothing' }
        case 'selectTab': return typeof a.params?.tab === 'string' ? { do: 'tab', tabs: typeof a.params?.tabs === 'string' ? a.params.tabs : '', tab: a.params.tab } : { do: 'nothing' }
        case 'refresh': return { do: 'refresh' }
      }
      if (a.op && CLIENT_OPS.has(a.op)) return { do: 'client', op: a.op, params: a.params || {} }
      { const spec = a.op ? commandSpecs.get(a.op) : undefined; if (spec && spec.method !== 'GET') return commandRequest(spec, a) }
      return { do: 'nothing' }
  }
  // An action type this app doesn't know does nothing.
  return { do: 'nothing' }
}

const numeric = (id: string): number | string => (/^\d+$/.test(id) ? Number(id) : id)

// withQuery: an address with some of its query changed; '' takes a parameter away.
export function withQuery(pathname: string, search: string, change: Record<string, string>): string {
  const q = new URLSearchParams(search)
  for (const [k, v] of Object.entries(change)) { if (v) q.set(k, v); else q.delete(k) }
  const s = q.toString()
  return pathname + (s ? '?' + s : '')
}
