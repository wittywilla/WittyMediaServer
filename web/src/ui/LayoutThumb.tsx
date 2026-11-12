// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { CSSProperties } from 'react'
import { specFor, type Cards, type Home, type Nav, type WireMap } from './layoutSpecs'

// A look's layout as a small wireframe: its menu (where and how it is drawn), how Home is arranged, and the cards. It is
// drawn in the colours it is given (the theme being tried: bg, surface, accent) or, failing those, the page's own, so the
// picker shows each look as it would wear the chosen theme. Decoration only: the card it sits on carries the name.
export type Colours = { bg?: string; surface?: string; accent?: string }

const W = 160, H = 100

function NavShape({ nav }: { nav: Nav }) {
  switch (nav) {
    case 'sidebar': return <g><rect x="0" y="0" width="30" height={H} className="lt-surface" />{[14, 24, 34, 44].map((y) => <rect key={y} x="5" y={y} width="20" height="4" rx="2" className={y === 14 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'rail': return <g><rect x="0" y="0" width="12" height={H} className="lt-surface" />{[14, 26, 38, 50].map((y) => <circle key={y} cx="6" cy={y} r="3" className={y === 14 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'railDrawer': return <g><rect x="0" y="0" width="12" height={H} className="lt-surface" />{[14, 26, 38].map((y) => <circle key={y} cx="6" cy={y} r="3" className={y === 26 ? 'lt-accent' : 'lt-mute'} />)}<rect x="12" y="0" width="28" height={H} className="lt-surface2" />{[18, 28, 38, 48].map((y) => <rect key={y} x="17" y={y} width="18" height="4" rx="2" className="lt-mute" />)}<rect x="17" y="8" width="14" height="5" rx="1" className="lt-fg" /></g>
    case 'floating': return <g><rect x="22" y="5" width="116" height="11" rx="5.5" className="lt-surface2" />{[36, 54, 72, 90].map((x) => <rect key={x} x={x} y="9" width="12" height="3" rx="1.5" className={x === 36 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'cable': return <g><rect x="0" y="0" width={W} height="13" className="lt-accent" />{[8, 30, 52, 74].map((x) => <rect key={x} x={x} y="5" width="16" height="3" className="lt-onaccent" />)}</g>
    case 'bevel': return <g><rect x="0" y="0" width={W} height="13" className="lt-surface2" /><rect x="0" y="12" width={W} height="1.5" className="lt-mute" />{[8, 30, 52, 74].map((x) => <rect key={x} x={x} y="4.5" width="16" height="4" rx="1" className={x === 8 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'tele': return <g><rect x="0" y="0" width={W} height="13" className="lt-surface" />{[0, 1, 2, 3].map((i) => <rect key={i} x={8 + i * 22} y="7" width="18" height="4" className={['lt-red', 'lt-green', 'lt-yellow', 'lt-accent'][i]} />)}</g>
    case 'crossbar': return <g>{[30, 56, 82, 108].map((x, i) => <g key={x}><circle cx={x + 6} cy="7" r="3.5" className={i === 1 ? 'lt-accent' : 'lt-mute'} /></g>)}</g>
    case 'bottom': return <g><rect x="14" y={H - 14} width={W - 28} height="10" rx="5" className="lt-surface2" />{[40, 60, 80, 100].map((x) => <circle key={x} cx={x} cy={H - 9} r="2.5" className={x === 40 ? 'lt-accent' : 'lt-mute'} />)}</g>
    case 'none': return <g />
    default: return <g><rect x="0" y="0" width={W} height="12" className="lt-surface" />{[8, 28, 48, 68].map((x) => <rect key={x} x={x} y="4.5" width="14" height="3" rx="1.5" className={x === 8 ? 'lt-accent' : 'lt-mute'} />)}</g>
  }
}

// The free area beside or under the menu.
function area(nav: Nav): { x: number; y: number; w: number; h: number } {
  if (nav === 'sidebar') return { x: 36, y: 8, w: W - 44, h: H - 12 }
  if (nav === 'rail') return { x: 18, y: 8, w: W - 26, h: H - 12 }
  if (nav === 'railDrawer') return { x: 46, y: 8, w: W - 54, h: H - 12 }
  if (nav === 'floating') return { x: 10, y: 22, w: W - 20, h: H - 26 }
  if (nav === 'crossbar') return { x: 10, y: 20, w: W - 20, h: H - 24 }
  if (nav === 'bottom') return { x: 10, y: 8, w: W - 20, h: H - 26 }
  if (nav === 'none') return { x: 10, y: 8, w: W - 20, h: H - 12 }
  return { x: 10, y: 19, w: W - 20, h: H - 23 }
}

function Cards({ kind, x, y, w, h }: { kind: Cards; x: number; y: number; w: number; h: number }) {
  const n = kind === 'big' ? 3 : kind === 'landscape' || kind === 'logo' ? 3 : 5
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

export function LayoutThumb({ wire, formFactor = 'desktop', colours, className }: { wire?: WireMap; formFactor?: keyof WireMap; colours?: Colours; className?: string }) {
  const s = specFor(wire, formFactor)
  const box = area(s.nav)
  const style = { ...(colours?.bg ? { '--lt-bg': colours.bg } : {}), ...(colours?.surface ? { '--lt-surface': colours.surface } : {}), ...(colours?.accent ? { '--lt-accent': colours.accent } : {}) } as CSSProperties
  return (
    <svg className={'layout-thumb' + (className ? ' ' + className : '')} style={style} viewBox={`0 0 ${W} ${H}`} aria-hidden="true" focusable="false">
      <rect x="0" y="0" width={W} height={H} className="lt-bg" />
      <HomeShape home={s.home} cards={s.cards} box={box} />
      <NavShape nav={s.nav} />
      {s.dock && <rect x="0" y={H - 9} width={W} height="9" className="lt-surface" />}
    </svg>
  )
}
