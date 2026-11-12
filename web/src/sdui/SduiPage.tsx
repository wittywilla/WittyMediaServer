// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A server-driven page, from asking for it to carrying out what is pressed on it
// (docs/SDUI.md). The drawing is render/; the rules are model.ts, routes.ts, focus.ts.

import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { announce } from '../a11y'
import { api, ApiError, downloadFile, dvdStartMode, insertFxOn, isDisc, type Item } from '../api'
import { useAudio, type AudioMode } from '../audio/player'
import { EmptyState, LoadError } from '../components/common'
import { IconInfo, IconTune } from '../icons'
import { useOpenItem } from '../openItem'
import { toast, useApp } from '../state'
import { LinkButton } from '../ui/Button'
import { ask } from '../ui/Overlay'
import { fetchItems, fetchPage, kept, onChange, remember, bump } from './client'
import { deviceKey, type Device } from './device'
import { focusMap, next as nextFocus, type Dir } from './focus'
import { applyItems, buildPage, sectionGap, type View } from './model'
import { hasTools, nativeInstead } from './native'
import { plan, withQuery } from './routes'
import { Ctx, px, type PageCtx } from './render/context'
import { scopeStyle, Sections } from './render/components'
import { setPageMove } from '../tv'
import type { Action, Document, Node } from './types'
import { useDevice } from './useSdui'
import { setPageRegions } from './regions'
import './sdui.css'
import './play.css'

const Customise = lazy(() => import('./Customise'))
const CardMenu = lazy(() => import('./render/CardMenu'))
const ScreenHost = lazy(() => import('./render/ScreenHost'))
const LayerHost = lazy(() => import('./render/LayerHost'))
const PageTools = lazy(() => import('./extras/PageTools'))

// ---------------------------------------------------------------- the tree, changed without changing it

function replaceNode(root: Node, id: string, fn: (n: Node) => Node | null): Node {
  if (root.id === id) return fn(root) || root
  if (!root.children?.length) return root
  let changed = false
  const kids: Node[] = []
  for (const c of root.children) {
    if (c.id === id) { const r = fn(c); changed = true; if (r) kids.push(r); continue }
    const r = replaceNode(c, id, fn)
    if (r !== c) changed = true
    kids.push(r)
  }
  return changed ? { ...root, children: kids } : root
}

export function findNode(root: Node, id: string): Node | null {
  if (root.id === id) return root
  for (const c of root.children || []) { const hit = findNode(c, id); if (hit) return hit }
  return null
}

// carryOver: the page came again while some of it had been filled in here (a row that
// arrived later, a grid scrolled three pages down). What was drawn stays until the new
// page has it differently, so nothing flickers back to grey.
function carryOver(fresh: Document, drawn: Document | null): Document {
  if (!drawn || fresh.page.id !== drawn.page.id) return fresh
  const walk = (n: Node): Node => {
    const was = n.state === 'deferred' || (n.props?.more && n.children?.length) ? findNode(drawn.root, n.id) : null
    if (was && was.type === n.type) {
      if (n.state === 'deferred' && was.state !== 'deferred' && was.state !== 'error') return was
      const a = n.children || [], b = was.children || []
      if (n.state !== 'deferred' && b.length > a.length && a.every((c, i) => c.id === b[i].id)) return { ...n, children: [...a, ...b.slice(a.length)], props: { ...n.props, more: was.props?.more } }
    }
    if (!n.children?.length) return n
    const kids = n.children.map(walk)
    return kids.some((k, i) => k !== n.children![i]) ? { ...n, children: kids } : n
  }
  const root = walk(fresh.root)
  return root === fresh.root ? fresh : { ...fresh, root }
}

// ---------------------------------------------------------------- drawing a document

type Handlers = Pick<PageCtx, 'act' | 'more' | 'busy' | 'failed' | 'setQuery' | 'menu'>

const NOTHING: Handlers = { act: () => {}, more: () => {}, busy: () => false, failed: () => false, setQuery: () => {}, menu: () => {} }

