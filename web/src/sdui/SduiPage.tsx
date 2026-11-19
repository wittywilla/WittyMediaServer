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
import { fetchItems, fetchPage, kept, onChange, remember, bump } from './client'
import { deviceKey, type Device } from './device'
import { focusMap, next as nextFocus, type Dir } from './focus'
import { applyItems, buildPage, sectionGap, type View } from './model'
import { hasTools, nativeInstead } from './native'
import { plan, withQuery } from './routes'
import { changedRows, refreshDelay } from './refresh'
import { Ctx, px, type PageCtx } from './render/context'
import { scopeStyle, Sections } from './render/components'
import { setPageMove } from '../tv'
import { useTvMode } from '../tvmode/mode'
import { OnAComputer, OnAComputerDialog } from '../tvmode/OnAComputer'
import type { Action, Document, Node } from './types'
import { useDevice } from './useSdui'
import { ask } from '../ui/Overlay'
import { setPageRegions } from './regions'
import { date, t, tn, tx } from '../i18n'
import { setWords } from './words'
import './sdui.css'
import './play.css'

const Customise = lazy(() => import('./Customise'))
const CardMenu = lazy(() => import('./render/CardMenu'))
const ScreenHost = lazy(() => import('./render/ScreenHost'))
const LayerHost = lazy(() => import('./render/LayerHost'))
const PageTools = lazy(() => import('./extras/PageTools'))

