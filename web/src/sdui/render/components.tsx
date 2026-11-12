// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The twenty-six components of a server-driven page (docs/SDUI.md, "Components"), drawn
// with this app's own widgets and classes, so a page looks like the rest of the app. Each
// takes a view from the layout step (../model.ts) and draws what the view says: the text
// parts, their line limits, the picture, the role and label. Nothing here works out what
// to show.

import { Fragment, lazy, memo, Suspense, useEffect, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { mediaUrl } from '../../api'
import { Placeholder } from '../../components/common'
import { countWords } from '../../libraryNouns'
import { useApp } from '../../state'
import { useAudio } from '../../audio/player'
import { IconCheck, IconDown, IconLeft, IconPause, IconPlay, IconRight, IconSearch, IconStar } from '../../icons'
import { tokenVar } from '../../themeOverrides'
import { Segmented, Tabs as TabStrip } from '../../ui/Choice'
import { Select } from '../../ui/Field'
import { DockView } from './DockView'
import { useList } from '../gridView'
import { KeyLegend } from '../../ui/KeyLegend'
import { safeLink as safeHref } from '../../safeLink'
import { buttonsOf, cardGap, creditGroups, factsOf, fieldOf, gridColumns, initials, type ButtonSpec, type View } from '../model'
import { plan } from '../routes'
import type { Node, Props } from '../types'
import { px, usePage } from './context'
import { kindIcon, sduiIcon } from './icons'
import { chooseEpisodeView, useEpisodeView } from '../extras/episodeView'
import { Markdown } from './markdown'

const NativeViewHost = lazy(() => import('./NativeViewHost'))
const DetailExtras = lazy(() => import('../extras/DetailExtras'))

const str = (p: Props | undefined, k: string): string => (p && typeof p[k] === 'string' ? p[k] as string : '')
const num = (p: Props | undefined, k: string): number => (p && typeof p[k] === 'number' ? p[k] as number : 0)
const obj = (p: Props | undefined, k: string): Props | undefined => (p && p[k] && typeof p[k] === 'object' && !Array.isArray(p[k]) ? p[k] as Props : undefined)

// A node's theme scope: the colour tokens it overrides, for it and everything in it.
export function scopeStyle(node: { tokens?: Record<string, string> } | undefined): CSSProperties | undefined {
  const tokens = node?.tokens
  if (!tokens || !Object.keys(tokens).length) return undefined
  const out: Record<string, string> = {}
  for (const [name, value] of Object.entries(tokens)) {
    if (!/^[a-zA-Z][a-zA-Z0-9]*$/.test(name) || !/^#[0-9a-fA-F]{3,8}$|^rgba?\([\d\s.,%/]+\)$/.test(value)) continue
    out[tokenVar(name)] = value
    const hex = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(value)
    if (hex && (name === 'bg' || name === 'accent')) out[`--${name}-rgb`] = hex.slice(1).map((h) => parseInt(h, 16)).join(' ')
    if (name === 'accent') { out['--focus'] = value; out['--accent-text'] = tokens.accentText || value }
  }
  return out as CSSProperties
}

const clamp = (lines: number | undefined): CSSProperties | undefined => (lines && lines > 0 ? { WebkitLineClamp: lines } : undefined)
const clampClass = (lines: number | undefined) => (lines && lines > 0 ? ' sdui-clamp' : '')

// The picture a card or block asks for: the address as the server wrote it.
const picture = (image: Props | undefined) => (image && typeof image.url === 'string' && image.url.startsWith('/api/') ? mediaUrl(image.url) : '')

// Without a picture, a titled thing gets the colour of its own the app's other pages give
// it (Placeholder); a thing with an icon, or no title, gets that icon on a plain ground.
function Picture({ image, kind, icon, contain, title }: { image: Props | undefined; kind?: unknown; icon?: unknown; contain?: boolean; title?: string }) {
  const src = picture(image)
  const [failed, setFailed] = useState(false)
  useEffect(() => setFailed(false), [src])
  // Someone with no picture is shown by their initials.
  const fill = title && !icon ? <Placeholder title={title} label={kind === 'person' ? initials(title) : undefined} /> : <span className="sdui-noart" aria-hidden="true">{icon ? sduiIcon(icon) : kindIcon(kind)}</span>
  if (!src || failed) return fill
  return <>{fill}<img className={contain ? 'sdui-contain' : undefined} src={src} alt="" loading="lazy" decoding="async" onError={() => setFailed(true)} /></>
}

// ---------------------------------------------------------------- anything pressable

// A thing a person can act on: a link when it goes somewhere in the app, so it can be
// opened in a new tab, and a button otherwise.
function Pressable({ view, className, children, label, style, first, title }: { view: View; className?: string; children: ReactNode; label?: string; style?: CSSProperties; first?: boolean; title?: string }) {
  const page = usePage()
  const node = view.node
  const step = plan(node.action, { externalLinks: page.layout.externalLinks })
  const common = {
    className, style, title,
    'data-sdui-focus': node.focus ? node.id : undefined,
    'data-card-id': view.type === 'Card' ? node.id : undefined,
    'data-tv-first': first || node.focus?.default || undefined,
    'aria-label': label ?? (view.label || undefined),
    onContextMenu: view.type === 'Card' && !page.inert ? (e: React.MouseEvent) => { e.preventDefault(); page.menu(view, { x: e.clientX, y: e.clientY }) } : undefined,
  }
  if (step.do === 'nothing' || page.inert) return <div {...common} role={view.role === 'button' ? 'group' : undefined}>{children}</div>
  if (step.do === 'go') return <Link {...common} to={step.to}>{children}</Link>
  if (step.do === 'open') return <a {...common} href={safeHref(step.url) || undefined} target="_blank" rel="noopener noreferrer">{children}</a>
  return <button type="button" {...common} onClick={() => page.act(node.action, view)}>{children}</button>
}

// ---------------------------------------------------------------- cards

const SHAPE_CLASS: Record<string, string> = { poster: '', landscape: ' wide', square: ' square', circle: ' square sdui-round', chip: '' }

export const CardView = memo(function CardView({ view, showTitles }: { view: View; showTitles: boolean }) {
  const p = view.node.props || {}
  const { libraries } = useApp()
  const style = str(p, 'style'), shape = str(p, 'shape') || 'poster'
  const shows = view.shows || []
  const text = view.text || {}
  if (shape === 'chip') {
    return <Pressable view={view} className="chip sdui-chip">{p.icon ? sduiIcon(p.icon) : null}<span>{text.title}</span></Pressable>
  }
  if (style === 'tile') {
    // A library, a collection: its name over the picture, or beside an icon on a plain tile.
    const image = obj(p, 'image')
    const libId = /^lib(\d+)$/.exec(str(p, 'itemId'))?.[1]
    const libraryType = libId ? libraries.find((l) => String(l.id) === libId)?.type : undefined
    return (
      <Pressable view={view} className={'lib-tile sdui-tile' + (image ? ' pictured' : '')}>
        {image ? <span className="sdui-tile-art"><Picture image={image} kind={p.kind} /></span> : <span className="lib-tile-ic" aria-hidden="true">{sduiIcon(p.icon) || kindIcon(p.kind)}</span>}
        <span className="lib-tile-txt"><b>{text.title}</b>{text.count !== undefined && <span className="muted">{countWords(libraryType, Number(text.count))}</span>}</span>
      </Pressable>
    )
  }
  const channel = style === 'channel'
  const now = obj(p, 'now')
  return (
    <Pressable view={view} className={'card' + (SHAPE_CLASS[shape] ?? '') + (style === 'game' ? ' sdui-game' : '') + (channel ? ' sdui-channel' : '')} title={str(p, 'title') || undefined}>
      {text.rank && <span className="sdui-rank" aria-hidden="true">{text.rank}</span>}
      <div className="poster">
        <Picture image={obj(p, 'image')} kind={p.kind} icon={p.icon} contain={channel || str(obj(p, 'image'), 'kind') === 'logo'} title={channel ? undefined : text.title || str(p, 'title') || undefined} />
        {shows.includes('badges') && <span className="sdui-badges">{(text.badges || '').split(' ').map((b) => <span key={b} className="badge">{b}</span>)}</span>}
        {shows.includes('unwatched') && <span className="badge count" aria-hidden="true">{text.unwatched}</span>}
        {shows.includes('watched') && <span className="badge seen" aria-hidden="true"><IconCheck /></span>}
        {str(p, 'playId') && <div className="play"><span><IconPlay /></span></div>}
        {view.progress !== undefined && view.progress > 0 && !channel && <div className="progress"><i style={{ width: Math.min(100, view.progress) + '%' }} /></div>}
      </div>
      {channel ? (
        <>
          <div className="title sdui-clamp" style={clamp(view.lines?.title)}>{text.number && <span className="sdui-ch-no">{text.number}</span>}{showTitles ? text.title : null}</div>
          {now && <div className="sub sdui-clamp" style={clamp(view.lines?.now)}>{text.now}</div>}
          {view.progress !== undefined && <div className="sdui-ch-bar" aria-hidden="true"><i style={{ width: Math.min(100, view.progress) + '%' }} /></div>}
          {text.next && <div className="sub sdui-next sdui-clamp" style={clamp(view.lines?.next)}>Next: {text.next}</div>}
        </>
      ) : (
        <>
          {text.title && <div className={'title' + (view.lines?.title && view.lines.title > 1 ? ' sdui-clamp' : '')} style={view.lines?.title && view.lines.title > 1 ? clamp(view.lines.title) : undefined}>{text.title}</div>}
          {(text.subtitle || text.remaining) && <div className="sub">{[text.subtitle, text.remaining].filter(Boolean).join(' · ')}</div>}
          {text.synopsis && <div className="sub sdui-synopsis sdui-clamp" style={clamp(view.lines?.synopsis)}>{text.synopsis}</div>}
        </>
      )}
    </Pressable>
  )
})

function Placeholders({ view, grid }: { view: View; grid?: boolean }) {
  const page = usePage()
  const n = grid ? Math.max(view.columns || 0, view.placeholders || 0) || 6 : view.placeholders || 6
  const shape = str(obj(view.node.props, 'card'), 'shape')
  return <>{Array.from({ length: n }, (_, i) => <div key={i} className={'card sdui-ph' + (SHAPE_CLASS[shape] ?? '') + (page.reducedMotion ? ' still' : '')} aria-hidden="true"><div className="poster"><i className="skel" /></div></div>)}</>
}

// ---------------------------------------------------------------- sections that hold cards

// Asks for more when the marker comes within a screen of being seen.
function Near({ onNear, margin = '100%', horizontal, root }: { onNear: () => void; margin?: string; horizontal?: boolean; root?: React.RefObject<HTMLElement | null> }) {
  const mark = useRef<HTMLSpanElement | null>(null)
  const fn = useRef(onNear)
  fn.current = onNear
  useEffect(() => {
    const el = mark.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver((entries) => { if (entries.some((e) => e.isIntersecting)) fn.current() }, { root: root?.current || null, rootMargin: horizontal ? `0px ${margin} 0px ${margin}` : `${margin} 0px ${margin} 0px` })
    io.observe(el)
    return () => io.disconnect()
  }, [margin, horizontal, root])
  return <span ref={mark} className="sdui-near" aria-hidden="true" />
}

function cardVars(view: View, pt: number): CSSProperties {
  const w = view.cardWidth || 0
  if (!w) return {}
  return { '--card-w': px(w, pt), '--card-wide-w': px(w, pt), '--sdui-card-h': px(view.cardHeight || 0, pt) } as CSSProperties
}

function Heading({ view, level = 2, id }: { view: View; level?: 2 | 3; id: string }) {
  const title = view.text?.title
  const sub = view.text?.subtitle
  const H = level === 2 ? 'h2' : 'h3'
  if (!title && !(view.seeAll && !view.seeAllCard)) return null
  return (
    <div className="hub-head" data-sdui-head>
      <div className="sdui-head-text">{title && <H id={id} className="sdui-clamp" style={clamp(view.lines?.title)}>{title}</H>}{sub && <p className="muted">{sub}</p>}</div>
      {view.seeAll && !view.seeAllCard && view.seeAll.startsWith('/') && <Link to={view.seeAll} aria-label={`See all: ${title || ''}`}>{str(obj(view.node.props, 'seeAll'), 'label') || 'See all'} <IconRight /></Link>}
    </div>
  )
}

function StateLine({ view }: { view: View }) {
  const page = usePage()
  if (view.state === 'empty') return <p className="muted sdui-empty">{view.text?.empty || 'Nothing here yet.'}</p>
  if (view.state === 'error' || page.failed(view.id)) {
    return <p className="sdui-error" role="alert"><span>Couldn’t load this</span> <button type="button" className="btn sm" onClick={() => page.more(view.id)} disabled={page.busy(view.id)}>Retry</button></p>
  }
  return null
}

const loading = (view: View, busy: boolean) => view.state === 'deferred' || (busy && view.children.length === 0)

// The card that ends a shelf: the same size as its cards, so the row keeps its rhythm, and a link to the whole list.
function SeeAllCard({ to, label, title, wide }: { to: string; label: string; title?: string; wide?: boolean }) {
  return (
    <Link to={to} className={'card sdui-seeall' + (wide ? ' wide' : '')} aria-label={`${label}: ${title || ''}`.trim()}>
      <div className="poster"><span className="sdui-seeall-in"><IconRight size={28} /><b>{label}</b></span></div>
    </Link>
  )
}

function ShelfView({ view, depth }: { view: View; depth: number }) {
  const page = usePage()
  const row = useRef<HTMLDivElement | null>(null)
  const prev = useRef<HTMLButtonElement | null>(null)
  const next = useRef<HTMLButtonElement | null>(null)
  const id = 'sdui-h-' + view.id
  const waiting = loading(view, page.busy(view.id))
  const failed = view.state === 'error' || page.failed(view.id)
  useEffect(() => {
    const el = row.current
    if (!el) return
    let frame = 0
    const update = () => {
      frame = 0
      if (prev.current) prev.current.hidden = el.scrollLeft < 8
      if (next.current) next.current.hidden = el.scrollLeft + el.clientWidth >= el.scrollWidth - 8
    }
    const later = () => { if (!frame) frame = requestAnimationFrame(update) }
    update()
    el.addEventListener('scroll', later, { passive: true })
    const size = new ResizeObserver(later)
    size.observe(el)
    return () => { el.removeEventListener('scroll', later); size.disconnect(); cancelAnimationFrame(frame) }
  }, [view.children.length])
  const by = (dir: number) => { const el = row.current; if (el) el.scrollBy({ left: dir * el.clientWidth * 0.8, behavior: page.reducedMotion ? 'auto' : 'smooth' }) }
  const showTitles = view.node.props?.showTitles !== false
  const tiles = view.cardStyle === 'tile' && !view.children.some((c) => obj(c.node.props, 'image'))
  const chips = view.cardStyle === 'chip'
  // See all is the last card of the row (as well as its heading's link): the end of the row says there is more.
  const seeAllCard = !!view.seeAllCard && !!view.seeAll && view.seeAll.startsWith('/') && view.children.length > 0 && !chips
  const seeAllLabel = str(obj(view.node.props, 'seeAll'), 'label') || 'See all'
  const wideCards = view.cardStyle === 'landscape' || view.children[0]?.node.props?.shape === 'landscape'
  return (
    <section className={'hub sdui-shelf' + (view.node.props?.numbered ? ' numbered' : '')} style={{ ...cardVars(view, page.pt), '--peek': view.peek || 0, ...scopeStyle(view.node) } as CSSProperties} data-scope={view.scope} data-sdui-section={depth === 1 ? view.id : undefined} aria-busy={waiting || undefined}>
      <Heading view={view} id={id} />
      {view.state === 'deferred' && !failed && <Near onNear={() => page.more(view.id)} />}
      {failed || view.state === 'empty' ? <StateLine view={view} /> : tiles && !waiting ? (
        // Plain tiles (libraries) wrap, as the app's library tiles always have.
        <div className="lib-tiles" role="list" aria-labelledby={view.text?.title ? id : undefined} aria-label={view.text?.title ? undefined : view.label} data-nav-group>
          {view.children.map((c) => <div role="listitem" key={c.id}><CardView view={c} showTitles={showTitles} /></div>)}
        </div>
      ) : (
        <div className="row-wrap">
          <button ref={prev} type="button" className="row-arrow prev" tabIndex={-1} aria-hidden="true" onClick={() => by(-1)} hidden><IconLeft /></button>
          <div className={'row' + (chips ? ' sdui-chips' : '')} ref={row} data-nav-group role="list" aria-labelledby={view.text?.title ? id : undefined} aria-label={view.text?.title ? undefined : view.label || undefined}>
            {waiting ? <Placeholders view={view} /> : view.children.map((c) => <div role="listitem" key={c.id} className="sdui-item"><CardView view={c} showTitles={showTitles} /></div>)}
            {view.hasMore && !waiting && <Near horizontal root={row} onNear={() => page.more(view.id)} />}
            {page.busy(view.id) && !waiting && <Placeholders view={{ ...view, placeholders: 2 }} />}
            {seeAllCard && !waiting && !view.hasMore && <div role="listitem" className="sdui-item"><SeeAllCard to={view.seeAll!} label={seeAllLabel} title={view.text?.title} wide={wideCards} /></div>}
          </div>
          <button ref={next} type="button" className="row-arrow next" tabIndex={-1} aria-hidden="true" onClick={() => by(1)} hidden><IconRight /></button>
        </div>
      )}
    </section>
  )
}

function GridView({ view, depth }: { view: View; depth: number }) {
  const page = usePage()
  const box = useRef<HTMLDivElement | null>(null)
  const sent = view.columns || 0
  const [cols, setCols] = useState(sent)
  const id = 'sdui-h-' + view.id
  const width = view.cardWidth || 0
  // The count for the window as it is now: the one sent is for the viewport sent.
  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    const work = () => {
      const n = gridColumns(page.layout, width, el.clientWidth / page.pt, sent)
      setCols(n)
      page.columns(view.id, n)
    }
    work()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(work)
    ro.observe(el)
    return () => ro.disconnect()
  }, [width, sent, page.layout, page.pt, view.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const waiting = loading(view, page.busy(view.id))
  const failed = view.state === 'error' || page.failed(view.id)
  const showTitles = view.node.props?.showTitles !== false
  const gap = px(cardGap(page.layout), page.pt)
  const tiles = view.cardStyle === 'tile'
  const role = view.role === 'grid' ? 'list' : view.role // cards are links: a list of them, in rows
  const { user } = useApp()
  const asList = useList(page.doc.page.id, view.id, user?.id ?? 0)
  return (
    <section className="sdui-grid-section" style={{ ...cardVars(view, page.pt), ...scopeStyle(view.node) }} data-scope={view.scope} data-sdui-section={depth === 1 ? view.id : undefined} aria-busy={waiting || undefined}>
      <Heading view={view} id={id} />
      {view.state === 'deferred' && !failed && <Near onNear={() => page.more(view.id)} />}
      {failed || view.state === 'empty' ? <StateLine view={view} /> : (
        <div ref={box} className={'grid sdui-grid' + (tiles ? ' sdui-tiles' : '') + (asList ? ' as-list' : '')} role={role} aria-labelledby={view.text?.title ? id : undefined} aria-label={view.text?.title ? undefined : view.label || undefined}
          data-nav-group data-cols={cols} style={{ gridTemplateColumns: asList ? undefined : `repeat(${Math.max(1, cols || 1)}, minmax(0, 1fr))`, columnGap: gap }}>
          {waiting ? <Placeholders view={view} grid /> : view.children.map((c) => <div role="listitem" key={c.id} className="sdui-item"><CardView view={c} showTitles={showTitles} /></div>)}
          {page.busy(view.id) && !waiting && <Placeholders view={{ ...view, columns: cols }} grid />}
        </div>
      )}
      {view.hasMore && !waiting && !failed && <Near margin="100%" onNear={() => page.more(view.id)} />}
    </section>
  )
}

// ---------------------------------------------------------------- spotlight

function ActionButtons({ buttons, view, className }: { buttons: ButtonSpec[]; view: View; className?: string }) {
  const page = usePage()
  return (
    <div className={className || 'actions'}>
      {buttons.map((b) => {
        const step = plan(b.action, { externalLinks: page.layout.externalLinks })
        const cls = 'btn' + (b.primary ? ' primary' : '')
        const body = <>{sduiIcon(b.icon)} {b.label}</>
        if (step.do === 'nothing') return null
        if (step.do === 'go' && !page.inert) return <Link key={b.id} className={cls} to={step.to} data-tv-first={b.primary || undefined}>{body}</Link>
        return <button key={b.id} type="button" className={cls} data-tv-first={b.primary || undefined} onClick={() => !page.inert && page.act(b.action, view)}>{body}</button>
      })}
    </div>
  )
}

function HeroView({ view }: { view: View }) {
  const page = usePage()
  const slides = view.children
  const [i, setI] = useState(0)
  const seconds = num(view.node.props, 'autoAdvanceSec')
  const [paused, setPaused] = useState(() => page.reducedMotion || seconds <= 0)
  const [hold, setHold] = useState(false)
  const box = useRef<HTMLDivElement | null>(null)
  const [seen, setSeen] = useState(true)
  useEffect(() => {
    const el = box.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver((e) => setSeen(e[e.length - 1].isIntersecting))
    io.observe(el)
    return () => io.disconnect()
  }, [])
  // It never moves by itself with reduced motion, while a button of its has focus or the
  // pointer is over it, or off screen.
  useEffect(() => {
    if (slides.length < 2 || paused || hold || !seen || seconds <= 0 || page.reducedMotion || page.inert) return
    const t = setInterval(() => setI((x) => (x + 1) % slides.length), seconds * 1000)
    return () => clearInterval(t)
  }, [slides.length, paused, hold, seen, seconds, page.reducedMotion, page.inert])
  const at = Math.min(i, slides.length - 1)
  const slide = slides[at]
  if (!slide) return view.state === 'deferred' ? <div className="hero sdui-hero skel-hero" aria-hidden="true"><div className="shade" /></div> : null
  const p = slide.node.props || {}
  const n = slides.length
  const near = (k: number) => k === at || k === (at + 1) % n || k === (at + n - 1) % n
  const facts = factsOf(p)
  const factIds = (Array.isArray(p.facts) ? p.facts : []).map((f) => str(f as Props, 'id'))
  const size = obj(view.node.props, 'size')
  const logo = picture(obj(p, 'logo'))
  const step = (d: number) => setI((x) => (x + d + n) % n)
  return (
    <div ref={box} className={'hero sdui-hero' + (str(view.node.props, 'style') === 'banner' ? ' banner' : '') + (page.reducedMotion ? ' cut' : '') + (picture(obj(p, 'image')) ? '' : ' bare')} role="region" aria-label={view.label}
      style={{ maxHeight: size ? px(num(size, 'height'), page.pt) : undefined, ...scopeStyle(view.node) }} data-sdui-stop={view.id} data-sdui-focus={view.node.focus ? view.id : undefined} data-sdui-section={view.id}
      onMouseEnter={() => setHold(true)} onMouseLeave={() => setHold(false)} onFocus={() => setHold(true)} onBlur={() => setHold(false)}
      data-sdui-hero-step onKeyDown={undefined}>
      {slides.map((s, k) => {
        const src = near(k) ? picture(obj(s.node.props, 'image')) : ''
        return <div key={s.id} className="bg" style={{ backgroundImage: src ? `url("${src}")` : undefined, opacity: k === at ? 1 : 0 }} />
      })}
      <div className="shade" />
      <div className="info" role="group" aria-label={slide.label} aria-live={hold ? undefined : 'polite'}>
        {logo ? <h1 className="sdui-logo"><img src={logo} alt={slide.text?.title || ''} /></h1> : <h1 className="sdui-clamp" style={clamp(slide.lines?.title)}>{slide.text?.title}</h1>}
        {facts.length > 0 && (
          <div className="meta">
            {facts.map((f, k) => (factIds[k] === 'contentRating' ? <span key={k} className="pill">{f}</span> : factIds[k] === 'rating' ? <span key={k} className="star"><IconStar /> {f}</span> : <span key={k}>{f}</span>))}
          </div>
        )}
        {slide.text?.synopsis && <p className="sdui-clamp" style={clamp(slide.lines?.synopsis)}>{slide.text.synopsis}</p>}
        <ActionButtons buttons={buttonsOf(p)} view={slide} />
      </div>
      {n > 1 && (
        <>
          {!page.touch && !page.tv && <>
            <button type="button" className="sdui-hero-arrow prev" tabIndex={-1} aria-hidden="true" onClick={() => step(-1)}><IconLeft /></button>
            <button type="button" className="sdui-hero-arrow next" tabIndex={-1} aria-hidden="true" onClick={() => step(1)}><IconRight /></button>
          </>}
          <div className="dots">
            {seconds > 0 && !page.reducedMotion && <button type="button" className="hero-pause" data-nav-skip={page.tv || undefined} aria-label={paused ? 'Start turning the featured titles' : 'Stop turning the featured titles'} title={paused ? 'Play' : 'Pause'} onClick={() => setPaused((x) => !x)}>{paused ? <IconPlay /> : <IconPause />}</button>}
            {slides.map((s, k) => <button key={s.id} type="button" data-nav-skip={page.tv || undefined} data-sdui-slide={k} aria-label={`Show featured title ${k + 1}`} aria-current={k === at || undefined} className={k === at ? 'active' : ''} onClick={() => setI(k)} />)}
          </div>
        </>
      )}
    </div>
  )
}

// ---------------------------------------------------------------- a title's header

// The looping video behind a title (canvas/): a film's or an episode's own, the one of
// what a show plays next, or an album's from its songs. Drawn only where one exists.
const TitleCanvas = lazy(() => import('../../canvas/CanvasVideo').then((m) => ({ default: m.TitleCanvas })))
const AlbumCanvas = lazy(() => import('../../canvas/CanvasVideo').then((m) => ({ default: m.AlbumCanvas })))
export function trackIdsOf(root: Node): number[] {
  const out: number[] = []
  const walk = (n: Node) => {
    if (n.type === 'Track' && /^\d+$/.test(str(n.props, 'itemId'))) out.push(Number(str(n.props, 'itemId')))
    for (const c of n.children || []) walk(c)
  }
  walk(root)
  return out
}
function HeaderCanvas({ header, doc }: { header: Node; doc: { root: Node } }) {
  const p = header.props || {}
  if (str(p, 'kind') === 'album') {
    const ids = trackIdsOf(doc.root)
    return ids.length ? <Suspense fallback={null}><AlbumCanvas trackIds={ids} /></Suspense> : null
  }
  const own = str(p, 'itemId')
  const plays = (buttonsOf(p).find((b) => b.action?.type === 'play')?.action?.item) || ''
  const id = /^\d+$/.test(own) ? Number(own) : /^\d+$/.test(plays) ? Number(plays) : 0
  return id ? <Suspense fallback={null}><TitleCanvas itemId={id} /></Suspense> : null
}

// Titles whose cover is square: what stands in for a missing one is square too.
const SQUARE_KINDS = ['album', 'podcast', 'audiobook']

function DetailHeaderView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const images = obj(p, 'images')
  const backdrop = picture(obj(images, 'backdrop'))
  const poster = obj(images, 'poster')
  const layout = str(p, 'layout') || 'backdrop'
  const [open, setOpen] = useState(false)
  const facts = factsOf(p)
  const factIds = (Array.isArray(p.facts) ? p.facts : []).map((f) => str(f as Props, 'id'))
  const logo = picture(obj(images, 'logo'))
  const buttons = buttonsOf(p)
  const primary = buttons.find((b) => b.primary)
  const overview = view.text?.overview
  return (
    <div className={'sdui-detail layout-' + layout} role="region" aria-label={view.label} style={scopeStyle(view.node)} data-sdui-stop={view.id} data-sdui-focus={view.node.focus ? view.id : undefined} data-sdui-section={view.id}>
      {layout !== 'compact' && (
        <div className={'detail-hero' + (backdrop ? '' : ' bare')}>
          <div className="bg" style={backdrop ? { backgroundImage: `url("${backdrop}")` } : undefined} />
          {!page.inert && <HeaderCanvas header={view.node} doc={page.doc} />}
          <div className="shade" />
        </div>
      )}
      <div className={'detail' + (layout === 'compact' ? ' compact' : '')}>
        {poster && layout !== 'compact' && <div><div className={'poster' + (str(poster, 'aspect') === '1/1' ? ' square' : '')}><Picture image={poster} kind={p.kind} title={view.text?.title} /></div></div>}
        {!poster && layout === 'poster' && <div><div className={'poster' + (SQUARE_KINDS.includes(str(p, 'kind')) ? ' square' : '')}><Picture image={undefined} kind={p.kind} title={view.text?.title} /></div></div>}
        <div>
          {logo ? <h1 className="sdui-logo"><img src={logo} alt={view.text?.title || ''} /></h1> : <h1 className="sdui-clamp" style={clamp(view.lines?.title)}>{view.text?.title}</h1>}
          {view.text?.tagline && <div className="tagline">{view.text.tagline}</div>}
          {facts.length > 0 && (
            <div className="meta">
              {facts.map((f, k) => (factIds[k] === 'contentRating' ? <span key={k} className="pill">{f}</span> : factIds[k] === 'rating' ? <span key={k} className="star"><IconStar /> {f}</span> : <span key={k}>{f}</span>))}
            </div>
          )}
          {view.progress !== undefined && view.progress > 0 && <div className="sdui-detail-progress" role="progressbar" aria-label="Watched" aria-valuemin={0} aria-valuemax={100} aria-valuenow={view.progress}><i style={{ width: view.progress + '%' }} /></div>}
          <div className="actions sdui-detail-actions" data-nav-group>
            {buttons.map((b) => {
              const step = plan(b.action, { externalLinks: page.layout.externalLinks })
              if (step.do === 'nothing') return null
              const cls = 'btn' + (b.primary ? ' primary' : '')
              const body = <>{sduiIcon(b.icon)} {b.label}</>
              if (step.do === 'go' && !page.inert) return <Link key={b.id} className={cls} to={step.to} data-tv-first={b === primary || undefined}>{body}</Link>
              return <button key={b.id} type="button" className={cls} data-tv-first={b === primary || undefined} onClick={() => !page.inert && page.act(b.action, view)}>{body}</button>
            })}
            {!page.inert && <Suspense fallback={null}><DetailExtras place="tools" header={view.node} doc={page.doc} /></Suspense>}
          </div>
          {overview && (
            <div className="sdui-overview">
              <p className={'overview' + (open ? '' : ' sdui-clamp')} style={open ? undefined : clamp(view.lines?.overview)}>{overview}</p>
              {overview.length > 280 && <button type="button" className="btn sm ghost" aria-expanded={open} onClick={() => setOpen((o) => !o)}>{open ? 'Less' : 'More'}</button>}
            </div>
          )}
          {!page.inert && <Suspense fallback={null}><DetailExtras place="below" header={view.node} doc={page.doc} /></Suspense>}
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- cast and crew

// A row of portraits that scrolls sideways, with the arrows the app's other rows have.
function PortraitRow({ views, label, labelledBy, reducedMotion }: { views: View[]; label: string; labelledBy?: string; reducedMotion: boolean }) {
  const row = useRef<HTMLDivElement | null>(null)
  const prev = useRef<HTMLButtonElement | null>(null)
  const next = useRef<HTMLButtonElement | null>(null)
  useEffect(() => {
    const el = row.current
    if (!el) return
    let frame = 0
    const update = () => {
      frame = 0
      if (prev.current) prev.current.hidden = el.scrollLeft < 8
      if (next.current) next.current.hidden = el.scrollLeft + el.clientWidth >= el.scrollWidth - 8
    }
    const later = () => { if (!frame) frame = requestAnimationFrame(update) }
    update()
    el.addEventListener('scroll', later, { passive: true })
    const size = new ResizeObserver(later)
    size.observe(el)
    return () => { el.removeEventListener('scroll', later); size.disconnect(); cancelAnimationFrame(frame) }
  }, [views.length])
  const by = (dir: number) => { const el = row.current; if (el) el.scrollBy({ left: dir * el.clientWidth * 0.8, behavior: reducedMotion ? 'auto' : 'smooth' }) }
  return (
    <div className="row-wrap">
      <button ref={prev} type="button" className="row-arrow prev" tabIndex={-1} aria-hidden="true" onClick={() => by(-1)} hidden><IconLeft /></button>
      <div className="row sdui-portraits" ref={row} data-nav-group role="list" aria-labelledby={labelledBy} aria-label={labelledBy ? undefined : label}>
        {views.map((c) => <div role="listitem" key={c.id} className="sdui-item"><CardView view={c} showTitles /></div>)}
      </div>
      <button ref={next} type="button" className="row-arrow next" tabIndex={-1} aria-hidden="true" onClick={() => by(1)} hidden><IconRight /></button>
    </div>
  )
}

// One department's people: each a link to their page, the name over the job.
function CrewPerson({ view }: { view: View }) {
  const text = view.text || {}
  return (
    <Pressable view={view} className="sdui-crew-person" label={[text.title, text.subtitle].filter(Boolean).join(', ')}>
      <span className="sdui-crew-name">{text.title}</span>
      {text.subtitle && <span className="sdui-crew-job">{text.subtitle}</span>}
    </Pressable>
  )
}

// Who is in a title and who made it (docs/SDUI.md, "Cast and crew"): the cast and the guest
// stars as rows of portraits, then each department as a heading and its people; Show all asks
// for everyone in place of the first few.
function CreditsView({ view, depth }: { view: View; depth: number }) {
  const page = usePage()
  const p = view.node.props || {}
  const id = 'sdui-h-' + view.id
  const groups = creditGroups(p)
  const byGroup = new Map<string, View[]>()
  for (const c of view.children) {
    const g = str(c.node.props, 'group')
    byGroup.set(g, [...(byGroup.get(g) || []), c])
  }
  const busy = page.busy(view.id)
  const failed = view.state === 'error' || page.failed(view.id)
  const rows = groups.filter((g) => g.role !== 'crew' && byGroup.get(g.id)?.length)
  const crew = groups.filter((g) => g.role === 'crew' && byGroup.get(g.id)?.length)
  const more = view.hasMore && !failed
  const title = view.text?.title
  return (
    <section className="hub sdui-credits" style={{ ...cardVars(view, page.pt), ...scopeStyle(view.node) }} data-scope={view.scope} data-sdui-section={depth === 1 ? view.id : undefined}
      role="region" aria-labelledby={title ? id : undefined} aria-label={title ? undefined : 'Cast and crew'} aria-busy={busy || undefined}>
      <div className="hub-head" data-sdui-head>
        <div className="sdui-head-text">{title && <h2 id={id} className="sdui-clamp" style={clamp(view.lines?.title)}>{title}</h2>}</div>
        {more && <button type="button" className="btn sm" onClick={() => page.more(view.id)} disabled={busy} aria-label={`Show all of ${title || 'the cast and crew'}`}>{view.buttons?.[0] || 'Show all'}</button>}
      </div>
      {failed ? <StateLine view={view} /> : (
        <>
          {rows.map((g) => {
            const hid = `${id}-${g.id}`
            // The cast needs no heading of its own under "Cast and crew"; guest stars do.
            const own = g.id !== 'cast' || rows.length > 1
            return (
              <div key={g.id} className="sdui-credits-part">
                {own && <h3 id={hid} className="sdui-credits-sub">{g.label}</h3>}
                <PortraitRow views={byGroup.get(g.id) || []} label={g.label} labelledBy={own ? hid : undefined} reducedMotion={page.reducedMotion} />
              </div>
            )
          })}
          {crew.length > 0 && (
            <div className="sdui-crew">
              {crew.map((g) => {
                const hid = `${id}-${g.id}`
                return (
                  <div key={g.id} className="sdui-dept">
                    <h3 id={hid} className="sdui-credits-sub">{g.label}</h3>
                    <ul className="sdui-dept-list" aria-labelledby={hid} data-nav-group>
                      {(byGroup.get(g.id) || []).map((c) => <li key={c.id}><CrewPerson view={c} /></li>)}
                    </ul>
                    {g.total > g.shown && !busy && <span className="sdui-dept-more muted">{g.total - g.shown} more</span>}
                  </div>
                )
              })}
            </div>
          )}
        </>
      )}
    </section>
  )
}

// ---------------------------------------------------------------- seasons and tracks

// The show a page is about, from its header ("show-5"), or ''.
function showOf(root: Node): string {
  if (root.type === 'DetailHeader') { const id = str(root.props, 'itemId'); return /^show-\d+$/.test(id) ? id : '' }
  for (const c of root.children || []) { const hit = showOf(c); if (hit) return hit }
  return ''
}

function SeasonListView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const seasons = (Array.isArray(p.seasons) ? p.seasons : []) as Props[]
  const showId = showOf(page.doc.root)
  const own = useEpisodeView(showId)
  const list = own ? own === 'list' : str(p, 'style') === 'list'
  const waiting = loading(view, page.busy(view.id) && view.children.length === 0)
  return (
    <section className="sdui-seasons" role="region" aria-label={view.label} style={{ ...cardVars(view, page.pt), ...scopeStyle(view.node) }} data-sdui-section={view.id}>
      {seasons.length > 0 && (
        <div className="season-tabs sdui-season-tabs" role="group" aria-label="Seasons" data-sdui-head data-nav-group>
          {seasons.map((s) => {
            const on = s.selected === true
            const count = num(s, 'count'), left = num(s, 'unwatched')
            return (
              <button key={str(s, 'title')} type="button" className={'season-tab sdui-season' + (on ? ' active' : '')} aria-pressed={on} aria-label={`${str(s, 'title')}${on ? ', selected' : ''}`}
                onClick={() => { if (!on && str(s, 'url')) page.more(view.id, str(s, 'url'), true) }}>
                <div className="t">{str(s, 'title')}</div>
                {count > 0 && <div className="sub">{count} ep{left > 0 && left < count ? ` · ${left} left` : ''}</div>}
              </button>
            )
          })}
        </div>
      )}
      {showId && !page.inert && (
        <div className="view-toggle sdui-episode-view">
          <span className="muted" id={view.id + '-layout'}>Layout</span>
          <div className="seg" role="group" aria-labelledby={view.id + '-layout'}>
            <button type="button" className={!list ? 'active' : ''} aria-pressed={!list} onClick={() => chooseEpisodeView(showId, 'cards')}>Cards</button>
            <button type="button" className={list ? 'active' : ''} aria-pressed={list} onClick={() => chooseEpisodeView(showId, 'list')}>Playlist</button>
          </div>
        </div>
      )}
      <StateLine view={view} />
      <div className={list ? 'sdui-episode-list' : 'episodes'} role="list" aria-label="Episodes" data-nav-group aria-busy={page.busy(view.id) || undefined}>
        {waiting ? Array.from({ length: 4 }, (_, k) => <div key={k} className="episode sdui-ph" aria-hidden="true"><span className="thumb"><i className="skel" /></span><div className="info" /></div>)
          : view.children.map((c) => <div role="listitem" key={c.id}><EpisodeRow view={c} compact={list} synopsis={p.showSynopsis !== false} /></div>)}
      </div>
      {view.hasMore && !waiting && <Near onNear={() => page.more(view.id)} />}
    </section>
  )
}

// One episode: its still (which plays it), then its number and name, running time and synopsis.
function EpisodeRow({ view, compact, synopsis }: { view: View; compact: boolean; synopsis: boolean }) {
  const page = usePage()
  const p = view.node.props || {}
  const text = view.text || {}
  const playId = str(p, 'playId')
  return (
    <div className={'episode' + (compact ? ' compact' : '')}>
      {!compact && (
        <button type="button" className="thumb" tabIndex={-1} aria-hidden="true" onClick={() => !page.inert && (playId ? page.act({ type: 'play', item: playId }, view) : page.act(view.node.action, view))}>
          <Picture image={obj(p, 'image')} kind={p.kind} />
          {playId && <span className="play"><span><IconPlay /></span></span>}
          {view.progress !== undefined && view.progress > 0 && <span className="progress"><i style={{ width: Math.min(100, view.progress) + '%' }} /></span>}
          {view.shows?.includes('watched') && <span className="badge seen"><IconCheck /></span>}
        </button>
      )}
      <div className="info">
        <Pressable view={view} className="t">{text.title}</Pressable>
        {text.subtitle && <div className="sub">{text.subtitle}{compact && view.shows?.includes('watched') ? ' · watched' : ''}</div>}
        {synopsis && text.synopsis && <p className="sdui-clamp" style={clamp(view.lines?.synopsis)}>{text.synopsis}</p>}
      </div>
    </div>
  )
}

function TrackListView({ view }: { view: View }) {
  const page = usePage()
  const { current, playing } = useAudio()
  const waiting = loading(view, page.busy(view.id))
  const showNumbers = view.node.props?.showNumbers !== false
  return (
    <section className="sdui-tracks" style={scopeStyle(view.node)} data-sdui-section={view.id}>
      <StateLine view={view} />
      <ol className="track-list" aria-label={view.label} data-nav-group>
        {waiting ? Array.from({ length: 6 }, (_, k) => <li key={k} className="sdui-ph" aria-hidden="true"><i className="skel" /></li>) : view.children.map((t) => {
          const p = t.node.props || {}
          const on = current != null && String(current.id) === str(p, 'itemId')
          return (
            <li key={t.id} className={on ? 'on' : ''}>
              <button type="button" aria-label={t.label} aria-current={on ? 'true' : undefined} data-sdui-focus={t.node.focus ? t.id : undefined} data-tv-first={t.node.focus?.default || undefined} onClick={() => !page.inert && page.act(t.node.action, t)}>
                <span className="tl-n">{on && playing ? <i className="ab-eq" aria-hidden="true"><b /><b /><b /></i> : showNumbers && num(p, 'number') ? num(p, 'number') : ''}</span>
                <span className="tl-t"><b>{t.text?.title}</b>{t.text?.subtitle && <span className="muted">{t.text.subtitle}</span>}
                  {t.progress !== undefined && t.progress > 0 && <span className="tl-prog" style={{ width: Math.min(100, t.progress) + '%' }} />}</span>
                <span className="tl-d muted">{t.text?.duration}</span>
              </button>
            </li>
          )
        })}
      </ol>
      {view.hasMore && !waiting && <Near onNear={() => page.more(view.id)} />}
    </section>
  )
}

// ---------------------------------------------------------------- tabs, filters, search

function TabsView({ view, depth }: { view: View; depth: number }) {
  const page = usePage()
  const tabs = view.children.filter((c) => c.type === 'Tab')
  const selected = tabs.find((t) => t.node.props?.selected === true) || tabs[0]
  if (!selected) return null
  const choose = (id: string) => { const t = tabs.find((x) => x.id === id); if (t && t !== selected && !page.inert) page.act(t.node.action, t) }
  const label = 'Views'
  const panel = 'sdui-panel-' + view.id
  const icon = (t: View) => sduiIcon(t.node.props?.icon)
  return (
    <section className="sdui-tabs" data-sdui-section={depth === 1 ? view.id : undefined} style={scopeStyle(view.node)}>
      <div data-sdui-tabs={view.id} data-sdui-head>
        {str(view.node.props, 'style') === 'segments'
          ? <Segmented label={label} value={selected.id} onChange={choose} options={tabs.map((t) => ({ value: t.id, label: t.text?.title || '', icon: icon(t) }))} />
          : <TabStrip label={label} value={selected.id} onChange={choose} panelId={panel} items={tabs.map((t) => ({ value: t.id, label: <>{icon(t)}{t.text?.title}</> }))} />}
      </div>
      <div id={panel} role="tabpanel" aria-label={selected.text?.title} className="sdui-stack" aria-busy={page.busy('page') || undefined}>
        {selected.children.map((c) => <Draw key={c.id} view={c} depth={depth + 1} />)}
      </div>
    </section>
  )
}

function FilterBarView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const sort = obj(p, 'sort') ? fieldOf(obj(p, 'sort'), 'sort') : null
  const fields = (Array.isArray(p.fields) ? p.fields : []).map((f) => fieldOf(f as Props))
  const chosen = fields.filter((f) => f.value)
  const first = sort?.options[0]?.value || ''
  return (
    <div className="sdui-filters toolbar" role="toolbar" aria-label={view.label} data-sdui-stop={view.id} data-sdui-focus={view.node.focus ? view.id : undefined} data-sdui-section={view.id} data-nav-group>
      {sort && sort.options.length > 0 && (
        <label className="sdui-filter"><span>Sort</span>
          <Select inputSize="sm" aria-label={`Sort: ${sort.options.find((o) => o.value === sort.value)?.label || ''}`} value={sort.value} disabled={page.inert}
            onChange={(e) => page.setQuery({ sort: e.target.value === first ? '' : e.target.value })} options={sort.options} />
        </label>
      )}
      {fields.map((f) => (
        <label className="sdui-filter" key={f.id}><span>{f.label}</span>
          <Select inputSize="sm" aria-label={`${f.label}: ${f.options.find((o) => o.value === f.value)?.label || 'Any'}`} value={f.value} disabled={page.inert}
            onChange={(e) => page.setQuery({ [f.id]: e.target.value })} options={[{ value: '', label: 'Any' }, ...f.options]} />
        </label>
      ))}
      {chosen.length > 0 && <button type="button" className="btn sm ghost" onClick={() => page.setQuery(Object.fromEntries(fields.map((f) => [f.id, ''])))}>Clear</button>}
      <span className="grow" />
      {view.text?.count && <span className="count" role="status">{Number(num(p, 'count')).toLocaleString()} {num(p, 'count') === 1 ? 'title' : 'titles'}</span>}
    </div>
  )
}

const KEYS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890'.split('')

function SearchBoxView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const onSearch = page.doc.page.kind === 'search'
  const sent = str(p, 'query')
  const [text, setText] = useState(sent)
  const field = useRef<HTMLInputElement | null>(null)
  const typed = useRef(sent)
  useEffect(() => { if (sent !== typed.current) { typed.current = sent; setText(sent) } }, [sent])
  // On the search page typing searches, a moment after it stops; anywhere else Enter opens Search.
  useEffect(() => {
    if (!onSearch || page.inert) return
    const q = text.trim()
    if (q === typed.current) return
    const t = setTimeout(() => { typed.current = q; page.setQuery({ q }) }, 300)
    return () => clearTimeout(t)
  }, [text, onSearch]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (p.autofocus === true && !page.tv && !page.touch && !page.inert) field.current?.focus({ preventScroll: true }) }, []) // eslint-disable-line react-hooks/exhaustive-deps
  const submit = () => {
    const q = text.trim()
    if (onSearch) { typed.current = q; page.setQuery({ q }) } else if (q) page.act({ type: 'navigate', to: '/search?q=' + encodeURIComponent(q) })
  }
  const type = (k: string) => setText((t) => t + (t && k !== ' ' ? k.toLowerCase() : k))
  const input = (
    <form className={page.tv ? 'tvm-search-field' : 'search-page-box sdui-search'} role="search" onSubmit={(e) => { e.preventDefault(); submit() }}>
      <IconSearch />
      <input ref={field} className="input" type="search" aria-label={view.label} placeholder={view.text?.placeholder} value={text} disabled={page.inert} autoComplete="off" spellCheck={false}
        data-sdui-focus={page.tv ? undefined : view.node.focus ? view.id : undefined} onChange={(e) => setText(e.target.value)} />
    </form>
  )
  if (!page.tv || !onSearch) return <div data-sdui-section={view.id}>{input}</div>
  // A television: letters a remote can reach, under the field a real keyboard types in.
  return (
    <div className="sdui-tv-search" data-sdui-section={view.id} data-sdui-stop={view.id} data-sdui-focus={view.node.focus ? view.id : undefined}>
      {input}
      <div className="tvm-keys sdui-keys" role="group" aria-label="Keyboard" data-nav-group data-sdui-keys>
        {KEYS.map((k, i) => <button key={k} data-tv-first={(i === 0 && view.node.focus?.default) || undefined} type="button" className="tvm-key" onClick={() => type(k)}>{k}</button>)}
        <button type="button" className="tvm-key wide" onClick={() => type(' ')}>Space</button>
        <button type="button" className="tvm-key wide" onClick={() => setText((t) => t.slice(0, -1))}>Delete</button>
        <button type="button" className="tvm-key wide" onClick={() => setText('')}>Clear</button>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------- blocks of text and pictures

function BannerView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const src = picture(obj(p, 'image'))
  const compact = str(p, 'style') === 'compact'
  const body = (
    <>
      {src ? <span className="shuffle-art" style={{ backgroundImage: `url("${src}")` }} /> : null}
      {compact && (src ? <img className="sdui-banner-thumb" src={src} alt="" loading="lazy" /> : <span className="lib-tile-ic" aria-hidden="true">{sduiIcon(p.icon) || kindIcon(p.kind)}</span>)}
      {!compact && !src && p.icon ? <span className="home-card-ic" aria-hidden="true">{sduiIcon(p.icon)}</span> : null}
      <span className="shuffle-info">
        {view.text?.title && <b className="sdui-banner-title sdui-clamp" style={clamp(view.lines?.title)}>{view.text.title}</b>}
        {view.text?.text && <span className="muted sdui-clamp" style={clamp(view.lines?.text)}>{view.text.text}</span>}
        {view.text?.button && <span className="actions"><span className="btn primary" aria-hidden="true">{str(view.node.action as unknown as Props, 'type') === 'play' ? <IconPlay /> : null} {view.text.button}</span></span>}
      </span>
    </>
  )
  return (
    <section className="hub sdui-banner-section" data-sdui-section={view.id} style={scopeStyle(view.node)} data-scope={view.scope}>
      <Pressable view={view} className={'shuffle sdui-banner' + (compact ? ' compact' : ' promo') + (src ? ' pictured' : '')} style={!compact ? { maxHeight: px(360, page.pt) } : undefined}>{body}</Pressable>
    </section>
  )
}

