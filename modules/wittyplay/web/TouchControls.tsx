import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Control, ControlsData, Layout, Orientation } from './consoles'
import { clamp, layoutFor, touchMask } from './touch'

// The on-screen controller, and the editor that rearranges it. Fingers are tracked
// on the whole area rather than on each button, so sliding from one button to the
// next works as it does on a real pad.

export function useOrientation(): Orientation {
  const get = (): Orientation => (window.matchMedia?.('(orientation: portrait)').matches ? 'portrait' : 'landscape')
  const [o, setO] = useState<Orientation>(get)
  useEffect(() => {
    const on = () => setO(get())
    window.addEventListener('resize', on)
    window.addEventListener('orientationchange', on)
    return () => { window.removeEventListener('resize', on); window.removeEventListener('orientationchange', on) }
  }, [])
  return o
}

function useSize(ref: React.RefObject<HTMLElement | null>) {
  const [size, setSize] = useState({ w: 0, h: 0 })
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const read = () => setSize({ w: el.clientWidth, h: el.clientHeight })
    read()
    const ro = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(read) : null
    ro?.observe(el)
    window.addEventListener('resize', read)
    return () => { ro?.disconnect(); window.removeEventListener('resize', read) }
  }, [ref])
  return size
}

function ControlFace({ c, active }: { c: Control; active: boolean }) {
  if (c.kind === 'dpad') {
    return (
      <svg viewBox="0 0 100 100" aria-hidden="true">
        <path className="wp-dpad-shape" d="M36 4h28a4 4 0 014 4v24h24a4 4 0 014 4v28a4 4 0 01-4 4H68v24a4 4 0 01-4 4H36a4 4 0 01-4-4V68H8a4 4 0 01-4-4V36a4 4 0 014-4h24V8a4 4 0 014-4z" />
        <path className="wp-dpad-arrows" d="M50 12l9 12H41zM50 88l-9-12h18zM12 50l12-9v18zM88 50l-12 9V41z" />
      </svg>
    )
  }
  return <span className={'wp-btn-face wp-btn-' + c.kind + (active ? ' on' : '')}>{c.label}</span>
}

// On a console with a touch screen, a finger that isn't on a button is the stylus.
export type OnPoint = (clientX: number, clientY: number, phase: 'down' | 'move' | 'up') => void

