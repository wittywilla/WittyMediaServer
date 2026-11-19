// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { t, tx } from '../i18n'
import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { Segmented } from '../ui'
import { actOf, clockText, FRAME, filterLabel, moveRange, percent, segLabel, timeAt, type Edge, type Filter, type Scene, type Seg } from './model'

// The timeline of a .wittyinfo: the video's length across, with the skippable parts, the
// scenes and the content filters as coloured ranges on rows of their own, and where the
// video is now.
//
// - Mouse and touch: drag a range to move it, its ends to change them; tap or click the row
//   to go there. The ends are big enough for a finger (bigger on touch screens).
// - Keyboard and remote: each end is a slider. Left and right move it a second, with Shift
//   a frame, Page Up and Down ten seconds; up and down leave it for the next row, as on any
//   page. Enter on a range picks it for the list below.
// - Zoom: a long film is hard to set to the second at full width, so the timeline can be
//   made 4 or 16 times wider, and keeps what is picked in view.

export type Pick = { list: 'segments' | 'scenes' | 'filters'; index: number } | null

type Props = {
  duration: number
  segments: Seg[]
  scenes: Scene[]
  filters: Filter[]
  pick: Pick
  onPick: (p: Pick) => void
  onSegment: (index: number, s: Seg) => void
  onScene: (index: number, s: Scene) => void
  onFilter: (index: number, f: Filter) => void
  now?: number
  onSeek?: (t: number) => void
  // Times from elsewhere that are in force now (chapters, detection), shown faintly.
  ghosts?: { kind: string; startSec: number; endSec: number; source: string }[]
}

type Drag = { list: Pick extends infer P ? P extends { list: infer L } ? L : never : never; index: number; edge: Edge; x0: number; width: number; orig: { start: number; end?: number | null }; moved: boolean }

// lanes stacks ranges that overlap, so each stays visible and grabbable.
function lanes(items: { start: number; end: number }[]): number[] {
  const ends: number[] = []
  return items.map((r) => {
    let lane = ends.findIndex((e) => e <= r.start)
    if (lane < 0) { lane = ends.length; ends.push(r.end) } else ends[lane] = r.end
    return lane
  })
}

const ticksFor = (duration: number, zoom: number) => {
  const target = duration / (8 * zoom)
  const steps = [1, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600]
  const step = steps.find((s) => s >= target) || 3600
  const out: number[] = []
  for (let t = 0; t <= duration; t += step) out.push(t)
  return out
}