const TEXT_TAG: Record<string, 'h1' | 'h2' | 'h3' | 'p'> = { 'heading 1': 'h1', 'heading 2': 'h2', 'heading 3': 'h3' }

function TextView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const role = str(p, 'role') || 'body'
  const cls = `sdui-text role-${role} align-${str(p, 'align') || 'start'}` + clampClass(view.lines?.text)
  const style = clamp(view.lines?.text)
  if (str(p, 'format') === 'markdown') {
    return <div className={cls + ' sdui-md'} style={style}><Markdown text={view.text?.text || ''} externalLinks={page.layout.externalLinks !== false} inert={page.inert} /></div>
  }
  const Tag = TEXT_TAG[view.role] || 'p'
  return <Tag className={cls} style={style}>{view.text?.text}</Tag>
}

const TONE: Record<string, string> = { info: 'info', success: 'ok', warning: 'warn', danger: 'danger' }

function NoticeView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const step = plan(view.node.action, { externalLinks: page.layout.externalLinks })
  const focus = { 'data-sdui-focus': view.node.focus ? view.id : undefined, 'data-tv-first': view.node.focus?.default || undefined }
  return (
    <div className={'ui-notice sdui-notice ' + (TONE[str(p, 'tone')] || 'info')} role={view.role} data-sdui-section={view.id} style={scopeStyle(view.node)}>
      {p.icon ? <span className="sdui-notice-ic" aria-hidden="true">{sduiIcon(p.icon)}</span> : null}
      <div className="ui-notice-text">{view.text?.title && <strong>{view.text.title}</strong>}<span>{view.text?.text}</span></div>
      {view.text?.button && step.do !== 'nothing' && (
        step.do === 'go' && !page.inert ? <Link className="btn sm" to={step.to} {...focus}>{view.text.button}</Link>
          : step.do === 'open' ? <a className="btn sm" href={safeHref(step.url) || undefined} target="_blank" rel="noopener noreferrer" {...focus}>{view.text.button}</a>
            : <button type="button" className="btn sm" {...focus} onClick={() => !page.inert && page.act(view.node.action, view)}>{view.text.button}</button>
      )}
    </div>
  )
}

