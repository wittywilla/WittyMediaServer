import { Component, lazy, useEffect, useState, type ComponentType, type Context, type ReactNode } from 'react'
import { api, mediaUrl } from './api'
import { ModuleMissing } from './components/ModuleMissing'
import type { ReaderHost } from '@wittywilla/wittyink'

// What modules show in the browser arrives as web bundles, with none of it in the app. The
// server says which bundles are in use. Anything that goes wrong with a bundle is told to the
// server, which takes a version out of use after three reports in a day, and the page says what's
// missing. Nothing here runs until a page needs it.

export type ReaderFormat = 'epub' | 'pdf' | 'cbz'
const READERS: Record<ReaderFormat, string> = { epub: 'wittyebook', pdf: 'wittypdf', cbz: 'wittycomics' }
const HOST_API = 3 // the newest module API moduleHost.ts provides

type Bundle = { id: string; version: string; entry: string; style?: string; base: string; api: number }
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
const scripts = new Map<string, Promise<Record<string, unknown>>>()
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

// importBundle loads a bundle's script and stylesheet once, whoever asks, and reports a
// failure once.
function importBundle(b: Bundle, shape: (m: Record<string, unknown>) => boolean) {
  let p = scripts.get(keyOf(b))
  if (!p) {
    p = (async () => {
      const [m] = await Promise.all([import(/* @vite-ignore */ mediaUrl(b.base + b.entry)) as Promise<Record<string, unknown>>, b.style ? addStyle(b) : undefined])
      if (!shape(m)) throw new Error('it is not the shape of bundle expected')
      return m
    })()
    p.catch((e) => reportFailure(b, e))
    scripts.set(keyOf(b), p)
  }
  return p
}

const isComponent = (c: unknown) => typeof c === 'function' || (typeof c === 'object' && c !== null && '$$typeof' in c)
const isInk = (m: Record<string, unknown>) => typeof m.useHost === 'function' && isComponent((m.HostContext as { Provider?: unknown })?.Provider)

// chooseReader finds the bundle to use for a kind of book, or null if it isn't here. WittyInk
// goes first, since every reader is built on it and shares it.
async function chooseReader(format: ReaderFormat, list: Bundle[]): Promise<BundledReader | null> {
  const bundle = list.find((b) => b.id === READERS[format])
  if (!bundle || bundle.api > HOST_API || failed.has(keyOf(bundle))) return null
  const { moduleHost } = await import('./moduleHost')
  const ink = list.find((b) => b.id === 'wittyink' && b.api <= HOST_API && !failed.has(keyOf(b))) ?? null
  if (!ink) return null
  const inkModule = await importBundle(ink, isInk)
  let r = readers.get(keyOf(bundle))
  if (!r) {
    r = (async () => {
      if (bundle.id === 'wittypdf') moduleHost.pdfjsModule = await moduleHost.pdfjs()
      moduleHost.wittyink = inkModule
      const m = await importBundle(bundle, (m) => isComponent(m.default))
      return { bundle, ink, Reader: m.default as ComponentType, HostContext: inkModule.HostContext as HostContextType }
    })()
    readers.set(keyOf(bundle), r)
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

export function useBundledReader(format: ReaderFormat | undefined) {
  const [list, setList] = useState<Bundle[] | null>(null)
  const [answer, setAnswer] = useState<{ format: ReaderFormat; reader: BundledReader | null } | null>(null)
  useEffect(() => {
    let live = true
    askServer().then((l) => { if (live) setList(l) })
    return () => { live = false }
  }, [])
  useEffect(() => {
    if (!format || !list) return
    let live = true
    const reader = list.length ? within(LOAD_MS, chooseReader(format, list).catch(() => null), null) : Promise.resolve(null)
    reader.then((r) => { if (live) setAnswer({ format, reader: r }) })
    return () => { live = false }
  }, [format, list])
  return answer && answer.format === format ? answer.reader : undefined
}

// Any other plugin's bundle, for when it's opened: its script, or null if it isn't here.
export async function bundledModule<T>(id: string, shape: (m: T) => boolean): Promise<T | null> {
  try {
    const list = await askServer()
    const b = list.find((x) => x.id === id)
    if (!b || b.api > HOST_API || failed.has(keyOf(b))) return null
    await import('./moduleHost')
    return await within(LOAD_MS, importBundle(b, (m) => shape(m as T)) as Promise<T | null>, null)
  } catch {
    return null
  }
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
export function lazyModule<P extends object>(id: string, pick: (m: Record<string, unknown>) => unknown) {
  return lazy(async () => {
    const m = await bundledModule<Record<string, unknown>>(id, (x) => isComponent(pick(x)))
    const c = m ? pick(m) as ComponentType<P> : null
    return { default: c ?? ((() => <ModuleMissing ids={[id]} onReady={() => window.location.reload()} />) as unknown as ComponentType<P>) }
  })
}
