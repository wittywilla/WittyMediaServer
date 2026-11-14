// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { CSSProperties } from 'react'
import { specFor, type Cards, type Home, type Nav, type WireMap } from './layoutSpecs'

// A look's layout as a small wireframe: its menu (where and how it is drawn), how Home is arranged, and the cards. It is
// drawn in the colours it is given (the theme being tried: bg, surface, accent) or, failing those, the page's own, so the
// picker shows each look as it would wear the chosen theme. Decoration only: the card it sits on carries the name.
export type Colours = { bg?: string; surface?: string; accent?: string }

// The frame each kind of device is drawn in: a phone upright, a tablet a little less tall, a computer 16:10, a TV 16:9.
export type Frame = 'phone' | 'tablet' | 'desktop' | 'tv'
const FRAMES: Record<Frame, [number, number]> = { phone: [76, 160], tablet: [120, 160], desktop: [160, 100], tv: [160, 90] }
export const frameFor = (formFactor: string): Frame => formFactor === 'phone' || formFactor === 'tablet' || formFactor === 'tv' ? formFactor : 'desktop'

// Places for n small items along a bar, from x0 a step apart; fewer on a narrow frame.
const along = (W: number, x0: number, step: number, n = 4) => Array.from({ length: W < 100 ? n - 1 : n }, (_, i) => x0 + i * step)