// PageView: a document, drawn. With no handlers it is a still drawing (the editor's preview).
export function PageView({ doc, device, handlers, width, children }: { doc: Document; device: Device; handlers?: Handlers; width?: number; children?: ReactNode }) {
  const model = useMemo(() => buildPage(doc), [doc])
  const l = model.layout
  const tv = l.formFactor === 'tv'
  const box = useRef<HTMLDivElement | null>(null)
  const [boxWidth, setBoxWidth] = useState(width || 0)
  useLayoutEffect(() => {
    if (width) { setBoxWidth(width); return }
    const el = box.current
    if (!el) return
    const read = () => setBoxWidth(el.clientWidth)
    read()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(read)
    ro.observe(el)
    return () => ro.disconnect()
  }, [width])
  // A television's page is laid out on a canvas 1920 points wide, whatever the window.
  const canvas = l.canvas?.width || 1920
  const safe = (l.safeArea?.left || 0) + (l.safeArea?.right || 0)
  const pt = tv && boxWidth > 0 ? boxWidth / (canvas - safe) : 1
  const cols = useRef<Record<string, number>>({})
  const inert = !handlers
  const h = handlers || NOTHING
  const reducedMotion = !!l.reducedMotion || device.capabilities.includes('reducedMotion')
  const ctx = useMemo<PageCtx>(() => ({
    doc, layout: l, device, tv, touch: device.capabilities.includes('touch') && !device.capabilities.includes('pointer'), reducedMotion, pt, inert,
    act: h.act, more: h.more, busy: h.busy, failed: h.failed, setQuery: h.setQuery, menu: h.menu,
    columns: (id, n) => { cols.current[id] = n },
  }), [doc, l, device, tv, reducedMotion, pt, inert, h.act, h.more, h.busy, h.failed, h.setQuery, h.menu])

  // The shell's optional regions (status bar, system dock) show what this page gives them, while it shows.
  useEffect(() => { if (inert) return; setPageRegions(doc.regions); return () => setPageRegions(undefined) }, [doc.regions, inert])

  // The arrows of a remote, a game pad or the keyboard follow the page's order.
  const left = useRef<Record<string, string>>({})
  useEffect(() => {
    const el = box.current
    if (!el || inert) return
    const map = focusMap(doc.root, (id, sent) => cols.current[id] || sent)
    const onFocus = (e: FocusEvent) => {
      const at = (e.target as HTMLElement).closest?.('[data-sdui-focus]')?.getAttribute('data-sdui-focus')
      if (at && map.stopOf[at] !== undefined) left.current[map.stops[map.stopOf[at]].id] = at
    }
    el.addEventListener('focusin', onFocus)
    const release = setPageMove(el, (cur, dir, focus) => movePage(el, cur, dir, focus, focusMap(doc.root, (id, sent) => cols.current[id] || sent), left.current))
    return () => { el.removeEventListener('focusin', onFocus); release() }
  }, [doc.root, inert])

  const first = model.root.children[0]
  const ownHeading = !first || first.type === 'Hero' || first.type === 'DetailHeader' || first.type === 'NativeView' || (first.type === 'Text' && first.role === 'heading 1')
  const style = {
    '--sdui-gap': px(sectionGap(l), pt), '--section-gap': px(sectionGap(l), pt), '--sdui-pt': String(pt),
    '--sdui-xs': px(l.spacing.xs || 4, pt), '--sdui-s': px(l.spacing.s || 8, pt), '--sdui-m': px(l.spacing.m || 16, pt), '--sdui-l': px(l.spacing.l || 24, pt), '--sdui-xl': px(l.spacing.xl || 48, pt),
    ...scopeStyle(doc.theme), ...scopeStyle(doc.root),
  } as CSSProperties
  const title = model.root.label || doc.page.title
  return (
    <Ctx.Provider value={ctx}>
      <div ref={box} className={`sdui ff-${l.formFactor} bp-${l.breakpoint} density-${l.density || 'comfortable'} kind-${doc.page.kind}` + (inert ? ' inert' : '')} style={style}
        data-sdui-page={doc.page.id} data-scope={doc.theme?.scope || undefined} data-source={doc.meta?.source === 'draft' ? 'draft' : undefined}>
        {title && <h1 className={tv || ownHeading ? 'sr-only' : 'sdui-page-title'}>{title}</h1>}
        {children}
        <Sections views={model.root.children} />
      </div>
    </Ctx.Provider>
  )
}

