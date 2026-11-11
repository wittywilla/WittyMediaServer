// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { api, serverBase, type Library } from '../../api'
import { photoThumb, takenMonth as monthYear, type Photo, type PhotoCluster } from '../../gallery'
import { IconClose, IconDown, IconGlobe, IconHome, IconLeft, IconRight, IconUp } from '../../icons'
import { isBackKey } from '../../ui/spatial'
import {
  MAX_ZOOM, boxParam, clampView, fitView, groupZoom, mercX, mercY, placeName, scaleOf, tileUrl, toScreen, viewBox, worldCopies, zoomAt,
  type Box, type Size, type View,
} from './mapMath'

// The photo map. By default it needs nothing from the internet: a small outline
// of the world's land ships with the app, and the groups of pictures come from
// this server, grouped there for the part in view. An administrator can add an
// online map (Settings, Sharing & photos); only then does this page ask anyone
// else for anything, says so under the map, and each device can still choose
// the outline.
//
// Moving the map moves one layer with a CSS transform; the pins are drawn
// again only when the movement settles, so a slow TV keeps up with a finger.

type Answer = { clusters: PhotoCluster[]; tiles: string; attribution: string; truncated: boolean; bounds?: (Box & { count: number }) | null }
type Area = { title: string; photos: Photo[]; next: string; bbox: string; group: PhotoCluster }

const OUTLINE_KEY = 'wms.photoMap.outline' // this device chose the outline over the online map
const readOutline = () => { try { return localStorage.getItem(OUTLINE_KEY) === '1' } catch { return false } }
const PIN = 52
const compact = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })
function when(c: PhotoCluster) {
  if (!c.from) return ''
  const a = monthYear(c.from), b = monthYear(c.to)
  return a === b ? a : `${a} to ${b}`
}
const pictures = (n: number) => `${n.toLocaleString()} ${n === 1 ? 'picture' : 'pictures'}`
const describe = (c: PhotoCluster) => [pictures(c.count), placeName(c.lat, c.lon), when(c)].filter(Boolean).join(', ')
const groupBox = (c: PhotoCluster): Box => ({ south: c.south, west: c.west, north: c.north, east: c.east })
const hostOf = (t: string) => { try { return new URL(t.replace(/\{[a-z]\}/g, '0')).host } catch { return 'an online map' } }

