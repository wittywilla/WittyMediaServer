// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Component, Suspense, useEffect, useMemo, useState, useSyncExternalStore, type ComponentType, type ReactNode } from 'react'
import { Route, Routes } from 'react-router-dom'
import { api } from './api'
import { bundledModule } from './moduleBundles'
import { ModuleMissing } from './components/ModuleMissing'
import { CardRouteContext } from './sdui/render/cardRoute'
import { GameArtContext, type GameArtApi, type GameArtArgs } from './sdui/render/glyphs'

// What modules of API 5 add to the web app (docs/MODULES-API5.md, "The web part"). A module's
// integration.json says what it claims: its routes and frames, its cards, players, readers,
// Settings sections, setup steps and menu folding. The server sends that with the bundle list
// (GET /modules/web, `integration`), so the app knows before any of the module's code loads.
// The bundle's register(host) export hands over the components when they are first needed.
//
// While a module has no version of API 5 in use, nothing here claims anything, and the app's
// own copy of the module's screens draws them as it always did.

export type WebRoute = { path: string; frame: 'fullscreen' | 'app' | 'tv' }
export type SettingsTab = { id: string; label: string; admin?: boolean; after?: string; search?: string[] }
export type WebPart = {
  routes?: WebRoute[]
  takeover?: { libraryKind: string; route: string }[]
  cards?: { mediaKind: string; art?: boolean; detail?: boolean; link?: boolean }[]
  players?: { mediaKind: string }[]
  readers?: { format: string }[]
  settings?: SettingsTab[]
  setup?: { id: string; after?: string }[]
  navFold?: { libraryKind: string; into: string }[]
}
type Bundle = { id: string; version: string; api: number; integration?: WebPart }

/** What a card hook draws: as GameArtContext's, for any media kind. */
export type CardArgs = GameArtArgs & { kind: string }
export type CardParts = {
  /** Wraps the app's frame and provides the card contexts itself (GameArtContext, CardRouteContext). */
  Provider?: ComponentType<{ children: ReactNode }>
  art?: (a: CardArgs) => ReactNode | null
  detail?: (a: CardArgs) => ReactNode
  cover?: (a: CardArgs) => ReactNode
  /** Where a card's link goes instead of where the server says. */
  link?: (to: string) => string
}

/** What a bundle's register(host) returns. Every key is optional; what it hands over must be
 * what its integration.json claims, and anything else is ignored. */
export type Registration = {
  routes?: Record<string, ComponentType<RouteProps>>
  cards?: Record<string, CardParts>
  players?: Record<string, ComponentType<{ host: unknown; itemId: string }>>
  readers?: Record<string, ComponentType<{ host: unknown }>>
  settings?: Record<string, ComponentType<{ host: unknown }>>
  /** A first-run step: given the wizard's own step props too, as the app's own step had them. */
  setup?: Record<string, ComponentType<Record<string, unknown>>>
}
export type RouteProps = { host: unknown; params: Record<string, string | undefined> }

export type ModuleWeb = { module: string; version: string; part: WebPart }

// ---------------------------------------------------------------- what the server says is in use

const CACHE = 'wms.moduleWeb.v1'
const HOST_API = 5

function readCache(): ModuleWeb[] {
  try {
    const v = JSON.parse(localStorage.getItem(CACHE) || '[]')
    return Array.isArray(v) ? v : []
  } catch { return [] }
}

// Known at once from the last answer this browser had, so a page a module draws isn't first
// drawn by the app's own copy; then from the server, and a change re-renders.
let known: ModuleWeb[] = readCache()
let asked = false
const listeners = new Set<() => void>()

function ask() {
  if (asked) return
  asked = true
  api.get<{ bundles: Bundle[] }>('/modules/web').then((r) => {
    const next = (r.bundles || []).filter((b) => b.api >= 5 && b.api <= HOST_API && b.integration).map((b) => ({ module: b.id, version: b.version, part: b.integration as WebPart }))
    const same = JSON.stringify(next) === JSON.stringify(known)
    try { localStorage.setItem(CACHE, JSON.stringify(next)) } catch { /* private mode */ }
    if (!same) {
      known = next
      listeners.forEach((l) => l())
    }
  }).catch(() => {})
}

const subscribe = (l: () => void) => { ask(); listeners.add(l); return () => { listeners.delete(l) } }

