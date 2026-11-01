import { useEffect, useRef, useState, type ReactNode } from 'react'
import { IconLeft, IconRight } from '../../icons'

// Covers in a row with one in front, the others turned away either side, and each with its
// reflection. A swipe, a press on a side cover, or the arrow keys bring another to the front;
// pressing the one in front opens it. Only the few nearest are on the page at all.

const REACH = 3

export type Cover<T> = { key: string; url: string; label: string; item: T }

export default function CoverFlow<T>({ covers, onChange, onOpen, tall, fallback }: { covers: Cover<T>[]; onChange?: (c: Cover<T>) => void; onOpen: (c: Cover<T>) => void; tall?: boolean; fallback: (c: Cover<T>) => ReactNode }) {
  const [at, setAt] = useState(0)
  const start = useRef<{ x: number; y: number } | null>(null)
  const dragged = useRef(false)
  const front = useRef<HTMLButtonElement>(null)
  const go = (i: number) => {
    const n = Math.max(0, Math.min(covers.length - 1, i))
    setAt(n)
    return n
  }
  useEffect(() => { if (covers[at]) onChange?.(covers[at]) }, [at, covers]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!covers.length) return null
  const shown = covers.map((c, i) => ({ c, i })).filter(({ i }) => Math.abs(i - at) <= REACH)
  return (
    <div className={'cf' + (tall ? ' tall' : '')} role="group" aria-roledescription="carousel" aria-label="Covers"
      onPointerDown={(e) => { start.current = { x: e.clientX, y: e.clientY }; dragged.current = false }}
      onPointerUp={(e) => {
        const s = start.current
        start.current = null
        if (!s) return
        const dx = e.clientX - s.x
        if (Math.abs(dx) > 36 && Math.abs(dx) > Math.abs(e.clientY - s.y)) { dragged.current = true; go(at + (dx < 0 ? 1 : -1)) }
      }}
      onKeyDown={(e) => {
        if (e.key === 'ArrowLeft' && at > 0) { e.preventDefault(); e.stopPropagation(); go(at - 1) }
        if (e.key === 'ArrowRight' && at < covers.length - 1) { e.preventDefault(); e.stopPropagation(); go(at + 1) }
      }}>
      <div className="cf-row">
        {shown.map(({ c, i }) => {
          const d = i - at
          return (
            <button key={c.key} type="button" ref={d === 0 ? front : undefined} className={'cf-cover' + (d === 0 ? ' front' : '')} style={{ '--d': d, '--a': Math.abs(d), zIndex: 10 - Math.abs(d) } as React.CSSProperties}
              tabIndex={d === 0 ? 0 : -1} aria-hidden={d === 0 ? undefined : true} aria-label={d === 0 ? `Open ${c.label}` : undefined}
              onClick={() => { if (dragged.current) return; if (d === 0) onOpen(c); else go(i) }}>
              <span className="cf-art">{c.url ? <img src={c.url} alt="" draggable={false} /> : fallback(c)}</span>
              {c.url && <span className="cf-reflect" aria-hidden="true"><img src={c.url} alt="" draggable={false} /></span>}
            </button>
          )
        })}
      </div>
      {covers.length > 1 && <>
        <button type="button" className="cf-step left" disabled={at === 0} onClick={() => { go(at - 1); front.current?.focus({ preventScroll: true }) }} aria-label="Previous cover"><IconLeft /></button>
        <button type="button" className="cf-step right" disabled={at === covers.length - 1} onClick={() => { go(at + 1); front.current?.focus({ preventScroll: true }) }} aria-label="Next cover"><IconRight /></button>
      </>}
    </div>
  )
}