export function TouchControls({ data, orientation, onMask, onPoint }: { data: ControlsData; orientation: Orientation; onMask: (mask: number) => void; onPoint?: OnPoint }) {
  const area = useRef<HTMLDivElement>(null)
  const size = useSize(area)
  const controls = data.controls
  const layout = useMemo(() => layoutFor(data, orientation), [data, orientation])
  const points = useRef(new Map<number, { x: number; y: number }>())
  const lastMask = useRef(0)
  const [mask, setMask] = useState(0)
  const stylus = useRef<number | null>(null)

  const update = useCallback(() => {
    const el = area.current
    if (!el) return
    const m = touchMask([...points.current.values()], controls, layout, { w: el.clientWidth, h: el.clientHeight })
    if (m !== lastMask.current) {
      if (m & ~lastMask.current) navigator.vibrate?.(8) // a press, not a release
      lastMask.current = m
      setMask(m)
      onMask(m)
    }
  }, [controls, layout, onMask])

  useEffect(() => () => onMask(0), [onMask])

  const track = (e: React.PointerEvent) => {
    if (e.pointerType === 'mouse' && e.buttons === 0) return
    const el = area.current!
    const r = el.getBoundingClientRect()
    const pt = { x: e.clientX - r.left, y: e.clientY - r.top }
    if (stylus.current === e.pointerId) { onPoint?.(e.clientX, e.clientY, 'move'); return }
    if (onPoint && e.type === 'pointerdown' && stylus.current === null && touchMask([pt], controls, layout, { w: el.clientWidth, h: el.clientHeight }) === 0) {
      stylus.current = e.pointerId
      onPoint(e.clientX, e.clientY, 'down')
      return
    }
    points.current.set(e.pointerId, pt)
    update()
  }
  const release = (e: React.PointerEvent) => {
    if (stylus.current === e.pointerId) {
      stylus.current = null
      onPoint?.(e.clientX, e.clientY, 'up')
      return
    }
    points.current.delete(e.pointerId)
    update()
  }
  const unit = Math.min(size.w, size.h)
  return (
    <div ref={area} className="wp-touch" aria-hidden="true" onPointerDown={(e) => { e.preventDefault(); try { (e.target as Element).setPointerCapture?.(e.pointerId) } catch { /* fine */ } track(e) }} onPointerMove={(e) => { if (points.current.has(e.pointerId) || stylus.current === e.pointerId) track(e) }}
      onPointerUp={release} onPointerCancel={release} onLostPointerCapture={release} onContextMenu={(e) => e.preventDefault()}>
      {size.w > 0 && controls.map((c) => {
        const p = layout[c.key]
        if (!p || p.hidden) return null
        const px = p.size * unit
        return (
          <div key={c.key} className={'wp-ctl wp-ctl-' + c.kind} style={{ left: p.x * size.w - px / 2, top: p.y * size.h - px / 2, width: px, height: c.kind === 'meta' ? px * 0.55 : c.kind === 'shoulder' ? px * 0.7 : px, opacity: p.opacity, fontSize: px * (c.kind === 'meta' ? 0.24 : 0.36) }}>
            <ControlFace c={c} active={c.kind !== 'dpad' && !!(mask & (1 << c.id))} />
          </div>
        )
      })}
    </div>
  )
}

