// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The four zones of a screen (docs/plans/DESIGN-2.md section 3): Bar, Stage, Dock, Layer. Each is marked in the
// page with data-nav-zone; this reads where focus is among them and moves it between them by one press.
import { cycleZone, type Zone } from './zonesModel'

export const ZONE_SELECTOR = '[data-nav-zone]'
const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

const shown = (el: HTMLElement) => el.getClientRects().length > 0 && !el.closest('[inert], [aria-hidden="true"]')

// The zone an element is in. A dialog, menu or the player is the Layer, marked or not.
export function zoneOf(el: Element | null): Zone | null {
  if (!el) return null
  if (el.closest('[role=dialog], [role=alertdialog], [role=menu], .modal, .picker-backdrop')) return 'layer'
  const z = el.closest<HTMLElement>(ZONE_SELECTOR)?.dataset.navZone
  return z === 'bar' || z === 'stage' || z === 'dock' || z === 'layer' ? z : null
}

// A zone can be several parts of the page (the Bar is the side menu and the top bar): those with something to focus.
const roots = (z: Zone) => Array.from(document.querySelectorAll<HTMLElement>(`[data-nav-zone="${z}"]`)).filter((r) => Array.from(r.querySelectorAll<HTMLElement>(FOCUSABLE)).some(shown))

// presentZones: the zones the screen has right now, in the order Bar, Stage, Dock.
export function presentZones(): Zone[] {
  const out: Zone[] = []
  for (const z of ['bar', 'stage', 'dock'] as Zone[]) {
    if (roots(z).length) out.push(z)
  }
  return out
}

// focusZone: put focus in a zone: the thing in it that was last focused (a bar's current tab), or its first control.
const last = new WeakMap<Element, HTMLElement>()
export function rememberZones() {
  window.addEventListener('focusin', (e) => {
    const el = e.target as HTMLElement
    const root = el.closest?.(ZONE_SELECTOR)
    if (root) last.set(root, el)
  }, { passive: true })
}
export function focusZone(z: Zone): boolean {
  const list = roots(z)
  if (!list.length) return false
  const was = list.map((r) => last.get(r)).find((el) => el?.isConnected && shown(el))
  const to = was
    || list.map((r) => r.querySelector<HTMLElement>('[aria-current=page], .active, [data-tv-first], .btn.primary')).find((el) => el && shown(el))
    || list.flatMap((r) => Array.from(r.querySelectorAll<HTMLElement>(FOCUSABLE))).find(shown)
  if (!to || !shown(to)) return false
  to.focus({ preventScroll: false })
  return true
}

// Start / F6: the next zone round, Stage to Dock to Bar and back to Stage.
export function focusNextZone(): boolean {
  const from = zoneOf(document.activeElement)
  const here = from === 'layer' ? null : from
  const to = cycleZone(here, presentZones())
  return to ? focusZone(to) : false
}
