// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type ReactNode } from 'react'

// A long list that only puts on the page the rows near the screen, with the page itself
// scrolling (or a box, with `inBox`). Rows are all one height. Short lists are drawn whole,
// so a page of a dozen songs costs nothing extra and finds-in-page still see every row.

const WHOLE = 120
// Rows kept beyond the screen each way: about a screen's worth, whatever the row's height.
const over = (rowHeight: number) => Math.max(2, Math.ceil(700 / rowHeight))

export function VirtualRows<T>({ items, rowHeight, render, className, inBox, label, role = 'list', whole: upTo = WHOLE }: {
  items: T[]; rowHeight: number; render: (it: T, i: number) => ReactNode; className?: string; inBox?: boolean; label?: string; role?: string
  // Up to this many rows are drawn whole (a row of a grid holds several cards, so it passes fewer).
  whole?: number
}) {
  const box = useRef<HTMLDivElement>(null)
  const [range, setRange] = useState<[number, number]>([0, Math.min(items.length, 60)])
  const whole = items.length <= upTo
  useEffect(() => {
    if (whole) return
    const el = box.current
    if (!el) return
    const scroller: HTMLElement | Window = inBox ? (el.parentElement as HTMLElement) : window
    let frame = 0
    const measure = () => {
      frame = 0
      const r = el.getBoundingClientRect()
      const view = inBox ? (scroller as HTMLElement).getBoundingClientRect() : { top: 0, bottom: window.innerHeight }
      const first = Math.max(0, Math.floor((view.top - r.top) / rowHeight) - over(rowHeight))
      const last = Math.min(items.length, Math.ceil((view.bottom - r.top) / rowHeight) + over(rowHeight))
      setRange((was) => (was[0] === first && was[1] === last ? was : [first, Math.max(first, last)]))
    }
    const later = () => { if (!frame) frame = requestAnimationFrame(measure) }
    measure()
    scroller.addEventListener('scroll', later, { passive: true })
    window.addEventListener('resize', later)
    return () => { cancelAnimationFrame(frame); scroller.removeEventListener('scroll', later); window.removeEventListener('resize', later) }
  }, [items, rowHeight, whole, inBox])
  const [first, last] = whole ? [0, items.length] : range
  return (
    <div ref={box} className={className} role={role} aria-label={label} aria-rowcount={role === 'grid' || role === 'table' ? items.length : undefined}
      style={whole ? undefined : { position: 'relative', height: items.length * rowHeight }}>
      {whole ? items.map((it, i) => render(it, i)) : (
        <div style={{ position: 'absolute', left: 0, right: 0, top: first * rowHeight }}>
          {items.slice(first, last).map((it, k) => render(it, first + k))}
        </div>
      )}
    </div>
  )
}
