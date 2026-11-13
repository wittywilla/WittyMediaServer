// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A game drawn in a cartridge, disc case, tape or cabinet marquee like the ones its console used (gamePictures.ts CASE_STYLES), and box
// art fitted into a tile over a blurred copy of itself. Every drawing is WittyPlay's own: plain SVG shapes that evoke an era, with no
// product's outline traced, no maker's name and no logo. A real logo appears only when an administrator turned brand logos on
// (brandLogos.ts); it sits on a small plate where the case has room for one. The drawings are vectors, so they stay crisp at any size,
// and they fit their tile whatever its shape (gameCase CSS in playChrome.css, "Game pictures").
import { useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import { styleDef, type CaseStyle, type LabelPick } from './gamePictures'
import './gameCase.css'

type Box = [x: number, y: number, w: number, h: number]
type Shape = { w: number; h: number; label: Box; r?: number; round?: boolean; brand?: Box; body: ReactNode; over?: ReactNode }

// Grooves: n lines between two points, for ridges and grips.
const rows = (n: number, x1: number, x2: number, y1: number, y2: number, cls = 'gc-groove') =>
  Array.from({ length: n }, (_, i) => { const y = y1 + ((y2 - y1) * i) / Math.max(1, n - 1); return <line key={'r' + i} className={cls} x1={x1} x2={x2} y1={y} y2={y} /> })
const cols = (n: number, x1: number, x2: number, y1: number, y2: number, cls = 'gc-groove') =>
  Array.from({ length: n }, (_, i) => { const x = x1 + ((x2 - x1) * i) / Math.max(1, n - 1); return <line key={'c' + i} className={cls} x1={x} x2={x} y1={y1} y2={y2} /> })
// The recess a label sits in: a little larger than the label, darker.
const well = ([x, y, w, h]: Box, pad: number, r: number) => <rect className="gc-well" x={x - pad} y={y - pad} width={w + pad * 2} height={h + pad * 2} rx={r} />
// A soft highlight down the left and across the top, as light from above would make.
const sheen = (d: string) => <path className="gc-sheen" d={d} />

// One drawing per shape, by its id: the styles (gamePictures.ts CASE_STYLES) say which drawing each uses, and whether at an angle.
const SHAPES: Record<string, Shape> = {
  // 8-bit: a grey front-loader, label window above a band of ridges.
  'cart-nes': {
    w: 120, h: 132, label: [14, 14, 92, 70], r: 2,
    body: <>
      <rect className="gc-body" x="0" y="0" width="120" height="132" rx="5" />
      {sheen('M5 1 H115 Q119 1 119 5 V8 H1 V5 Q1 1 5 1 Z')}
      {well([14, 14, 92, 70], 4, 4)}
      <rect className="gc-band" x="6" y="94" width="108" height="34" rx="3" />
      {rows(7, 10, 110, 99, 123)}
      {cols(2, 3, 117, 20, 86, 'gc-edge')}
    </>,
  },
  // First generation: short and dark, a wide label over vertical grips.
  'cart-atari': {
    w: 120, h: 100, label: [12, 10, 96, 54], r: 2,
    body: <>
      <path className="gc-body" d="M6 0 H114 Q120 0 120 6 V82 L112 100 H8 L0 82 V6 Q0 0 6 0 Z" />
      {sheen('M6 1 H114 Q119 1 119 6 V8 H1 V6 Q1 1 6 1 Z')}
      {well([12, 10, 96, 54], 4, 3)}
      {cols(13, 18, 102, 74, 94)}
    </>,
  },
  // 16-bit: rounded at the top with grips cut into its shoulders.
  'cart-super': {
    w: 140, h: 120, label: [24, 22, 92, 66], r: 3, brand: [44, 100, 52, 10],
    body: <>
      <path className="gc-body" d="M20 0 H120 Q140 0 140 20 V116 Q140 120 136 120 H4 Q0 120 0 116 V20 Q0 0 20 0 Z" />
      <path className="gc-grip" d="M0 34 Q10 40 0 46 Z M140 34 Q130 40 140 46 Z" />
      {sheen('M20 1 H120 Q138 1 139 14 H1 Q2 1 20 1 Z')}
      {well([24, 22, 92, 66], 5, 6)}
      {rows(3, 16, 124, 7, 13)}
    </>,
  },
  // 64-bit: tall and dark, its top sloping in and ridged.
  'cart-64': {
    w: 120, h: 128, label: [16, 32, 88, 82], r: 3, brand: [32, 5, 56, 11],
    body: <>
      <path className="gc-body" d="M14 0 H106 L120 22 V122 Q120 128 114 128 H6 Q0 128 0 122 V22 Z" />
      {rows(4, 18, 102, 4, 18)}
      {sheen('M0 22 H120 V26 H0 Z')}
      {well([16, 32, 88, 82], 5, 5)}
    </>,
  },
  // Black 16-bit: a slanted top over the label, ridges at its foot.
  'cart-mega': {
    w: 120, h: 132, label: [12, 24, 96, 72], r: 2, brand: [26, 6, 68, 11],
    body: <>
      <path className="gc-body" d="M0 20 Q0 4 16 2 L104 2 Q120 4 120 20 V128 Q120 132 116 132 H4 Q0 132 0 128 Z" />
      <path className="gc-band" d="M14 4 H106 L114 18 H6 Z" />
      {well([12, 24, 96, 72], 4, 3)}
      {rows(6, 12, 108, 106, 126)}
      {sheen('M4 20 Q4 6 16 5 V128 H4 Z')}
    </>,
  },
  // A handheld cartridge: small, its top corner cut away, ridges along the top and an arrow at its foot.
  'cart-pocket': {
    w: 100, h: 112, label: [12, 24, 76, 62], r: 2, brand: [12, 4, 58, 11],
    body: <>
      <path className="gc-body" d="M5 0 H84 L100 16 V107 Q100 112 95 112 H5 Q0 112 0 107 V5 Q0 0 5 0 Z" />
      {rows(3, 10, 74, 5, 15)}
      {well([12, 24, 76, 62], 4, 3)}
      <path className="gc-mark" d="M44 96 H56 L50 104 Z" />
    </>,
  },
  // A wide, short handheld cartridge.
  'cart-advance': {
    w: 120, h: 76, label: [10, 14, 100, 52], r: 2,
    body: <>
      <path className="gc-body" d="M0 12 Q0 6 6 6 H40 L46 0 H74 L80 6 H114 Q120 6 120 12 V70 Q120 76 114 76 H6 Q0 76 0 70 Z" />
      {rows(2, 50, 70, 2.5, 5)}
      {well([10, 14, 100, 52], 3, 3)}
    </>,
  },
  // A thin game card with one corner cut.
  'card-slim': {
    w: 100, h: 116, label: [10, 14, 80, 80], r: 3, brand: [14, 100, 72, 10],
    body: <>
      <path className="gc-body" d="M12 0 H95 Q100 0 100 5 V111 Q100 116 95 116 H5 Q0 116 0 111 V12 Z" />
      {sheen('M12 1 H95 Q99 1 99 5 V7 H6 Z')}
      {well([10, 14, 80, 80], 3, 4)}
    </>,
  },
  // A CD jewel case: the insert under clear plastic, the hinge down the left.
  'case-jewel': {
    w: 140, h: 124, label: [16, 4, 120, 116], r: 1, brand: [20, 7, 44, 10],
    body: <>
      <rect className="gc-body" x="0" y="0" width="140" height="124" rx="3" />
      <rect className="gc-hinge" x="0" y="0" width="13" height="124" rx="2" />
      {rows(2, 2, 11, 10, 114, 'gc-hinge-line')}
    </>,
    over: <>
      <path className="gc-glare" d="M16 4 H82 L40 120 H16 Z" />
      <rect className="gc-clear" x="14" y="2" width="124" height="120" rx="2" />
    </>,
  },
  // A tall DVD case: the cover in a clear sleeve, a ridged spine.
  'case-keep': {
    w: 100, h: 142, label: [8, 6, 88, 130], r: 2, brand: [12, 8, 80, 12],
    body: <>
      <rect className="gc-body" x="0" y="0" width="100" height="142" rx="5" />
      {cols(3, 2.5, 5.5, 14, 128, 'gc-hinge-line')}
    </>,
    over: <>
      <path className="gc-glare" d="M8 6 H56 L20 136 H8 Z" />
      <rect className="gc-clear" x="7" y="5" width="90" height="132" rx="2" />
    </>,
  },
  // A smaller, squarer disc case.
  'case-mini': {
    w: 112, h: 132, label: [8, 6, 98, 120], r: 2, brand: [12, 8, 90, 12],
    body: <>
      <rect className="gc-body" x="0" y="0" width="112" height="132" rx="6" />
      {cols(3, 2.5, 5.5, 16, 116, 'gc-hinge-line')}
    </>,
    over: <>
      <path className="gc-glare" d="M8 6 H60 L22 126 H8 Z" />
      <rect className="gc-clear" x="7" y="5" width="100" height="122" rx="2" />
    </>,
  },
  // A small disc in a shell: rounded at the top, its label above a window onto the disc.
  'umd-bare': {
    w: 100, h: 112, label: [12, 10, 76, 54], r: 3, brand: [14, 68, 44, 9],
    body: <>
      <path className="gc-body" d="M0 30 Q0 0 30 0 H70 Q100 0 100 30 V104 Q100 112 92 112 H8 Q0 112 0 104 Z" />
      {well([12, 10, 76, 54], 3, 5)}
      <circle className="gc-window" cx="50" cy="90" r="17" />
      <circle className="gc-disc" cx="50" cy="90" r="14" />
      <circle className="gc-window" cx="50" cy="90" r="4" />
    </>,
  },
  // A 3.5-inch floppy: a metal shutter at the top, the label below, one corner cut.
  'disk-floppy': {
    w: 120, h: 124, label: [14, 50, 92, 68], r: 2,
    body: <>
      <path className="gc-body" d="M4 0 H112 L120 8 V120 Q120 124 116 124 H4 Q0 124 0 120 V4 Q0 0 4 0 Z" />
      <rect className="gc-metal" x="28" y="0" width="64" height="40" rx="1" />
      <rect className="gc-window" x="66" y="6" width="14" height="28" rx="1" />
      <rect className="gc-window" x="6" y="112" width="7" height="7" />
      {well([14, 50, 92, 68], 3, 2)}
    </>,
  },
  // A cassette: the label across it, a window onto two reels, the head opening below.
  'tape-cassette': {
    w: 140, h: 92, label: [10, 7, 120, 56], r: 3,
    body: <>
      <rect className="gc-body" x="0" y="0" width="140" height="92" rx="6" />
      <path className="gc-band" d="M24 92 L32 70 H108 L116 92 Z" />
      {[44, 70, 96].map((x) => <circle key={x} className="gc-window" cx={x} cy="83" r="3" />)}
      {[5, 135].map((x) => [5, 87].map((y) => <circle key={x + ':' + y} className="gc-screw" cx={x} cy={y} r="2" />))}
    </>,
    over: <>
      <rect className="gc-window" x="38" y="27" width="64" height="20" rx="10" />
      {[52, 88].map((x) => <g key={x}><circle className="gc-reel" cx={x} cy="37" r="7.5" /><circle className="gc-window" cx={x} cy="37" r="2.5" /></g>)}
    </>,
  },
  // An arcade cabinet's top: the lit marquee, a dark screen below.
  'arcade-marquee': {
    w: 140, h: 120, label: [10, 10, 120, 46], r: 1,
    body: <>
      <path className="gc-body" d="M0 4 Q0 0 4 0 H136 Q140 0 140 4 V120 H0 Z" />
      <rect className="gc-band" x="6" y="64" width="128" height="56" rx="2" />
      <rect className="gc-screen" x="16" y="72" width="108" height="48" rx="6" />
      {rows(6, 18, 122, 78, 114, 'gc-scan')}
    </>,
    over: <>
      <rect className="gc-lit" x="10" y="10" width="120" height="46" rx="1" />
      <path className="gc-glare" d="M10 10 H70 L54 56 H10 Z" />
    </>,
  },
  // A plain box, its side edge in shadow.
  'generic-box': {
    w: 108, h: 140, label: [4, 4, 94, 132], r: 2,
    body: <>
      <path className="gc-side" d="M98 2 L106 6 V138 L98 140 Z" />
      <rect className="gc-body" x="0" y="0" width="100" height="140" rx="3" />
    </>,
    over: <path className="gc-glare" d="M4 4 H40 L16 136 H4 Z" />,
  },
  // A thin card, wide, with its contacts along one edge.
  'card-hu': {
    w: 140, h: 92, label: [8, 8, 124, 62], r: 3,
    body: <>
      <rect className="gc-body" x="0" y="0" width="140" height="92" rx="6" />
      {cols(9, 30, 110, 76, 86, 'gc-contact')}
    </>,
  },
  'box-cart': box(100, 140, 6),
  'box-cart-wide': box(140, 100, 6),
  'box-small': box(110, 110, 5),
  'box-big': box(112, 140, 10),
  // A hinged plastic clamshell: a clear sleeve over the cover, ribs top and bottom.
  'case-clamshell': {
    w: 104, h: 142, label: [10, 10, 90, 122], r: 2,
    body: <>
      <rect className="gc-body" x="0" y="0" width="104" height="142" rx="4" />
      {rows(2, 12, 92, 4, 7, 'gc-hinge-line')}
      {rows(2, 12, 92, 135, 138, 'gc-hinge-line')}
      <rect className="gc-hinge" x="0" y="0" width="7" height="142" rx="2" />
    </>,
    over: <><path className="gc-glare" d="M10 10 H54 L22 132 H10 Z" /><rect className="gc-clear" x="9" y="9" width="92" height="124" rx="2" /></>,
  },
  // A small case for a game card.
  'case-card': {
    w: 100, h: 124, label: [7, 5, 88, 114], r: 2,
    body: <>
      <rect className="gc-body" x="0" y="0" width="100" height="124" rx="5" />
      {cols(2, 2.5, 4.5, 14, 110, 'gc-hinge-line')}
    </>,
    over: <><path className="gc-glare" d="M7 5 H52 L18 119 H7 Z" /><rect className="gc-clear" x="6" y="4" width="90" height="116" rx="2" /></>,
  },
  // A shorter, slimmer disc case with a band along its top.
  'case-blu': {
    w: 100, h: 126, label: [7, 17, 88, 104], r: 2, brand: [12, 5, 76, 9],
    body: <>
      <rect className="gc-body" x="0" y="0" width="100" height="126" rx="5" />
      <rect className="gc-band" x="7" y="4" width="88" height="11" rx="1" />
    </>,
    over: <><path className="gc-glare" d="M7 17 H52 L18 121 H7 Z" /><rect className="gc-clear" x="6" y="3" width="90" height="119" rx="2" /></>,
  },
  // The disc alone: the art printed on it, a clear ring and the hole in the middle.
  'disc-bare': disc(120, 18, 7),
  'disc-mini': disc(120, 24, 9),
  // The tall, narrow case a small disc shell came in.
  'case-umd': {
    w: 84, h: 140, label: [6, 6, 74, 128], r: 2,
    body: <>
      <rect className="gc-body" x="0" y="0" width="84" height="140" rx="5" />
      {cols(2, 2, 4, 14, 126, 'gc-hinge-line')}
    </>,
    over: <><path className="gc-glare" d="M6 6 H44 L16 134 H6 Z" /><rect className="gc-clear" x="5" y="5" width="76" height="130" rx="2" /></>,
  },
  // A 5.25-inch floppy: a square sleeve, a label across the top, the hub hole and the read slot.
  'disk-525': {
    w: 124, h: 124, label: [12, 8, 100, 44], r: 1,
    body: <>
      <rect className="gc-body" x="0" y="0" width="124" height="124" rx="2" />
      <circle className="gc-window" cx="62" cy="76" r="13" />
      <circle className="gc-metal" cx="62" cy="76" r="7" />
      <rect className="gc-window" x="56" y="94" width="12" height="26" rx="6" />
      <rect className="gc-window" x="10" y="70" width="5" height="5" />
    </>,
  },
  // A cassette in its clear box: the insert behind, a hinge along the foot.
  'case-tape': {
    w: 140, h: 92, label: [6, 6, 128, 78], r: 2,
    body: <rect className="gc-body" x="0" y="0" width="140" height="92" rx="4" />,
    over: <>
      <rect className="gc-clear" x="4" y="4" width="132" height="82" rx="2" />
      <path className="gc-glare" d="M6 6 H70 L46 84 H6 Z" />
      <rect className="gc-hinge" x="0" y="86" width="140" height="6" rx="2" />
    </>,
  },
  // An arcade board: a green board, chips, a label stuck on, its edge connector.
  'arcade-pcb': {
    w: 140, h: 110, label: [34, 12, 72, 48], r: 1,
    body: <>
      <rect className="gc-pcb" x="0" y="0" width="140" height="102" rx="3" />
      {[[10, 70], [40, 70], [70, 70], [100, 70], [10, 14], [112, 14]].map(([x, y]) => <rect key={x + ':' + y} className="gc-chip" x={x} y={y} width={x > 100 || x === 10 && y === 14 ? 18 : 26} height={y === 14 ? 40 : 18} rx="1" />)}
      {cols(24, 14, 126, 102, 110, 'gc-contact')}
    </>,
  },
  // A download: a rounded tile, the art filling it, a small arrow in its corner.
  'digital-tile': {
    w: 120, h: 120, label: [6, 6, 108, 108], r: 18,
    body: <rect className="gc-body" x="0" y="0" width="120" height="120" rx="22" />,
    over: <>
      <circle className="gc-badge" cx="98" cy="98" r="15" />
      <path className="gc-badge-mark" d="M98 88 V104 M91 98 L98 105 L105 98" />
    </>,
  },
}

// A cardboard box: its front the label, its side in shadow, a flap line along the top.
function box(w: number, h: number, depth: number): Shape {
  return {
    w: w + depth, h, label: [3, 3, w - 6, h - 6], r: 1,
    body: <>
      <path className="gc-side" d={`M${w - 2} 2 L${w + depth} ${depth} V${h - depth} L${w - 2} ${h} Z`} />
      <rect className="gc-body" x="0" y="0" width={w} height={h} rx="2" />
    </>,
    over: <><path className="gc-glare" d={`M3 3 H${w * 0.4} L${w * 0.15} ${h - 3} H3 Z`} /><line className="gc-hinge-line" x1="3" x2={w - 3} y1="3.5" y2="3.5" /></>,
  }
}
// A disc of size w: the art over its whole face, a clear ring of radius ring and the hole.
function disc(w: number, ring: number, hole: number): Shape {
  const c = w / 2
  return {
    w, h: w, label: [2, 2, w - 4, w - 4], round: true,
    body: <circle className="gc-disc" cx={c} cy={c} r={c} />,
    over: <>
      <circle className="gc-disc-edge" cx={c} cy={c} r={c - 1} />
      <circle className="gc-disc-ring" cx={c} cy={c} r={ring} />
      <circle className="gc-disc-hub" cx={c} cy={c} r={ring * 0.6} />
      <circle className="gc-hole" cx={c} cy={c} r={hole} />
      <path className="gc-glare" d={`M${c} 2 A${c - 2} ${c - 2} 0 0 1 ${w - 6} ${c * 0.7} L${c} ${c} Z`} />
    </>,
  }
}

// Colours by console, where its cases or cartridges came in one: plain shades, never a maker's mark.
const TINT: Record<string, string> = {
  nes: '#8e9096', a2600: '#1e1b19', a5200: '#1e1b19', a7800: '#26282c', snes: '#b7b6bb', n64: '#3b3c40', jaguar: '#232428',
  md: '#17171a', '32x': '#17171a', sms: '#17171a', neogeo: '#141416', gb: '#c4c2bd', gbc: '#bdbcc2', gg: '#1b1b1e', ngp: '#202024', ws: '#2a2b30', vb: '#2b1013',
  gba: '#4b4c55', nds: '#56575c', '3ds': '#4a4b51', switch: '#3e4046', switch2: '#3e4046', pce: '#e6e3dc', lynx: '#cfcac0',
  ps1: '#222327', saturn: '#222327', segacd: '#222327', dc: '#e9e9ec', ps2: '#141519', xbox: '#16391e', wii: '#e8eaee', wiiu: '#e4e7ec', ps3: '#1c2a49',
  gc: '#1e1d24', psp: '#16171b', amiga: '#2b2d33', dos: '#2b2d33', c64: '#ddd8cc', zx: '#1d1d20', msx: '#ddd8cc', cpc: '#1d1d20', arcade: '#141418',
}
const LIGHT = new Set(['snes', 'gb', 'gbc', 'pce', 'lynx', 'dc', 'wii', 'wiiu', 'c64', 'msx', 'nes'])
export const tintFor = (consoleId: string) => TINT[consoleId]

const pct = (v: number, of: number) => (v / of) * 100 + '%'
const place = (b: Box, s: Shape): CSSProperties => ({ left: pct(b[0], s.w), top: pct(b[1], s.h), width: pct(b[2], s.w), height: pct(b[3], s.h) })

// The art on a label, or in a tile: the whole picture over a blurred, dimmed copy of itself, so a picture of any shape fills the space
// without bars and without anything cut off. alt is empty: the card says the game's name in words.
export function FitArt({ src, className, onFail }: { src: string; className?: string; onFail?: () => void }) {
  return (
    <span className={'gpic-fit' + (className ? ' ' + className : '')}>
      <img className="gpic-fit-back" src={src} alt="" aria-hidden="true" loading="lazy" decoding="async" />
      <img className="gpic-fit-main" src={src} alt="" loading="lazy" decoding="async" onError={onFail} />
    </span>
  )
}

// What goes on a case's label: the box art, the game's logo picture, or its name in type (and its name when there is no picture).
function Label({ art, logo, title, label }: { art: string; logo?: string; title: string; label: LabelPick }) {
  const [failed, setFailed] = useState('')
  const src = label === 'title' ? '' : label === 'logo' && logo && failed !== logo ? logo : art
  useEffect(() => setFailed(''), [art, logo])
  if (!src || failed === src) return title ? <span className="gcase-title"><span>{title}</span></span> : <span className="gcase-blank" />
  if (src === logo && label === 'logo') return <span className="gcase-logo"><img src={src} alt="" loading="lazy" decoding="async" onError={() => setFailed(src)} /></span>
  return <FitArt src={src} onFail={() => setFailed(src)} />
}

// One game in its case. consoleId picks the colour; brand is a downloaded console logo's address, or nothing.
export function GameCase({ style, consoleId, art, logo, title, label, brand }: { style: CaseStyle; consoleId?: string; art: string; logo?: string; title: string; label: LabelPick; brand?: string | null }) {
  const def = styleDef(style) || styleDef('generic-box')!
  const s = SHAPES[def.draw] || SHAPES['generic-box']
  const tint = consoleId ? TINT[consoleId] : undefined
  const light = consoleId ? LIGHT.has(consoleId) : false
  const vars = { '--gc-ar': `${s.w} / ${s.h}`, '--gc-ratio': s.w / s.h, ...(tint ? { '--gc-tint': tint } : {}) } as CSSProperties
  return (
    <span className="gcase-fit">
      <span className={'gcase gcase-' + def.draw + (def.angled ? ' angled' : '') + (def.tone ? ' tone-' + def.tone : '') + (light ? ' light' : '')} style={vars} data-case={style}>
        <svg className="gcase-body" viewBox={`0 0 ${s.w} ${s.h}`} aria-hidden="true" focusable="false">{s.body}</svg>
        <span className="gcase-label" style={{ ...place(s.label, s), borderRadius: s.round ? '50%' : s.r ? pct(s.r, s.w) : undefined }}><Label art={art} logo={logo} title={title} label={label} /></span>
        {s.over && <svg className="gcase-over" viewBox={`0 0 ${s.w} ${s.h}`} aria-hidden="true" focusable="false">{s.over}</svg>}
        {brand && s.brand && <span className="gcase-brand" style={place(s.brand, s)}><img src={brand} alt="" loading="lazy" decoding="async" /></span>}
      </span>
    </span>
  )
}

// A small drawing of a case alone, with a made-up label, for the choices in Settings (no game's art).
export function CaseSample({ style, consoleId, children }: { style: CaseStyle; consoleId?: string; children?: ReactNode }) {
  return (
    <span className="gcase-sample" aria-hidden="true">
      <GameCase style={style} consoleId={consoleId} art="" title="" label="title" />
      {children}
    </span>
  )
}
