// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which buttons WittyPlay Online draws: the keyboard's keys, or an Xbox (XInput), PlayStation (DualSense) or Nintendo pad's. The
// person chooses on the first launch (the first step of the tip) and on the controllers page; it is kept per profile (/me/ui). The
// pads' glyphs are the game pages' own (pages/games/prompts.tsx PadGlyph, the same as WittyPlay's player), so a button looks the
// same before and inside a game. A Nintendo choice also makes the right-hand button confirm (ui/gamepad.ts setPadSwap).
import { useEffect, useId } from 'react'
import { PadGlyph, type PromptButton } from '../pages/games/prompts'
import type { GlyphKey } from '../sdui/render/glyphs'
import { setPadSwap } from '../ui/gamepad'
import { useUserPref } from '../ui/userPrefs'
import { GameTitleSample } from '../pages/games/GameName'
import { GAME_TITLE_CHOICES, type GameTitlePref } from '../pages/games/titles'
import { useGameTitlePref } from '../pages/games/useGameTitles'
import { moveTab, PLAY_TABS, PLAY_TABS_KEY, tabsPref, type PlayTabsPref, isPlayPads, layoutOf, padsFor, PLAY_KEYS, PLAY_LAYOUT_KEY, PLAY_LAYOUTS, PLAY_PADS, PLAY_PADS_KEY, swapFor, type PlayLayout, type PlayPads, openOf, PLAY_OPEN_KEY, PLAY_OPENS, type PlayOpen } from './playPaths'

// By position: A at the bottom, B on the right, X on the left, Y at the top. Nintendo's letters for those places are the pad's own.
const PLACE: Record<GlyphKey, PromptButton> = { a: 'confirm', b: 'back', x: 'west', y: 'north' }

export function PlayGlyph({ pads, g }: { pads: PlayPads; g: GlyphKey }) {
  if (pads === 'keyboard') return <kbd className="gm-glyph key">{PLAY_KEYS[g]}</kbd>
  return <PadGlyph button={PLACE[g]} family={pads} />
}

const connectedIds = () => {
  try { return Array.from(navigator.getGamepads?.() || []).filter((p): p is Gamepad => !!p && p.connected).map((p) => p.id) } catch { return [] }
}

// The choice, or what a connected pad says until there is one; whether it was chosen; and the setter.
export function usePlayPads(): [PlayPads, boolean, (p: PlayPads) => void] {
  const [saved, save] = useUserPref<string>(PLAY_PADS_KEY, '')
  const chosen = isPlayPads(saved)
  const pads = chosen ? saved : padsFor(connectedIds())
  // While WittyPlay shows, a Nintendo choice trades the bottom and right buttons; leaving puts the pads back as they were.
  useEffect(() => { setPadSwap(swapFor(pads)); return () => setPadSwap(null) }, [pads])
  return [pads, chosen, (p) => { void save(p).catch(() => {}) }]
}

// The four choices, each drawn with its own buttons: a radio group of buttons, so a remote's or a pad's arrows reach each one and
// A (or Enter, or a click) chooses it.
export function PadsChooser({ value, onChange, labelledBy }: { value: PlayPads; onChange: (p: PlayPads) => void; labelledBy: string }) {
  return (
    <div className="wplay-pads" role="radiogroup" aria-labelledby={labelledBy} data-nav-group>
      {PLAY_PADS.map((p) => (
        <button key={p.id} type="button" role="radio" aria-checked={p.id === value} className={'wplay-pad' + (p.id === value ? ' on' : '')} onClick={() => onChange(p.id)} data-pads={p.id}>
          <span className="wplay-pad-glyphs" aria-hidden="true">{(['y', 'x', 'b', 'a'] as GlyphKey[]).map((g) => <span key={g} className={'at-' + g}><PlayGlyph pads={p.id} g={g} /></span>)}</span>
          <span className="wplay-pad-text"><b>{p.label}</b><span>{p.note}</span></span>
        </button>
      ))}
    </div>
  )
}

// The controllers page's own part: the same choice, any time after the first.
export function PadsSetting() {
  const [pads, , setPads] = usePlayPads()
  const id = useId()
  return (
    <section className="wplay-pads-setting">
      <h2 id={id}>Buttons shown in WittyPlay</h2>
      <PadsChooser value={pads} onChange={setPads} labelledBy={id} />
    </section>
  )
}