function ButtonView({ view }: { view: View }) {
  const p = view.node.props || {}
  const variant = str(p, 'variant')
  return <Pressable view={view} className={'btn' + (variant === 'primary' ? ' primary' : variant === 'ghost' ? ' ghost' : '')} label={view.label}>{sduiIcon(p.icon)} <span className="sdui-clamp" style={clamp(view.lines?.title)}>{view.text?.title}</span></Pressable>
}

function ImageView({ view }: { view: View }) {
  const page = usePage()
  const p = view.node.props || {}
  const image = obj(p, 'image')
  const src = picture(image)
  const [failed, setFailed] = useState(false)
  const aspect = (str(image, 'aspect') || str(p, 'aspect') || '16/9').replace('/', ' / ')
  const body = (
    <span className={'sdui-image' + (p.rounded === false ? '' : ' rounded') + (str(p, 'fit') === 'contain' ? ' contain' : '')} style={{ aspectRatio: aspect, maxWidth: px(960, page.pt) }}>
      {src && !failed ? <img src={src} alt={view.role === 'img' && !view.node.action ? view.label : ''} loading="lazy" decoding="async" onError={() => setFailed(true)} /> : <span className="sdui-noart" aria-hidden="true">{sduiIcon('photo')}</span>}
    </span>
  )
  if (view.node.action) return <Pressable view={view} className="sdui-image-link" label={view.label || 'Open'}>{body}</Pressable>
  return <div role={view.role === 'img' ? undefined : 'presentation'} aria-hidden={view.role === 'none' || undefined}>{body}</div>
}

