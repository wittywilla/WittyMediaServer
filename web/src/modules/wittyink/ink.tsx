import { memo, useState, type ReactNode } from 'react'
import { IconEraser, IconMarker, IconMore, IconPen, IconPencil, IconRedo, IconUndo } from './icons'

// Handwriting for the readers: pen, pencil and highlighter strokes with pen
// pressure and tilt, an eraser, and the tool settings. Strokes are flat
// [x, y, pressure, ...] lists so a long stroke stays small on the wire and
// its outline is worked out once, then only moved with the page.

export type Tool = 'pen' | 'pencil' | 'marker' | 'eraser'
export type DrawTool = Exclude<Tool, 'eraser'>
// t: tool, w: thickness, o: opacity, a: where the stroke is anchored (the
// reader decides what that means), pts: points in the same units as w.
export type Stroke = { t: DrawTool; w: number; o: number; a: number; pts: number[] }

export const INK_COLORS = [
  '#000000', '#3a3a3c', '#636366', '#8e8e93', '#c7c7cc', '#ffffff',
  '#7f1d1d', '#c0262d', '#ff3b30', '#ff6b6b', '#ff8a00', '#ffb340',
  '#7a5c00', '#d4a000', '#ffd60a', '#1b5e20', '#2e9d4a', '#34c759',
  '#0b5d5e', '#00a6a6', '#5ac8fa', '#0a2a7a', '#0a5bd8', '#0a84ff',
  '#3d1a78', '#6e3bd8', '#af52de', '#8a1c4a', '#d6336c', '#ff6fa8',
  '#4e342e', '#8d6e63', '#c8a27a', '#1f3a4d', '#4b6a88', '#9fb4c7',
]
export const MARKER_COLORS = [
  '#ffd60a', '#fff176', '#ffe08a', '#ffb340', '#ff9f0a', '#ff8a65',
  '#ff6b6b', '#ff6fa8', '#f48fb1', '#e0a0ff', '#b39ddb', '#9fa8ff',
  '#5ac8fa', '#80deea', '#64ffda', '#69f0ae', '#b2ff59', '#d4e157',
]
const TOOLS: [Tool, ReactNode, string][] = [['pen', <IconPen />, 'Pen'], ['pencil', <IconPencil />, 'Pencil'], ['marker', <IconMarker />, 'Highlighter'], ['eraser', <IconEraser />, 'Eraser']]
const SIZES: Record<DrawTool, number[]> = { pen: [0.75, 1.5, 2.5, 4, 6, 9], pencil: [0.5, 1, 1.5, 2.5, 4, 6], marker: [8, 12, 16, 22, 30, 40] }

export type InkPrefs = {
  tool: Tool; color: Record<DrawTool, string>; size: Record<DrawTool, number>; opacity: Record<DrawTool, number>
  pressure: boolean // pen pressure changes thickness
  penOnly: boolean // palm rejection: fingers turn pages, only a pen draws
  autoPen: boolean // a pen draws without switching to the ink tools first
  snap: boolean // highlighter strokes over text become text highlights
  recent: string[]
}
export const INK_DEF: InkPrefs = {
  tool: 'pen', color: { pen: '#0a2a7a', pencil: '#3a3a3c', marker: '#ffd60a' }, size: { pen: 2.5, pencil: 1.5, marker: 16 },
  opacity: { pen: 1, pencil: 0.85, marker: 0.4 }, pressure: true, penOnly: false, autoPen: true, snap: true, recent: [],
}
const KEY = 'wms.ink'
export function loadInk(): InkPrefs {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) || '{}')
    if (v.tool === 'eraser') v.tool = 'pen' // a new session starts with something that writes
    return { ...INK_DEF, ...v, color: { ...INK_DEF.color, ...v.color }, size: { ...INK_DEF.size, ...v.size }, opacity: { ...INK_DEF.opacity, ...v.opacity } }
  } catch { return INK_DEF }
}
export function saveInk(p: InkPrefs) { try { localStorage.setItem(KEY, JSON.stringify(p)) } catch { /* ignore */ } }

