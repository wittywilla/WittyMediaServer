// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The drawings behind GameCase.tsx: one entry per drawing id in SHAPES (the styles in gamePictures.ts CASE_STYLES say which drawing each
// uses, in which finish, and whether turned at an angle). Every drawing is WittyPlay's own: plain SVG shapes that echo how an era's media
// were shaped, with no product's outline traced, no maker's name and no logo. They share one light (from the top left), one set of
// gradients (Defs, coloured by the CSS variable --gc-col so a console's shade or a style's finish recolours them), one bevel language
// (a lit top-left rim and a shaded bottom-right rim) and one family of corner radii, so every style sits well beside every other.
import type { ReactNode } from 'react'

export type Box = [x: number, y: number, w: number, h: number]
// thick: how deep the drawing is when turned at an angle (in the drawing's own units), and how far its side face is inset from the top and bottom.
export type Shape = { w: number; h: number; label: Box; r?: number; round?: boolean; brand?: Box; thick?: { d: number; t?: number; b?: number }; body: (k: string) => ReactNode; over?: (k: string) => ReactNode }

const U = (k: string, id: string) => `url(#${k}${id})`
const stop = (o: number, c: string, op?: number) => <stop offset={o} style={{ stopColor: c, stopOpacity: op }} />
const mix = (pct: number, to: string) => `color-mix(in srgb, var(--gc-col) ${pct}%, ${to})`

// The gradients every drawing uses, with ids made unique per picture (k) so each keeps its own colour.
export function Defs({ k }: { k: string }) {
  return (
    <defs>
      <linearGradient id={k + 'b'} x1="0" y1="0" x2=".3" y2="1">{stop(0, mix(90, '#fff'))}{stop(0.5, 'var(--gc-col)')}{stop(1, mix(84, '#000'))}</linearGradient>
      <linearGradient id={k + 'bd'} x1="0" y1="0" x2="0" y2="1">{stop(0, mix(58, '#000'))}{stop(1, mix(76, '#000'))}</linearGradient>
      <linearGradient id={k + 'sd'} x1="0" y1="0" x2="1" y2="0">{stop(0, mix(58, '#000'))}{stop(1, mix(44, '#000'))}</linearGradient>
      <linearGradient id={k + 'ct'} x1="0" y1="1" x2="1" y2="0">{stop(0, mix(84, '#fff'))}{stop(1, mix(70, '#fff'))}</linearGradient>
      <linearGradient id={k + 'rim'} x1="0" y1="0" x2="1" y2="1">{stop(0, '#fff', 0.38)}{stop(0.45, '#fff', 0)}{stop(0.6, '#000', 0)}{stop(1, '#000', 0.34)}</linearGradient>
      <linearGradient id={k + 'rimr'} x1="0" y1="0" x2="1" y2="1">{stop(0, '#000', 0.6)}{stop(0.5, '#000', 0)}{stop(0.6, '#fff', 0)}{stop(1, '#fff', 0.34)}</linearGradient>
      <linearGradient id={k + 'sh'} x1="0" y1="0" x2="0" y2="1">{stop(0, '#fff', 0.28)}{stop(1, '#fff', 0)}</linearGradient>
      <linearGradient id={k + 'gl'} x1="0" y1="0" x2="1" y2="1">{stop(0, '#fff', 0.14)}{stop(0.99, '#fff', 0.14)}{stop(1, '#fff', 0)}</linearGradient>
      <linearGradient id={k + 'mt'} x1="0" y1="0" x2="0" y2="1">{stop(0, '#f4f6fa')}{stop(0.45, '#b9bfcb')}{stop(0.55, '#8f96a5')}{stop(1, '#cdd2dc')}</linearGradient>
      <linearGradient id={k + 'au'} x1="0" y1="0" x2="0" y2="1">{stop(0, '#f2d98a')}{stop(0.5, '#c79a3c')}{stop(1, '#8a6420')}</linearGradient>
      <radialGradient id={k + 'ds'} cx=".5" cy=".5" r=".5">{stop(0.7, '#000', 0)}{stop(1, '#000', 0.16)}</radialGradient>
      <linearGradient id={k + 'ph'} x1="0" y1="0" x2="0" y2="1">{stop(0, '#6a778c')}{stop(1, '#232b3c')}</linearGradient>
    </defs>
  )
}

// A solid part of the drawing: lit fill, then the bevel rim over it.
const part = (k: string, d: string, cls = 'gc-body', fill = 'b', rim = 1.3) => (
  <>
    <path className={cls} d={d} fill={U(k, fill)} />
    <path d={d} fill="none" stroke={U(k, 'rim')} strokeWidth={rim} />
  </>
)
const prect = (k: string, x: number, y: number, w: number, h: number, r: number, cls = 'gc-body', fill = 'b', rim = 1.3) => (
  <>
    <rect className={cls} x={x} y={y} width={w} height={h} rx={r} fill={U(k, fill)} />
    <rect x={x} y={y} width={w} height={h} rx={r} fill="none" stroke={U(k, 'rim')} strokeWidth={rim} />
  </>
)
// A recess cut into the surface (for a label to sit in): dark, with a shaded top-left lip and a lit bottom-right lip.
const recess = (k: string, [x, y, w, h]: Box, padIn: number, r: number) => {
  const pad = padIn * 0.62
  return (
  <>
    <rect x={x - pad} y={y - pad} width={w + pad * 2} height={h + pad * 2} rx={r} fill={U(k, 'bd')} />
    <rect x={x - pad} y={y - pad} width={w + pad * 2} height={h + pad * 2} rx={r} fill="none" stroke={U(k, 'rimr')} strokeWidth="1.2" />
  </>
  )
}
// Grooves: lines between two points with a lit twin just below, so they read as cut in.
const ridges = (n: number, x1: number, x2: number, y1: number, y2: number) =>
  Array.from({ length: n }, (_, i) => { const y = y1 + ((y2 - y1) * i) / Math.max(1, n - 1); return <g key={'r' + i}><line className="gc-groove" x1={x1} x2={x2} y1={y} y2={y} /><line className="gc-groove-hi" x1={x1} x2={x2} y1={y + 1} y2={y + 1} /></g> })
