// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// What a look's layout is, as a small wireframe: where its menu is and how it is drawn, how Home is arranged and what its
// cards are like. The server sends it for each experience and form factor as `wire` (GET /sdui/experiences, docs/SDUI.md
// "Experiences"); this turns one entry into what the thumbnail draws (layoutSpecs.test.mjs pins it to the conformance table).

export type Nav = 'sidebar' | 'rail' | 'railDrawer' | 'top' | 'floating' | 'cable' | 'bevel' | 'tele' | 'crossbar' | 'bottom' | 'none'
export type Home = 'billboard' | 'banner' | 'carousel' | 'list' | 'widgets' | 'rows' | 'tiles'
export type Cards = 'poster' | 'plate' | 'landscape' | 'big' | 'block' | 'list' | 'logo'
export type Spec = { nav: Nav; home: Home; cards: Cards; dock?: boolean }

// One entry of `wire` for a form factor.
export type Wire = { nav: 'top' | 'side' | 'rail' | 'bottom' | 'none'; open?: boolean; style: string; home: string; cards: string; dock?: boolean; brand?: string; finish?: string }
export type WireMap = Partial<Record<'tv' | 'phone' | 'tablet' | 'desktop' | 'web' | 'headset', Wire>>

const HOME: Record<string, Home> = { hero: 'billboard', banner: 'banner', carousel: 'carousel', cross: 'rows', tiles: 'tiles', widgets: 'widgets', list: 'list' }

// specFromWire: the wireframe for one entry. The nav's place and its style together say how it is drawn: a rail or side menu
// is a column; a top bar is drawn in its style (a gloss pill floats, cable and bevel are bars, tele has coloured keys).
export function specFromWire(w: Wire): Spec {
  let nav: Nav
  if (w.nav === 'side') nav = 'sidebar'
  else if (w.nav === 'rail') nav = w.open ? 'railDrawer' : 'rail'
  else if (w.nav === 'bottom') nav = 'bottom'
  else if (w.nav === 'none') nav = w.style === 'crossbar' || w.home === 'cross' ? 'crossbar' : 'none'
  else nav = ({ gloss: 'floating', cable: 'cable', bevel: 'bevel', tele: 'tele', crossbar: 'crossbar' } as Record<string, Nav>)[w.style] || 'top'
  const cards = (['poster', 'landscape', 'plate', 'block', 'logo'].includes(w.cards) ? w.cards : 'poster') as Cards
  return { nav, home: HOME[w.home] || 'billboard', cards: w.home === 'tiles' ? 'big' : cards, dock: !!w.dock }
}

// specFor: the entry for this form factor (the web's own, else the desktop's), else a plain top-tabs layout.
export function specFor(wire: WireMap | undefined, formFactor: keyof WireMap = 'desktop'): Spec {
  const w = wire?.[formFactor] || wire?.desktop || wire?.web
  return w ? specFromWire(w) : { nav: 'top', home: 'billboard', cards: 'poster' }
}
