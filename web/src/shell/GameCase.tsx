// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// A game drawn in a cartridge, disc case, tape or cabinet marquee like the ones its console used (gamePictures.ts CASE_STYLES), and box
// art fitted into a tile over a blurred copy of itself. Every drawing is WittyPlay's own: plain SVG shapes that evoke an era, with no
// product's outline traced, no maker's name and no logo. A real logo appears only when an administrator turned brand logos on
// (brandLogos.ts); it sits on a small plate where the case has room for one. The drawings are vectors, so they stay crisp at any size,
// and they fit their tile whatever its shape (gameCase CSS in playChrome.css, "Game pictures").
import { useEffect, useId, useState, type CSSProperties, type ReactNode } from 'react'
import { styleDef, type CaseStyle, type LabelPick } from './gamePictures'
import { Defs, SHAPES, type Box, type Shape } from './gameCaseShapes'
import './gameCase.css'

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
  const k = 'gc' + useId().replace(/:/g, '') + '-'
  const vars = { '--gc-ar': `${s.w} / ${s.h}`, '--gc-ratio': s.w / s.h, ...(tint ? { '--gc-tint': tint } : {}) } as CSSProperties
  return (
    <span className="gcase-fit">
      <span className={'gcase gcase-' + def.draw + (def.angled ? ' angled' : '') + (def.tone ? ' tone-' + def.tone : '') + (light ? ' light' : '')} style={vars} data-case={style}>
        <svg className="gcase-body" viewBox={`0 0 ${s.w} ${s.h}`} aria-hidden="true" focusable="false"><Defs k={k} />{s.body(k)}</svg>
        <span className="gcase-label" style={{ ...place(s.label, s), borderRadius: s.round ? '50%' : s.r ? pct(s.r, s.w) : undefined }}><Label art={art} logo={logo} title={title} label={label} /></span>
        {s.over && <svg className="gcase-over" viewBox={`0 0 ${s.w} ${s.h}`} aria-hidden="true" focusable="false">{s.over(k)}</svg>}
        {def.angled && s.thick && <span className="gcase-edge" style={{ left: '100%', top: pct(s.thick.t || 0, s.h), bottom: pct(s.thick.b || 0, s.h), width: pct(s.thick.d, s.w) }} />}
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
