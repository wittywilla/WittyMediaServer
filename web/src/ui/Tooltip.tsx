import { cloneElement, useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type FocusEvent, type PointerEvent, type ReactElement, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

// A short hint for a control, shown on hover and on keyboard or remote focus.
// It never holds anything you need in order to act: on touch it doesn't appear,
// so the control's own name or a visible hint has to be enough. Escape dismisses
// it, and the pointer can move onto it without it vanishing (WCAG 1.4.13).

const SHOW_AFTER = 450
const HIDE_AFTER = 120
const EDGE = 8

type TriggerProps = {
  onPointerEnter?: (e: PointerEvent<HTMLElement>) => void
  onPointerLeave?: (e: PointerEvent<HTMLElement>) => void
  onFocus?: (e: FocusEvent<HTMLElement>) => void
  onBlur?: (e: FocusEvent<HTMLElement>) => void
  'aria-describedby'?: string
}

export function Tooltip({ text, side = 'top', describes = true, children }: { text: ReactNode; side?: 'top' | 'bottom' | 'right'; describes?: boolean; children: ReactElement<TriggerProps> }) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)
  const timer = useRef(0)
  const tip = useRef<HTMLDivElement | null>(null)
  const id = useId()
  const clear = () => window.clearTimeout(timer.current)
  const hideSoon = useCallback(() => { clear(); timer.current = window.setTimeout(() => setAnchor(null), HIDE_AFTER) }, [])
  useEffect(() => clear, [])
  useLayoutEffect(() => {
    if (!anchor || !tip.current) { setPos(null); return }
    const a = anchor.getBoundingClientRect(), t = tip.current.getBoundingClientRect()
    let left = a.left + a.width / 2 - t.width / 2
    let top = side === 'bottom' || a.top - t.height - EDGE < 0 ? a.bottom + EDGE : a.top - t.height - EDGE
    if (side === 'right') { left = a.right + EDGE; top = a.top + a.height / 2 - t.height / 2 }
    setPos({ left: Math.max(EDGE, Math.min(left, innerWidth - t.width - EDGE)), top: Math.max(EDGE, Math.min(top, innerHeight - t.height - EDGE)) })
  }, [anchor, side])
  useEffect(() => {
    if (!anchor) return
    const away = (e: KeyboardEvent) => { if (e.key === 'Escape') setAnchor(null) }
    const gone = () => setAnchor(null)
    window.addEventListener('keydown', away)
    window.addEventListener('scroll', gone, { capture: true, passive: true })
    return () => { window.removeEventListener('keydown', away); window.removeEventListener('scroll', gone, { capture: true }) }
  }, [anchor])
  if (!text) return children
  const p = children.props
  const trigger = cloneElement(children, {
    onPointerEnter: (e) => { p.onPointerEnter?.(e); if (e.pointerType !== 'mouse') return; clear(); const el = e.currentTarget; timer.current = window.setTimeout(() => setAnchor(el), SHOW_AFTER) },
    onPointerLeave: (e) => { p.onPointerLeave?.(e); hideSoon() },
    onFocus: (e) => { p.onFocus?.(e); if (e.currentTarget.matches(':focus-visible')) { clear(); setAnchor(e.currentTarget) } },
    onBlur: (e) => { p.onBlur?.(e); hideSoon() },
    'aria-describedby': describes && anchor ? id : p['aria-describedby'],
  })
  return (
    <>
      {trigger}
      {anchor && createPortal(
        <div ref={tip} id={id} role="tooltip" className="ui-tooltip" style={pos ? { left: pos.left, top: pos.top } : { visibility: 'hidden' }}
          onPointerEnter={clear} onPointerLeave={hideSoon}>{text}</div>, document.body)}
    </>
  )
}