// The editor: drag anything, or choose it from the list to change it with sliders
// and keys. Everything here works without a touch screen: with arrow keys or a D-pad,
// OK picks a control up, the arrows move it, and OK puts it down.
export function TouchEditor({ data, orientation, onSave, onClose }: { data: ControlsData; orientation: Orientation; onSave: (l: Layout | null) => Promise<void>; onClose: () => void }) {
  const area = useRef<HTMLDivElement>(null)
  const size = useSize(area)
  const controls = data.controls
  const [layout, setLayout] = useState<Layout>(() => layoutFor(data, orientation))
  const [sel, setSel] = useState<string>(controls[0]?.key || '')
  const [grabbed, setGrabbed] = useState(false)
  const [note, setNote] = useState('')
  const drag = useRef<{ id: number; key: string; dx: number; dy: number } | null>(null)
  useEffect(() => { setLayout(layoutFor(data, orientation)) }, [data, orientation])

  const change = (k: string, patch: Partial<Layout[string]>) => setLayout((l) => ({ ...l, [k]: { ...l[k], ...patch } }))
  const name = (k: string) => controls.find((c) => c.key === k)?.label || k
  const finish = async () => {
    try { await onSave(layout); onClose() } catch (e: any) { setNote(e?.message || 'That couldn’t be saved') }
  }
  const reset = () => { setLayout(data.defaults.touch[orientation]); setNote('Back to the default layout') }
  const unit = Math.min(size.w, size.h)
  const selP = layout[sel]

  const startDrag = (e: React.PointerEvent, key: string) => {
    e.preventDefault()
    e.stopPropagation()
    setSel(key)
    const r = area.current!.getBoundingClientRect()
    const p = layout[key]
    drag.current = { id: e.pointerId, key, dx: e.clientX - r.left - p.x * r.width, dy: e.clientY - r.top - p.y * r.height }
    try { (e.currentTarget as Element).setPointerCapture?.(e.pointerId) } catch { /* fine */ }
  }
  const moveDrag = (e: React.PointerEvent) => {
    const d = drag.current
    if (!d || d.id !== e.pointerId) return
    const r = area.current!.getBoundingClientRect()
    change(d.key, { x: clamp((e.clientX - r.left - d.dx) / r.width, 0, 1), y: clamp((e.clientY - r.top - d.dy) / r.height, 0, 1) })
  }
  const nudge = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') { // pick up, or put down
      e.preventDefault()
      setGrabbed((g) => { setNote(g ? `${name(sel)} put down` : `${name(sel)} picked up: use the arrows to move it`); return !g })
      return
    }
    if (!grabbed) return // the arrows move between controls, as everywhere else
    const step = e.shiftKey ? 0.05 : 0.01
    const move: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] }
    const m = move[e.key]
    if (!m || !selP) return
    e.preventDefault()
    change(sel, { x: clamp(selP.x + m[0], 0, 1), y: clamp(selP.y + m[1], 0, 1) })
  }

  return (
    <div className="wp-editor" role="dialog" aria-label={`Touch controls for ${orientation} screens`}>
      <div ref={area} className="wp-editor-area" onPointerMove={moveDrag} onPointerUp={() => { drag.current = null }} onPointerCancel={() => { drag.current = null }}>
        {size.w > 0 && controls.map((c) => {
          const p = layout[c.key]
          if (!p) return null
          const px = p.size * unit
          return (
            <button key={c.key} type="button" className={'wp-ctl wp-ctl-' + c.kind + ' wp-ctl-edit' + (sel === c.key ? ' selected' : '') + (p.hidden ? ' hidden' : '')}
              style={{ left: p.x * size.w - px / 2, top: p.y * size.h - px / 2, width: px, height: c.kind === 'meta' ? px * 0.55 : c.kind === 'shoulder' ? px * 0.7 : px, opacity: p.hidden ? 0.25 : p.opacity, fontSize: px * (c.kind === 'meta' ? 0.24 : 0.36) }}
              aria-label={`${c.label}: press to pick up, then move with the arrow keys`} aria-pressed={sel === c.key && grabbed}
              onPointerDown={(e) => startDrag(e, c.key)} onFocus={() => { setSel(c.key); setGrabbed(false) }} onKeyDown={nudge}>
              <ControlFace c={c} active={false} />
            </button>
          )
        })}
      </div>
      <div className="wp-editor-panel">
        <h2>Touch controls <span className="wp-muted">· {orientation}</span></h2>
        <p className="wp-muted">Saved for this device and this console. Drag a control, or press it and use the arrow keys.</p>
        {selP && (
          <div className="wp-editor-sel">
            <b>{name(sel)}</b>
            <label>Size<input type="range" min={0.05} max={0.6} step={0.005} value={selP.size} onChange={(e) => change(sel, { size: Number(e.target.value) })} /></label>
            <label>Opacity<input type="range" min={0.1} max={1} step={0.05} value={selP.opacity} onChange={(e) => change(sel, { opacity: Number(e.target.value) })} /></label>
          </div>
        )}
        <fieldset className="wp-editor-list">
          <legend>Show</legend>
          {controls.map((c) => (
            <label key={c.key} className={sel === c.key ? 'on' : ''}>
              <input type="checkbox" checked={!layout[c.key]?.hidden} onChange={(e) => { change(c.key, { hidden: !e.target.checked }); setSel(c.key) }} /> {c.label}
            </label>
          ))}
        </fieldset>
        <label className="wp-editor-all">All controls’ opacity
          <input type="range" min={0.1} max={1} step={0.05} defaultValue={selP?.opacity ?? 0.55} onChange={(e) => {
            const v = Number(e.target.value)
            setLayout((l) => Object.fromEntries(Object.entries(l).map(([k, p]) => [k, { ...p, opacity: v }])))
          }} />
        </label>
        <div className="wp-row" aria-live="polite">{note}</div>
        <div className="wp-row">
          <button type="button" className="wp-btn" onClick={reset}>Reset</button>
          <span className="wp-grow" />
          <button type="button" className="wp-btn" onClick={onClose}>Cancel</button>
          <button type="button" className="wp-btn primary" onClick={finish}>Save</button>
        </div>
      </div>
    </div>
  )
}