function ListView({ view, depth }: { view: View; depth: number }) {
  const page = usePage()
  const id = 'sdui-h-' + view.id
  const waiting = loading(view, page.busy(view.id))
  return (
    <section className={'sdui-list-section' + (view.node.props?.dense === true ? ' dense' : '')} data-sdui-section={depth === 1 ? view.id : undefined} style={scopeStyle(view.node)}>
      <Heading view={view} id={id} />
      {view.state === 'deferred' && <Near onNear={() => page.more(view.id)} />}
      <StateLine view={view} />
      {view.state !== 'empty' && (
        <ul className="sdui-list" aria-labelledby={view.text?.title ? id : undefined} aria-label={view.text?.title ? undefined : view.label || undefined} data-nav-group>
          {waiting ? Array.from({ length: 4 }, (_, k) => <li key={k} className="sdui-ph" aria-hidden="true"><i className="skel" /></li>) : view.children.map((c) => <li key={c.id}><ListItemView view={c} /></li>)}
        </ul>
      )}
      {view.hasMore && !waiting && <Near onNear={() => page.more(view.id)} />}
    </section>
  )
}

// A line of a list. A title's picture keeps its own shape at the row's height (a poster
// upright, a still wide, a cover square), with how far it was watched under it, and the
// title's badges (two at most) after its words (docs/SDUI.md, "Layout options").
const shapeOf = (image: Props | undefined): string => {
  const a = str(image, 'aspect')
  return a === '2/3' ? 'poster' : a === '16/9' ? 'wide' : a === '1/1' ? 'square' : str(image, 'kind') === 'poster' ? 'poster' : ''
}

