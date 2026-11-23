// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Component, lazy, useEffect, useState, useSyncExternalStore, type ComponentType, type Context, type ReactNode } from 'react'
import { api, mediaUrl } from './api'
import { ModuleMissing } from './components/ModuleMissing'
import { PageNotFound } from './components/PageNotFound'
import { bundleCache, pickFrom, type Loaded } from './moduleLoad'
import type { ReaderHost } from '@wittywilla/wittyink'

// What modules show in the browser arrives as web bundles, with none of it in the app. The
// server says which bundles are in use. Anything that goes wrong with a bundle is told to the
// server, which takes a version out of use after three reports in a day, and the page says what's
// missing. Nothing here runs until a page needs it.

// A kind of book, as the server's book answer names its reader: epub, pdf, cbz. Which module reads
// it is the module's own say (integration.json web.readers): nothing here names one.
export type ReaderFormat = string
// The reader kit every reader is built on and shares: the app's copy of @wittywilla/wittyink, as
// the builder's stand-in reads it (moduleHost.wittyink), is WittyInk's bundle.
const READER_KIT = 'wittyink'
// The module API whose bundles this host loads: 5, the only one a server takes. A bundle is
// given web host API 5 (moduleHost.ts: the router, the app's client, state, UI kit and contexts)
// and hands over register(host) (moduleRegistry.tsx).
const HOST_API = 5

type Page = { id: string; title: string; where: 'main' | 'settings'; icon?: string }
export type Bundle = { id: string; version: string; entry: string; style?: string; base: string; api: number; pages?: Page[]; views?: string[]; integration?: { readers?: { format: string }[] } }
type HostContextType = Context<ReaderHost | null>
export type BundledReader = { bundle: Bundle; ink: Bundle | null; Reader: ComponentType; HostContext: HostContextType }

// How long a page waits. Slow isn't broken, so this isn't reported; the download carries on
// for next time.
const ASK_MS = 5000
const LOAD_MS = 60000
const within = <T,>(ms: number, p: Promise<T>, otherwise: T) =>
  Promise.race([p, new Promise<T>((done) => setTimeout(() => done(otherwise), ms))])

const keyOf = (b: Bundle) => `${b.id}@${b.version}`
const failed = new Set<string>() // not tried again in this page's life
const readers = new Map<string, Promise<BundledReader>>()
const links = new Map<string, HTMLLinkElement>()

function reportFailure(b: Bundle, why: unknown) {
  if (failed.has(keyOf(b))) return
  failed.add(keyOf(b))
  links.get(keyOf(b))?.remove()
  // A browser's message can quote the address, which can carry the sign-in.
  const reason = String((why as Error)?.message || why).replace(/([?&]token=)[^&\s]*/g, '$1hidden').slice(0, 400)
  console.warn(`${b.id} ${b.version} failed: ${reason}`)
  api.post(`/modules/${b.id}/failed`, { version: b.version, reason }).catch(() => {})
}

function addStyle(b: Bundle) {
  return new Promise<void>((done, fail) => {
    const link = document.createElement('link')
    link.rel = 'stylesheet'
    link.href = mediaUrl(b.base + b.style)
    link.onload = () => done()
    link.onerror = () => fail(new Error('its stylesheet would not load'))
    links.set(keyOf(b), link)
    document.head.appendChild(link)
  })
}

// loadBundle loads a bundle's script and stylesheet once, whoever asks, and reports a bundle that
// won't load or won't run. What it holds is the module: what each caller wants from it (a page,
// a view, a reader) is checked by that caller, and a module without it is not broken (#592).
const loadBundle = bundleCache<Bundle>(keyOf, async (b) => {
  const [m] = await Promise.all([import(/* @vite-ignore */ mediaUrl(b.base + b.entry)) as Promise<Loaded>, b.style ? addStyle(b) : undefined])
  return m
}, reportFailure)

// importBundle is a loaded bundle that has the shape a caller needs. One that loads but hasn't
// is null for that caller, and isn't reported.
async function importBundle(b: Bundle, shape: (m: Loaded) => boolean): Promise<Loaded> {
  const m = await loadBundle(b)
  if (!shape(m)) throw new Error('it is not the shape of bundle expected')
  return m
}

const isComponent = (c: unknown) => typeof c === 'function' || (typeof c === 'object' && c !== null && '$$typeof' in c)
const isInk = (m: Record<string, unknown>) => typeof m.useHost === 'function' && isComponent((m.HostContext as { Provider?: unknown })?.Provider)

// readerOf: the bundle whose module says it reads a kind of book (the readers hook), if one is in use.
export const readerOf = (format: string, list: Bundle[]) =>
  list.find((b) => b.api <= HOST_API && (b.integration?.readers || []).some((r) => r.format === format)) ?? null

