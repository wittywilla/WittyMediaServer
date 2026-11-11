// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A list a person puts in order: drag a row by its handle, press the arrow keys on the
// handle, or use the Move buttons (which is what a remote and a touch screen have).
// "Customise this page" and "Arrange the menu" are both this list.

import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react'
import { announce } from '../a11y'
import { IconDown, IconGrip, IconUp } from '../icons'
import { IconButton } from '../ui/Button'
import { Switch } from '../ui/Field'

// locked: never moves and is never hidden. hideFixed: moves, but is always shown (Settings in the menu).
export type ArrangeRow = { id: string; title: string; icon?: ReactNode; locked?: boolean; hideFixed?: boolean; hidden?: boolean; pinned?: boolean; note?: string }

export function ArrangeList({ rows, label, canMove, canHide, canPin, onMove, onHidden, onPinned, extra }: {
  rows: ArrangeRow[]; label: string; canMove: boolean; canHide: boolean; canPin?: boolean
  onMove: (id: string, to: number) => void; onHidden: (id: string, hidden: boolean) => void; onPinned?: (id: string, pinned: boolean) => void
  extra?: (row: ArrangeRow) => ReactNode
}) {
  const list = useRef<HTMLUListElement | null>(null)
  const dragging = useRef<string | null>(null)
  const [held, setHeld] = useState<string | null>(null)
  const [over, setOver] = useState<number | null>(null)
  const places = useRef<{ top: number; bottom: number }[]>([])
  const refocus = useRef<string | null>(null)
  useEffect(() => {
    if (!refocus.current) return
    const key = refocus.current
    refocus.current = null
    list.current?.querySelector<HTMLElement>(`[data-arrange-key="${CSS.escape(key)}"]`)?.focus({ preventScroll: true })
  })
  const move = (i: number, to: number, key?: string) => {
    if (to < 0 || to >= rows.length || to === i) return
    if (key) refocus.current = key
    onMove(rows[i].id, to)
    announce(`${rows[i].title} moved to position ${to + 1} of ${rows.length}`)
  }
  const gripKeys = (e: KeyboardEvent, i: number) => {
    const to = e.key === 'ArrowUp' ? i - 1 : e.key === 'ArrowDown' ? i + 1 : -1
    if (to < 0 || to >= rows.length) return
    e.preventDefault(); e.stopPropagation()
    move(i, to, 'grip:' + rows[i].id)
  }
  // The rows' places are read when the drag starts: nothing is measured while it moves.
  const startDrag = (e: PointerEvent, id: string) => {
    e.preventDefault()
    e.currentTarget.setPointerCapture(e.pointerId)
    dragging.current = id
    setHeld(id)
    places.current = [...(list.current?.children || [])].map((row) => row.getBoundingClientRect())
  }
  const onDrag = (e: PointerEvent) => {
    if (!dragging.current) return
    const at = places.current.findIndex((p) => e.clientY >= p.top && e.clientY < p.bottom)
    setOver(at >= 0 ? at : null)
  }
  const endDrag = () => {
    const id = dragging.current
    dragging.current = null
    setHeld(null)
    const from = rows.findIndex((r) => r.id === id)
    if (id && over !== null && from >= 0 && over !== from) move(from, over)
    setOver(null)
  }
  return (
    <ul className="nav-custom-list sdui-arrange" ref={list} aria-label={label}>
      {rows.map((r, i) => (
        <li key={r.id} className={'nav-custom-row' + (held === r.id ? ' held' : '') + (r.hidden ? ' off' : '') + (over === i && held && held !== r.id ? ' drop' : '') + (r.locked ? ' locked' : '')}>
          {canMove && !r.locked
            ? <button type="button" className="nav-custom-grip" data-arrange-key={'grip:' + r.id} aria-label={`Move ${r.title}. Position ${i + 1} of ${rows.length}. Use the up and down arrow keys.`}
                onKeyDown={(e) => gripKeys(e, i)} onPointerDown={(e) => startDrag(e, r.id)} onPointerMove={onDrag} onPointerUp={endDrag} onPointerCancel={endDrag}><IconGrip /></button>
            : <span className="nav-custom-grip none" aria-hidden="true" />}
          <span className="nav-custom-name">{r.icon}<span>{r.title}{r.note && <small className="muted"> · {r.note}</small>}</span></span>
          {extra?.(r)}
          {canMove && !r.locked && (
            <span className="nav-custom-move">
              <IconButton label={`Move ${r.title} up`} size="sm" disabled={i === 0} data-arrange-key={'up:' + r.id} onClick={() => move(i, i - 1, i - 1 === 0 ? 'down:' + r.id : 'up:' + r.id)}><IconUp /></IconButton>
              <IconButton label={`Move ${r.title} down`} size="sm" disabled={i === rows.length - 1} data-arrange-key={'down:' + r.id} onClick={() => move(i, i + 1, i + 1 === rows.length - 1 ? 'up:' + r.id : 'down:' + r.id)}><IconDown /></IconButton>
            </span>
          )}
          {canPin && onPinned && !r.locked && <button type="button" className={'btn sm' + (r.pinned ? ' primary' : ' ghost')} aria-pressed={!!r.pinned} aria-label={`${r.pinned ? 'Unpin' : 'Pin'} ${r.title}${r.pinned ? '' : ' to the top'}`} onClick={() => onPinned(r.id, !r.pinned)}>{r.pinned ? 'Pinned' : 'Pin'}</button>}
          {r.locked || r.hideFixed ? <span className="muted sdui-locked">Always shown</span> : canHide && <Switch label={`Show ${r.title}`} checked={!r.hidden} onChange={(on) => onHidden(r.id, !on)} />}
        </li>
      ))}
    </ul>
  )
}