function ListItemView({ view }: { view: View }) {
  const p = view.node.props || {}
  const image = obj(p, 'image')
  const progress = typeof p.progress === 'number' && p.progress > 0 ? Math.min(1, p.progress) : 0
  const badges = (Array.isArray(p.badges) ? p.badges : []).filter((b): b is string => typeof b === 'string').slice(0, 2)
  return (
    <Pressable view={view} className="sdui-row">
      {image ? (
        <span className={'sdui-row-art' + (shapeOf(image) ? ' ' + shapeOf(image) : '')}>
          <Picture image={image} kind={p.kind} title={view.text?.title} />
          {progress > 0 && <span className="sdui-row-progress" aria-hidden="true"><i style={{ width: Math.round(progress * 100) + '%' }} /></span>}
        </span>
      ) : p.icon ? <span className="sdui-row-ic" aria-hidden="true">{sduiIcon(p.icon)}</span> : null}
      <span className="sdui-row-text"><b className="sdui-clamp" style={clamp(view.lines?.title)}>{view.text?.title}</b>{view.text?.subtitle && <span className="muted sdui-clamp" style={clamp(view.lines?.subtitle)}>{view.text.subtitle}</span>}</span>
      {badges.length > 0 && <span className="sdui-row-badges">{badges.map((b) => <span key={b} className="badge">{b}</span>)}</span>}
      {view.text?.trailing && <span className="muted sdui-row-end">{view.text.trailing}</span>}
      {view.node.action && <span className="sdui-row-go" aria-hidden="true"><IconRight /></span>}
    </Pressable>
  )
}