const ribs = (n: number, x1: number, x2: number, y1: number, y2: number) =>
  Array.from({ length: n }, (_, i) => { const x = x1 + ((x2 - x1) * i) / Math.max(1, n - 1); return <g key={'c' + i}><line className="gc-groove" x1={x} x2={x} y1={y1} y2={y2} /><line className="gc-groove-hi" x1={x + 1} x2={x + 1} y1={y1} y2={y2} /></g> })
// Gold contact fingers along an edge.
const fingers = (k: string, n: number, x1: number, x2: number, y: number, h: number) =>
  Array.from({ length: n }, (_, i) => { const w = ((x2 - x1) / n) * 0.62; const x = x1 + ((x2 - x1) * i) / n; return <rect key={'f' + i} x={x} y={y} width={w} height={h} rx={w / 3} fill={U(k, 'au')} /> })
const sheen = (d: string, k: string) => <path d={d} fill={U(k, 'sh')} opacity=".22" />
const glare = (d: string, k: string) => <path d={d} fill={U(k, 'gl')} />
// The inner board a see-through cartridge shows (hidden unless the finish is clear).
const inner = (x: number, y: number, w: number, h: number) => <g className="gc-inner"><rect x={x} y={y} width={w} height={h} rx="2" /><rect x={x + w * 0.12} y={y + h * 0.18} width={w * 0.3} height={h * 0.3} rx="1" fill="#0d0e12" /><rect x={x + w * 0.55} y={y + h * 0.18} width={w * 0.3} height={h * 0.3} rx="1" fill="#0d0e12" /></g>

// ---- the box helper: a cardboard box seen from above and to the right, so its front, side and top all show ----
function box(fw: number, fh: number, sd: number, td: number, opts: { flip?: boolean; seam?: boolean; r?: number } = {}): Shape {
  const W = fw + sd, H = fh + td
  return {
    w: W, h: H, label: [0, td, fw, fh], r: 1,
    thick: undefined,
    body: (k) => (
      <>
        {/* the top: lit, with the lid's seam */}
        <path fill={U(k, 'ct')} d={`M0 ${td} L${sd} 0 H${W} L${fw} ${td} Z`} />
        <path d={`M0 ${td} L${sd} 0 H${W} L${fw} ${td} Z`} fill="none" stroke="rgb(0 0 0 / .28)" strokeWidth=".7" />
        {opts.seam !== false && <path className="gc-card-line" d={`M${sd * 0.42} ${td * 0.58} H${fw + sd * 0.42}`} />}
        {/* the side: in shade, with a flap line */}
        <path fill={U(k, 'sd')} d={`M${fw} ${td} L${W} 0 V${fh} L${fw} ${H} Z`} />
        <path d={`M${fw} ${td} L${W} 0 V${fh} L${fw} ${H} Z`} fill="none" stroke="rgb(0 0 0 / .35)" strokeWidth=".7" />
        <path className="gc-card-line" d={`M${fw} ${td + fh * 0.16} L${W} ${fh * 0.16}`} />
        <path className="gc-card-line" d={`M${fw + sd * 0.5} ${td * 0.5} V${fh + td * 0.5}`} opacity=".4" />
        {/* the front, behind its art */}
        <rect fill={U(k, 'b')} x="0" y={td} width={fw} height={fh} rx="1" />
      </>
    ),
    over: (k) => (
      <>
        <rect x="0.4" y={td + 0.4} width={fw - 0.8} height={fh - 0.8} fill="none" stroke={U(k, 'rim')} strokeWidth="1" />
        <path d={`M0 ${td + 0.8} H${fw}`} stroke="rgb(255 255 255 / .35)" strokeWidth=".8" />
        {opts.flip && <path d={`M0 ${td + fh * 0.2} H${fw}`} stroke="rgb(0 0 0 / .3)" strokeWidth=".9" />}
        {glare(`M0 ${td} H${fw * 0.5} L${fw * 0.12} ${H} H0 Z`, k)}
        {/* a corner worn through to the board, so it reads as cardboard */}
        <path d={`M${fw - 7} ${H} L${fw} ${H - 7} V${H} Z`} fill="rgb(0 0 0 / .1)" />
      </>
    ),
  }
}

// ---- discs ----
function disc(w: number, ring: number, hole: number): Shape {
  const c = w / 2
  return {
    w, h: w, label: [2.2, 2.2, w - 4.4, w - 4.4], round: true,
    body: () => (
      <>
        <circle cx={c} cy={c} r={c} fill="rgb(0 0 0 / .001)" />
        <circle className="gc-disc" cx={c} cy={c} r={c - 0.4} fill="#cdd2dc" />
      </>
    ),
    over: (k) => (
      <>
        <circle className="gc-disc-rim" cx={c} cy={c} r={c - 0.9} fill="none" strokeWidth="1.6" />
        <circle cx={c} cy={c} r={c - 2.2} fill="none" stroke="rgb(0 0 0 / .35)" strokeWidth=".6" />
        {/* the clear stacking ring, the mirror band and the hole */}
        <circle className="gc-disc-ring" cx={c} cy={c} r={ring * 1.18} />
        <circle cx={c} cy={c} r={ring * 1.18} fill="none" stroke="rgb(0 0 0 / .3)" strokeWidth=".7" />
        <circle className="gc-disc-hub" cx={c} cy={c} r={ring * 0.74} />
        <circle cx={c} cy={c} r={ring * 0.74} fill="none" stroke="rgb(255 255 255 / .55)" strokeWidth=".7" />
        <circle className="gc-hole" cx={c} cy={c} r={hole} />
        <circle cx={c} cy={c} r={hole} fill="none" stroke="rgb(255 255 255 / .35)" strokeWidth=".7" />
        <circle cx={c} cy={c} r={c - 0.5} fill={U(k, 'ds')} />
        {glare(`M${c} 3 A${c - 3} ${c - 3} 0 0 1 ${w - 6} ${c * 0.74} L${c} ${c} Z`, k)}
      </>
    ),
  }
}

// A clear cover over a case's insert: a lit lip along the top and left and a diagonal gloss.
const clearCover = (k: string, [x, y, w, h]: Box, r = 2) => (
  <>
    <rect x={x} y={y} width={w} height={h} rx={r} fill="none" stroke={U(k, 'rim')} strokeWidth="1.2" />
    {glare(`M${x} ${y} H${x + w * 0.62} L${x + w * 0.2} ${y + h} H${x} Z`, k)}
    <path d={`M${x + w * 0.7} ${y} L${x + w * 0.58} ${y + h}`} stroke="rgb(255 255 255 / .1)" strokeWidth="3" />
  </>
)