export default function PhotoMap({ lib, onOpen, onList }: { lib: Library; onOpen: (id: number) => void; onList: (l: Photo[]) => void }) {
  const box = useRef<HTMLDivElement | null>(null)
  const layer = useRef<HTMLDivElement | null>(null)
  const [size, setSize] = useState<Size>({ w: 0, h: 0 })
  const [view, setView] = useState<View | null>(null)
  const [first, setFirst] = useState<Answer | null>(null) // with the box around every placed picture
  const [answer, setAnswer] = useState<Answer | null>(null)
  const [failed, setFailed] = useState(false)
  const [land, setLand] = useState('')
  const [area, setArea] = useState<Area | null>(null)
  const [moving, setMoving] = useState(false) // the arrows move the map (Enter turns it on, so a remote can still leave)
  const [focused, setFocused] = useState(false)
  const [lit, setLit] = useState<number | null>(null) // the cover of the group the list points at
  const [outline, setOutline] = useState(readOutline)
  const [tilesBroken, setTilesBroken] = useState(false)
  const tileLoads = useRef({ ok: 0, bad: 0 })
  const cache = useRef(new Map<string, Answer>())
  const asked = useRef('')
  const areaHead = useRef<HTMLHeadingElement | null>(null)
  const ids = useId()

  // What's on screen right now, which leads what React last drew while a gesture runs.
  const live = useRef<View | null>(null)
  const viewRef = useRef(view), sizeRef = useRef(size)
  viewRef.current = view
  sizeRef.current = size
  const settle = useRef(0)

  useEffect(() => {
    const el = box.current
    if (!el) return
    const ro = new ResizeObserver(() => setSize({ w: el.clientWidth, h: el.clientHeight }))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  useEffect(() => { fetch(serverBase() + '/world-land.txt').then((r) => (r.ok ? r.text() : '')).then(setLand).catch(() => {}) }, [])

  // First, where the pictures are, to start the map around them.
  useEffect(() => {
    api.get<Answer>(`/photos/map?library=${lib.id}&fit=1`).then(setFirst).catch(() => setFailed(true))
  }, [lib.id])
  useEffect(() => {
    if (!first || !size.w || !size.h) return
    if (!viewRef.current) {
      const v = first.bounds ? fitView(first.bounds, size) : clampView({ x: 0.5, y: 0.5, z: 0 }, size)
      live.current = v
      setView(v)
    } else {
      const v = clampView(live.current || viewRef.current, size)
      live.current = v
      setView(v)
    }
  }, [first, size])

  // Then the groups for the part in view, once the map stops moving.
  useEffect(() => {
    if (!view || !size.w) return
    const url = `/photos/map?library=${lib.id}&zoom=${groupZoom(view.z)}&bbox=${boxParam(viewBox(view, size))}`
    const known = cache.current.get(url)
    if (known) { asked.current = url; setAnswer(known); return }
    const ask = () => {
      asked.current = url
      api.get<Answer>(url).then((r) => {
        cache.current.set(url, r)
        if (cache.current.size > 24) cache.current.delete(cache.current.keys().next().value!)
        if (asked.current === url) { setAnswer(r); setFailed(false) }
      }).catch(() => { if (asked.current === url) setFailed(true) })
    }
    if (!asked.current) { ask(); return } // the first view: nothing is moving yet
    const t = setTimeout(ask, 150)
    return () => clearTimeout(t)
  }, [view, size, lib.id])

  // Draws the live view by moving the layer from where React last drew it.
  const show = () => {
    const V = viewRef.current, L = live.current, el = layer.current, s = sizeRef.current
    if (!V || !L || !el) return
    const k = scaleOf(L.z) / scaleOf(V.z)
    let dx = V.x - L.x
    dx -= Math.round(dx) // the same place in the nearest copy of the world
    const sc = scaleOf(V.z)
    const tx = (s.w / 2) * (1 - k) + k * dx * sc, ty = (s.h / 2) * (1 - k) + k * (V.y - L.y) * sc
    el.style.transform = Math.abs(tx) < 0.01 && Math.abs(ty) < 0.01 && Math.abs(k - 1) < 1e-6 ? '' : `translate(${tx}px, ${ty}px) scale(${k})`
  }
  useLayoutEffect(show, [view, size])
  const move = (to: View, after: number) => {
    live.current = clampView(to, sizeRef.current)
    show()
    clearTimeout(settle.current)
    settle.current = window.setTimeout(() => setView(live.current), after)
  }
  useEffect(() => () => clearTimeout(settle.current), [])
  const zoomBy = (dz: number, px = sizeRef.current.w / 2, py = sizeRef.current.h / 2) => {
    if (live.current) move(zoomAt(live.current, sizeRef.current, px, py, dz), 0)
  }
  const pan = (dx: number, dy: number) => { // in quarters of the map
    const L = live.current, s = sizeRef.current
    if (L) move({ ...L, x: L.x + (dx * s.w) / 4 / scaleOf(L.z), y: L.y + (dy * s.h) / 4 / scaleOf(L.z) }, 0)
  }
  const fitAll = () => {
    const b = first?.bounds
    move(b ? fitView(b, sizeRef.current) : { x: 0.5, y: 0.5, z: 0 }, 0)
  }

  // The wheel zooms around the pointer. React's wheel handler can't stop the page scrolling.
  useEffect(() => {
    const el = box.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      if (!live.current) return
      e.preventDefault()
      const r = el.getBoundingClientRect()
      const unit = e.deltaMode === 1 ? 33 : e.deltaMode === 2 ? 400 : 1
      const dz = Math.max(-1, Math.min(1, (-e.deltaY * unit) / (e.ctrlKey ? 60 : 200))) // ctrl: a touchpad's pinch
      move(zoomAt(live.current, sizeRef.current, e.clientX - r.left, e.clientY - r.top, dz), 180)
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  // One finger or the mouse drags; two fingers pinch. A drag that starts on a
  // pin still moves the map, and then doesn't open it.
  const pointers = useRef(new Map<number, { x: number; y: number }>())
  const gesture = useRef<{ dragged: boolean; x: number; y: number; from: View; dist: number; mid: { x: number; y: number } } | null>(null)
  const swallowClick = useRef(false)
  const lastTap = useRef({ t: 0, x: 0, y: 0 })
  const capture = (id: number) => { try { box.current?.setPointerCapture(id) } catch { /* that pointer has already gone */ } }
  const at = (e: React.PointerEvent) => { const r = box.current!.getBoundingClientRect(); return { x: e.clientX - r.left, y: e.clientY - r.top } }
  const restart = () => {
    const ps = [...pointers.current.values()], L = live.current
    if (!ps.length || !L) { gesture.current = null; return }
    const mid = ps.length > 1 ? { x: (ps[0].x + ps[1].x) / 2, y: (ps[0].y + ps[1].y) / 2 } : ps[0]
    const dist = ps.length > 1 ? Math.hypot(ps[0].x - ps[1].x, ps[0].y - ps[1].y) : 0
    gesture.current = { dragged: gesture.current?.dragged || false, x: mid.x, y: mid.y, from: L, dist, mid }
  }
  const onPointerDown = (e: React.PointerEvent) => {
    if ((e.pointerType === 'mouse' && e.button !== 0) || (e.target as Element).closest('.gal-map-ctl')) return
    pointers.current.set(e.pointerId, at(e))
    if (pointers.current.size > 2) return
    if (pointers.current.size === 1) gesture.current = null
    restart()
  }
  const onPointerMove = (e: React.PointerEvent) => {
    const p = pointers.current.get(e.pointerId), g = gesture.current
    if (!p || !g) return
    Object.assign(p, at(e))
    const ps = [...pointers.current.values()]
    if (ps.length === 1) {
      if (!g.dragged && Math.hypot(p.x - g.x, p.y - g.y) < 6) return
      if (!g.dragged) { g.dragged = true; capture(e.pointerId) }
      const sc = scaleOf(g.from.z)
      move({ ...g.from, x: g.from.x - (p.x - g.x) / sc, y: g.from.y - (p.y - g.y) / sc }, 250)
    } else if (ps.length === 2 && g.dist > 0) {
      if (!g.dragged) { g.dragged = true; for (const id of pointers.current.keys()) capture(id) }
      const mid = { x: (ps[0].x + ps[1].x) / 2, y: (ps[0].y + ps[1].y) / 2 }
      const z = zoomAt(g.from, sizeRef.current, g.mid.x, g.mid.y, Math.log2(Math.hypot(ps[0].x - ps[1].x, ps[0].y - ps[1].y) / g.dist))
      const sc = scaleOf(z.z)
      move({ ...z, x: z.x - (mid.x - g.mid.x) / sc, y: z.y - (mid.y - g.mid.y) / sc }, 250)
    }
  }
  const onPointerUp = (e: React.PointerEvent) => {
    if (!pointers.current.delete(e.pointerId)) return
    const g = gesture.current
    if (pointers.current.size) { restart(); return } // a finger left a pinch: carry on with the other
    gesture.current = null
    if (g?.dragged) {
      swallowClick.current = true
      setTimeout(() => { swallowClick.current = false }, 0)
      if (live.current) move(live.current, 0)
      return
    }
    // Two taps zoom in where they were, as a double click does.
    if (e.pointerType === 'touch' && e.type === 'pointerup' && !(e.target as Element).closest('button')) {
      const p = at(e), last = lastTap.current
      if (e.timeStamp - last.t < 320 && Math.hypot(p.x - last.x, p.y - last.y) < 30) { zoomBy(1, p.x, p.y); lastTap.current = { t: 0, x: 0, y: 0 } }
      else lastTap.current = { t: e.timeStamp, ...p }
    }
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const t = e.target as HTMLElement
    const z = e.key === '+' || e.key === '=' ? 1 : e.key === '-' || e.key === '_' ? -1 : moving && e.key === 'PageUp' ? 1 : moving && e.key === 'PageDown' ? -1 : 0
    if (z) { e.preventDefault(); zoomBy(z); return }
    if (t !== box.current) return // a pin or a button: arrows go from one to the next
    if (!moving) {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setMoving(true) }
      return
    }
    if (e.key === 'Enter' || e.key === 'Escape' || e.key === 'Backspace' || isBackKey(e)) { e.preventDefault(); setMoving(false); return }
    const d = ({ ArrowUp: [0, -1], ArrowDown: [0, 1], ArrowLeft: [-1, 0], ArrowRight: [1, 0] } as Record<string, number[]>)[e.key]
    if (d) { e.preventDefault(); pan(d[0], d[1]) }
  }

  // A group spread over the map zooms in, so it splits; a group in one spot,
  // or one that can't split further, lists its pictures.
  const choose = (c: PhotoCluster) => {
    const v = live.current || view
    if (!v) return
    const sc = scaleOf(v.z)
    const spread = Math.max((mercX(c.east) - mercX(c.west)) * sc, (mercY(c.south) - mercY(c.north)) * sc)
    if (c.count > 1 && v.z < MAX_ZOOM - 0.5 && (spread > PIN || (c.count > 30 && spread > 0.5))) {
      move(fitView(groupBox(c), sizeRef.current, MAX_ZOOM - 1), 0)
      return
    }
    openArea(c)
  }
  // The pictures of one place, a page at a time. Only the last place asked for is shown,
  // whichever answer comes back first.
  const areaNow = useRef<{ group: PhotoCluster; photos: Photo[] } | null>(null)
  const openArea = (c: PhotoCluster, after = '', title = describe(c)) => {
    const pad = 1e-5
    const bbox = boxParam({ south: Math.max(-90, c.south - pad), west: Math.max(-180, c.west - pad), north: Math.min(90, c.north + pad), east: Math.min(180, c.east + pad) })
    const before = after && areaNow.current?.group === c ? areaNow.current.photos : []
    areaNow.current = { group: c, photos: before }
    api.get<{ photos: Photo[]; nextCursor: string }>(`/photos/list?library=${lib.id}&limit=200&bbox=${bbox}${after ? `&cursor=${encodeURIComponent(after)}` : ''}`).then((r) => {
      if (areaNow.current?.group !== c) return // another place was chosen meanwhile
      const photos = [...before, ...r.photos]
      areaNow.current = { group: c, photos }
      setArea({ title, photos, next: r.nextCursor, bbox, group: c })
      onList(photos)
      if (!after) requestAnimationFrame(() => areaHead.current?.focus())
    }).catch(() => {})
  }

  const s = size
  const tiles = first && answer?.tiles && !outline ? answer.tiles : ''
  const tileImgs = useMemo(() => {
    if (!tiles || !view || !s.w) return []
    const tz = Math.max(0, Math.min(MAX_ZOOM, Math.round(view.z)))
    const n = 2 ** tz, scale = scaleOf(view.z), size = scale / n
    const out: { key: string; src: string; left: number; top: number; size: number }[] = []
    const x0 = Math.floor((view.x - s.w / scale) * n), x1 = Math.ceil((view.x + s.w / scale) * n)
    const y0 = Math.max(0, Math.floor((view.y - s.h / scale) * n)), y1 = Math.min(n - 1, Math.ceil((view.y + s.h / scale) * n))
    for (let tx = x0; tx <= x1 && out.length < 400; tx++) for (let ty = y0; ty <= y1; ty++) {
      const p = toScreen(view, s, tx / n, ty / n)
      out.push({ key: `${tz}/${tx}/${ty}`, src: tileUrl(tiles, tz, ((tx % n) + n) % n, ty), left: p.left, top: p.top, size })
    }
    return out
  }, [tiles, view, s])
  useEffect(() => { tileLoads.current = { ok: 0, bad: 0 }; setTilesBroken(false) }, [tiles])
  const tileDone = (ok: boolean) => { // six failures in a row: the provider isn't answering (any more)
    const t = tileLoads.current
    if (ok) { t.ok++; t.bad = 0 } else t.bad++
    if (t.bad >= 6) setTilesBroken(true)
  }


  // Pins for each copy of the world in view, in reading order, so Tab and a
  // screen reader go left to right and top to bottom.
  const clusters = answer?.clusters ?? []
  const pins = useMemo(() => {
    if (!view || !s.w) return []
    const m = s.w / 2 // half a screen either side, for dragging into
    const out: { c: PhotoCluster; key: string; left: number; top: number; seen: boolean }[] = []
    for (const k of worldCopies(view, s, m)) for (const c of clusters) {
      const p = toScreen(view, s, mercX(c.lon), mercY(c.lat), k)
      if (p.left < -m || p.left > s.w + m || p.top < -s.h || p.top > s.h * 2) continue
      out.push({ c, key: `${k}:${c.coverId}`, left: p.left, top: p.top, seen: p.left > -PIN / 2 && p.left < s.w + PIN / 2 && p.top > -PIN / 2 && p.top < s.h + PIN / 2 })
    }
    return out.sort((a, b) => Math.round(a.top / PIN) - Math.round(b.top / PIN) || a.left - b.left)
  }, [clusters, view, s])
  const inView = useMemo(() => {
    const seen = new Set<PhotoCluster>()
    for (const p of pins) if (p.seen) seen.add(p.c)
    return [...seen].sort((a, b) => b.count - a.count)
  }, [pins])
  const shownInList = inView.slice(0, 100)
  const total = first?.bounds?.count ?? 0
  const landCopies = view && s.w ? worldCopies(view, s, s.w / 2) : []
  const scale = view ? scaleOf(view.z) : 1
  const hint = moving ? 'Arrows move the map, + and − zoom. Enter or Back when done.' : 'Press Enter to move the map with the arrows. + and − zoom. Tab or the arrows go from group to group.'

  return (
    <div className="gal-map-wrap">
      <div className="gal-map-main">
        <div className={'gal-map' + (moving ? ' moving' : '') + (tiles && !tilesBroken ? ' online' : '')} ref={box} tabIndex={0}
          role="group" aria-roledescription="map" aria-label="Map of where pictures were taken" aria-describedby={`${ids}-hint`}
          onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={onPointerUp}
          onClickCapture={(e) => { if (swallowClick.current) { e.stopPropagation(); e.preventDefault() } }}
          onDoubleClick={(e) => { if ((e.target as Element).closest('button')) return; const r = box.current!.getBoundingClientRect(); zoomBy(1, e.clientX - r.left, e.clientY - r.top) }}
          onKeyDown={onKeyDown} onFocus={(e) => setFocused(e.target === e.currentTarget)} onBlur={() => { setFocused(false); setMoving(false) }}>
          <div className="gal-map-layer" ref={layer}>
            {land && view && (
              <svg className="gal-land" width={s.w} height={s.h} aria-hidden="true">
                <defs><path id={`${ids}-land`} d={land} fillRule="evenodd" /></defs>
                {landCopies.map((k) => {
                  const o = toScreen(view, s, 0, 0, k)
                  return <use key={k} href={`#${ids}-land`} transform={`translate(${o.left} ${o.top}) scale(${scale / 1000})`} />
                })}
              </svg>
            )}
            {tiles && !tilesBroken && tileImgs.map((t) => (
              <img key={t.key} className="gal-tile-img" src={t.src} alt="" draggable={false} referrerPolicy="no-referrer" decoding="async"
                onLoad={() => tileDone(true)} onError={(e) => { (e.target as HTMLImageElement).style.visibility = 'hidden'; tileDone(false) }}
                style={{ left: t.left, top: t.top, width: t.size + 0.5, height: t.size + 0.5 }} />
            ))}
            {pins.map(({ c, key, left, top, seen }) => (
              <button key={key} type="button" className={'gal-pin' + (lit === c.coverId ? ' lit' : '')} style={{ left, top }} onClick={() => choose(c)}
                aria-label={describe(c)} title={describe(c)} tabIndex={seen ? 0 : -1} aria-hidden={seen ? undefined : true}>
                <img src={photoThumb(c.coverId, 160)} alt="" loading="lazy" draggable={false} />
                {c.count > 1 && <b aria-hidden="true">{compact.format(c.count)}</b>}
              </button>
            ))}
          </div>
          <div className="gal-map-ctl" role="group" aria-label="Map controls">
            <button type="button" className="btn icon" onClick={() => zoomBy(1)} aria-label="Zoom in" title="Zoom in">＋</button>
            <button type="button" className="btn icon" onClick={() => zoomBy(-1)} aria-label="Zoom out" title="Zoom out">－</button>
            <button type="button" className="btn icon" onClick={() => pan(0, -1)} aria-label="Move north" title="Move north"><IconUp /></button>
            <button type="button" className="btn icon" onClick={() => pan(0, 1)} aria-label="Move south" title="Move south"><IconDown /></button>
            <button type="button" className="btn icon" onClick={() => pan(-1, 0)} aria-label="Move west" title="Move west"><IconLeft /></button>
            <button type="button" className="btn icon" onClick={() => pan(1, 0)} aria-label="Move east" title="Move east"><IconRight /></button>
            <button type="button" className="btn icon" onClick={fitAll} aria-label="Show every picture" title="Show every picture"><IconHome /></button>
            {answer?.tiles && (
              <button type="button" className={'btn icon' + (outline ? '' : ' on')} aria-pressed={!outline} aria-label="Online map" title={outline ? 'Use the online map' : 'Use the outline, which asks nothing online'}
                onClick={() => { const v = !outline; setOutline(v); try { localStorage.setItem(OUTLINE_KEY, v ? '1' : '0') } catch { /* this device can't remember it */ } }}>
                <IconGlobe />
              </button>
            )}
          </div>
          <p id={`${ids}-hint`} className={'gal-map-hint' + (focused || moving ? ' shown' : '')}>{hint}</p>
          <span className="gal-map-credit">{tiles && !tilesBroken ? <>Map: {answer?.attribution || hostOf(tiles)}</> : 'Outline: Natural Earth'}</span>
          {first && total === 0 && !failed && <div className="gal-map-empty"><b>No pictures with a location yet</b><p>Phones and some cameras save where a picture was taken. Those pictures appear here.</p></div>}
          {failed && <div className="gal-map-empty" role="alert"><b>The map couldn’t be loaded</b><p>The server didn’t answer. It tries again when you move the map.</p></div>}
        </div>
        {tiles && (
          <p className="gal-map-note muted">
            {tilesBroken ? 'The online map isn’t answering, so the outline is shown.' : <>This map is drawn by {hostOf(tiles)}: your device asks it for the area in view, so it learns roughly where you look. Your pictures and their places stay on this server.</>}
          </p>
        )}
        {answer?.truncated && <p className="gal-map-note muted">There are too many places to show at once here. Zoom in to see them all.</p>}
      </div>
      <section className="gal-map-list" aria-labelledby={`${ids}-list`}>
        <h2 id={`${ids}-list`}>{inView.length ? `${inView.length} ${inView.length === 1 ? 'place' : 'places'} in view` : 'Places in view'}</h2>
        {first && total > 0 && !inView.length && <p className="muted">No pictures here. <button type="button" className="btn sm ghost" onClick={fitAll}>Show every picture</button></p>}
        <ul data-nav-group>
          {shownInList.map((c) => (
            <li key={c.coverId}>
              <button type="button" onClick={() => openArea(c)} onMouseEnter={() => setLit(c.coverId)} onMouseLeave={() => setLit(null)} onFocus={() => setLit(c.coverId)} onBlur={() => setLit(null)}>
                <img src={photoThumb(c.coverId, 160)} alt="" loading="lazy" />
                <span><b>{pictures(c.count)}</b><span className="muted">{placeName(c.lat, c.lon)}</span>{when(c) && <span className="muted">{when(c)}</span>}</span>
              </button>
            </li>
          ))}
        </ul>
        {inView.length > shownInList.length && <p className="muted">And {inView.length - shownInList.length} more. Zoom in to see them.</p>}
      </section>
      {area && (
        <section className="gal-map-area" aria-labelledby={`${ids}-area`}>
          <div className="gal-sub">
            <h2 id={`${ids}-area`} ref={areaHead} tabIndex={-1}>{pictures(area.photos.length)}{area.next ? ' so far' : ''} <span className="muted">· {placeName(area.group.lat, area.group.lon)}</span></h2>
            <button type="button" className="btn sm ghost" onClick={() => { move(fitView(groupBox(area.group), sizeRef.current, MAX_ZOOM - 1), 0) }}>Show on the map</button>
            <button type="button" className="btn sm ghost" onClick={() => { areaNow.current = null; setArea(null) }}><IconClose /> Close</button>
          </div>
          <div className="gal-grid small">
            {area.photos.map((p) => (
              <button key={p.id} type="button" className="gal-tile" style={{ flexGrow: p.width && p.height ? p.width / p.height : 1.4, flexBasis: ((p.width && p.height ? p.width / p.height : 1.4) * 130) }} aria-label={p.title} onClick={() => onOpen(p.id)}>
                <img src={photoThumb(p.id, 320, p.updatedAt)} alt="" loading="lazy" />
              </button>
            ))}
          </div>
          {area.next && <button type="button" className="btn" onClick={() => openArea(area.group, area.next, area.title)}>Show more</button>}
        </section>
      )}
    </div>
  )
}