// Width multiplier for a pressure reading. Mice and fingers report 0.5 while
// pressed, which gives exactly the chosen thickness.
const pressureScale = (p: number) => 0.3 + 1.4 * Math.min(1, Math.max(0, p))

// The pressure to store for a point: real pressure from pens only, and for
// the pencil, tilt widens the line like the side of a lead.
export function pointPressure(e: PointerEvent, tool: DrawTool, usePressure: boolean): number {
  if (e.pointerType !== 'pen' || !usePressure) return 0.5
  let p = e.pressure || 0.5
  if (tool === 'pencil') p = Math.min(1, p * (1 + Math.min(1, Math.hypot(e.tiltX || 0, e.tiltY || 0) / 90) * 1.2))
  return p
}

const f = (n: number) => Math.round(n * 1000) / 1000
// A smooth curve through flat [x, y, ...] points (quadratics through the
// midpoints), continuing from the current point at xy[0], xy[1].
function curve(xy: number[]): string {
  let s = ''
  for (let i = 2; i < xy.length - 2; i += 2) s += `Q${f(xy[i])},${f(xy[i + 1])} ${f((xy[i] + xy[i + 2]) / 2)},${f((xy[i + 1] + xy[i + 3]) / 2)}`
  return s + `L${f(xy[xy.length - 2])},${f(xy[xy.length - 1])}`
}

// The centre line of a stroke (for the highlighter, drawn with a flat width).
export function centerline(pts: number[]): string {
  const xy: number[] = []
  for (let i = 0; i < pts.length; i += 3) xy.push(pts[i], pts[i + 1])
  if (xy.length === 2) xy.push(xy[0] + 0.01, xy[1])
  return `M${f(xy[0])},${f(xy[1])}` + curve(xy)
}

// The outline of a pen or pencil stroke as a filled shape whose width
// follows the pressure, with round ends.
export function outline(pts: number[], w: number, varying: boolean): string {
  const n = pts.length / 3
  const rad = (i: number) => (w / 2) * (varying ? pressureScale(pts[i * 3 + 2]) : 1)
  if (n === 0) return ''
  if (n === 1) {
    const r = rad(0), x = pts[0], y = pts[1]
    return `M${f(x - r)},${f(y)}a${f(r)},${f(r)} 0 1 0 ${f(2 * r)},0a${f(r)},${f(r)} 0 1 0 ${f(-2 * r)},0Z`
  }
  const L: number[] = [], R: number[] = []
  let r = rad(0)
  const r0 = r
  for (let i = 0; i < n; i++) {
    const a = Math.max(0, i - 1), b = Math.min(n - 1, i + 1)
    const dx = pts[b * 3] - pts[a * 3], dy = pts[b * 3 + 1] - pts[a * 3 + 1]
    const len = Math.hypot(dx, dy) || 1
    r = r * 0.5 + rad(i) * 0.5 // eases pressure spikes so they don't knot
    const nx = (-dy / len) * r, ny = (dx / len) * r
    L.push(pts[i * 3] + nx, pts[i * 3 + 1] + ny)
    R.push(pts[i * 3] - nx, pts[i * 3 + 1] - ny)
  }
  const back: number[] = []
  for (let i = R.length - 2; i >= 0; i -= 2) back.push(R[i], R[i + 1])
  return `M${f(L[0])},${f(L[1])}${curve(L)}A${f(r)},${f(r)} 0 0 0 ${f(back[0])},${f(back[1])}${curve(back)}A${f(r0)},${f(r0)} 0 0 0 ${f(L[0])},${f(L[1])}Z`
}

