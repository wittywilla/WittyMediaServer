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
import { isPlayPads, padsFor, PLAY_KEYS, PLAY_PADS, PLAY_PADS_KEY, swapFor, type PlayPads } from './playPaths'

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