// movePage: one press of an arrow on a server-driven page. true: focus moved (or stays,
// at the end of a row); undefined: nothing on this page that way, so whatever is around
// the page may have the key.
function movePage(page: HTMLElement, cur: HTMLElement, dir: Dir, focus: (el: HTMLElement) => void, map: ReturnType<typeof focusMap>, left: Record<string, string>): boolean | undefined {
  const byId = (id: string) => page.querySelector<HTMLElement>(`[data-sdui-focus="${CSS.escape(id)}"]`)
  const parts = (stop: HTMLElement) => Array.from(stop.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled])')).filter((el) => !el.hasAttribute('data-nav-skip') && el.tabIndex >= 0 && el.getClientRects().length > 0)
  const into = (id: string | null): boolean | undefined => {
    if (!id) return undefined
    const el = byId(id)
    if (!el) return undefined
    // A stop with parts of its own (a spotlight, a title's header) starts on its main one.
    if (el.hasAttribute('data-sdui-stop')) {
      const inside = parts(el)
      const to = inside.find((x) => x.matches('[data-tv-first]')) || inside[0]
      if (!to) return undefined
      focus(to)
      return true
    }
    focus(el)
    return true
  }
  const sideways = (list: HTMLElement[]): boolean | undefined => {
    const i = list.indexOf(cur)
    const to = list[i + (dir === 'right' ? 1 : -1)]
    if (to) { focus(to); return true }
    return dir === 'right' || !document.querySelector('.sidebar:not([aria-hidden=true])') ? true : undefined
  }
  // Inside a stop: its own buttons or fields, in the order given.
  const stop = cur.closest<HTMLElement>('[data-sdui-stop]')
  if (stop && page.contains(stop)) {
    const id = stop.getAttribute('data-sdui-stop')!
    if (dir === 'left' || dir === 'right') {
      const keys = cur.closest('[data-sdui-keys]')
      if (keys) return undefined // a keyboard of letters is a grid of its own
      const list = parts(stop)
      const i = list.indexOf(cur)
      const to = list[i + (dir === 'right' ? 1 : -1)]
      if (to) { focus(to); return true }
      // Past the ends of a spotlight's buttons: the next or previous title.
      const dots = Array.from(stop.querySelectorAll<HTMLElement>('[data-sdui-slide]'))
      if (dots.length > 1) {
        const at = dots.findIndex((d) => d.getAttribute('aria-current') === 'true')
        dots[(at + (dir === 'right' ? 1 : -1) + dots.length) % dots.length].click()
        requestAnimationFrame(() => { const again = parts(stop); (again.find((x) => x.matches('[data-tv-first]')) || again[0])?.focus({ preventScroll: true }) })
        return true
      }
      return sideways(list)
    }
    if (cur.closest('[data-sdui-keys]')) return undefined
    return into(nextFocus(map, id, dir, left))
  }
  // A row's heading (its See all, a show's seasons): beside the row, above its cards.
  const head = cur.closest<HTMLElement>('[data-sdui-head]')
  if (head && page.contains(head)) {
    const section = head.parentElement
    const member = section?.querySelector<HTMLElement>('[data-sdui-focus]:not([data-sdui-stop])')
    const memberId = member?.getAttribute('data-sdui-focus') || ''
    if (dir === 'left' || dir === 'right') return sideways(parts(head))
    if (dir === 'down') {
      if (!member || map.stopOf[memberId] === undefined) return undefined
      const group = map.stops[map.stopOf[memberId]]
      return into(left[group.id] && group.members.includes(left[group.id]) ? left[group.id] : group.members[0])
    }
    if (!memberId || map.stopOf[memberId] === undefined) return undefined
    const above = map.stops[map.stopOf[memberId] - 1]
    return above ? into(left[above.id] && above.members.includes(left[above.id]) ? left[above.id] : above.members[0]) : undefined
  }
  const id = cur.closest('[data-sdui-focus]')?.getAttribute('data-sdui-focus')
  if (!id || map.stopOf[id] === undefined) return undefined
  const to = nextFocus(map, id, dir, left)
  if (to === null) {
    if (dir === 'left') return document.querySelector('.sidebar:not([aria-hidden=true])') ? undefined : true
    if (dir === 'right') return true
    return undefined
  }
  // Up out of a group's first row: its own heading first, where that has something to press.
  if (dir === 'up' && map.stopOf[to] !== map.stopOf[id]) {
    const own = byId(id)?.closest('[data-sdui-section], .sdui-tabs')?.querySelector<HTMLElement>(':scope > [data-sdui-head]')
    const pressable = own ? parts(own) : []
    if (pressable.length) { focus(pressable.find((x) => x.matches('.active, [aria-selected=true], [aria-checked=true], [aria-pressed=true]')) || pressable[0]); return true }
  }
  return into(to)
}

// ---------------------------------------------------------------- the page, live