// The layout step (model.ts) is also run by node's tests, so it is handed the app's words here.
setWords(t, tn, tx)

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

  // A page whose sections have all taken themselves off (each had nothing to show) says so, instead of a title over a blank page.
  const [blank, setBlank] = useState(false)
  useEffect(() => {
    const el = box.current
    if (!el || inert) return
    const look = () => setBlank(!Array.from(el.querySelectorAll('section, .hub, a, button, img, canvas, video, input, select, [role], [aria-busy=true], .sdui-draft, .sdui-screentime, .ui-notice')).some((x) => !x.closest('.sdui-nothing')))
    const t = window.setTimeout(look, 1500)
    const mo = new MutationObserver(() => { window.clearTimeout(t); window.setTimeout(look, 600) })
    mo.observe(el, { childList: true, subtree: true })
    return () => { window.clearTimeout(t); mo.disconnect() }
  }, [doc, inert])

  // The shell's optional regions (status bar, system dock) show what this page gives them, while it shows.
  useEffect(() => { if (inert) return; setPageRegions(doc.regions && { statusBar: doc.regions.statusBar?.map((s) => (s.text ? { ...s, text: tx(s.text) } : s)), systemDock: doc.regions.systemDock?.map((d) => ({ ...d, label: tx(d.label) })) }); return () => setPageRegions(undefined) }, [doc.regions, inert])

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
  // Home never shows the word "Home" over its own picture: with a featured picture anywhere on it (even below a
  // "Finish setting up" notice) the page's name is for screen readers only. A look whose Home has no picture keeps it.
  const homeWithPicture = doc.page.kind === 'home' && model.root.children.some((c) => c.type === 'Hero')
  const ownHeading = !first || homeWithPicture || first.type === 'Hero' || first.type === 'DetailHeader' || first.type === 'NativeView' || (first.type === 'Text' && first.role === 'heading 1')
  const style = {
    '--sdui-gap': px(sectionGap(l), pt), '--section-gap': px(sectionGap(l), pt), '--sdui-pt': String(pt),
    '--sdui-xs': px(l.spacing.xs || 4, pt), '--sdui-s': px(l.spacing.s || 8, pt), '--sdui-m': px(l.spacing.m || 16, pt), '--sdui-l': px(l.spacing.l || 24, pt), '--sdui-xl': px(l.spacing.xl || 48, pt),
    ...scopeStyle(doc.theme), ...scopeStyle(doc.root),
  } as CSSProperties
  const title = model.root.label || tx(doc.page.title)
  return (
    <Ctx.Provider value={ctx}>
      <div ref={box} className={`sdui ff-${l.formFactor} bp-${l.breakpoint} density-${l.density || 'comfortable'} kind-${doc.page.kind}` + (inert ? ' inert' : '')} style={style}
        data-sdui-page={doc.page.id} data-scope={doc.theme?.scope || undefined} data-source={doc.meta?.source === 'draft' ? 'draft' : undefined}>
        {title && <h1 className={tv || ownHeading ? 'sr-only' : 'sdui-page-title'}>{title}</h1>}
        {children}
        <Sections views={model.root.children} />
        {/* A title's own facts, where else it is listed and the small print go here, after its episodes, cast and more like it (DetailExtras). */}
        {doc.page.kind === 'detail' && <div className="sdui-page-end" data-sdui-end />}
        {blank && <div className="sdui-nothing"><EmptyState icon={<IconInfo />} title={t('Nothing here yet')} text={t('When there is something for this page, it will appear here. Check back after you add media or turn a module on.')} /></div>}
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
  const tvMode = useTvMode()
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
  // quiet: the next ask is the page's own refresh (meta.refreshSec), drawn with no placeholders,
  // busy state or error view. settled counts answers, so the next refresh waits for the last one.
  const quiet = useRef(false)
  const [settled, setSettled] = useState(0)
  const drawn = useRef<Document | null>(doc)
  drawn.current = doc
  const dkey = deviceKey(device, window.innerWidth)

  // Ask for the page: at once what was kept, then what it is now.
  useEffect(() => {
    const ctl = new AbortController()
    const still = quiet.current
    quiet.current = false
    const had = kept(page, device, params)
    if (had && had !== drawn.current && drawn.current?.page.id !== had.page.id) setDoc(had)
    if (!still) setBusy((b) => new Set(b).add('page'))
    fetchPage(page, device, params, { signal: ctl.signal }).then((d) => {
      // A page that was in progress and now isn't (a scan finished): say how it ended, once.
      const was = drawn.current
      if (was && was.page.id === d.page.id && refreshDelay(was) && !refreshDelay(d)) {
        const line = changedRows(was.root, d.root)[0]
        if (line) announce(line)
      }
      setProblem(null)
      setFailed((f) => (f.size ? new Set() : f))
      setDoc((was) => (was && was.meta?.etag === d.meta?.etag && was.page.id === d.page.id && was.page.route === d.page.route ? was : carryOver(d, was)))
    }).catch((e) => {
      // A refresh that didn't arrive leaves the page as it is; the next one tries again.
      if (ctl.signal.aborted || still) return
      setProblem({ status: e instanceof ApiError ? e.status : 0, message: (e as Error).message })
    }).finally(() => {
      if (ctl.signal.aborted) return
      if (!still) setBusy((b) => { const n = new Set(b); n.delete('page'); return n })
      setSettled((n) => n + 1)
    })
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
  // Something on the page is in progress (meta.refreshSec): ask again that long after each answer,
  // quietly, only while the page is open and the tab is showing; once at once when it shows again.
  // Closing the page or layer unmounts this and stops it; an answer without the hint ends it.
  const refreshMs = refreshDelay(doc)
  useEffect(() => {
    if (!refreshMs) return
    let t = 0
    const again = () => { quiet.current = true; setTick((n) => n + 1) }
    if (!document.hidden) t = window.setTimeout(again, refreshMs)
    const seen = () => { window.clearTimeout(t); if (!document.hidden) again() }
    document.addEventListener('visibilitychange', seen)
    return () => { window.clearTimeout(t); document.removeEventListener('visibilitychange', seen) }
  }, [refreshMs, key, settled])
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
          const said = a?.op === 'watchLater.add' ? t('Added to Watch later') : a?.op === 'watched.set' ? t('Marked as watched') : a?.op === 'watched.clear' ? t('Marked as not watched') : t('Done')
          toast(said, { tone: 'ok' }); announce(said)
          bump()
        }).catch((e) => toast((e as Error).message, { tone: 'danger' }))
        break
    }
  }, [nav, play, setQuery])
  // A command with a question asks it first, with Cancel beside, and runs only on a yes.
  const act = useCallback((a: Action | undefined, from?: View) => {
    if (a?.confirm) { void ask({ title: tx(a.confirm), confirm: String(from?.text?.title || from?.label || t('Yes')) }).then((yes) => { if (yes) actNow({ ...a, confirm: undefined }, from) }); return }
    actNow(a, from)
  }, [actNow])

  const handlers = useMemo<Handlers>(() => ({
    act, more, setQuery,
    busy: (id) => busy.has(id), failed: (id) => failed.has(id),
    menu: (card, at) => setMenu({ card, ...at }),
  }), [act, more, setQuery, busy, failed])

  if (problem && (!doc || problem.status === 404 || problem.status === 403)) {
    if (problem.status === 404) {
      return <><h1 className="sr-only">{t('Not available')}</h1><EmptyState icon={<IconInfo />} title={t('This isn’t available')} text={t('It may have been removed, or this profile may not be allowed to see it.')} action={<LinkButton variant="primary" to="/">{t('Go to Home')}</LinkButton>} /></>
    }
    if (problem.status === 0) return <LoadError what={t('This page')} onRetry={() => setTick((n) => n + 1)} />
    return <div role="alert"><EmptyState icon={<IconInfo />} title={t('This page didn’t load')} text={problem.message} action={<button type="button" className="btn primary" onClick={() => setTick((n) => n + 1)}>{t('Try again')}</button>} /></div>
  }
  if (!doc) return <PageLoading device={device} />
  // One of the app's own screens stands in for this page (native.ts says when).
  if (fallback && nativeInstead(doc, libraries)) return <>{fallback}</>

  // A page for a kind of library, with no such library yet: say so and offer the next step (canvas 'empty').
  const wantsLibrary = ({ photos: ['photos'], books: ['books', 'audiobooks'], music: ['music'] } as Record<string, string[]>)[doc.page.kind]
  if (wantsLibrary && !libraries.some((l) => wantsLibrary.includes(l.type))) {
    const kind = doc.page.kind === 'photos' ? 'photos' : doc.page.kind === 'books' ? 'books' : 'music'
    const words = {
      photos: { name: t('Photos'), title: t('No photos yet'), add: t('Add a library of photos and it appears here.'), ask: t('Ask whoever runs this server to add a library of photos.') },
      books: { name: t('Books'), title: t('No books yet'), add: t('Add a library of books and it appears here.'), ask: t('Ask whoever runs this server to add a library of books.') },
      music: { name: t('Music'), title: t('No music yet'), add: t('Add a library of music and it appears here.'), ask: t('Ask whoever runs this server to add a library of music.') },
    }[kind]
    return <><h1 className="sr-only">{tx(doc.page.title) || words.name}</h1><EmptyState icon={<IconInfo />} title={words.title}
      text={user?.isAdmin ? words.add : words.ask}
      action={user?.isAdmin ? <LinkButton variant="primary" to="/settings/libraries">{t('Add a library')}</LinkButton> : undefined} /></>
  }
  const personal = doc.personal
  const canCustomise = !!personal?.editable && (personal.allow || []).length > 0 && (personal.sections || []).length > 0
  const st = doc.screenTime
  // A page that says it isn't for a television (its tv level, docs/TV-MODE.md) is the card that says
  // to use a computer there; one that says view is shown with its controls off.
  if (tvMode && (doc.page.tv === 'desktopOnly' || doc.page.tv === 'view')) {
    return <OnAComputer level={doc.page.tv} title={doc.page.title ? tx(doc.page.title) : t('This page')} path={loc.pathname + loc.search}><PageView doc={doc} device={device} handlers={handlers} /></OnAComputer>
  }
  return (
    <>
      <PageView doc={doc} device={device} handlers={handlers}>
        {hasTools(doc, libraries) && <Suspense fallback={null}><PageTools doc={doc} /></Suspense>}
        {doc.meta?.source === 'draft' && <div className="ui-notice info sdui-draft" role="status"><div className="ui-notice-text"><strong>{t('This is the draft')}</strong><span>{t('Only you see it. Publish it from Settings, Pages and menu.')}</span></div><Link className="btn sm" to="/settings/pages">{t('Open the editor')}</Link></div>}
        {st && (
          <div className="ui-notice warn sdui-screentime" role="status">
            <div className="ui-notice-text"><strong>{t('Screen time is over')}</strong><span>{tx(st.message) || t('This profile can’t watch any more right now.')}{st.nextAt ? ' ' + t('You can watch again at {time}.', { time: date(st.nextAt * 1000, { hour: 'numeric', minute: '2-digit' }) }) : ''}</span></div>
          </div>
        )}
      </PageView>
      {(canCustomise || user?.isAdmin) && (
        <div className="home-foot sdui-foot">
          {canCustomise && <button type="button" className="btn sm ghost" data-tv-options aria-haspopup="dialog" onClick={() => setCustomising(true)} title={t('Rearrange, hide or pin the sections of this page')}><IconTune /> {t('Customise this page')}</button>}
          {user?.isAdmin && !device.capabilities.includes('touch') && device.formFactor !== 'tv' && <Link className="btn sm ghost" to={`/settings/pages/${encodeURIComponent(doc.page.id)}`} title={t('Change this page for everyone')}>{t('Edit for everyone')}</Link>}
        </div>
      )}
      <Suspense fallback={null}>
        {customising && personal && (tvMode ? <OnAComputerDialog part="page.customise" onClose={() => setCustomising(false)} /> : <Customise doc={doc} onClose={() => setCustomising(false)} />)}
        {menu && <CardMenu card={menu.card} x={menu.x} y={menu.y} act={act} onClose={() => setMenu(null)} />}
        {screen && <ScreenHost {...screen} onClose={() => setScreen(null)} />}
        {layer && <LayerHost to={layer} onClose={() => setLayer('')} />}
      </Suspense>
    </>
  )
}