function NavShape({ nav, W, H }: { nav: Nav; W: number; H: number }) {
  const sw = Math.round(W * 0.19)
  switch (nav) {
    case 'sidebar': return <g><rect x="0" y="0" width={sw} height={H} className="lt-surface" />{[14, 24, 34, 44].map((y) => <rect key={y} x={sw * 0.17} y={y} width={sw * 0.66} height="4" rx="2" className={y === 14 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'rail': return <g><rect x="0" y="0" width="12" height={H} className="lt-surface" />{[14, 26, 38, 50].map((y) => <circle key={y} cx="6" cy={y} r="3" className={y === 14 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'railDrawer': return <g><rect x="0" y="0" width="12" height={H} className="lt-surface" />{[14, 26, 38].map((y) => <circle key={y} cx="6" cy={y} r="3" className={y === 26 ? 'lt-accent' : 'lt-mute'} />)}<rect x="12" y="0" width="28" height={H} className="lt-surface2" />{[18, 28, 38, 48].map((y) => <rect key={y} x="17" y={y} width="18" height="4" rx="2" className="lt-mute" />)}<rect x="17" y="8" width="14" height="5" rx="1" className="lt-fg" /></g>
    case 'floating': { const x = W * 0.14, w = W * 0.72; return <g><rect x={x} y="5" width={w} height="11" rx="5.5" className="lt-surface2" />{along(W, x + 8, (w - 16) / 4).map((cx, i) => <rect key={cx} x={cx} y="9" width={Math.min(12, w / 6)} height="3" rx="1.5" className={i === 0 ? 'lt-accent' : 'lt-mute'} />)}</g> }
    case 'cable': return <g><rect x="0" y="0" width={W} height="13" className="lt-accent" />{along(W, 8, 22).map((x) => <rect key={x} x={x} y="5" width="16" height="3" className="lt-onaccent" />)}</g>
    case 'bevel': return <g><rect x="0" y="0" width={W} height="13" className="lt-surface2" /><rect x="0" y="12" width={W} height="1.5" className="lt-mute" />{along(W, 8, 22).map((x) => <rect key={x} x={x} y="4.5" width="16" height="4" rx="1" className={x === 8 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'tele': return <g><rect x="0" y="0" width={W} height="13" className="lt-surface" />{along(W, 8, 22).map((x, i) => <rect key={i} x={x} y="7" width="18" height="4" className={['lt-red', 'lt-green', 'lt-yellow', 'lt-accent'][i]} />)}</g>
    case 'crossbar': { const xs = along(W, 0, 1); return <g>{xs.map((_, i) => <circle key={i} cx={W * (i + 1) / (xs.length + 1)} cy="7" r="3.5" className={i === 1 ? 'lt-accent' : 'lt-mute'} />)}</g> }
    case 'bottom': { const xs = along(W, 0, 1); return <g><rect x={W * 0.09} y={H - 14} width={W * 0.82} height="10" rx="5" className="lt-surface2" />{xs.map((_, i) => <circle key={i} cx={W * 0.09 + W * 0.82 * (i + 1) / (xs.length + 1)} cy={H - 9} r="2.5" className={i === 0 ? 'lt-accent' : 'lt-mute'} />)}</g> }
    case 'none': return <g />
    default: return <g><rect x="0" y="0" width={W} height="12" className="lt-surface" />{along(W, 8, 20).map((x) => <rect key={x} x={x} y="4.5" width="14" height="3" rx="1.5" className={x === 8 ? 'lt-accent' : 'lt-mute'} />)}</g>
  }
}

// The free area beside or under the menu.
function area(nav: Nav, W: number, H: number): { x: number; y: number; w: number; h: number } {
  const sw = Math.round(W * 0.19)
  if (nav === 'sidebar') return { x: sw + 6, y: 8, w: W - sw - 14, h: H - 12 }
  if (nav === 'rail') return { x: 18, y: 8, w: W - 26, h: H - 12 }
  if (nav === 'railDrawer') return { x: 46, y: 8, w: W - 54, h: H - 12 }
  if (nav === 'floating') return { x: 10, y: 22, w: W - 20, h: H - 26 }
  if (nav === 'crossbar') return { x: 10, y: 20, w: W - 20, h: H - 24 }
  if (nav === 'bottom') return { x: 10, y: 8, w: W - 20, h: H - 26 }
  if (nav === 'none') return { x: 10, y: 8, w: W - 20, h: H - 12 }
  return { x: 10, y: 19, w: W - 20, h: H - 23 }
}

function Cards({ kind, x, y, w, h }: { kind: Cards; x: number; y: number; w: number; h: number }) {
  const wide = kind === 'big' || kind === 'landscape' || kind === 'logo'
  const n = w < 90 ? (wide ? 2 : 3) : wide ? 3 : 5
  const gap = 4
  const cw = (w - gap * (n - 1)) / n
  if (kind === 'list') return <g>{[0, 1, 2].map((i) => <g key={i}><rect x={x} y={y + i * 11} width="9" height="8" rx="1" className="lt-surface2" /><rect x={x + 13} y={y + 2 + i * 11} width={w * 0.5} height="3" rx="1.5" className="lt-fg" /><rect x={x + 13} y={y + 6 + i * 11} width={w * 0.3} height="2" rx="1" className="lt-mute" /></g>)}</g>
  const ch = kind === 'landscape' || kind === 'logo' ? cw * 0.56 : kind === 'block' ? Math.min(h, 14) : Math.min(h, cw * 1.45)
  return (
    <g>
      {Array.from({ length: n }).map((_, i) => {
        const cx = x + i * (cw + gap)
        return (
          <g key={i}>
            <rect x={cx} y={y} width={cw} height={ch} rx={kind === 'block' ? 0 : 2} className={kind === 'block' ? (['lt-red', 'lt-green', 'lt-yellow', 'lt-accent', 'lt-surface2'][i % 5]) : 'lt-surface2'} />
            {kind === 'plate' && <rect x={cx} y={y + ch - 6} width={cw} height="6" className="lt-scrim" />}
            {kind !== 'plate' && kind !== 'block' && <rect x={cx} y={y + ch + 2} width={cw * 0.7} height="2.5" rx="1" className="lt-mute" />}
          </g>
        )
      })}
    </g>
  )
}

function HomeShape({ home, cards, box }: { home: Home; cards: Cards; box: { x: number; y: number; w: number; h: number } }) {
  const { x, y, w, h } = box
  switch (home) {
    case 'billboard': return <g><rect x={x} y={y} width={w} height={h * 0.42} rx="3" className="lt-surface" /><rect x={x + 5} y={y + h * 0.42 - 12} width={w * 0.38} height="4" rx="2" className="lt-fg" /><rect x={x + 5} y={y + h * 0.42 - 6} width="16" height="4" rx="2" className="lt-accent" /><Cards kind={cards} x={x} y={y + h * 0.42 + 6} w={w} h={h * 0.5} /></g>
    case 'tiles': return <g><Cards kind="big" x={x} y={y + 4} w={w} h={h * 0.5} /><Cards kind="big" x={x} y={y + h * 0.55} w={w} h={h * 0.4} /></g>
    case 'banner': return <g><rect x={x} y={y} width={w} height="10" rx="2" className="lt-surface" /><rect x={x + 4} y={y + 3.5} width={w * 0.3} height="3" rx="1.5" className="lt-fg" /><Cards kind={cards} x={x} y={y + 16} w={w} h={h - 18} /></g>
    case 'carousel': { const cw = w * 0.26; const cx = x + (w - cw) / 2; return <g><rect x={x + 6} y={y + 10} width={cw * 0.7} height={h * 0.55} rx="2" className="lt-surface2" /><rect x={x + w - 6 - cw * 0.7} y={y + 10} width={cw * 0.7} height={h * 0.55} rx="2" className="lt-surface2" /><rect x={cx} y={y + 2} width={cw} height={h * 0.75} rx="2" className="lt-surface" stroke="var(--lt-accent)" strokeWidth="1.2" /><rect x={cx + 2} y={y + h * 0.75 + 6} width={cw - 4} height="3" rx="1.5" className="lt-fg" /></g> }
    case 'list': return <g>{[0, 1, 2, 3].map((i) => <g key={i}><rect x={x} y={y + i * 13} width={w} height="11" rx={cards === 'block' ? 0 : 2} className={i === 0 ? 'lt-accent' : 'lt-surface'} /><rect x={x + 4} y={y + 4 + i * 13} width={w * (0.3 + (i % 2) * 0.2)} height="3" rx="1.5" className={i === 0 ? 'lt-onaccent' : 'lt-fg'} /></g>)}</g>
    case 'widgets': return <g>{[[0, 0, 0.55, 0.5], [0.58, 0, 0.42, 0.5], [0, 0.54, 0.3, 0.46], [0.33, 0.54, 0.67, 0.46]].map(([a, b, c, d], i) => <rect key={i} x={x + a * w} y={y + b * h} width={c * w - 2} height={d * h - 2} rx="3" className={i === 0 ? 'lt-surface' : 'lt-surface2'} />)}</g>
    default: return <g><rect x={x} y={y} width={w * 0.3} height="6" rx="3" className="lt-accent" />{[0, 1, 2].map((i) => <g key={i}><rect x={x} y={y + 12 + i * 14} width="14" height="10" rx="2" className="lt-surface2" /><rect x={x + 18} y={y + 14 + i * 14} width={w * 0.5} height="3" rx="1.5" className="lt-fg" /></g>)}</g>
  }
}

// formFactor picks the wireframe; frame the shape it is drawn in (by default the form factor's own).
export function LayoutThumb({ wire, formFactor = 'desktop', frame, colours, className }: { wire?: WireMap; formFactor?: keyof WireMap; frame?: Frame; colours?: Colours; className?: string }) {
  const f = frame || frameFor(formFactor)
  const [W, H] = FRAMES[f]
  const s = specFor(wire, formFactor)
  const box = area(s.nav, W, H)
  const style = { ...(colours?.bg ? { '--lt-bg': colours.bg } : {}), ...(colours?.surface ? { '--lt-surface': colours.surface } : {}), ...(colours?.accent ? { '--lt-accent': colours.accent } : {}) } as CSSProperties
  return (
    <svg className={'layout-thumb lt-' + f + (className ? ' ' + className : '')} style={style} viewBox={`0 0 ${W} ${H}`} aria-hidden="true" focusable="false">
      <rect x="0" y="0" width={W} height={H} className="lt-bg" />
      <HomeShape home={s.home} cards={s.cards} box={box} />
      <NavShape nav={s.nav} W={W} H={H} />
      {s.dock && <rect x="0" y={H - 9} width={W} height="9" className="lt-surface" />}
    </svg>
  )
}