type Problem = { status: number; message: string }

export default function SduiPage({ page, params, fallback }: { page: string; params: Record<string, string>; fallback?: ReactNode }) {
  const device = useDevice()
  const { libraries, user } = useApp()
  const nav = useNavigate()
  const loc = useLocation()
  const audio = useAudio()
  const opener = useOpenItem()
  const key = page + '?' + JSON.stringify(params)
  const [doc, setDoc] = useState<Document | null>(() => kept(page, device, params))
  const [problem, setProblem] = useState<Problem | null>(null)
  const [busy, setBusy] = useState<Set<string>>(() => new Set())
  const [failed, setFailed] = useState<Set<string>>(() => new Set())
  const [customising, setCustomising] = useState(false)
  const [menu, setMenu] = useState<{ card: View; x: number; y: number } | null>(null)
  const [screen, setScreen] = useState<{ screen: string; item: string; title: string } | null>(null)
  // A page shown over this one (navigate with as "layer"): the game's quick menu.
  const [layer, setLayer] = useState('')
  const [tick, setTick] = useState(0)
  const drawn = useRef<Document | null>(doc)
  drawn.current = doc
  const dkey = deviceKey(device, window.innerWidth)

  // Ask for the page: at once what was kept, then what it is now.
  useEffect(() => {
    const ctl = new AbortController()
    const had = kept(page, device, params)
    if (had && had !== drawn.current && drawn.current?.page.id !== had.page.id) setDoc(had)
    setBusy((b) => new Set(b).add('page'))
    fetchPage(page, device, params, { signal: ctl.signal }).then((d) => {
      setProblem(null)
      setFailed(new Set())
      setDoc((was) => (was && was.meta?.etag === d.meta?.etag && was.page.id === d.page.id && was.page.route === d.page.route ? was : carryOver(d, was)))
    }).catch((e) => {
      if (ctl.signal.aborted) return
      setProblem({ status: e instanceof ApiError ? e.status : 0, message: (e as Error).message })
    }).finally(() => { if (!ctl.signal.aborted) setBusy((b) => { const n = new Set(b); n.delete('page'); return n }) })
    return () => ctl.abort()
  }, [key, dkey, tick]) // eslint-disable-line react-hooks/exhaustive-deps

  // A different page: nothing of the last one stays.
  const lastKey = useRef(key)
  if (lastKey.current !== key) {
    lastKey.current = key
    const had = kept(page, device, params)
    if (doc && (!had || had.page.id !== doc.page.id || had.page.route !== doc.page.route)) { setDoc(had); setProblem(null) }
  }

  // Again after its time to live, while the page is being looked at; and when something
  // it shows was changed from here.
  const ttl = doc?.meta?.ttlSeconds || 0
  useEffect(() => {
    if (!ttl) return
    const t = setInterval(() => { if (!document.hidden) setTick((n) => n + 1) }, Math.max(15, ttl) * 1000)
    return () => clearInterval(t)
  }, [ttl, key])
  useEffect(() => onChange(() => setTick((n) => n + 1)), [])
  // Offline: try again every few seconds, so the page comes back by itself.
  useEffect(() => {
    if (problem?.status !== 0) return
    const t = setTimeout(() => setTick((n) => n + 1), 5000)
    return () => clearTimeout(t)
  }, [problem])

  const more = useCallback((sectionId: string, url?: string, replace?: boolean) => {
    const d = drawn.current
    const node = d ? findNode(d.root, sectionId) : null
    const from = url || (node?.props?.more as { url?: string } | undefined)?.url
    if (!d || !node || !from) return
    let going = false
    setBusy((b) => { if (b.has(sectionId)) { going = true; return b } return new Set(b).add(sectionId) })
    if (going) return
    setFailed((f) => { if (!f.has(sectionId)) return f; const n = new Set(f); n.delete(sectionId); return n })
    fetchItems(from, device).then((answer) => {
      setDoc((was) => {
        if (!was) return was
        const next = { ...was, root: replaceNode(was.root, sectionId, (n) => applyItems(n, { ...answer, replace: replace || answer.replace })) }
        remember(page, device, params, next)
        return next
      })
    }).catch(() => setFailed((f) => new Set(f).add(sectionId))).finally(() => setBusy((b) => { const n = new Set(b); n.delete(sectionId); return n }))
  }, [device, page, key]) // eslint-disable-line react-hooks/exhaustive-deps

  const setQuery = useCallback((change: Record<string, string>) => {
    nav(withQuery(loc.pathname, loc.search, change), { replace: true })
  }, [nav, loc.pathname, loc.search])

  // Start playing with the app's own player: what kind of thing it is decides which.
  const play = useCallback(async (id: string, resume: boolean) => {
    try {
      const it = await api.get<Item>(`/items/${encodeURIComponent(id)}`)
      if (it.kind === 'audio') {
        const type = libraries.find((l) => l.id === it.libraryId)?.type
        const mode = (type === 'audiobooks' || type === 'podcasts' ? type : 'music') as AudioMode
        // A track of an album plays the album from there.
        let queue: Item[] = []
        if (it.album) queue = await api.get<Item[]>(`/music/tracks?library=${it.libraryId}&artist=${encodeURIComponent(it.albumArtist || it.artist || '')}&album=${encodeURIComponent(it.album)}`).catch(() => [])
        const at = queue.findIndex((t) => t.id === it.id)
        if (at >= 0) audio.play(queue, at, mode); else audio.play([it], 0, mode)
        return
      }
      if (isDisc(it)) { nav(resume ? `/play/${it.id}?mode=${dvdStartMode(it)}${insertFxOn(it) ? '&insert=1' : ''}` : `/play/${it.id}?mode=title&start=0`); return }
      if (!resume && it.kind !== 'book' && it.kind !== 'game' && it.kind !== 'cd-image') { nav(`/play/${it.id}?start=0`); return }
      opener.open(it)
    } catch (e) { toast((e as Error).message, { tone: 'danger' }) }
  }, [audio, libraries, nav, opener])

  const actNow = useCallback((a: Action | undefined, from?: View) => {
    const step = plan(a, { externalLinks: drawn.current?.layout.externalLinks })
    switch (step.do) {
      case 'go': nav(step.to); break
      case 'layer': setLayer(step.to); break
      case 'back': nav(-1); break
      case 'play': play(step.item, step.resume); break
      case 'open': window.open(step.url, '_blank', 'noopener,noreferrer'); break
      case 'tab': setQuery({ tab: step.tab }); break
      case 'refresh': setTick((n) => n + 1); break
      case 'screen': {
        if (step.screen === 'download' && step.item && /^\d+$/.test(step.item)) { downloadFile(`/items/${step.item}/original`, from?.label || 'download').catch((e) => toast((e as Error).message, { tone: 'danger' })); break }
        if (step.screen === 'nowPlaying') { document.querySelector<HTMLElement>('.audio-bar [data-open-full], .audio-bar .ab-open')?.click(); break }
        if (step.item) setScreen({ screen: step.screen, item: step.item, title: from?.text?.title || from?.label || drawn.current?.page.title || '' })
        break
      }
      // The game player's own panels answer these: it listens for the event.
      case 'client': window.dispatchEvent(new CustomEvent('wms:command', { detail: { op: step.op, params: step.params } })); break
      case 'request':
        (step.method === 'POST' ? api.post(step.path, step.body) : step.method === 'PUT' ? api.put(step.path, step.body) : api.del(step.path)).then(() => {
          const said = a?.op === 'watchLater.add' ? 'Added to Watch later' : a?.op === 'watched.set' ? 'Marked as watched' : a?.op === 'watched.clear' ? 'Marked as not watched' : 'Done'
          toast(said, { tone: 'ok' }); announce(said)
          bump()
        }).catch((e) => toast((e as Error).message, { tone: 'danger' }))
        break
    }
  }, [nav, play, setQuery])
  // A command with a question asks it first, with Cancel beside, and runs only on a yes.
  const act = useCallback((a: Action | undefined, from?: View) => {
    if (a?.confirm) { void ask({ title: a.confirm, confirm: String(from?.text?.title || from?.label || 'Yes') }).then((yes) => { if (yes) actNow({ ...a, confirm: undefined }, from) }); return }
    actNow(a, from)
  }, [actNow])

  const handlers = useMemo<Handlers>(() => ({
    act, more, setQuery,
    busy: (id) => busy.has(id), failed: (id) => failed.has(id),
    menu: (card, at) => setMenu({ card, ...at }),
  }), [act, more, setQuery, busy, failed])

  if (problem && (!doc || problem.status === 404 || problem.status === 403)) {
    if (problem.status === 404) {
      return <><h1 className="sr-only">Not available</h1><EmptyState icon={<IconInfo />} title="This isn’t available" text="It may have been removed, or this profile may not be allowed to see it." action={<LinkButton variant="primary" to="/">Go to Home</LinkButton>} /></>
    }
    if (problem.status === 0) return <LoadError what="This page" onRetry={() => setTick((n) => n + 1)} />
    return <div role="alert"><EmptyState icon={<IconInfo />} title="This page didn’t load" text={problem.message} action={<button type="button" className="btn primary" onClick={() => setTick((n) => n + 1)}>Try again</button>} /></div>
  }
  if (!doc) return <PageLoading device={device} />
  // One of the app's own screens stands in for this page (native.ts says when).
  if (fallback && nativeInstead(doc, libraries)) return <>{fallback}</>

  // A page for a kind of library, with no such library yet: say so and offer the next step (canvas 'empty').
  const wantsLibrary = ({ photos: ['photos'], books: ['books', 'audiobooks'], music: ['music'] } as Record<string, string[]>)[doc.page.kind]
  if (wantsLibrary && !libraries.some((l) => wantsLibrary.includes(l.type))) {
    const noun = doc.page.kind === 'photos' ? 'photos' : doc.page.kind === 'books' ? 'books' : 'music'
    return <><h1 className="sr-only">{doc.page.title || noun}</h1><EmptyState icon={<IconInfo />} title={`No ${noun} yet`}
      text={user?.isAdmin ? `Add a library of ${noun} and it appears here.` : `Ask whoever runs this server to add a library of ${noun}.`}
      action={user?.isAdmin ? <LinkButton variant="primary" to="/settings/libraries">Add a library</LinkButton> : undefined} /></>
  }
  const personal = doc.personal
  const canCustomise = !!personal?.editable && (personal.allow || []).length > 0 && (personal.sections || []).length > 0
  const st = doc.screenTime
  return (
    <>
      <PageView doc={doc} device={device} handlers={handlers}>
        {hasTools(doc, libraries) && <Suspense fallback={null}><PageTools doc={doc} /></Suspense>}
        {doc.meta?.source === 'draft' && <div className="ui-notice info sdui-draft" role="status"><div className="ui-notice-text"><strong>This is the draft</strong><span>Only you see it. Publish it from Settings, Pages and menu.</span></div><Link className="btn sm" to="/settings/pages">Open the editor</Link></div>}
        {st && (
          <div className="ui-notice warn sdui-screentime" role="status">
            <div className="ui-notice-text"><strong>Screen time is over</strong><span>{st.message || 'This profile can’t watch any more right now.'}{st.nextAt ? ` You can watch again at ${new Date(st.nextAt * 1000).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}.` : ''}</span></div>
          </div>
        )}
      </PageView>
      {(canCustomise || user?.isAdmin) && (
        <div className="home-foot sdui-foot">
          {canCustomise && <button type="button" className="btn sm ghost" data-tv-options aria-haspopup="dialog" onClick={() => setCustomising(true)} title="Rearrange, hide or pin the sections of this page"><IconTune /> Customise this page</button>}
          {user?.isAdmin && !device.capabilities.includes('touch') && device.formFactor !== 'tv' && <Link className="btn sm ghost" to={`/settings/pages/${encodeURIComponent(doc.page.id)}`} title="Change this page for everyone">Edit for everyone</Link>}
        </div>
      )}
      <Suspense fallback={null}>
        {customising && personal && <Customise doc={doc} onClose={() => setCustomising(false)} />}
        {menu && <CardMenu card={menu.card} x={menu.x} y={menu.y} act={act} onClose={() => setMenu(null)} />}
        {screen && <ScreenHost {...screen} onClose={() => setScreen(null)} />}
        {layer && <LayerHost to={layer} onClose={() => setLayer('')} />}
      </Suspense>
    </>
  )
}

// A whole page arriving for the first time: three rows of grey cards.
export function PageLoading({ device }: { device: Device }) {
  const still = device.capabilities.includes('reducedMotion')
  return (
    <div className={'sdui sdui-loading skeleton-grid' + (still ? ' still' : '')} role="status" aria-label="Loading" aria-busy="true">
      {[0, 1, 2].map((r) => (
        <div className="hub" key={r} aria-hidden="true">
          <div className="hub-head"><i className="skel sdui-skel-title" /></div>
          <div className="row">{Array.from({ length: 8 }, (_, i) => <div key={i} className="card sdui-ph"><div className="poster"><i className="skel" /></div></div>)}</div>
        </div>
      ))}
    </div>
  )
}