// A whole page arriving for the first time, in the shape of what is coming: a title and a poster for a title's page,
// a header and a grid for a library, rows of cards for anything else (Home, shelves). The same shapes at every size.
export function PageLoading({ device }: { device: Device }) {
  const still = device.capabilities.includes('reducedMotion')
  const path = useLocation().pathname
  const line = (w: string, h = '1em') => <i className="skel" style={{ width: w, height: h }} />
  const card = (i: number) => <div key={i} className="card sdui-ph"><div className="poster"><i className="skel" /></div></div>
  const detail = path.startsWith('/item/') || path.startsWith('/show/')
  const library = path.startsWith('/library/')
  return (
    <div className={'sdui sdui-loading skeleton-grid' + (still ? ' still' : '')} role="status" aria-label={t('Loading')} aria-busy="true">
      {detail ? (
        <div className="hub sdui-loading-detail" aria-hidden="true">
          <i className="skel skel-detail-poster" />
          <div className="skel-lines">{line('5rem', '0.75rem')}{line('60%', '2rem')}{line('40%')}<div className="skel-actions">{line('9rem', 'var(--control-h)')}{line('6rem', 'var(--control-h)')}</div>{line('100%')}{line('100%')}{line('70%')}</div>
        </div>
      ) : library ? (
        <div className="hub" aria-hidden="true">
          <div className="hub-head">{line('9rem', '1.75rem')}</div>
          <div className="skel-chips">{line('4.5rem', '2rem')}{line('7rem', '2rem')}{line('6rem', '2rem')}</div>
          <div className="row sdui-loading-grid">{Array.from({ length: 12 }, (_, i) => card(i))}</div>
        </div>
      ) : [0, 1, 2].map((r) => (
        <div className="hub" key={r} aria-hidden="true">
          <div className="hub-head"><i className="skel sdui-skel-title" /></div>
          <div className="row">{Array.from({ length: 8 }, (_, i) => card(i))}</div>
        </div>
      ))}
    </div>
  )
}