// ---- the layout of WittyPlay's games: Shelf, Grid or List, per profile ----
export function usePlayLayout(): [PlayLayout, (l: PlayLayout) => void] {
  const [saved, save] = useUserPref<string>(PLAY_LAYOUT_KEY, '')
  return [layoutOf(saved), (l) => { void save(l).catch(() => {}) }]
}

// A drawing of each layout, for the choices.
function LayoutIcon({ id }: { id: PlayLayout }) {
  const r = (x: number, y: number, w: number, h: number) => <rect key={x + ':' + y} x={x} y={y} width={w} height={h} rx="1.5" />
  const shapes = id === 'shelf' ? [r(2, 6, 8, 9), r(12, 7, 5, 7), r(19, 7, 4, 7)]
    : id === 'grid' ? [r(2, 3, 5, 5), r(9.5, 3, 5, 5), r(17, 3, 5, 5), r(2, 10, 5, 5), r(9.5, 10, 5, 5), r(17, 10, 5, 5), r(2, 17, 5, 4), r(9.5, 17, 5, 4), r(17, 17, 5, 4)]
    : [r(2, 4, 4, 4), r(8, 5, 14, 2), r(2, 10, 4, 4), r(8, 11, 14, 2), r(2, 16, 4, 4), r(8, 17, 14, 2)]
  return <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true">{shapes}</svg>
}

// The quick switch at the end of the tab row: three buttons, the chosen one pressed.
export function LayoutSwitch() {
  const [layout, setLayout] = usePlayLayout()
  return (
    <div className="wplay-layout-switch" role="group" aria-label="Layout">
      {PLAY_LAYOUTS.map((l) => (
        <button key={l.id} type="button" aria-pressed={layout === l.id} aria-label={l.label} title={l.label} onClick={() => setLayout(l.id)}><LayoutIcon id={l.id} /></button>
      ))}
    </div>
  )
}

// The full choice, with words: WittyPlay's Settings and the first-time tip.
export function LayoutChooser({ value, onChange, labelledBy }: { value: PlayLayout; onChange: (l: PlayLayout) => void; labelledBy: string }) {
  return (
    <div className="wplay-pads" role="radiogroup" aria-labelledby={labelledBy} data-nav-group>
      {PLAY_LAYOUTS.map((l) => (
        <button key={l.id} type="button" role="radio" aria-checked={l.id === value} className={'wplay-pad' + (l.id === value ? ' on' : '')} onClick={() => onChange(l.id)}>
          <span className="wplay-layout-art" aria-hidden="true"><LayoutIcon id={l.id} /></span>
          <span className="wplay-pad-text"><b>{l.label}</b><span>{l.note}</span></span>
        </button>
      ))}
    </div>
  )
}

// What choosing a game does (/me/ui wittyplayOpen): its page first, or straight into the game.
export function usePlayOpen(): [PlayOpen, (o: PlayOpen) => void] {
  const [saved, save] = useUserPref<string>(PLAY_OPEN_KEY, '')
  return [openOf(saved), (o) => { void save(o).catch(() => {}) }]
}

function OpenIcon({ id }: { id: PlayOpen }) {
  return id === 'info'
    ? <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true"><rect x="2" y="4" width="7" height="10" rx="1.5" /><rect x="11" y="5" width="11" height="2" rx="1" /><rect x="11" y="9" width="8" height="2" rx="1" /><rect x="2" y="16" width="9" height="4" rx="2" /></svg>
    : <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true"><path d="M7 4.5v15l12-7.5z" /></svg>
}

export function OpenChooser({ value, onChange, labelledBy }: { value: PlayOpen; onChange: (o: PlayOpen) => void; labelledBy: string }) {
  return (
    <div className="wplay-pads" role="radiogroup" aria-labelledby={labelledBy} data-nav-group>
      {PLAY_OPENS.map((o) => (
        <button key={o.id} type="button" role="radio" aria-checked={o.id === value} className={'wplay-pad' + (o.id === value ? ' on' : '')} onClick={() => onChange(o.id)}>
          <span className="wplay-layout-art" aria-hidden="true"><OpenIcon id={o.id} /></span>
          <span className="wplay-pad-text"><b>{o.label}</b><span>{o.note}</span></span>
        </button>
      ))}
    </div>
  )
}