/** The modules of API 5 in use and what they add to the web app. */
export function useModuleWeb(): ModuleWeb[] {
  return useSyncExternalStore(subscribe, () => known, () => known)
}

/** Asks the server again: after a module is added, removed or updated. */
export function refreshModuleWeb() {
  asked = false
  ask()
}

// ---------------------------------------------------------------- loading a bundle's registration

const registrations = new Map<string, Promise<Registration | null>>()

/** A module's registration: its bundle loaded and register(host) called, once. */
export function registrationOf(module: string): Promise<Registration | null> {
  let p = registrations.get(module)
  if (!p) {
    p = (async () => {
      const m = await bundledModule<Record<string, unknown>>(module, (x) => typeof x.register === 'function')
      if (!m) return null
      const { hostFor } = await import('./moduleHost')
      return ((m.register as (h: unknown) => Registration)(hostFor(module))) || {}
    })().catch(() => null)
    registrations.set(module, p)
  }
  return p
}

function useRegistration(module: string | undefined): Registration | null | undefined {
  const [reg, setReg] = useState<Registration | null | undefined>(undefined)
  useEffect(() => {
    if (!module) return
    let live = true
    registrationOf(module).then((r) => { if (live) setReg(r) })
    return () => { live = false }
  }, [module])
  return module ? reg : null
}

// ---------------------------------------------------------------- routes and frames

export type RouteMatch = { module: string; route: WebRoute; params: Record<string, string | undefined> }

// A route's path as a pattern: ":name" is one part of the address, "*" at the end is the rest,
// and a part ending in "*" (wittyplay-*) is any part starting so.
function routeRegex(path: string): { re: RegExp; names: string[] } {
  const names: string[] = []
  const parts = path.split('/').filter(Boolean).map((seg, i, all) => {
    if (seg === '*' && i === all.length - 1) { names.push('*'); return '(?:/(.*))?' }
    if (seg.startsWith(':')) { names.push(seg.slice(1)); return '/([^/]+)' }
    const esc = (t: string) => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    if (seg.endsWith('*')) return '/' + esc(seg.slice(0, -1)) + '[^/]*'
    return '/' + esc(seg)
  })
  return { re: new RegExp('^' + (parts.join('') || '/') + '/?$'), names }
}
const regexCache = new Map<string, { re: RegExp; names: string[] }>()

/** The module route a path is, if a module of API 5 in use claims it. The first claim wins. */
export function matchModuleRoute(list: ModuleWeb[], pathname: string, frames?: WebRoute['frame'][]): RouteMatch | null {
  for (const m of list) {
    for (const r of m.part.routes || []) {
      if (frames && !frames.includes(r.frame)) continue
      let rx = regexCache.get(r.path)
      if (!rx) { rx = routeRegex(r.path); regexCache.set(r.path, rx) }
      const hit = rx.re.exec(pathname)
      if (hit) return { module: m.module, route: r, params: Object.fromEntries(rx.names.map((n, i) => [n, hit[i + 1]])) }
    }
  }
  return null
}

/** Where a module takes a library of a kind it draws itself (takeover), if one does. */
export function takeoverFor(list: ModuleWeb[], libraryKind: string | undefined, library: string): string | null {
  if (!libraryKind) return null
  for (const m of list) {
    const t = (m.part.takeover || []).find((x) => x.libraryKind === libraryKind)
    if (t) return t.route.replace('{library}', library)
  }
  return null
}

/** One module's screen for a route it claims, with what's missing said if it won't load. */
export function ModuleRouteView({ match, fallback }: { match: RouteMatch; fallback?: ReactNode }) {
  const reg = useRegistration(match.module)
  if (reg === undefined) return <>{fallback ?? null}</>
  const C = reg?.routes?.[match.route.path]
  // Not there at all may change when it's added; there but without this screen won't.
  if (!C) return reg ? <ModuleMissing ids={[match.module]} /> : <ModuleMissing ids={[match.module]} onReady={() => window.location.reload()} />
  const view = <Guarded module={match.module}><C host={hostNow(match.module)} params={match.params} /></Guarded>
  // A path with parameters is drawn inside a route of the same path, so the module's own code
  // reads them (useParams) as it did in the app's routes. One with "*" isn't: a module's own
  // <Routes> under it match whole addresses, as they did in the app's frame.
  if (!match.route.path.includes(':') || match.route.path.includes('*')) return view
  return <Routes><Route path={match.route.path} element={view} /></Routes>
}

