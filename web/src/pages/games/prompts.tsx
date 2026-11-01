import { useEffect, useState, type ReactNode } from 'react'
import './prompts.css'

// Button prompts that match what's in someone's hands: a controller's own letters or
// shapes, a keyboard's keys or a remote's. The shapes are drawn here as plain geometry,
// with no maker's marks. The game pages must work with WittyPlay missing, so this is the
// app's own: the player has a copy of its own for the screens inside a game.

type PadFamily = 'xbox' | 'playstation' | 'nintendo' | 'generic'
type InputKind = 'pad' | 'keyboard' | 'remote' | 'pointer'
type PromptButton = 'confirm' | 'back' | 'west' | 'north' | 'lb' | 'rb' | 'lt' | 'rt' | 'start' | 'select' | 'dpad'

function familyOf(id: string): PadFamily {
  const s = id.toLowerCase()
  if (/nintendo|pro controller|joy-?con|vendor: 057e|^057e-/.test(s)) return 'nintendo'
  if (/playstation|dualshock|dualsense|sony|vendor: 054c|^054c-/.test(s)) return 'playstation'
  if (/xbox|xinput|microsoft|vendor: 045e|^045e-/.test(s)) return 'xbox'
  return 'generic'
}

const isTV = () => (window.matchMedia?.('(pointer: none)').matches ?? false) || /\b(TV|AFT[A-Z]*|BRAVIA|GoogleTV|SmartTV|CrKey|Tizen|Web0S)\b/.test(navigator.userAgent)

type State = { kind: InputKind; family: PadFamily }
let state: State = { kind: 'pointer', family: 'generic' }
const listeners = new Set<(s: State) => void>()
let installed = false

function set(next: Partial<State>) {
  const merged = { ...state, ...next }
  if (merged.kind === state.kind && merged.family === state.family) return
  state = merged
  for (const l of listeners) l(state)
}

function firstPad(): string {
  try {
    for (const p of navigator.getGamepads?.() || []) if (p) return p.id
  } catch { /* some browsers keep gamepads from insecure pages */ }
  return ''
}

function install() {
  if (installed) return
  installed = true
  // A key press made by tv.ts from a controller isn't "trusted", and a real key is.
  window.addEventListener('keydown', (e) => {
    if (!e.isTrusted) { const id = firstPad(); set({ kind: 'pad', family: id ? familyOf(id) : state.family }); return }
    if (['Shift', 'Control', 'Alt', 'Meta'].includes(e.key)) return
    set({ kind: isTV() ? 'remote' : 'keyboard' })
  }, { capture: true, passive: true })
  window.addEventListener('pointerdown', () => set({ kind: 'pointer' }), { capture: true, passive: true })
  window.addEventListener('gamepadconnected', (e) => set({ kind: 'pad', family: familyOf((e as GamepadEvent).gamepad?.id || '') }))
  const id = firstPad()
  if (id) state = { kind: 'pad', family: familyOf(id) }
  else if (isTV()) state = { kind: 'remote', family: 'generic' }
}

// useInputKind follows whichever input was used last.
export function useInputKind(): State {
  const [s, setS] = useState<State>(() => { install(); return state })
  useEffect(() => {
    listeners.add(setS)
    setS(state)
    return () => { listeners.delete(setS) }
  }, [])
  return s
}

const LETTERS: Record<PadFamily, Partial<Record<PromptButton, string>>> = {
  xbox: { confirm: 'A', back: 'B', west: 'X', north: 'Y', lb: 'LB', rb: 'RB', lt: 'LT', rt: 'RT' },
  // On a Nintendo pad the right-hand button confirms: tv.ts swaps them, so A means A.
  nintendo: { confirm: 'A', back: 'B', west: 'Y', north: 'X', lb: 'L', rb: 'R', lt: 'ZL', rt: 'ZR', start: '+', select: '−' },
  playstation: { lb: 'L1', rb: 'R1', lt: 'L2', rt: 'R2' },
  generic: { confirm: 'A', back: 'B', west: 'X', north: 'Y', lb: 'LB', rb: 'RB', lt: 'LT', rt: 'RT' },
}