// The reader kit's bundle, if it's in use.
export const readerKitOf = (list: Bundle[]) => list.find((b) => b.id === READER_KIT && b.api <= HOST_API) ?? null

// chooseReader finds the reader for a kind of book, or null if none is here. The reader kit goes
// first, since every reader is built on it and shares it: its bundle is what the reader's
// stand-in for @wittywilla/wittyink reads, so it has to be there before the reader's script runs.
async function chooseReader(format: ReaderFormat, list: Bundle[]): Promise<BundledReader | null> {
  const bundle = readerOf(format, list)
  if (!bundle || failed.has(keyOf(bundle))) return null
  const { moduleHost, hostFor } = await import('./moduleHost')
  const ink = readerKitOf(list)
  if (!ink || failed.has(keyOf(ink))) return null
  const inkModule = await importBundle(ink, isInk).catch((e) => { reportFailure(ink, e); throw e })
  let r = readers.get(keyOf(bundle) + '|' + format)
  if (!r) {
    r = (async () => {
      if (format === 'pdf') moduleHost.pdfjsModule = await moduleHost.pdfjs()
      moduleHost.wittyink = inkModule
      const m = await importBundle(bundle, (x) => typeof x.register === 'function')
      const reg = (m.register as (h: unknown) => { readers?: Record<string, unknown> })(hostFor(bundle.id)) || {}
      const Reader = reg.readers?.[format]
      if (!isComponent(Reader)) throw new Error(`it says it reads ${format} but hands over no reader for it`)
      return { bundle, ink, Reader: Reader as ComponentType, HostContext: inkModule.HostContext as HostContextType }
    })()
    r.catch((e) => reportFailure(bundle, e))
    readers.set(keyOf(bundle) + '|' + format, r)
  }
  // A script runs once, so a reader keeps the WittyInk it first met, whatever is in
  // use by now. If that one has since failed, so has this reader.
  const loaded = await r
  return usable(loaded) ? loaded : null
}

const usable = (r: BundledReader) => !failed.has(keyOf(r.bundle)) && !(r.ink && failed.has(keyOf(r.ink)))

// The Reader page asks here. undefined until the answer is known; null means it isn't here. It
// asks the server once each time a reader opens, so a tab left open finds out about a new version
// when next used.
const askServer = () => within(ASK_MS, api.get<{ bundles: Bundle[] }>('/modules/web').then((r) => r.bundles || []).catch(() => [] as Bundle[]), [] as Bundle[])

export function useBundledReader(format: ReaderFormat | undefined, list: Bundle[] | null) {
  const [answer, setAnswer] = useState<{ format: ReaderFormat; reader: BundledReader | null } | null>(null)
  useEffect(() => {
    if (!format || !list) return
    let live = true
    const reader = list.length ? within(LOAD_MS, chooseReader(format, list).catch(() => null), null) : Promise.resolve(null)
    reader.then((r) => { if (live) setAnswer({ format, reader: r }) })
    return () => { live = false }
  }, [format, list])
  return answer && answer.format === format ? answer.reader : undefined
}

// useReaderBundles: the bundles in use, asked for each time a reader opens; null until known.
export function useReaderBundles(): Bundle[] | null {
  const [list, setList] = useState<Bundle[] | null>(null)
  useEffect(() => {
    let live = true
    askServer().then((l) => { if (live) setList(l) })
    return () => { live = false }
  }, [])
  return list
}

// loadedModule: a plugin's bundle, loaded, or null if it isn't here or won't load.
async function loadedModule(id: string): Promise<Loaded | null> {
  try {
    const list = await askServer()
    const b = list.find((x) => x.id === id)
    if (!b || b.api > HOST_API || failed.has(keyOf(b))) return null
    await import('./moduleHost')
    return await within(LOAD_MS, loadBundle(b) as Promise<Loaded | null>, null)
  } catch {
    return null
  }
}

// Any other plugin's bundle, for when it's opened: its script, or null if it isn't here or isn't
// what was wanted.
export async function bundledModule<T>(id: string, shape: (m: T) => boolean): Promise<T | null> {
  const m = await loadedModule(id)
  return m && shape(m as T) ? m as T : null
}

// A crash names the file it came from, so WittyInk answers for its own.
function blamed(err: unknown, r: BundledReader) {
  const stack = String((err as Error)?.stack)
  const inReader = stack.indexOf(r.bundle.base)
  const inInk = r.ink ? stack.indexOf(r.ink.base) : -1
  return r.ink && inInk >= 0 && (inReader < 0 || inInk < inReader) ? r.ink : r.bundle
}

