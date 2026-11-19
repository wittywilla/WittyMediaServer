// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState, type RefObject } from 'react'

// Controls that step out of the way when nothing is being done, and come back as soon as something is: Now playing's
// fade over the visualizer and karaoke's top row share this, so the two behave alike.
//   - A mouse, a key or a wheel brings them back; a few quiet seconds after the last one, they go again.
//   - Focus on any of them brings them back and keeps them (a keyboard or a screen reader never meets a hidden control:
//     they stay in the focus order, and show as they're reached).
//   - startHidden: a phone or a TV starts with them away.
//   - tap: a phone. A tap on the screen (not on a control) shows them, a second tap hides them again; a mouse moving
//     isn't how a phone says anything.
//   - firstKey: a TV. With them away, the first press of the remote or the controller only brings them back, and does
//     nothing else (Back still goes back).
// busy: true while they must stay (a menu open, the pointer resting on them).
export const IDLE_MS = 4000

export function useIdleHide(root: RefObject<HTMLElement | null>, on: boolean, o: { busy?: () => boolean; startHidden?: boolean; tap?: boolean; firstKey?: boolean; controls?: string } = {}) {
  const [idle, setIdle] = useState(!!(on && o.startHidden))
  const opts = useRef(o)
  opts.current = o
  const idleNow = useRef(idle)
  idleNow.current = idle
  useEffect(() => {
    const el = root.current
    if (!on || !el) { setIdle(false); return }
    let t: ReturnType<typeof setTimeout> | undefined
    const busy = () => !!opts.current.busy?.() || (!!document.activeElement && document.activeElement !== el && el.contains(document.activeElement) &&
      !!(document.activeElement as HTMLElement).closest?.(opts.current.controls || '*'))
    const quiet = () => { if (busy()) t = setTimeout(quiet, IDLE_MS); else setIdle(true) }
    const wake = () => { setIdle(false); clearTimeout(t); t = setTimeout(quiet, IDLE_MS) }
    const hide = () => { clearTimeout(t); setIdle(true) }
    const onPointer = (e: PointerEvent) => {
      if (e.pointerType === 'mouse' || e.pointerType === 'pen' || !opts.current.tap) { if (e.type === 'pointermove' || e.type === 'pointerdown') wake(); return }
      if (e.type !== 'pointerdown') return
      // A finger: on a control it's using it, so they stay; on the screen itself it shows them, or hides them again.
      const onControl = (e.target as Element | null)?.closest?.('button, input, select, a, [role=radio], [role=menu], .mp-pop-body, .kar-fx, .kar-mixp')
      if (onControl) wake()
      else if (idleNow.current) wake()
      else hide()
    }
    const onKey = (e: KeyboardEvent) => {
      const back = e.key === 'Escape' || e.key === 'GoBack' || e.key === 'BrowserBack'
      if (opts.current.firstKey && idleNow.current && !back && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault()
        e.stopImmediatePropagation()
      }
      wake()
    }
    // A control inside taking focus is someone reaching for it; the screen itself (or the player under karaoke) taking
    // it back after a press isn't.
    const onFocus = (e: FocusEvent) => { const at = e.target as Node | null; if (at && at !== el && el.contains(at)) wake() }
    const onWheel = () => wake()
    document.addEventListener('pointermove', onPointer, { passive: true })
    document.addEventListener('pointerdown', onPointer, { passive: true })
    // Before anything else hears the key, so the first press on a TV can be only that.
    window.addEventListener('keydown', onKey, true)
    document.addEventListener('focusin', onFocus)
    document.addEventListener('wheel', onWheel, { passive: true })
    if (!opts.current.startHidden) wake()
    return () => {
      clearTimeout(t)
      document.removeEventListener('pointermove', onPointer)
      document.removeEventListener('pointerdown', onPointer)
      window.removeEventListener('keydown', onKey, true)
      document.removeEventListener('focusin', onFocus)
      document.removeEventListener('wheel', onWheel)
    }
  }, [on]) // eslint-disable-line react-hooks/exhaustive-deps
  return idle
}