// Whether (x, y) is within tol of the stroke (same units as the stroke).
export function hits(pts: number[], w: number, x: number, y: number, tol: number): boolean {
  const lim = tol + w / 2
  for (let i = 0; i < pts.length; i += 3) {
    const ax = pts[i], ay = pts[i + 1]
    const bx = i + 3 < pts.length ? pts[i + 3] : ax, by = i + 3 < pts.length ? pts[i + 4] : ay
    const vx = bx - ax, vy = by - ay
    const t = vx || vy ? Math.max(0, Math.min(1, ((x - ax) * vx + (y - ay) * vy) / (vx * vx + vy * vy))) : 0
    if (Math.hypot(ax + t * vx - x, ay + t * vy - y) <= lim) return true
  }
  return false
}

// One stroke as SVG, in its own units; the caller positions and scales it.
export const InkStroke = memo(function InkStroke({ s, color, blend, grain = 'ink-grain' }: { s: Stroke; color: string; blend: boolean; grain?: string }) {
  if (s.t === 'marker') {
    return <path d={centerline(s.pts)} fill="none" stroke={color} strokeWidth={s.w} strokeLinecap="round" strokeLinejoin="round" opacity={s.o} style={blend ? { mixBlendMode: 'multiply' } : undefined} />
  }
  return <path d={outline(s.pts, s.w, true)} fill={color} opacity={s.o} filter={s.t === 'pencil' ? `url(#${grain})` : undefined} />
})

// Pencil grain. The frequency is per stroke unit: 9 suits strokes kept in
// ems (about 16 px each), 0.7 suits PDF points.
export const InkDefs = ({ id = 'ink-grain', freq = 9 }: { id?: string; freq?: number }) => (
  <defs>
    <filter id={id} x="-10%" y="-10%" width="120%" height="120%">
      <feTurbulence type="fractalNoise" baseFrequency={freq} numOctaves="1" seed="4" result="n" />
      <feColorMatrix in="n" type="matrix" values="0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  0 0 0 -2 1.6" result="m" />
      <feComposite in="SourceGraphic" in2="m" operator="in" />
    </filter>
  </defs>
)

// How full a range input's track is drawn (see .rd-range).
const pct = (v: number, min: number, max: number) => ({ ['--pct' as string]: `${((v - min) / (max - min)) * 100}%` })

