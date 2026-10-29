import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { IconClose, IconDown, IconEject, IconFf, IconLeft, IconNext, IconPause, IconPlay, IconPrev, IconRew, IconRight, IconStop, IconUp } from '../icons'

// Keys of a classic DVD remote. Disc-only keys are disabled for plain videos.
export type RemoteKey =
  | 'eject' | 'topmenu' | 'menu' | 'return' | 'up' | 'down' | 'left' | 'right' | 'enter'
  | 'audio' | 'subtitle' | 'angle' | 'prev' | 'next' | 'rew' | 'ff' | 'play' | 'pause' | 'stop'
  | `digit${0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9}`

export const DVD_ONLY: RemoteKey[] = ['topmenu', 'menu', 'return', 'up', 'down', 'left', 'right', 'enter', 'angle',
  'digit0', 'digit1', 'digit2', 'digit3', 'digit4', 'digit5', 'digit6', 'digit7', 'digit8', 'digit9']

type Props = {
  onKey: (k: RemoteKey) => void
  enabled: (k: RemoteKey) => boolean
  onClose: () => void
  label?: string
}

// Remembers whether the remote is shown, separately for discs and videos.
export function useRemoteToggle(kind: 'dvd' | 'video'): [boolean, (v: boolean) => void] {
  const key = 'wms.remote.' + kind
  const read = () => { try { const v = localStorage.getItem(key); return v == null ? kind === 'dvd' : v === '1' } catch { return kind === 'dvd' } }
  const [on, setOn] = useState(read)
  const set = (v: boolean) => { setOn(v); try { localStorage.setItem(key, v ? '1' : '0') } catch { /* ignore */ } }
  return [on, set]
}

// Where the remote sits, how big and how see-through it is; remembered per
// browser. x/y are pixels from the player's top-left corner (null = default
// spot in the bottom-right).
type Geom = { x: number | null; y: number | null; scale: number; alpha: number }
const GEOM_KEY = 'wms.remote.geom'
const ALPHAS = [0.92, 0.75, 0.55, 0.35]

function loadGeom(): Geom {
  try {
    const g = JSON.parse(localStorage.getItem(GEOM_KEY) || '')
    return { x: g.x ?? null, y: g.y ?? null, scale: Math.min(1.6, Math.max(0.6, g.scale || 1)), alpha: g.alpha || ALPHAS[0] }
  } catch { return { x: null, y: null, scale: 1, alpha: ALPHAS[0] } }
}