function StackView({ view, depth }: { view: View; depth: number }) {
  const page = usePage()
  const p = view.node.props || {}
  const row = str(p, 'direction') === 'row'
  const gap = page.layout.spacing[str(p, 'gap') || 'm'] ?? page.layout.spacing.m ?? 16
  const align = str(p, 'align') || 'stretch'
  const style: CSSProperties = { gap: px(gap, page.pt), alignItems: align === 'start' ? 'flex-start' : align === 'end' ? 'flex-end' : align, flexWrap: row && p.wrap !== false ? 'wrap' : undefined, ...scopeStyle(view.node) }
  return <div className={'sdui-stack' + (row ? ' row-dir' : '')} style={style} data-nav-group={row || undefined} data-sdui-section={depth === 1 ? view.id : undefined} data-scope={view.scope}>{view.children.map((c) => <Draw key={c.id} view={c} depth={depth + 1} />)}</div>
}

const BREAKS = ['compact', 'medium', 'expanded', 'large']

function SplitView({ view, depth }: { view: View; depth: number }) {
  const page = usePage()
  const p = view.node.props || {}
  const below = str(p, 'stackBelow') || 'medium'
  const ff = page.layout.formFactor
  const side = ff === 'tv' ? true : ff === 'phone' ? false : BREAKS.indexOf(page.layout.breakpoint) >= BREAKS.indexOf(below)
  const [a, b] = (str(p, 'ratio') || '1:1').split(':')
  const gap = page.layout.spacing[str(p, 'gap') || 'm'] ?? 16
  return (
    <div className="sdui-split" data-sdui-section={depth === 1 ? view.id : undefined} style={{ gap: px(gap, page.pt), gridTemplateColumns: side ? `minmax(0, ${a || 1}fr) minmax(0, ${b || 1}fr)` : 'minmax(0, 1fr)', ...scopeStyle(view.node) }}>
      {view.children.map((c) => <div key={c.id} className="sdui-stack"><Draw view={c} depth={depth + 1} /></div>)}
    </div>
  )
}