// How games are named (/me/ui gameTitles): each choice with one game's name as it would show. Also fit for a first-launch step.
export function GameTitlesChooser({ value, onChange, labelledBy }: { value: GameTitlePref; onChange: (p: GameTitlePref) => void; labelledBy: string }) {
  return (
    <div className="wplay-pads" role="radiogroup" aria-labelledby={labelledBy} data-nav-group>
      {GAME_TITLE_CHOICES.map((o) => (
        <button key={o.id} type="button" role="radio" aria-checked={o.id === value} className={'wplay-pad' + (o.id === value ? ' on' : '')} onClick={() => onChange(o.id)}>
          <span className="wplay-pad-text"><b>{o.label}</b><GameTitleSample pref={o.id} /></span>
        </button>
      ))}
    </div>
  )
}

// WittyPlay Settings' page for these choices.
export function LookSetting() {
  const [pads, , setPads] = usePlayPads()
  const [layout, setLayout] = usePlayLayout()
  const [open, setOpen] = usePlayOpen()
  const [titles, setTitles] = useGameTitlePref()
  const a = useId(), b = useId(), c = useId(), d = useId()
  return (
    <div className="wplay-look-setting">
      <section className="wplay-pads-setting"><h2 id={c}>When you choose a game</h2><OpenChooser value={open} onChange={setOpen} labelledBy={c} /></section>
      <section className="wplay-pads-setting"><h2 id={d}>Game titles</h2><GameTitlesChooser value={titles} onChange={setTitles} labelledBy={d} /></section>
      <section className="wplay-pads-setting"><h2 id={a}>Layout</h2><LayoutChooser value={layout} onChange={setLayout} labelledBy={a} /></section>
      <TabsSetting />
      <section className="wplay-pads-setting"><h2 id={b}>Buttons shown in WittyPlay</h2><PadsChooser value={pads} onChange={setPads} labelledBy={b} /></section>
    </div>
  )
}

// ---- the tabs: someone's own order and which show ----
export function usePlayTabs(): [PlayTabsPref, (p: PlayTabsPref) => void] {
  const [saved, save] = useUserPref<unknown>(PLAY_TABS_KEY, null)
  return [tabsPref(saved), (p) => { void save(p).catch(() => {}) }]
}

// The tabs in order, each with Up, Down and Show: plain buttons a remote or a pad reaches with the arrows. Recent always shows.
export function TabsSetting() {
  const [pref, setPref] = usePlayTabs()
  const id = useId()
  const label = (t: string) => PLAY_TABS.find((x) => x.id === t)?.label || t
  const order = [...pref.order, ...PLAY_TABS.map((t) => t.id).filter((t) => !pref.order.includes(t))].filter((t) => PLAY_TABS.some((x) => x.id === t))
  return (
    <section className="wplay-pads-setting">
      <h2 id={id}>Tabs</h2>
      <p className="wplay-console-note">The order of the tabs along the top, and which of them show.</p>
      <ol className="wplay-tabs-setting" aria-labelledby={id}>
        {order.map((t, i) => {
          const shown = !pref.hidden.includes(t)
          return (
            <li key={t} className={shown ? '' : 'off'}>
              <b>{label(t)}</b>
              <span className="wplay-tabs-buttons">
                <button type="button" disabled={i === 0} onClick={() => setPref({ ...pref, order: moveTab(order, t, -1) })} aria-label={'Move ' + label(t) + ' up'}>Up</button>
                <button type="button" disabled={i === order.length - 1} onClick={() => setPref({ ...pref, order: moveTab(order, t, 1) })} aria-label={'Move ' + label(t) + ' down'}>Down</button>
                <button type="button" aria-pressed={shown} disabled={t === 'recent'} onClick={() => setPref({ ...pref, hidden: shown ? [...pref.hidden, t] : pref.hidden.filter((h) => h !== t) })} aria-label={'Show ' + label(t)}>{shown ? 'Shown' : 'Hidden'}</button>
              </span>
            </li>
          )
        })}
      </ol>
    </section>
  )
}
