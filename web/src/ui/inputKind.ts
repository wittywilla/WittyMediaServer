// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which input the person is using right now (keyboard and mouse, a game pad, a TV remote, a touch screen), kept
// in one place so the key legend can match it and hide itself on touch (docs/plans/DESIGN-2.md section 3). It is
// also written to <html data-input="..."> so styles can answer to it.
import { nextKind, padFamily, type InputEvent, type InputKind, type PadFamily } from './inputModel'

let kind = 'keys' as InputKind
let family = 'generic' as PadFamily
const listeners = new Set<() => void>()
let snapshot = { kind, family }

function publish() {
  snapshot = { kind, family }
  if (document.documentElement.dataset) document.documentElement.dataset.input = kind
  listeners.forEach((l) => l())
}

// noteInput: something was done with this sort of input. A key that a pad pressed (tv.ts presses the keys a pad
// stands for) comes in as 'pad', not 'key'.
export function noteInput(ev: InputEvent, o: { key?: string; padId?: string } = {}) {
  const tenFoot = !!document.documentElement.classList?.contains('ten-foot')
  const next = nextKind(kind, ev, { tenFoot, key: o.key })
  const f = o.padId ? padFamily(o.padId) : family
  if (next === kind && f === family) return
  kind = next; family = f
  publish()
}

let installed = false
export function trackInput() {
  if (installed) return
  installed = true
  // A device that is only ever touched or driven from across the room starts that way.
  const media = (q: string) => typeof matchMedia === 'function' && matchMedia(q).matches
  if (media('(pointer: none)')) kind = 'remote'
  else if (media('(pointer: coarse)') && !media('(any-pointer: fine)')) kind = 'touch'
  publish()
  window.addEventListener('keydown', (e) => { if (e.isTrusted) noteInput('key', { key: e.key }) }, { capture: true, passive: true })
  window.addEventListener('pointerdown', (e) => noteInput(e.pointerType === 'touch' ? 'touch' : 'pointer'), { capture: true, passive: true })
  window.addEventListener('mousemove', (e) => { if (e.movementX || e.movementY) noteInput('pointer') }, { passive: true })
  window.addEventListener('gamepadconnected', (e) => { { const id = (e as GamepadEvent).gamepad?.id; if (id) family = padFamily(id) } })
}

// The store for useInput (ui/useInput.ts, which holds the React part so that tv.ts stays free of it).
export const subscribeInput = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }
export const inputSnapshot = () => snapshot
