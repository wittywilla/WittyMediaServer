// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

// A long list of rows of one height, of which only those near the screen are drawn: thousands
// of channels stay quick to scroll through. Up and Down move between rows, scrolling as they go,
// from whatever in a row has the focus to the row's own control (marked data-vfocus), so a
// keyboard or a remote can go the whole length of it, not only through the rows drawn.

const EXTRA = 6

export default function VirtualList({ count, rowH, height, render, label, className, onRowKey }: {
  count: number; rowH: number; height: number; render: (i: number) => ReactNode; label: string; className?: string
  // A key on a row the list doesn't use itself: true when it was used.
  onRowKey?: (i: number, e: React.KeyboardEvent) => boolean
}) {
  const box = useRef<HTMLDivElement | null>(null)
  const [top, setTop] = useState(0)
  const [want, setWant] = useState<number | null>(null)
  const first = Math.max(0, Math.floor(top / rowH) - EXTRA)
  const last = Math.min(count - 1, Math.ceil((top + height) / rowH) + EXTRA)
  const rows: ReactNode[] = []
  for (let i = first; i <= last; i++) {
    rows.push(<div key={i} role="listitem" data-vrow={i} className="ctv-vrow" style={{ top: i * rowH, height: rowH }}>{render(i)}</div>)
  }
  const go = (n: number) => {
    const el = box.current
    if (!el || n < 0 || n >= count) return
    const t = n * rowH
    if (t < el.scrollTop) el.scrollTop = t
    else if (t + rowH > el.scrollTop + el.clientHeight) el.scrollTop = t + rowH - el.clientHeight
    setTop(el.scrollTop)
    setWant(n)
  }
  useLayoutEffect(() => {
    if (want == null) return
    const el = box.current?.querySelector<HTMLElement>(`[data-vrow="${want}"] [data-vfocus]`)
    if (el) { setWant(null); el.focus({ preventScroll: true }) }
  })
  // A list that got shorter: what's scrolled to is still in it.
  useLayoutEffect(() => { if (box.current && box.current.scrollTop !== top) setTop(box.current.scrollTop) }, [count]) // eslint-disable-line react-hooks/exhaustive-deps
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const row = (e.target as HTMLElement).closest<HTMLElement>('[data-vrow]')
    if (!row) return
    const i = Number(row.dataset.vrow)
    if (onRowKey?.(i, e)) { e.preventDefault(); e.stopPropagation(); return }
    let n = -1
    if (e.key === 'ArrowDown') n = i + 1
    else if (e.key === 'ArrowUp') n = i - 1
    else if (e.key === 'PageDown') n = Math.min(count - 1, i + Math.max(1, Math.floor(height / rowH) - 1))
    else if (e.key === 'PageUp') n = Math.max(0, i - Math.max(1, Math.floor(height / rowH) - 1))
    // The ends hand the arrows on, so a remote can leave the list.
    if (n < 0 || n >= count) return
    e.preventDefault()
    e.stopPropagation()
    go(n)
  }
  return (
    <div ref={box} className={'ctv-vlist' + (className ? ' ' + className : '')} role="list" aria-label={label} style={{ height: Math.min(height, Math.max(rowH, count * rowH)) }}
      onScroll={(e) => setTop((e.target as HTMLElement).scrollTop)} onKeyDown={onKeyDown}>
      <div className="ctv-vlist-body" style={{ height: count * rowH }}>{rows}</div>
    </div>
  )
}