export default function Timeline(p: Props) {
  const { duration } = p
  const [zoom, setZoom] = useState<'1' | '4' | '16'>('1')
  const scroller = useRef<HTMLDivElement | null>(null)
  const track = useRef<HTMLDivElement | null>(null)
  const drag = useRef<Drag | null>(null)
  const z = Number(zoom)
  const len = (end: number | null | undefined) => end ?? duration

  const segLanes = useMemo(() => lanes(p.segments.map((s) => ({ start: s.start, end: len(s.end) }))), [p.segments, duration]) // eslint-disable-line react-hooks/exhaustive-deps
  const filterLanes = useMemo(() => lanes(p.filters.map((f) => ({ start: f.start, end: f.end }))), [p.filters])
  const segRows = Math.max(1, ...segLanes.map((l) => l + 1))
  const filterRows = Math.max(1, ...filterLanes.map((l) => l + 1))

  // Keep what is picked in view when zoomed in.
  useEffect(() => {
    const el = scroller.current
    if (!el || z === 1 || !p.pick || duration <= 0) return
    const item = p.pick.list === 'segments' ? p.segments[p.pick.index] : p.pick.list === 'scenes' ? p.scenes[p.pick.index] : p.filters[p.pick.index]
    if (!item) return
    const x = (item.start / duration) * el.scrollWidth
    if (x < el.scrollLeft || x > el.scrollLeft + el.clientWidth - 40) el.scrollTo({ left: Math.max(0, x - el.clientWidth / 3) })
  }, [p.pick, zoom]) // eslint-disable-line react-hooks/exhaustive-deps

  const change = (list: Drag['list'], index: number, next: { start: number; end?: number | null }) => {
    if (list === 'segments') p.onSegment(index, { ...p.segments[index], ...next })
    else if (list === 'scenes') p.onScene(index, { ...p.scenes[index], start: next.start })
    else p.onFilter(index, { ...p.filters[index], start: next.start, end: next.end as number })
  }
  const itemOf = (list: Drag['list'], index: number): { start: number; end?: number | null } => (list === 'segments' ? p.segments[index] : list === 'scenes' ? { start: p.scenes[index].start } : p.filters[index])

  const down = (list: Drag['list'], index: number, edge: Edge) => (e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0 || !track.current) return
    e.stopPropagation()
    p.onPick({ list, index })
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    drag.current = { list, index, edge, x0: e.clientX, width: track.current.getBoundingClientRect().width, orig: itemOf(list, index), moved: false }
  }
  const move = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.x0
    if (!d.moved && Math.abs(dx) < 3) return
    d.moved = true
    const delta = Math.round((dx / d.width) * duration * 10) / 10
    change(d.list, d.index, moveRange(d.orig, d.list === 'scenes' ? 'start' : d.edge, delta, duration))
  }
  const up = () => { drag.current = null }

  const key = (list: Drag['list'], index: number, edge: Edge) => (e: KeyboardEvent<HTMLElement>) => {
    const step = { ArrowLeft: -1, ArrowRight: 1, PageDown: -10, PageUp: 10 }[e.key as 'ArrowLeft']
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); p.onPick({ list, index }); return }
    if (step == null && e.key !== 'Home' && e.key !== 'End') return
    e.preventDefault()
    e.stopPropagation()
    const item = itemOf(list, index)
    if (e.key === 'Home' || e.key === 'End') {
      const at = e.key === 'Home' ? 0 : duration
      const cur = edge === 'end' ? len(item.end) : item.start
      change(list, index, moveRange(item, edge, at - cur, duration))
      return
    }
    const delta = (step as number) * (e.shiftKey && Math.abs(step as number) === 1 ? FRAME : 1)
    change(list, index, moveRange(item, list === 'scenes' ? 'start' : edge, delta, duration))
  }

  const seekAt = (e: PointerEvent<HTMLDivElement>) => {
    if (!p.onSeek || !track.current || e.target !== e.currentTarget) return
    const r = track.current.getBoundingClientRect()
    p.onSeek(timeAt((e.clientX - r.left) / r.width, duration))
  }

  if (duration <= 0) return <p className="muted wi-help">{t('There is no length to draw a timeline for yet (the files haven’t been read). Times can still be typed below.')}</p>
  // Where the next scene starts: a scene's title stops before it.
  const nextStart = (t: number) => Math.min(duration, ...p.scenes.map((x) => x.start).filter((x) => x > t))
  const picked = (list: string, i: number) => p.pick?.list === list && p.pick.index === i

  const handle = (list: Drag['list'], i: number, edge: 'start' | 'end', at: number, name: string) => (
    <span className={'wi-handle ' + edge} role="slider" tabIndex={0} aria-label={edge === 'start' ? t('{name}, start', { name }) : t('{name}, end', { name })}
      aria-valuemin={0} aria-valuemax={Math.round(duration)} aria-valuenow={Math.round(at * 10) / 10} aria-valuetext={clockText(at)}
      onPointerDown={down(list, i, edge)} onKeyDown={key(list, i, edge)} onFocus={() => p.onPick({ list, index: i })} />
  )

  return (
    <div className="wi-timeline">
      <div className="wi-tl-head">
        <span className="muted wi-tl-len">{clockText(duration)}</span>
        <Segmented label={t('Zoom')} value={zoom} onChange={setZoom} options={[{ value: '1', label: t('Whole') }, { value: '4', label: '4×' }, { value: '16', label: '16×' }]} />
      </div>
      <div className="wi-tl-scroll" ref={scroller}>
        <div className="wi-tl-inner" style={{ width: `${z * 100}%` }} ref={track} onPointerMove={move} onPointerUp={up} onPointerCancel={up}>
          <div className="wi-ruler" aria-hidden="true" onPointerDown={seekAt}>
            {ticksFor(duration, z).map((t) => <span key={t} className="wi-tick" style={{ left: `${percent(t, duration)}%` }}>{clockText(t)}</span>)}
          </div>

          <div className="wi-row" style={{ '--lanes': segRows } as React.CSSProperties} role="group" aria-label={t('Parts to skip')} onPointerDown={seekAt}>
            <span className="wi-row-name" aria-hidden="true">{t('Skips')}</span>
            {(p.ghosts || []).filter((g) => !g.source.startsWith('wittyinfo')).map((g, i) => (
              <span key={'g' + i} className={'wi-ghost wi-t-' + g.kind} style={{ left: `${percent(g.startSec, duration)}%`, width: `${Math.max(0.3, percent(g.endSec, duration) - percent(g.startSec, duration))}%` }} title={t('{what} found elsewhere ({source})', { what: tx(segLabel(g.kind)), source: g.source })} aria-hidden="true" />
            ))}
            {p.segments.map((s, i) => {
              const end = len(s.end)
              const name = s.label ? `${tx(segLabel(s.type))} “${s.label}”` : tx(segLabel(s.type))
              return (
                <div key={s.id || i} className={'wi-range wi-t-' + s.type + (actOf(s) === 'mark' ? ' wi-mark' : '') + (picked('segments', i) ? ' wi-picked' : '')}
                  style={{ left: `${percent(s.start, duration)}%`, width: `${Math.max(0.4, percent(end, duration) - percent(s.start, duration))}%`, '--lane': segLanes[i] } as React.CSSProperties}
                  onPointerDown={down('segments', i, 'both')} title={`${name}: ${clockText(s.start)} – ${s.end == null ? t('the end') : clockText(end)}`}>
                  {handle('segments', i, 'start', s.start, name)}
                  <span className="wi-range-label">{s.label || tx(segLabel(s.type))}</span>
                  {handle('segments', i, 'end', end, name)}
                </div>
              )
            })}
          </div>

          <div className="wi-row wi-scenes" role="group" aria-label={t('Scenes')} onPointerDown={seekAt}>
            <span className="wi-row-name" aria-hidden="true">{t('Scenes')}</span>
            {p.scenes.map((s, i) => (
              <span key={s.id || i} className={'wi-pin' + (picked('scenes', i) ? ' wi-picked' : '')} style={{ left: `${percent(s.start, duration)}%`, maxWidth: `max(10px, calc(${percent(nextStart(s.start), duration) - percent(s.start, duration)}% - 4px))` }}
                role="slider" tabIndex={0} aria-label={t('Scene “{name}”', { name: s.title || String(i + 1) })} aria-valuemin={0} aria-valuemax={Math.round(duration)} aria-valuenow={Math.round(s.start)} aria-valuetext={clockText(s.start)}
                onPointerDown={down('scenes', i, 'start')} onKeyDown={key('scenes', i, 'start')} onFocus={() => p.onPick({ list: 'scenes', index: i })} title={`${s.title} · ${clockText(s.start)}`}>
                <i aria-hidden="true" /><b>{s.title}</b>
              </span>
            ))}
          </div>

          {p.filters.length > 0 && (
            <div className="wi-row wi-filters" style={{ '--lanes': filterRows } as React.CSSProperties} role="group" aria-label={t('Content filters')} onPointerDown={seekAt}>
              <span className="wi-row-name" aria-hidden="true">{t('Filters')}</span>
              {p.filters.map((f, i) => {
                const name = tx(filterLabel(f.category))
                return (
                  <div key={f.id || i} className={'wi-range wi-f' + (picked('filters', i) ? ' wi-picked' : '')}
                    style={{ left: `${percent(f.start, duration)}%`, width: `${Math.max(0.4, percent(f.end, duration) - percent(f.start, duration))}%`, '--lane': filterLanes[i] } as React.CSSProperties}
                    onPointerDown={down('filters', i, 'both')} title={`${name}: ${clockText(f.start)} – ${clockText(f.end)}`}>
                    {handle('filters', i, 'start', f.start, name)}
                    <span className="wi-range-label">{name}</span>
                    {handle('filters', i, 'end', f.end, name)}
                  </div>
                )
              })}
            </div>
          )}
          {p.now != null && <span className="wi-now" style={{ left: `${percent(p.now, duration)}%` }} aria-hidden="true" />}
        </div>
      </div>
    </div>
  )
}