// The ink toolbar: tools, colours, thickness, opacity and pen settings.
export function InkBar({ p, set, undo, redo, canUndo, canRedo, done, extra }: {
  p: InkPrefs; set: (v: Partial<InkPrefs>) => void; undo: () => void; redo: () => void; canUndo: boolean; canRedo: boolean; done: () => void; extra?: React.ReactNode
}) {
  const [open, setOpen] = useState<null | 'color' | 'more'>(null)
  const t: DrawTool = p.tool === 'eraser' ? 'pen' : p.tool
  const color = p.color[t]
  const pick = (c: string) => {
    set({ color: { ...p.color, [t]: c }, recent: [c, ...p.recent.filter((x) => x !== c)].slice(0, 10), tool: t })
    setOpen(null)
  }
  const palette = t === 'marker' ? MARKER_COLORS : INK_COLORS
  return (
    <div className="ink-bar" role="toolbar" aria-label="Handwriting tools" onPointerDown={(e) => e.stopPropagation()}>
      <div className="ink-row">
        {TOOLS.map(([v, icon, label]) => (
          <button key={v} className={'ink-tool' + (p.tool === v ? ' on' : '')} aria-pressed={p.tool === v} title={`${label}${v === 'eraser' ? ' (e)' : ''}`} aria-label={label} onClick={() => set({ tool: v })}>{icon}</button>
        ))}
        <span className="ink-sep" />
        <button className="ink-swatch" style={{ background: color }} aria-label={`Colour ${color}`} aria-expanded={open === 'color'} title="Colour" onClick={() => setOpen(open === 'color' ? null : 'color')} />
        {p.recent.filter((c) => c !== color).slice(0, 4).map((c) => <button key={c} className="ink-swatch sm" style={{ background: c }} aria-label={`Colour ${c}`} onClick={() => pick(c)} />)}
        <span className="ink-sep" />
        <div className="ink-sizes" role="radiogroup" aria-label="Thickness">
          {SIZES[t].map((s) => (
            <button key={s} role="radio" aria-checked={p.size[t] === s} className={p.size[t] === s ? 'on' : ''} title={`${s} px`} aria-label={`${s} pixels`} onClick={() => set({ size: { ...p.size, [t]: s }, tool: p.tool === 'eraser' ? t : p.tool })}>
              <i style={{ width: Math.min(22, Math.max(3, t === 'marker' ? s / 2 : s * 1.6)), height: Math.min(22, Math.max(3, t === 'marker' ? s / 2 : s * 1.6)) }} />
            </button>
          ))}
        </div>
        <span className="ink-sep" />
        <button className="ink-tool" onClick={undo} disabled={!canUndo} aria-label="Undo" title="Undo (Ctrl+Z)"><IconUndo /></button>
        <button className="ink-tool" onClick={redo} disabled={!canRedo} aria-label="Redo" title="Redo (Ctrl+Y)"><IconRedo /></button>
        <button className={'ink-tool' + (open === 'more' ? ' on' : '')} onClick={() => setOpen(open === 'more' ? null : 'more')} aria-expanded={open === 'more'} aria-label="Pen settings" title="Pen settings"><IconMore /></button>
        {extra}
        <span className="ink-sep" />
        <button className="btn sm primary ink-done" onClick={done}>Done</button>
      </div>
      {open === 'color' && (
        <div className="ink-pop">
          <div className="ink-grid">{palette.map((c) => <button key={c} className={'ink-swatch' + (c === color ? ' on' : '')} style={{ background: c }} aria-label={c} onClick={() => pick(c)} />)}</div>
          {t !== 'marker' && <div className="ink-grid">{MARKER_COLORS.slice(0, 6).map((c) => <button key={c} className="ink-swatch" style={{ background: c }} aria-label={c} onClick={() => pick(c)} />)}</div>}
          <label className="ink-field">Any colour <input type="color" value={color} onChange={(e) => pick(e.target.value)} /></label>
        </div>
      )}
      {open === 'more' && (
        <div className="ink-pop ink-more">
          <label className="ink-field">Thickness <span className="muted">{p.size[t]} px</span>
            <input type="range" className="rd-range" style={pct(p.size[t], t === 'marker' ? 4 : 0.25, t === 'marker' ? 60 : 16)} min={t === 'marker' ? 4 : 0.25} max={t === 'marker' ? 60 : 16} step={0.25} value={p.size[t]} onChange={(e) => set({ size: { ...p.size, [t]: Number(e.target.value) } })} />
          </label>
          <label className="ink-field">Opacity <span className="muted">{Math.round(p.opacity[t] * 100)}%</span>
            <input type="range" className="rd-range" style={pct(p.opacity[t], 0.1, 1)} min={0.1} max={1} step={0.05} value={p.opacity[t]} onChange={(e) => set({ opacity: { ...p.opacity, [t]: Number(e.target.value) } })} />
          </label>
          <label className="check-row"><input type="checkbox" checked={p.pressure} onChange={(e) => set({ pressure: e.target.checked })} /> Pen pressure changes thickness</label>
          <label className="check-row"><input type="checkbox" checked={p.penOnly} onChange={(e) => set({ penOnly: e.target.checked })} /> Only a pen draws (fingers turn pages)</label>
          <label className="check-row"><input type="checkbox" checked={p.autoPen} onChange={(e) => set({ autoPen: e.target.checked })} /> Draw with a pen any time</label>
          <label className="check-row"><input type="checkbox" checked={p.snap} onChange={(e) => set({ snap: e.target.checked })} /> Highlighter over text makes a text highlight</label>
          <p className="muted small">A pen's eraser end or side button erases. Pencil lines widen when the pen is tilted.</p>
        </div>
      )}
    </div>
  )
}