// The host object a component is given; the bundle's register(host) had the same one.
const hostCache = new Map<string, unknown>()
function hostNow(module: string): unknown {
  return hostCache.get(module)
}
export function rememberHost(module: string, host: unknown) {
  hostCache.set(module, host)
}

// A module's screen that crashes is reported and replaced with what's missing; the app goes on.
class Guarded extends Component<{ module: string; children: ReactNode }, { crashed: boolean }> {
  state = { crashed: false }
  static getDerivedStateFromError() { return { crashed: true } }
  componentDidCatch(err: unknown) {
    const reason = String((err as Error)?.message || err).slice(0, 400)
    api.post(`/modules/${this.props.module}/failed`, { version: known.find((k) => k.module === this.props.module)?.version || '', reason }).catch(() => {})
  }
  render() { return this.state.crashed ? <ModuleMissing ids={[this.props.module]} /> : this.props.children }
}

// ---------------------------------------------------------------- cards

/** Provides a module's card parts for the kinds it claims, over the app's own. Its bundle is
 * loaded once the page is idle; until then the app's own drawing stands. */
export function ModuleCards({ children }: { children: ReactNode }) {
  const list = useModuleWeb()
  const owner = list.find((m) => (m.part.cards || []).some((c) => c.mediaKind === 'game'))?.module
  const [parts, setParts] = useState<CardParts | null>(null)
  useEffect(() => {
    if (!owner) { setParts(null); return }
    let live = true
    const go = () => registrationOf(owner).then((r) => { if (live) setParts(r?.cards?.game || null) })
    const idle = (window as unknown as { requestIdleCallback?: (f: () => void) => number }).requestIdleCallback
    if (idle) idle(go); else setTimeout(go, 1)
    return () => { live = false }
  }, [owner])
  const art = useMemo<GameArtApi | null>(() => parts && parts.art ? {
    art: (a) => parts.art!({ ...a, kind: 'game' }),
    detail: parts.detail ? (a) => parts.detail!({ ...a, kind: 'game' }) : undefined,
    cover: parts.cover ? (a) => parts.cover!({ ...a, kind: 'game' }) : undefined,
  } : null, [parts])
  if (parts?.Provider) return <parts.Provider>{children}</parts.Provider>
  let out = children
  if (art) out = <GameArtContext.Provider value={art}>{out}</GameArtContext.Provider>
  if (parts?.link) out = <CardRouteContext.Provider value={parts.link}>{out}</CardRouteContext.Provider>
  return <>{out}</>
}

// ---------------------------------------------------------------- Settings

export type ModuleSettingsTab = SettingsTab & { module: string }

/** The Settings sections modules of API 5 draw. */
export function useModuleSettingsTabs(): ModuleSettingsTab[] {
  const list = useModuleWeb()
  return useMemo(() => list.flatMap((m) => (m.part.settings || []).map((s) => ({ ...s, module: m.module }))), [list])
}

/** A module's Settings section. */
export function ModuleSettingsView({ tab }: { tab: ModuleSettingsTab }) {
  const reg = useRegistration(tab.module)
  if (reg === undefined) return null
  const C = reg?.settings?.[tab.id]
  if (!C) return <ModuleMissing ids={[tab.module]} />
  return <Guarded module={tab.module}><C host={hostNow(tab.module)} /></Guarded>
}

// ---------------------------------------------------------------- lazily, for App.tsx

/** A module's full-window screen, for App.tsx, with its own Suspense. */
export const ModuleFrame = ({ match, fallback }: { match: RouteMatch; fallback: ReactNode }) =>
  <Suspense fallback={fallback}><ModuleRouteView match={match} fallback={fallback} /></Suspense>

// ---------------------------------------------------------------- first-run setup

/** A module's first-run step for an id, if a module of API 5 in use declares one: its component,
 * once its bundle is loaded; undefined while loading, null when none. */
export function useModuleSetupStep(id: string): ComponentType<Record<string, unknown>> | null | undefined {
  const list = useModuleWeb()
  const owner = list.find((m) => (m.part.setup || []).some((s) => s.id === id))?.module
  const reg = useRegistration(owner)
  if (!owner) return null
  if (reg === undefined) return undefined
  return reg?.setup?.[id] || null
}
