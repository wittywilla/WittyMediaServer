import { useCallback, useEffect, useRef, useState, type CSSProperties } from 'react'
import type { GameConsole, Item } from '../../api'
import { Placeholder } from '../../components/common'
import { heroCover } from './data'

// The covers at the top of the games home: one at the front, the others turned away on
// either side, standing on a shiny floor. Only what's near the front is on the page, and
// everything moves with transform alone, so it costs a cheap TV next to nothing.
//
// It takes one Tab stop. Left and right move along it, and at either end those arrows
// carry on to whatever's beside it, as they do everywhere else.

const NEAR = 4 // covers shown on each side
const REFLECTED = 2 // of which these have a reflection

function place(d: number): CSSProperties {
  const a = Math.abs(d), side = Math.sign(d)
  return {
    '--flow-x': side === 0 ? 0 : side * (0.74 + (a - 1) * 0.3),
    '--flow-turn': `${-side * (a === 0 ? 0 : 58)}deg`,
    '--flow-back': a === 0 ? 0 : 0.5 + a * 0.08,
    '--flow-dim': a === 0 ? 0 : Math.min(0.62, 0.3 + a * 0.08),
    zIndex: 9 - a,
    opacity: a > NEAR - 1 ? 0 : 1,
  } as CSSProperties
}

export function CoverFlow({ items, index, onIndex, onOpen, consoles, label }: {
  items: Item[]; index: number; onIndex: (i: number) => void; onOpen: (it: Item) => void; consoles: Map<string, GameConsole>; label: string
}) {
  const stage = useRef<HTMLDivElement>(null)
  const refocus = useRef(false)
  const go = useCallback((to: number) => {
    const next = Math.max(0, Math.min(items.length - 1, to))
    if (next === index) return false
    refocus.current = !!stage.current?.contains(document.activeElement)
    onIndex(next)
    return true
  }, [items.length, index, onIndex])
  // Focus stays on the front cover as it changes.
  useEffect(() => {
    if (!refocus.current) return
    refocus.current = false
    stage.current?.querySelector<HTMLElement>('[data-front="1"]')?.focus({ preventScroll: true })
  }, [index])

  const onKey = (e: React.KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return
    const by = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0
    if (by && go(index + by)) e.preventDefault()
    else if (e.key === 'Home' && go(0)) e.preventDefault()
    else if (e.key === 'End' && go(items.length - 1)) e.preventDefault()
  }

  // Dragging moves one cover at a time, by how far the finger has travelled.
  const drag = useRef<{ x: number; moved: boolean } | null>(null)
  const down = (e: React.PointerEvent) => { if (e.button === 0) drag.current = { x: e.clientX, moved: false } }
  const move = (e: React.PointerEvent) => {
    const d = drag.current
    if (!d || !stage.current) return
    const step = stage.current.clientHeight * 0.32
    const by = (d.x - e.clientX) / step
    if (Math.abs(by) >= 1) {
      d.moved = true
      d.x = e.clientX
      go(index + Math.sign(by))
    }
  }
  const wheel = useRef(0)
  // A sideways swipe on a trackpad. Up and down stay the page's.
  const onWheel = (e: React.WheelEvent) => {
    if (Math.abs(e.deltaX) <= Math.abs(e.deltaY)) return
    wheel.current += e.deltaX
    if (Math.abs(wheel.current) > 60) { go(index + Math.sign(wheel.current)); wheel.current = 0 }
  }
  // Each cover's own shape, once its picture has come.
  const [shapes, setShapes] = useState<Record<number, number>>({})

  const first = Math.max(0, index - NEAR), last = Math.min(items.length - 1, index + NEAR)
  const shown = items.slice(first, last + 1)
  return (
    <div ref={stage} className="gm-flow" role="group" aria-roledescription="carousel" aria-label={label} onKeyDown={onKey}
      onPointerDown={down} onPointerMove={move} onPointerUp={() => { window.setTimeout(() => { drag.current = null }, 0) }} onPointerCancel={() => { drag.current = null }} onWheel={onWheel}>
      {shown.map((it, n) => {
        const i = first + n, d = i - index, a = Math.abs(d), c = consoles.get(it.console || '')
        return (
          <button key={it.id} type="button" className="gm-flow-item" style={{ ...place(d), ...(shapes[it.id] ? { '--flow-ar': shapes[it.id] } as CSSProperties : null) }} tabIndex={d === 0 ? 0 : -1} data-front={d === 0 ? '1' : undefined} data-gm-flow={it.id}
            aria-hidden={d === 0 ? undefined : true} aria-label={`${it.title}${c ? ', ' + c.short : ''}. ${i + 1} of ${items.length}. Open`}
            onClick={() => { if (drag.current?.moved) return; if (d === 0) onOpen(it); else go(i) }}>
            <span className="gm-flow-art">
              {it.poster
                ? <img src={heroCover(it)} alt="" draggable={false} decoding="async" onLoad={(e) => { const m = e.currentTarget; if (m.naturalHeight) { const ar = m.naturalWidth / m.naturalHeight; setShapes((l) => (l[it.id] === ar ? l : { ...l, [it.id]: ar })) } }} />
                : <Placeholder title={it.title} label={c?.short} />}
            </span>
            {a <= REFLECTED && it.poster && <span className="gm-flow-floor" aria-hidden="true"><img src={heroCover(it)} alt="" draggable={false} decoding="async" /></span>}
          </button>
        )
      })}
    </div>
  )
}