class Guard extends Component<{ r: BundledReader; onCrash: () => void; children: ReactNode }, { crashed: boolean }> {
  state = { crashed: false }
  static getDerivedStateFromError() { return { crashed: true } }
  componentDidCatch(err: unknown) {
    reportFailure(blamed(err, this.props.r), err)
    this.props.onCrash()
  }
  render() { return this.state.crashed ? null : this.props.children }
}

// Shows a bundled reader, with the app's copy to fall back on if it crashes. The fallback
// is outside the guard, so its own problems are never held against the bundle.
export function BundledReaderView({ r, host, fallback, children }: { r: BundledReader; host: ReaderHost; fallback: ReactNode; children?: ReactNode }) {
  const [, crashed] = useState(0)
  const ok = usable(r)
  useEffect(() => {
    // Its look applies only while it is on screen, so a WittyInk bundle's stylesheet
    // can't reach a reader that is using the app's WittyInk.
    const on = [r.bundle, r.ink].flatMap((b) => (b && links.get(keyOf(b))) || [])
    if (!ok) return
    on.forEach((l) => { l.disabled = false })
    return () => on.forEach((l) => { l.disabled = true })
  }, [r, ok])
  if (!ok) return <>{fallback}</>
  return (
    <Guard key={keyOf(r.bundle)} r={r} onCrash={() => crashed((n) => n + 1)}>
      <r.HostContext.Provider value={host}>
        {children}
        <r.Reader />
      </r.HostContext.Provider>
    </Guard>
  )
}

// lazyModule is a component from a module's bundle, for React.lazy's place: until it's there it
// waits, and if it isn't on this server it shows what's missing, with a way to add it.
// A module that is here but has no such component (an address naming a page it doesn't have)
// is a page not found, and nothing is reported.
export function lazyModule<P extends object>(id: string, pick: (m: Record<string, unknown>) => unknown) {
  return lazy(async () => {
    const m = await loadedModule(id)
    if (!m) return { default: (() => <ModuleMissing ids={[id]} onReady={() => window.location.reload()} />) as unknown as ComponentType<P> }
    const c = pickFrom<ComponentType<P>>(m, pick, isComponent)
    return { default: c.found ? c.value : (PageNotFound as unknown as ComponentType<P>) }
  })
}

// The pages modules add to the app. Asked for once a session: adding a module reloads the page.
export type ModulePage = Page & { module: string }
let pagesAsked: Promise<ModulePage[]> | null = null
export function useModulePages(): ModulePage[] {
  const [pages, setPages] = useState<ModulePage[]>([])
  useEffect(() => {
    let live = true
    pagesAsked ||= askServer().then((l) => l.flatMap((b) => (b.pages ?? []).map((p) => ({ ...p, module: b.id }))))
    pagesAsked.then((p) => { if (live) setPages(p) })
    return () => { live = false }
  }, [])
  return pages
}

// ---------------------------------------------------------------- views for server-driven pages

// A module's own view (docs/SDUI-MODULES.md, "Views"): the bundle exports `views`, a
// component for each view ID, and the app says it can draw them (views= with its own), so
// a page sends the NativeView itself rather than its fallback. Each is mounted with
// `{host, view, library?, ...the node's other props}`, `host` being what a module's page gets.
export type ModuleViewProps = { host: unknown; view: string; library?: string } & Record<string, unknown>

let bundlesKnown: Bundle[] = []
let viewIds: string[] = []
let viewsAsked = false
const viewListeners = new Set<() => void>()
function askViews() {
  if (viewsAsked) return
  viewsAsked = true
  askServer().then((list) => {
    bundlesKnown = list.filter((b) => b.api <= HOST_API)
    const next = bundlesKnown.flatMap((b) => (b.views || []).filter((v) => v.startsWith(b.id + '.')))
    if (next.join(',') === viewIds.join(',')) return
    viewIds = next
    viewListeners.forEach((l) => l())
  })
}
const subscribeViews = (l: () => void) => { askViews(); viewListeners.add(l); return () => { viewListeners.delete(l) } }

// useModuleViews: the IDs of the module views this app can draw; empty until the server has said.
export function useModuleViews(): string[] {
  return useSyncExternalStore(subscribeViews, () => viewIds, () => viewIds)
}

// moduleView: the component a bundle exports for a view, or null when it has none or won't load.
export async function moduleView(viewId: string): Promise<ComponentType<ModuleViewProps> | null> {
  const b = bundlesKnown.find((x) => (x.views || []).includes(viewId))
  if (!b || failed.has(keyOf(b))) return null
  try {
    await import('./moduleHost')
    const m = await importBundle(b, (x) => typeof x.views === 'object' && x.views !== null)
    const c = (m.views as Record<string, unknown>)[viewId]
    return isComponent(c) ? c as ComponentType<ModuleViewProps> : null
  } catch { return null }
}