export const SHAPES: Record<string, Shape> = {
  // 8-bit: a grey front-loader: a label recess over a ridged band, side grips, a stepped foot with its contact slot.
  'cart-nes': {
    w: 120, h: 138, label: [14, 12, 92, 72], r: 2, thick: { d: 16, t: 2, b: 8 },
    body: (k) => (
      <>
        {part(k, 'M7 0 H113 Q120 0 120 7 V122 L113 132 H105 V138 H15 V132 H7 L0 122 V7 Q0 0 7 0 Z')}
        {inner(14, 90, 92, 36)}
        {sheen('M7 1 H113 Q119 1 119.6 7 V10 H0.4 V7 Q1 1 7 1 Z', k)}
        {recess(k, [14, 12, 92, 72], 5, 4)}
        {part(k, 'M10 92 H110 Q114 92 114 96 V124 Q114 128 110 128 H10 Q6 128 6 124 V96 Q6 92 10 92 Z', 'gc-plate', 'bd', 1)}
        {ridges(7, 12, 108, 98, 122)}
        {ribs(4, 3.5, 6.5, 18, 80)}{ribs(4, 113.5, 116.5, 18, 80)}
        <rect x="26" y="131" width="68" height="6" rx="1.5" fill="#0b0c10" />
        {fingers(k, 14, 28, 92, 132.5, 4)}
      </>
    ),
  },
  // First generation: short and ribbed along the top, a wide recessed label, deep grips at its foot.
  'cart-atari': {
    w: 130, h: 106, label: [12, 18, 106, 52], r: 2, thick: { d: 15, t: 2, b: 10 },
    body: (k) => (
      <>
        {part(k, 'M7 0 H123 Q130 0 130 7 V90 L121 106 H9 L0 90 V7 Q0 0 7 0 Z')}
        {part(k, 'M7 3 H123 Q127 3 127 7 V12 H3 V7 Q3 3 7 3 Z', 'gc-plate', 'bd', 1)}
        {ribs(16, 12, 118, 4.5, 11)}
        {recess(k, [12, 18, 106, 52], 5, 3)}
        {ribs(15, 14, 116, 80, 98)}
        <path className="gc-grain" d="M10 82 Q40 76 70 84 T120 80 M12 92 Q50 86 80 94 T118 90" />
        {sheen('M7 1 H123 Q129 1 129.6 7 V9 H0.4 V7 Q1 1 7 1 Z', k)}
      </>
    ),
  },
  // 16-bit: rounded at the top, grip dimples in its shoulders, a narrower foot that slides into the slot.
  'cart-super': {
    w: 140, h: 122, label: [22, 18, 96, 66], r: 3, brand: [40, 105, 60, 10], thick: { d: 15, t: 4, b: 12 },
    body: (k) => (
      <>
        {part(k, 'M24 0 H116 Q140 0 140 24 V94 Q140 100 134 100 H6 Q0 100 0 94 V24 Q0 0 24 0 Z')}
        {inner(14, 70, 112, 26)}
        {part(k, 'M10 96 H130 Q134 96 134 100 V116 Q134 122 128 122 H12 Q6 122 6 116 V100 Q6 96 10 96 Z', 'gc-plate', 'bd', 1.1)}
        {sheen('M24 1 H116 Q138 1 139.4 20 H0.6 Q2 1 24 1 Z', k)}
        {ridges(3, 26, 114, 6, 12)}
        {recess(k, [22, 18, 96, 66], 6, 6)}
        {[[7, 54], [133, 54]].map(([cx, cy]) => <g key={cx}><ellipse cx={cx} cy={cy} rx="3.6" ry="9" fill="rgb(0 0 0 / .45)" /><ellipse cx={cx} cy={cy + 0.8} rx="3.6" ry="9" fill="none" stroke="rgb(255 255 255 / .3)" strokeWidth=".8" /></g>)}
        {fingers(k, 18, 28, 112, 117, 5)}
      </>
    ),
  },
  // 64-bit: wide and short, its top edge rounded into a ridged lip, one large label, a recessed foot.
  'cart-64': {
    w: 134, h: 110, label: [10, 26, 114, 68], r: 3, brand: [41, 7, 52, 10], thick: { d: 16, t: 4, b: 8 },
    body: (k) => (
      <>
        {part(k, 'M16 2 H118 Q134 2 134 18 V96 Q134 108 122 108 H12 Q0 108 0 96 V18 Q0 2 16 2 Z')}
        {inner(14, 80, 106, 24)}
        {part(k, 'M12 4 H122 Q130 4 131 18 H3 Q4 4 12 4 Z', 'gc-plate', 'bd', 1)}
        {sheen('M16 3 H118 Q132 3 133.4 14 H0.6 Q2 3 16 3 Z', k)}
        {ribs(5, 12, 32, 7, 15)}{ribs(5, 102, 122, 7, 15)}
        {recess(k, [10, 26, 114, 68], 5, 6)}
        <path d="M24 100 H110" stroke="rgb(0 0 0 / .45)" strokeWidth="2" strokeLinecap="round" />
        <path d="M24 101.6 H110" stroke="rgb(255 255 255 / .25)" strokeWidth="1" strokeLinecap="round" />
        {fingers(k, 16, 30, 104, 105, 3)}
      </>
    ),
  },
  // Black 16-bit: a slanted lip over the label, ribbed rails down the sides, a ridged foot.
  'cart-mega': {
    w: 124, h: 140, label: [12, 30, 100, 72], r: 2, brand: [28, 8, 68, 11], thick: { d: 15, t: 3, b: 8 },
    body: (k) => (
      <>
        {part(k, 'M0 24 Q0 6 16 4 H108 Q124 6 124 24 V134 Q124 140 118 140 H6 Q0 140 0 134 Z')}
        {inner(12, 108, 100, 24)}
        {part(k, 'M12 6 H112 L121 24 H3 Z', 'gc-plate', 'bd', 1)}
        {recess(k, [12, 30, 100, 72], 5, 3)}
        {ribs(4, 3, 7, 40, 94)}{ribs(4, 117, 121, 40, 94)}
        {ridges(6, 14, 110, 110, 130)}
        <path d="M6 24 H118" stroke="rgb(255 255 255 / .2)" strokeWidth=".8" />
        <rect x="22" y="134" width="80" height="6" rx="1.5" fill="#050507" />
        {fingers(k, 16, 26, 98, 135, 5)}
      </>
    ),
  },
  // A handheld cartridge: its top-right corner cut away, ridges along the top, an arrow at its foot.
  'cart-pocket': {
    w: 100, h: 118, label: [11, 27, 78, 62], r: 2, brand: [10, 4, 54, 10], thick: { d: 13, t: 2, b: 8 },
    body: (k) => (
      <>
        {part(k, 'M6 0 H78 L100 22 V110 Q100 118 92 118 H8 Q0 118 0 110 V6 Q0 0 6 0 Z')}
        {inner(10, 90, 80, 22)}
        {sheen('M6 1 H77 L98 20 H0.6 V6 Q1 1 6 1 Z', k)}
        {ridges(3, 68, 94, 8, 14)}
        {recess(k, [11, 27, 78, 62], 5, 5)}
        <path className="gc-mark" d="M44 99 H56 L50 108 Z" />
        {ridges(2, 14, 38, 100, 105)}{ridges(2, 62, 86, 100, 105)}
        {fingers(k, 12, 20, 80, 113, 4)}
      </>
    ),
  },
  // A wide, short handheld cartridge: sloped shoulders around a raised middle, a flat label, a narrow foot.
  'cart-advance': {
    w: 130, h: 78, label: [10, 16, 110, 46], r: 2, thick: { d: 12, t: 4, b: 6 },
    body: (k) => (
      <>
        {part(k, 'M0 14 Q0 8 6 8 H42 L49 0 H81 L88 8 H124 Q130 8 130 14 V72 Q130 78 124 78 H6 Q0 78 0 72 Z')}
        {inner(10, 60, 110, 14)}
        {sheen('M6 9 H42 L49 1.5 H81 L88 9 H124 Q129 9 129.5 14 V16 H0.5 V14 Q1 9 6 9 Z', k)}
        {ridges(2, 54, 76, 3, 6)}
        {recess(k, [10, 16, 110, 46], 5, 4)}
        {ribs(3, 3, 6, 22, 56)}{ribs(3, 124, 127, 22, 56)}
        {fingers(k, 22, 24, 106, 72, 4)}
      </>
    ),
  },
  // A thin game card with one corner cut and a thumb notch, its contacts along the foot.
  'card-slim': {
    w: 96, h: 112, label: [9, 11, 78, 66], r: 3, brand: [10, 82, 76, 9], thick: { d: 6, t: 2, b: 2 },
    body: (k) => (
      <>
        {part(k, 'M16 0 H88 Q96 0 96 8 V104 Q96 112 88 112 H8 Q0 112 0 104 V16 Z')}
        <path d="M96 22 q-6 5 0 10" fill="none" stroke="rgb(0 0 0 / .4)" strokeWidth="1.4" />
        {sheen('M16 1 H88 Q95 1 95.6 8 V12 H3 Z', k)}
        {recess(k, [9, 11, 78, 66], 4, 4)}
        <rect x="9" y="96" width="78" height="12" rx="2" fill={U(k, 'bd')} />
        {fingers(k, 13, 13, 83, 98, 9)}
      </>
    ),
  },
  // A thin, wide card with its contacts along one edge: no thicker than a credit card.
  'card-hu': {
    w: 140, h: 94, label: [9, 9, 122, 58], r: 3, thick: { d: 5, t: 2, b: 2 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 140, 94, 8)}
        <path d="M0 70 H140" stroke="rgb(0 0 0 / .18)" strokeWidth=".8" />
        {sheen('M8 1 H132 Q139 1 139.5 8 V10 H0.5 V8 Q1 1 8 1 Z', k)}
        {recess(k, [9, 9, 122, 58], 4, 4)}
        {fingers(k, 34, 14, 126, 76, 14)}
      </>
    ),
  },
  // A CD jewel case: the insert under clear plastic, the hinge with its knuckles down the left, the tray's lugs.
  'case-jewel': {
    w: 144, h: 128, label: [16, 4, 124, 120], r: 1, brand: [20, 7, 44, 10], thick: { d: 16, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 144, 128, 4)}
        {prect(k, 0, 0, 14, 128, 3, 'gc-hinge', 'bd', 1)}
        {[8, 52, 96].map((y) => <g key={y}><rect x="2" y={y} width="9" height="22" rx="2.5" fill={U(k, 'b')} stroke="rgb(0 0 0 / .4)" strokeWidth=".6" /><line x1="2.6" x2="10.4" y1={y + 4} y2={y + 4} stroke="rgb(255 255 255 / .35)" strokeWidth=".6" /></g>)}
        <rect x="14" y="0.6" width="2" height="126.8" fill="rgb(0 0 0 / .35)" />
      </>
    ),
    over: (k) => (
      <>
        {clearCover(k, [15, 3, 126, 122])}
        <path d="M141 6 V122" stroke="rgb(255 255 255 / .3)" strokeWidth="1.2" />
        <circle cx="136" cy="64" r="2.2" fill="rgb(0 0 0 / .25)" />
      </>
    ),
  },
  // A DVD keep case: a rounded spine edge, the cover in a clear sleeve with its lip, a thumb notch.
  'case-keep': {
    w: 100, h: 144, label: [10, 6, 86, 132], r: 2, brand: [14, 8, 72, 12], thick: { d: 12, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 100, 144, 6)}
        {prect(k, 0, 0, 9, 144, 5, 'gc-hinge', 'bd', 1)}
        <line x1="9" x2="9" y1="2" y2="142" stroke="rgb(0 0 0 / .45)" strokeWidth="1" />
        <path d="M2.5 10 V134" stroke="rgb(255 255 255 / .3)" strokeWidth="1" />
        <path d="M96 66 q-4 6 0 12" fill="none" stroke="rgb(0 0 0 / .35)" strokeWidth="1.2" />
      </>
    ),
    over: (k) => (
      <>
        {clearCover(k, [9, 4, 88, 136], 2.5)}
        <rect x="9" y="3" width="88" height="3" fill="rgb(255 255 255 / .12)" />
      </>
    ),
  },
  // A smaller, squarer disc case.
  'case-mini': {
    w: 112, h: 134, label: [10, 7, 96, 120], r: 2, brand: [12, 9, 88, 11], thick: { d: 11, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 112, 134, 7)}
        {prect(k, 0, 0, 8, 134, 5, 'gc-hinge', 'bd', 1)}
        <path d="M2.5 12 V122" stroke="rgb(255 255 255 / .3)" strokeWidth="1" />
        <rect x="104" y="60" width="4" height="14" rx="2" fill="rgb(0 0 0 / .4)" />
      </>
    ),
    over: (k) => <>{clearCover(k, [8, 5, 100, 124], 2.5)}</>,
  },
  // A small disc in a shell: rounded shoulders, a notched corner, a label above a window onto the disc.
  'umd-bare': {
    w: 100, h: 114, label: [11, 11, 78, 54], r: 3, brand: [14, 69, 44, 9], thick: { d: 8, t: 4, b: 6 },
    body: (k) => (
      <>
        {part(k, 'M0 30 Q0 0 30 0 H80 L100 20 V104 Q100 114 90 114 H10 Q0 114 0 104 Z')}
        {recess(k, [11, 11, 78, 54], 4, 5)}
        <circle cx="50" cy="91" r="19" fill="#06070a" />
        <circle cx="50" cy="91" r="19" fill="none" stroke={U(k, 'rimr')} strokeWidth="1.2" />
        <circle cx="50" cy="91" r="16" fill="#cfd4de" />
        <circle cx="50" cy="91" r="16" fill={U(k, 'ds')} />
        <circle className="gc-disc-iri" cx="50" cy="91" r="16" />
        <circle cx="50" cy="91" r="5.5" fill="#06070a" />
        <circle cx="50" cy="91" r="9" fill="none" stroke="rgb(255 255 255 / .5)" strokeWidth=".8" />
        <rect x="70" y="106" width="22" height="4" rx="2" fill="rgb(0 0 0 / .4)" />
      </>
    ),
    over: (k) => <path d="M10 91 A40 40 0 0 1 40 70 L30 112 H10 Z" fill={U(k, 'gl')} opacity=".6" />,
  },
  // A 3.5-inch floppy: its metal shutter at the top, the label below, one corner cut, the write-protect window.
  'disk-floppy': {
    w: 120, h: 124, label: [14, 52, 92, 66], r: 2, thick: { d: 4, t: 2, b: 2 },
    body: (k) => (
      <>
        {part(k, 'M5 0 H112 L120 8 V119 Q120 124 115 124 H5 Q0 124 0 119 V5 Q0 0 5 0 Z')}
        <rect x="26" y="0" width="68" height="42" rx="1.5" fill={U(k, 'mt')} />
        <rect x="26" y="0" width="68" height="42" rx="1.5" fill="none" stroke="rgb(0 0 0 / .4)" strokeWidth=".8" />
        <rect x="64" y="6" width="17" height="30" rx="1.5" fill="#0a0b0f" />
        <rect x="64" y="6" width="17" height="30" rx="1.5" fill="none" stroke="rgb(255 255 255 / .45)" strokeWidth=".7" />
        <path d="M26 34 H64" stroke="rgb(255 255 255 / .5)" strokeWidth="1" />
        {recess(k, [14, 52, 92, 66], 3, 2)}
        <rect x="5" y="108" width="10" height="10" rx="1" fill="#0a0b0f" />
        <rect x="104" y="108" width="10" height="10" rx="1" fill="#0a0b0f" />
        <rect x="108" y="108" width="3" height="10" fill="rgb(255 255 255 / .5)" />
      </>
    ),
  },
  // A cassette: a label above, a window onto two reels, the tape guide below with its head openings.
  'tape-cassette': {
    w: 140, h: 92, label: [11, 7, 118, 52], r: 3, thick: { d: 12, t: 2, b: 2 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 140, 92, 7)}
        <path d="M22 92 L30 70 H110 L118 92 Z" fill={U(k, 'bd')} />
        <path d="M22 92 L30 70 H110 L118 92 Z" fill="none" stroke={U(k, 'rimr')} strokeWidth="1" />
        {[44, 70, 96].map((x) => <circle key={x} cx={x} cy="83" r={x === 70 ? 3.4 : 2.6} fill="#06070a" />)}
        {[[5, 5], [135, 5], [5, 87], [135, 87]].map(([x, y]) => <g key={x + ':' + y}><circle cx={x} cy={y} r="2.4" fill={U(k, 'mt')} /><line x1={x - 1.4} x2={x + 1.4} y1={y} y2={y} stroke="rgb(0 0 0 / .5)" strokeWidth=".6" /></g>)}
        <rect x="60" y="1.6" width="20" height="3" rx="1.5" fill="rgb(0 0 0 / .4)" />
      </>
    ),
    over: (k) => (
      <>
        <rect x="34" y="60" width="72" height="12" rx="6" fill="rgb(6 7 10 / .92)" />
        <rect x="34" y="60" width="72" height="12" rx="6" fill="none" stroke="rgb(255 255 255 / .35)" strokeWidth=".7" />
        {[50, 90].map((x) => <g key={x}><circle cx={x} cy="66" r="7.5" fill="#3a2a22" /><circle cx={x} cy="66" r="5.6" fill="#efece2" /><circle cx={x} cy="66" r="2.2" fill="#06070a" />{[0, 60, 120, 180, 240, 300].map((a) => <line key={a} x1={x} y1="66" x2={x + 4.6 * Math.cos((a * Math.PI) / 180)} y2={66 + 4.6 * Math.sin((a * Math.PI) / 180)} stroke="#06070a" strokeWidth=".9" />)}</g>)}
        {glare('M34 60 H70 L62 72 H34 Z', k)}
      </>
    ),
  },
  // An arcade cabinet's top: a lit marquee in a metal frame over a dark bezel and its glass screen.
  'arcade-marquee': {
    w: 144, h: 124, label: [12, 10, 120, 42], r: 1, thick: { d: 14, t: 0, b: 0 },
    body: (k) => (
      <>
        {part(k, 'M0 6 Q0 0 6 0 H138 Q144 0 144 6 V124 H0 Z')}
        <rect x="8" y="6" width="128" height="50" rx="2" fill="#07080c" />
        <rect x="8" y="6" width="128" height="50" rx="2" fill="none" stroke={U(k, 'mt')} strokeWidth="2.4" />
        <rect x="6" y="60" width="132" height="62" rx="3" fill={U(k, 'bd')} />
        <rect x="14" y="66" width="116" height="52" rx="8" fill="#050609" />
        <rect x="14" y="66" width="116" height="52" rx="8" fill="none" stroke={U(k, 'rimr')} strokeWidth="1.4" />
        <rect x="20" y="72" width="104" height="40" rx="6" fill="#0a1626" />
        {ridges(8, 22, 122, 75, 109)}
        <path d="M20 72 H80 L62 112 H20 Z" fill="rgb(255 255 255 / .08)" />
        <path d="M6 58 H138" stroke="rgb(240 190 90 / .7)" strokeWidth="1.6" />
      </>
    ),
    over: (k) => (
      <>
        <rect className="gc-lit" x="12" y="10" width="120" height="42" rx="1" />
        {glare('M12 10 H74 L58 52 H12 Z', k)}
      </>
    ),
  },
  // A plain box seen from above and the side (the same language as the cardboard boxes).
  'generic-box': box(96, 132, 12, 7),
  'box-cart': box(98, 136, 14, 8),
  'box-cart-wide': box(136, 98, 14, 8),
  'box-small': box(92, 108, 10, 7),
  'box-big': box(112, 142, 30, 16),
  'box-tall': box(84, 150, 16, 8),
  'box-flip': box(100, 126, 16, 14, { flip: true }),
  // A hinged plastic clamshell: a clear lid over the cover, ribs along its top and foot, a latch.
  'case-clamshell': {
    w: 108, h: 146, label: [12, 10, 90, 126], r: 2, thick: { d: 14, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 108, 146, 6)}
        {prect(k, 0, 0, 9, 146, 4, 'gc-hinge', 'bd', 1)}
        {[10, 66, 118].map((y) => <rect key={y} x="1.5" y={y} width="6" height="18" rx="2" fill={U(k, 'b')} stroke="rgb(0 0 0 / .4)" strokeWidth=".6" />)}
        {ridges(2, 18, 90, 3, 6)}{ridges(2, 18, 90, 138, 141)}
        <rect x="102" y="64" width="4" height="18" rx="2" fill="rgb(0 0 0 / .5)" />
      </>
    ),
    over: (k) => <>{clearCover(k, [10, 8, 92, 130])}</>,
  },
  // A small case for a game card: a clear lid, a card-shaped well behind.
  'case-card': {
    w: 104, h: 128, label: [8, 6, 90, 116], r: 2, thick: { d: 10, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 104, 128, 6)}
        {prect(k, 0, 0, 7, 128, 4, 'gc-hinge', 'bd', 1)}
        <path d="M100 4 V30" stroke="rgb(255 255 255 / .3)" strokeWidth="1" />
      </>
    ),
    over: (k) => <>{clearCover(k, [7, 4, 92, 120])}</>,
  },
  // A shorter, slimmer disc case with a header band along its top.
  'case-blu': {
    w: 102, h: 130, label: [8, 19, 90, 104], r: 2, brand: [12, 5, 78, 10], thick: { d: 10, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 102, 130, 6)}
        {prect(k, 0, 0, 7, 130, 4, 'gc-hinge', 'bd', 1)}
        {prect(k, 8, 3.5, 88, 14, 2, 'gc-band', 'bd', 1)}
        <path d="M10 5 H94" stroke="rgb(255 255 255 / .35)" strokeWidth=".8" />
      </>
    ),
    over: (k) => <>{clearCover(k, [7, 3, 92, 122])}</>,
  },
  // The disc alone: the art printed on it, the clear stacking ring, the mirror band and the hole.
  'disc-bare': disc(124, 19, 7),
  'disc-mini': disc(112, 24, 9),
  // A paper sleeve with the disc half out of it.
  'disc-sleeve': {
    w: 120, h: 124, label: [10, 2, 100, 100], round: true,
    body: () => <circle cx="60" cy="52" r="50" fill="#cdd2dc" />,
    over: (k) => (
      <>
        <circle cx="60" cy="52" r="49.4" fill="none" stroke="rgb(0 0 0 / .35)" strokeWidth=".7" />
        <circle className="gc-disc-ring" cx="60" cy="52" r="19" />
        <circle className="gc-disc-hub" cx="60" cy="52" r="12" />
        <circle className="gc-hole" cx="60" cy="52" r="6.5" />
        <circle cx="60" cy="52" r="50" fill={U(k, 'ds')} />
        <path className="gc-body" d="M2 118 V70 Q2 64 8 64 H36 Q60 76 84 64 H112 Q118 64 118 70 V118 Q118 124 112 124 H8 Q2 124 2 118 Z" fill={U(k, 'ct')} />
        <path d="M2 118 V70 Q2 64 8 64 H36 Q60 76 84 64 H112 Q118 64 118 70 V118 Q118 124 112 124 H8 Q2 124 2 118 Z" fill="none" stroke="rgb(0 0 0 / .3)" strokeWidth=".9" />
        <path d="M10 76 H30 M90 76 H110" stroke="rgb(0 0 0 / .12)" strokeWidth="1" />
        {glare('M8 64 H44 L20 124 H8 Z', k)}
      </>
    ),
  },
  // The tall, narrow case a small disc shell came in.
  'case-umd': {
    w: 86, h: 142, label: [8, 7, 72, 128], r: 2, thick: { d: 9, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 86, 142, 6)}
        {prect(k, 0, 0, 6, 142, 4, 'gc-hinge', 'bd', 1)}
        <rect x="8" y="3" width="70" height="3" rx="1.5" fill="rgb(255 255 255 / .2)" />
      </>
    ),
    over: (k) => <>{clearCover(k, [6, 5, 76, 132])}</>,
  },
  // A 5.25-inch floppy: a black jacket with its hub hole, the read slot, the write-protect notch and index hole.
  'disk-525': {
    w: 124, h: 124, label: [12, 8, 100, 44], r: 1, thick: { d: 3, t: 1, b: 1 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 124, 124, 3)}
        <path d="M0 40 H124" stroke="rgb(255 255 255 / .06)" strokeWidth="14" />
        <circle cx="62" cy="80" r="22" fill="none" stroke="rgb(255 255 255 / .14)" strokeWidth="5" />
        <circle cx="62" cy="80" r="14" fill="#06070a" />
        <circle cx="62" cy="80" r="14" fill="none" stroke={U(k, 'rimr')} strokeWidth="1.2" />
        <circle cx="62" cy="80" r="5" fill="#d9dce4" />
        <circle cx="62" cy="80" r="14" fill="none" stroke="rgb(255 255 255 / .35)" strokeWidth=".7" />
        <rect x="55" y="94" width="14" height="24" rx="7" fill="#06070a" />
        <rect x="55" y="94" width="14" height="24" rx="7" fill="none" stroke="rgb(255 255 255 / .35)" strokeWidth=".7" />
        <circle cx="80" cy="64" r="2.6" fill="#06070a" />
        <rect x="116" y="68" width="8" height="12" fill="#12131a" />
        {recess(k, [12, 8, 100, 44], 3, 1)}
      </>
    ),
  },
  // A cassette in its clear box: the insert behind, a hinge down the left.
  'case-tape': {
    w: 144, h: 94, label: [10, 6, 128, 80], r: 2, thick: { d: 12, t: 0, b: 0 },
    body: (k) => (
      <>
        {prect(k, 0, 0, 144, 94, 5)}
        {prect(k, 0, 0, 7, 94, 3, 'gc-hinge', 'bd', 1)}
        {[8, 60].map((y) => <rect key={y} x="1.5" y={y} width="4" height="22" rx="1.5" fill={U(k, 'b')} stroke="rgb(0 0 0 / .4)" strokeWidth=".5" />)}
      </>
    ),
    over: (k) => (
      <>
        {clearCover(k, [8, 4, 134, 86])}
        <rect x="8" y="88" width="134" height="3" fill="rgb(255 255 255 / .1)" />
      </>
    ),
  },
  // An arcade board: green with traces, chips and their legs, capacitors, DIP switches, a gold edge connector, a sticker label.
  'arcade-pcb': {
    w: 144, h: 112, label: [38, 14, 68, 46], r: 1, thick: { d: 6, t: 2, b: 2 },
    body: (k) => (
      <>
        <rect x="0" y="0" width="144" height="102" rx="4" fill="#1c5a34" />
        <rect x="0" y="0" width="144" height="102" rx="4" fill={U(k, 'sh')} opacity=".22" />
        <rect x="0" y="0" width="144" height="102" rx="4" fill="none" stroke={U(k, 'rim')} strokeWidth="1.2" />
        <g fill="none" stroke="rgb(190 240 190 / .35)" strokeWidth="1">
          <path d="M8 66 H32 V84 H56 M60 66 H84 V90 H112 M118 66 V80 H136 M8 94 H44 M74 94 H100 M12 8 V40 H30 M118 8 V30 H134" />
          <path d="M112 16 H134 V50 M8 46 H30 V58" />
        </g>
        {[[10, 70, 28, 18], [44, 70, 28, 18], [78, 70, 28, 18], [112, 70, 22, 18], [8, 12, 24, 44], [114, 12, 22, 44]].map(([x, y, w, h]) => (
          <g key={x + ':' + y}>
            <rect x={x} y={y} width={w} height={h} rx="1.5" fill="#15161b" />
            <rect x={x} y={y} width={w} height={h} rx="1.5" fill="none" stroke="rgb(255 255 255 / .22)" strokeWidth=".7" />
            {Array.from({ length: Math.floor((h > w ? h : w) / 4.5) }, (_, i) => h > w
              ? <g key={i}><line x1={x - 2} x2={x} y1={y + 3 + i * 4.5} y2={y + 3 + i * 4.5} stroke="#c8ccd6" strokeWidth="1" /><line x1={x + w} x2={x + w + 2} y1={y + 3 + i * 4.5} y2={y + 3 + i * 4.5} stroke="#c8ccd6" strokeWidth="1" /></g>
              : <g key={i}><line y1={y - 2} y2={y} x1={x + 3 + i * 4.5} x2={x + 3 + i * 4.5} stroke="#c8ccd6" strokeWidth="1" /><line y1={y + h} y2={y + h + 2} x1={x + 3 + i * 4.5} x2={x + 3 + i * 4.5} stroke="#c8ccd6" strokeWidth="1" /></g>)}
            <circle cx={x + 4} cy={y + 4} r="1.3" fill="rgb(255 255 255 / .35)" />
          </g>
        ))}
        {[[38, 8], [40, 62], [108, 62]].map(([x, y]) => <g key={x + ':' + y}><circle cx={x} cy={y} r="3.4" fill="#2d56a8" /><circle cx={x} cy={y} r="3.4" fill="none" stroke="rgb(255 255 255 / .4)" strokeWidth=".6" /></g>)}
        <rect x="112" y="6" width="26" height="8" rx="1" fill="#d6c24a" /><line x1="119" x2="119" y1="6" y2="14" stroke="#7a6a1a" /><line x1="126" x2="126" y1="6" y2="14" stroke="#7a6a1a" /><line x1="133" x2="133" y1="6" y2="14" stroke="#7a6a1a" />
        <rect x="4" y="102" width="136" height="10" fill="#10351f" />
        {fingers(k, 36, 8, 136, 102, 10)}
      </>
    ),
  },
  // A whole upright arcade cabinet; the game's art is on its screen.
  'arcade-cabinet': {
    w: 112, h: 156, label: [18, 40, 76, 58], r: 5, thick: { d: 16, t: 0, b: 0 },
    body: (k) => (
      <>
        {part(k, 'M10 0 H102 Q112 0 112 10 V146 Q112 156 102 156 H10 Q0 156 0 146 V10 Q0 0 10 0 Z')}
        <rect x="8" y="5" width="96" height="27" rx="2" fill="#07080c" />
        <rect x="8" y="5" width="96" height="27" rx="2" fill="none" stroke={U(k, 'mt')} strokeWidth="2" />
        <rect x="11" y="8" width="90" height="21" rx="1" className="gc-lit-fill" />
        <rect x="8" y="35" width="96" height="68" rx="8" fill="#050609" />
        <rect x="8" y="35" width="96" height="68" rx="8" fill="none" stroke={U(k, 'rimr')} strokeWidth="1.4" />
        <path d="M0 108 H112 L106 126 H6 Z" fill={U(k, 'bd')} />
        <path d="M0 108 H112" stroke="rgb(255 255 255 / .3)" strokeWidth="1" />
        <circle cx="30" cy="116" r="4" fill="#15161b" /><rect x="29" y="106" width="2" height="9" fill="#9aa0ad" /><circle cx="30" cy="106" r="4.4" fill="#c63e45" />
        {[56, 68, 80, 92].map((x, i) => <circle key={x} cx={x} cy={i % 2 ? 118 : 114} r="4.2" fill={['#e0b43a', '#3b79d6', '#c63e45', '#3fae6a'][i]} stroke="rgb(0 0 0 / .45)" strokeWidth=".7" />)}
        <rect x="14" y="128" width="84" height="22" rx="2" fill={U(k, 'bd')} />
        <rect x="46" y="134" width="20" height="10" rx="1" fill="#06070a" /><rect x="50" y="137" width="12" height="2.4" fill="#d9a53a" />
        <path d="M2 10 V146" stroke="rgb(240 190 90 / .55)" strokeWidth="1.4" />
        <path d="M110 10 V146" stroke="rgb(240 190 90 / .55)" strokeWidth="1.4" />
      </>
    ),
    over: (k) => (
      <>
        {glare('M18 40 H66 L48 98 H18 Z', k)}
        <rect x="18" y="40" width="76" height="58" rx="5" fill="none" stroke="rgb(0 0 0 / .45)" strokeWidth="1.2" />
      </>
    ),
  },
  // A download: a glossy rounded tile, the art filling it, a small badge with an arrow in its corner.
  'digital-tile': {
    w: 124, h: 124, label: [5, 5, 114, 114], r: 24,
    body: (k) => <>{prect(k, 0, 0, 124, 124, 28, 'gc-body', 'b', 1.4)}</>,
    over: (k) => (
      <>
        <rect x="5" y="5" width="114" height="114" rx="24" fill="none" stroke="rgb(0 0 0 / .35)" strokeWidth=".8" />
        <path d="M5 40 Q5 5 40 5 H84 Q119 5 119 40 Q62 56 5 40 Z" fill={U(k, 'sh')} opacity=".22" />
        <circle className="gc-badge" cx="100" cy="100" r="15" />
        <path className="gc-badge-mark" d="M100 90 V106 M93 100 L100 107 L107 100" />
      </>
    ),
  },
  // A tall store-style poster: the art on top, a footer with a download arrow and a progress bar.
  'digital-poster': {
    w: 100, h: 144, label: [4, 4, 92, 108], r: 8,
    body: (k) => (
      <>
        {prect(k, 0, 0, 100, 144, 11)}
        <rect x="4" y="116" width="92" height="24" rx="6" fill="#0b0e1a" />
        <circle cx="18" cy="128" r="8" fill="rgb(255 255 255 / .1)" />
        <path d="M18 122.5 V132 M13.6 128.4 L18 132.6 L22.4 128.4" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        <rect x="32" y="125" width="56" height="6" rx="3" fill="rgb(255 255 255 / .16)" />
        <rect x="32" y="125" width="38" height="6" rx="3" className="gc-prog" />
      </>
    ),
    over: (k) => (
      <>
        <rect x="4" y="4" width="92" height="108" rx="8" fill="none" stroke="rgb(0 0 0 / .3)" strokeWidth=".8" />
        {glare('M4 4 H56 L22 112 H4 Z', k)}
      </>
    ),
  },
  // A wide hero banner with a play button and a progress bar along the foot.
  'digital-banner': {
    w: 168, h: 98, label: [4, 4, 160, 90], r: 8,
    body: (k) => <>{prect(k, 0, 0, 168, 98, 11)}</>,
    over: (k) => (
      <>
        <rect x="4" y="4" width="160" height="90" rx="8" fill="none" stroke="rgb(0 0 0 / .3)" strokeWidth=".8" />
        <path d="M4 66 H164 V86 Q164 94 156 94 H12 Q4 94 4 86 Z" fill="rgb(6 8 18 / .62)" />
        <circle cx="22" cy="80" r="8" fill="rgb(255 255 255 / .2)" /><path d="M19.5 75.6 L26.8 80 L19.5 84.4 Z" fill="#fff" />
        <rect x="38" y="77.6" width="104" height="4.8" rx="2.4" fill="rgb(255 255 255 / .22)" /><rect x="38" y="77.6" width="66" height="4.8" rx="2.4" className="gc-prog" />
        {glare('M4 4 H92 L58 94 H4 Z', k)}
      </>
    ),
  },
  // An app window: a title bar with its three buttons, the art as the content, a status bar with a progress bar.
  'digital-window': {
    w: 156, h: 118, label: [4, 20, 148, 76], r: 1,
    body: (k) => (
      <>
        {prect(k, 0, 0, 156, 118, 9)}
        <path d="M9 0 H147 Q156 0 156 9 V20 H0 V9 Q0 0 9 0 Z" fill={U(k, 'bd')} />
        {[10, 20, 30].map((x, i) => <circle key={x} cx={x} cy="10" r="3.2" fill={['#e0605a', '#e0b43a', '#4cb86a'][i]} />)}
        <rect x="52" y="6" width="52" height="8" rx="4" fill="rgb(255 255 255 / .12)" />
        <path d="M0 96 H156 V109 Q156 118 147 118 H9 Q0 118 0 109 Z" fill={U(k, 'bd')} />
        <rect x="10" y="104" width="92" height="5" rx="2.5" fill="rgb(255 255 255 / .16)" /><rect x="10" y="104" width="58" height="5" rx="2.5" className="gc-prog" />
        <rect x="112" y="102" width="34" height="9" rx="4.5" className="gc-prog" />
      </>
    ),
    over: (k) => <>{glare('M4 20 H78 L50 96 H4 Z', k)}</>,
  },
}
