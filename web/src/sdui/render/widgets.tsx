// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { useMedia } from '../../shell/useNav'
import type { View } from '../model'
import type { Props } from '../types'

// WidgetGrid and Widget (docs/SDUI.md): a grid of tiles, each in its own cell by x, y, w and h, as the Dashboard look draws
// it. A phone has two columns at most, and a tile wider than that takes the full width; anywhere else a tile keeps its
// cell. What a tile shows depends on its kind: a clock needs no data, a server's status is a line per figure, and the
// kinds that list titles hold their cards.

const int = (p: Props | undefined, k: string, d: number, lo: number, hi: number) => {
  const v = p?.[k]
  return typeof v === 'number' && isFinite(v) ? Math.min(hi, Math.max(lo, Math.trunc(v))) : d
}

export function WidgetGridView({ view, draw }: { view: View; draw: (child: View) => ReactNode }) {
  const phone = useMedia('(max-width: 700px)')
  const columns = phone ? 2 : int(view.node.props, 'columns', 4, 1, 12)
  return (
    <div className="sdui-widgets" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, '--widget-cols': columns } as CSSProperties} data-nav-group data-columns={columns}>
      {view.children.map((c) => <WidgetCell key={c.id} view={c} columns={columns} phone={phone} draw={draw} />)}
    </div>
  )
}

function WidgetCell({ view, columns, phone, draw }: { view: View; columns: number; phone: boolean; draw: (child: View) => ReactNode }) {
  const p = view.node.props
  const w = Math.min(columns, int(p, 'w', 1, 1, 12))
  const h = int(p, 'h', 1, 1, 8)
  const kind = typeof p?.kind === 'string' ? p.kind : 'continue'
  // Placed by its column and row, clamped so it stays inside the grid. A phone has no room for columns of its own: its
  // tiles take the order they have and a width of one column, or the whole row when they are wider than one.
  const x = Math.min(int(p, 'x', 0, 0, 11), columns - w)
  const y = int(p, 'y', 0, 0, 40)
  const place: CSSProperties = phone
    ? { gridColumn: w >= 2 ? '1 / -1' : 'auto', gridRow: `span ${h}`, order: y * 12 + x }
    : { gridColumn: `${x + 1} / span ${w}`, gridRow: `${y + 1} / span ${h}` }
  const title = view.text?.title
  return (
    <section className={'sdui-widget elev-1 kind-' + kind} style={place} aria-label={view.label || title} data-w={w} data-h={h}>
      {title && <h3 className="sdui-widget-title">{title}</h3>}
      <div className="sdui-widget-body">
        {kind === 'clock' ? <LiveClock /> : view.children.map((c) => <div key={c.id} className="sdui-widget-item">{draw(c)}</div>)}
      </div>
    </section>
  )
}

// The clock: the time and the date in the device's own format, needing no data; it moves on at each minute, and a
// screen reader gets the time once, not every second.
function LiveClock() {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    let t: ReturnType<typeof setTimeout>
    const tick = () => { setNow(new Date()); t = setTimeout(tick, 60000 - (Date.now() % 60000) + 50) }
    t = setTimeout(tick, 60000 - (Date.now() % 60000) + 50)
    return () => clearTimeout(t)
  }, [])
  return (
    <p className="sdui-clock">
      <time className="sdui-clock-time" dateTime={now.toISOString()}>{now.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}</time>
      <span className="sdui-clock-date">{now.toLocaleDateString([], { weekday: 'long' })}</span>
    </p>
  )
}