export default function Remote({ onKey, enabled, onClose, label = 'WittyDVD' }: Props) {
  const [pressed, setPressed] = useState<RemoteKey | null>(null)
  const t = useRef<any>(null)
  useEffect(() => () => clearTimeout(t.current), [])
  const el = useRef<HTMLDivElement | null>(null)
  const [geom, setGeomState] = useState<Geom>(loadGeom)
  const setGeom = useCallback((g: Geom, persist = true) => {
    setGeomState(g)
    if (persist) { try { localStorage.setItem(GEOM_KEY, JSON.stringify(g)) } catch { /* ignore */ } }
  }, [])
  // Keep the remote inside the player (after drags, resizes and window changes).
  const clamp = useCallback((g: Geom): Geom => {
    const r = el.current, parent = r?.parentElement
    if (!r || !parent) return g
    const w = r.offsetWidth * g.scale, h = r.offsetHeight * g.scale
    const W = parent.clientWidth, H = parent.clientHeight
    const x = g.x == null ? W - w - 18 : g.x
    const y = g.y == null ? H - h - 96 : g.y
    return { ...g, x: Math.max(4, Math.min(W - w - 4, x)), y: Math.max(4, Math.min(H - h - 4, y)) }
  }, [])
  useLayoutEffect(() => {
    const fix = () => setGeomState((g) => clamp(g))
    fix()
    window.addEventListener('resize', fix)
    return () => window.removeEventListener('resize', fix)
  }, [clamp])
  // Drag by the header.
  const startDrag = (e: React.PointerEvent) => {
    if ((e.target as HTMLElement).closest('button')) return
    e.preventDefault()
    const sx = e.clientX, sy = e.clientY, g0 = clamp(geom)
    const at = (ev: PointerEvent) => clamp({ ...g0, x: (g0.x || 0) + ev.clientX - sx, y: (g0.y || 0) + ev.clientY - sy })
    const move = (ev: PointerEvent) => setGeom(at(ev), false)
    const up = (ev: PointerEvent) => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); setGeom(at(ev)) }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  // Resize from the bottom-right corner (double-click resets the size).
  const startResize = (e: React.PointerEvent) => {
    e.preventDefault(); e.stopPropagation()
    const sx = e.clientX, g0 = clamp(geom), w0 = (el.current?.offsetWidth || 196) * g0.scale
    const at = (ev: PointerEvent) => clamp({ ...g0, scale: Math.min(1.6, Math.max(0.6, g0.scale * (w0 + ev.clientX - sx) / w0)) })
    const move = (ev: PointerEvent) => setGeom(at(ev), false)
    const up = (ev: PointerEvent) => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); setGeom(at(ev)) }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  const cycleAlpha = () => setGeom({ ...geom, alpha: ALPHAS[(ALPHAS.indexOf(geom.alpha) + 1) % ALPHAS.length] })
  const style = {
    left: geom.x ?? undefined, top: geom.y ?? undefined, right: geom.x == null ? 18 : 'auto', bottom: geom.y == null ? 96 : 'auto',
    transform: 'scale(' + geom.scale + ')', '--ra': geom.alpha,
  } as React.CSSProperties
  const B = ({ k, children, cls = '', title }: { k: RemoteKey; children: React.ReactNode; cls?: string; title?: string }) => {
    const on = enabled(k)
    return (
      <button type="button" className={'rk ' + cls + (pressed === k ? ' down' : '')} disabled={!on} title={title}
        onClick={(e) => { e.stopPropagation(); setPressed(k); clearTimeout(t.current); t.current = setTimeout(() => setPressed(null), 140); onKey(k) }}>
        {children}
      </button>
    )
  }
  return (
    <div className="remote" ref={el} style={style} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()} role="group" aria-label="DVD remote">
      <div className="remote-head" onPointerDown={startDrag} title="Drag to move">
        <B k="eject" cls="round red" title="Eject (leave the player)"><IconEject size={16} /></B>
        <span className="remote-brand">{label}</span>
        <button type="button" className="rk round ghost" title="Transparency" onClick={(e) => { e.stopPropagation(); cycleAlpha() }}>{'\u25d0'}</button>
        <button type="button" className="rk round ghost" title="Hide remote (r)" onClick={(e) => { e.stopPropagation(); onClose() }}><IconClose size={14} /></button>
      </div>
      <div className="remote-row2">
        <B k="topmenu" cls="pill" title="Top menu (disc title menu)">TOP MENU</B>
        <B k="menu" cls="pill" title="Menu (title set root menu)">MENU</B>
      </div>
      <div className="remote-dpad">
        <B k="up" cls="d up" title="Up"><IconUp /></B>
        <B k="left" cls="d left" title="Left"><IconLeft /></B>
        <B k="enter" cls="d enter" title="Enter">ENTER</B>
        <B k="right" cls="d right" title="Right"><IconRight /></B>
        <B k="down" cls="d down" title="Down"><IconDown /></B>
      </div>
      <div className="remote-row2">
        <B k="return" cls="pill" title="Return (go back)">RETURN</B>
        <B k="angle" cls="pill" title="Angle">ANGLE</B>
      </div>
      <div className="remote-row2">
        <B k="audio" cls="pill" title="Next audio track">AUDIO</B>
        <B k="subtitle" cls="pill" title="Next subtitle track">SUBTITLE</B>
      </div>
      <div className="remote-row4">
        <B k="prev" title="Previous chapter"><IconPrev size={16} /></B>
        <B k="rew" title="Rewind 10 s"><IconRew size={16} /></B>
        <B k="ff" title="Forward 30 s"><IconFf size={16} /></B>
        <B k="next" title="Next chapter"><IconNext size={16} /></B>
      </div>
      <div className="remote-row3">
        <B k="stop" title="Stop"><IconStop size={14} /></B>
        <B k="play" cls="accent" title="Play"><IconPlay size={16} /></B>
        <B k="pause" title="Pause"><IconPause size={16} /></B>
      </div>
      <div className="remote-digits">
        {[1, 2, 3, 4, 5, 6, 7, 8, 9].map((d) => <B key={d} k={`digit${d}` as RemoteKey} title={`Button / chapter ${d}`}>{d}</B>)}
        <span />
        <B k="digit0" title="0">0</B>
        <span />
      </div>
      <span className="remote-grip" onPointerDown={startResize} onDoubleClick={() => setGeom(clamp({ ...geom, scale: 1 }))} title="Drag to resize, double-click to reset" />
    </div>
  )
}
