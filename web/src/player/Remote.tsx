import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useRef, useState } from 'react'
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
  // Resize from any edge or corner. The remote keeps its shape, so the size
  // follows the larger pointer movement, and the opposite edge stays put.
  const startResize = (dx: -1 | 0 | 1, dy: -1 | 0 | 1) => (e: React.PointerEvent) => {
    e.preventDefault(); e.stopPropagation()
    const sx = e.clientX, sy = e.clientY, g0 = clamp(geom)
    const bw = el.current?.offsetWidth || 196, bh = el.current?.offsetHeight || 640
    const at = (ev: PointerEvent): Geom => {
      const gx = dx ? (dx * (ev.clientX - sx)) / bw : 0
      const gy = dy ? (dy * (ev.clientY - sy)) / bh : 0
      const scale = Math.min(1.6, Math.max(0.6, g0.scale + (Math.abs(gx) > Math.abs(gy) ? gx : gy)))
      const x = dx === -1 ? (g0.x || 0) + (g0.scale - scale) * bw : g0.x
      const y = dy === -1 ? (g0.y || 0) + (g0.scale - scale) * bh : g0.y
      return clamp({ ...g0, scale, x, y })
    }
    const move = (ev: PointerEvent) => setGeom(at(ev), false)
    const up = (ev: PointerEvent) => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up); setGeom(at(ev)) }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  const handles: [string, -1 | 0 | 1, -1 | 0 | 1][] = [['n', 0, -1], ['s', 0, 1], ['e', 1, 0], ['w', -1, 0], ['ne', 1, -1], ['nw', -1, -1], ['se', 1, 1], ['sw', -1, 1]]
  const cycleAlpha = () => setGeom({ ...geom, alpha: ALPHAS[(ALPHAS.indexOf(geom.alpha) + 1) % ALPHAS.length] })
  const style = {
    left: geom.x ?? undefined, top: geom.y ?? undefined, right: geom.x == null ? 18 : 'auto', bottom: geom.y == null ? 96 : 'auto',
    transform: 'scale(' + geom.scale + ')', '--ra': geom.alpha,
  } as React.CSSProperties
  const press = (k: RemoteKey) => { setPressed(k); clearTimeout(t.current); t.current = setTimeout(() => setPressed(null), 140); onKey(k) }
  return (
    <KeyCtx.Provider value={{ enabled, press, pressed }}>
    <div className="remote" ref={el} style={style} onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => e.stopPropagation()} role="group" aria-label={label + ' remote'}>
      <div className="remote-head" onPointerDown={startDrag}>
        <B k="eject" cls="round red" tip={label === 'WittyDVD' ? 'Eject the DVD' : label === 'WittyBluray' ? 'Eject the Blu-ray' : 'Stop and close'}><IconEject size={16} /></B>
        <span className="remote-brand">{label}</span>
        <button type="button" className="rk round ghost" data-tip="See-through" aria-label="Change how see-through the remote is" onClick={(e) => { e.stopPropagation(); cycleAlpha() }}>{'\u25d0'}</button>
        <button type="button" className="rk round ghost" data-tip="Close the Remote" aria-label="Close the Remote (r)" onClick={(e) => { e.stopPropagation(); onClose() }}><IconClose size={14} /></button>
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
        <B k="angle" cls="pill" title="Video version (camera angle)">ANGLE</B>
      </div>
      <div className="remote-row2">
        <B k="audio" cls="pill" title="Audio & language">AUDIO</B>
        <B k="subtitle" cls="pill" title="Subtitles">SUBTITLE</B>
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
      {handles.map(([n, dx, dy]) => (
        <span key={n} className={'remote-rs rs-' + n} onPointerDown={startResize(dx, dy)} onDoubleClick={() => setGeom(clamp({ ...geom, scale: 1 }))} aria-hidden="true" />
      ))}
      <span className="remote-grip" aria-hidden="true" />
    </div>
    </KeyCtx.Provider>
  )
}

// Buttons are module-level components (not defined inside Remote), so a
// re-render keeps the same DOM buttons: focus, hover and keyboard/controller
// navigation survive the player's frequent updates.
const KeyCtx = createContext<{ enabled: (k: RemoteKey) => boolean; press: (k: RemoteKey) => void; pressed: RemoteKey | null }>({ enabled: () => true, press: () => {}, pressed: null })

// tip: a see-through tooltip instead of the browser's (used in the header).
function B({ k, children, cls = '', title, tip }: { k: RemoteKey; children: React.ReactNode; cls?: string; title?: string; tip?: string }) {
  const { enabled, press, pressed } = useContext(KeyCtx)
  return (
    <button type="button" className={'rk ' + cls + (pressed === k ? ' down' : '')} disabled={!enabled(k)} title={tip ? undefined : title} data-tip={tip} aria-label={tip || title}
      onClick={(e) => { e.stopPropagation(); press(k) }}>
      {children}
    </button>
  )
}

// RemoteBar is the same remote laid out as one strip above the seek bar, for
// viewers who prefer it out of the picture.
export function RemoteBar({ onKey, enabled, disc = true }: { onKey: (k: RemoteKey) => void; enabled: (k: RemoteKey) => boolean; disc?: boolean }) {
  return (
    <KeyCtx.Provider value={{ enabled, press: onKey, pressed: null }}>
    <div className="remote-bar" role="toolbar" aria-label="Disc remote" onClick={(e) => e.stopPropagation()}>
      <BarKey k="topmenu" label="Top menu">TOP MENU</BarKey>
      <BarKey k="menu" label="Menu">MENU</BarKey>
      <BarKey k="return" label="Return">RETURN</BarKey>
      <span className="rb-sep" />
      <BarKey k="left" label="Left"><IconLeft size={16} /></BarKey>
      <BarKey k="up" label="Up"><IconUp size={16} /></BarKey>
      <BarKey k="enter" label="Enter" cls="accent">OK</BarKey>
      <BarKey k="down" label="Down"><IconDown size={16} /></BarKey>
      <BarKey k="right" label="Right"><IconRight size={16} /></BarKey>
      <span className="rb-sep" />
      <BarKey k="audio" label="Audio & language">AUDIO</BarKey>
      <BarKey k="subtitle" label="Subtitles">SUBTITLE</BarKey>
      <BarKey k="angle" label="Video version (camera angle)">ANGLE</BarKey>
      <span className="rb-sep hide-narrow" />
      <span className="rb-digits hide-narrow">
        {[1, 2, 3, 4, 5, 6, 7, 8, 9, 0].map((d) => <BarKey key={d} k={`digit${d}` as RemoteKey} label={`Button or chapter ${d}`}>{d}</BarKey>)}
      </span>
      <BarKey k="eject" label={disc ? 'Eject the disc' : 'Stop and close'} cls="red"><IconEject size={14} /></BarKey>
    </div>
    </KeyCtx.Provider>
  )
}

function BarKey({ k, children, label, cls = '' }: { k: RemoteKey; children: React.ReactNode; label: string; cls?: string }) {
  const { enabled, press } = useContext(KeyCtx)
  return <button type="button" className={'rb ' + cls} disabled={!enabled(k)} title={label} aria-label={label} onClick={(e) => { e.stopPropagation(); press(k) }}>{children}</button>
}