// ---------------------------------------------------------------- everything, by type

export function Draw({ view, depth }: { view: View; depth: number }): ReactNode {
  const page = usePage()
  switch (view.type) {
    case 'Hero': return <HeroView view={view} />
    case 'Shelf': case 'Carousel': return <ShelfView view={view} depth={depth} />
    case 'Grid': return <GridView view={view} depth={depth} />
    case 'Card': return <CardView view={view} showTitles />
    case 'Tabs': case 'CrossBar': return <TabsView view={view} depth={depth} />
    case 'NumberedRow': return <ListView view={view} depth={depth} />
    case 'WidgetGrid': return <div className="sdui-widgets" style={{ gridTemplateColumns: `repeat(${Math.max(1, num(view.node.props, 'columns') || 4)}, minmax(0, 1fr))` }} data-nav-group>{view.children.map((c) => <Draw key={c.id} view={c} depth={depth + 1} />)}</div>
    case 'Widget': return <section className="sdui-widget elev-1" style={{ gridColumn: `span ${Math.max(1, num(view.node.props, 'w') || 1)}`, gridRow: `span ${Math.max(1, num(view.node.props, 'h') || 1)}` }} aria-label={view.label}>{view.text?.title && <h3>{view.text.title}</h3>}{view.children.map((c) => <Draw key={c.id} view={c} depth={depth + 1} />)}</section>
    case 'Tab': return <div className="sdui-stack">{view.children.map((c) => <Draw key={c.id} view={c} depth={depth + 1} />)}</div>
    case 'FilterBar': return <FilterBarView view={view} />
    case 'DetailHeader': return <DetailHeaderView view={view} />
    case 'SeasonList': return <SeasonListView view={view} />
    case 'TrackList': return <TrackListView view={view} />
    case 'Credits': return <CreditsView view={view} depth={depth} />
    case 'Track': return null // drawn by its list
    case 'Banner': return <BannerView view={view} />
    case 'Text': return <TextView view={view} />
    case 'Notice': return <NoticeView view={view} />
    // Two components of the catalogue (docs/plans/DESIGN-2.md): a screen's Dock and the key legend.
    case 'Dock': return <DockView view={view} />
    case 'KeyLegend': return <KeyLegend />
    case 'Divider': return <hr className={'sdui-divider' + (view.node.props?.inset === true ? ' inset' : '')} />
    case 'Spacer': {
      const size = page.layout.spacing[str(view.node.props, 'size') || 'm'] ?? 16
      return <div className="sdui-spacer" aria-hidden="true" style={{ height: px(size, page.pt), width: px(size, page.pt) }} />
    }
    case 'Split': return <SplitView view={view} depth={depth} />
    case 'Button': return <ButtonView view={view} />
    case 'Image': return <ImageView view={view} />
    case 'SearchBox': return <SearchBoxView view={view} />
    case 'List': return <ListView view={view} depth={depth} />
    case 'ListItem': return <ListItemView view={view} />
    case 'NativeView': return (
      <Suspense fallback={<div className="loading-box" role="status" aria-label="Loading"><div className="spinner" /></div>}>
        <NativeViewHost view={str(view.node.props, 'view')} library={str(view.node.props, 'library')} inert={!!page.inert} props={view.node.props || {}}>
          {view.children.map((c) => <Draw key={c.id} view={c} depth={depth + 1} />)}
        </NativeViewHost>
      </Suspense>
    )
    // A Stack, and anything this app doesn't know: a plain column of what is inside.
    default: return <StackView view={view} depth={depth} />
  }
}

export function Sections({ views }: { views: View[] }) {
  return <>{views.map((v) => <Fragment key={v.id}><Draw view={v} depth={1} /></Fragment>)}</>
}

export type { Node }
export { IconDown }