function Shape({ button }: { button: PromptButton }) {
  const d = { fill: 'none', stroke: 'currentColor', strokeWidth: 2.4, strokeLinecap: 'round', strokeLinejoin: 'round' } as const
  switch (button) {
    case 'confirm': return <path {...d} d="M7 7l10 10M17 7L7 17" />
    case 'back': return <circle {...d} cx="12" cy="12" r="5.6" />
    case 'west': return <rect {...d} x="7" y="7" width="10" height="10" rx=".8" />
    case 'north': return <path {...d} d="M12 6.2l6 10.3H6z" />
  }
  return null
}

function Round({ children, label }: { children: ReactNode; label: string }) {
  return (
    <span className="gm-glyph round" role="img" aria-label={label}>
      <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="11" className="gm-glyph-back" />{children}</svg>
    </span>
  )
}

function PadGlyph({ button, family }: { button: PromptButton; family: PadFamily }) {
  if (button === 'dpad') return <Round label="D-pad"><path fill="currentColor" d="M9.8 4h4.4v5.8H20v4.4h-5.8V20H9.8v-5.8H4V9.8h5.8z" /></Round>
  if (family === 'playstation' && ['confirm', 'back', 'west', 'north'].includes(button)) {
    const names: Record<string, string> = { confirm: 'cross', back: 'circle', west: 'square', north: 'triangle' }
    return <Round label={names[button]}><Shape button={button} /></Round>
  }
  const letter = LETTERS[family][button]
  if (button === 'start' && family !== 'nintendo') {
    return <span className="gm-glyph pill" role="img" aria-label={family === 'playstation' ? 'Options' : 'Menu'}><svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M6 8h12v1.8H6zm0 3.1h12v1.8H6zm0 3.1h12V16H6z" /></svg></span>
  }
  if (button === 'select' && family !== 'nintendo') {
    return <span className="gm-glyph pill" role="img" aria-label={family === 'playstation' ? 'Create' : 'View'}><svg viewBox="0 0 24 24" aria-hidden="true"><path fill="none" stroke="currentColor" strokeWidth="1.8" d="M5.5 7h9v7h-9zM9.5 14v3h9v-7h-4" /></svg></span>
  }
  return <span className={'gm-glyph ' + (letter && letter.length > 1 ? 'pill' : 'round')}>{letter}</span>
}

const KEYS: Partial<Record<PromptButton, string>> = { confirm: 'Enter', back: 'Esc', lb: 'PgUp', rb: 'PgDn', dpad: 'Arrows' }
const REMOTE: Partial<Record<PromptButton, string>> = { confirm: 'OK', back: 'Back', dpad: 'Arrows' }

export type Prompt = { button: PromptButton; label: string; key?: string }

function PromptItem({ p, input }: { p: Prompt; input: State }) {
  if (input.kind === 'pad') return <span className="gm-prompt"><PadGlyph button={p.button} family={input.family} />{p.label}</span>
  const cap = p.key || (input.kind === 'remote' ? REMOTE : KEYS)[p.button]
  if (!cap) return null
  return <span className="gm-prompt"><kbd className="gm-glyph key">{cap}</kbd>{p.label}</span>
}

// A row of prompts along the bottom of a screen. It only shows for controllers, remotes
// and keyboards being used: a mouse or a finger can see what to press.
export function PromptBar({ items, always, className }: { items: Prompt[]; always?: boolean; className?: string }) {
  const input = useInputKind()
  if (input.kind === 'pointer' && !always) return null
  const shown = input.kind === 'pointer' ? { kind: 'keyboard' as const, family: input.family } : input
  return (
    <div className={'gm-prompt-row' + (className ? ' ' + className : '')} aria-label="Buttons you can press" role="group">
      {items.map((p) => <PromptItem key={p.label} p={p} input={shown} />)}
    </div>
  )
}
