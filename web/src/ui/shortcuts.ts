// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The app-wide keys of the one input model (docs/plans/DESIGN-2.md section 3), answered the same whatever sent
// them: a keyboard, or a game pad whose buttons tv.ts presses as these keys. The rules for what a key means are in
// inputModel.ts; this does it to the page. The keys of the page's own navigation (arrows, Enter) are tv.ts's.
import { HELP_EVENT } from './KeyHelp.event'
import { shortcut, type Action } from './inputModel'
import { focusNextZone, focusZone, rememberZones, zoneOf } from './zones'

const typingIn = (el: Element | null) => {
  if (!el) return false
  const t = el.tagName
  if (t === 'TEXTAREA' || (el as HTMLElement).isContentEditable) return true
  return t === 'INPUT' && !['button', 'checkbox', 'radio', 'submit', 'reset', 'range'].includes((el as HTMLInputElement).type)
}
const shown = (el: HTMLElement) => el.getClientRects().length > 0 && !el.closest('[inert], [aria-hidden="true"]')
// Whatever is open as a dialog, menu or the full-screen player has the keys; the app-wide ones wait.
const BUSY = '[role=dialog], [role=alertdialog], [role=menu], .modal, .picker-backdrop, .player:not(.mini):not(.inline), .reader, .wp-playing, .wp-capture'

function search(): boolean {
  const field = Array.from(document.querySelectorAll<HTMLInputElement>('.topbar .search input, input[type=search][aria-label=Search]')).find(shown)
  if (field) { field.focus(); field.select(); return true }
  const link = Array.from(document.querySelectorAll<HTMLElement>('[data-nav-zone="bar"] a[href="/search"], .tvm-pill[href="/search"], a[href="/search"][data-tab]')).find(shown)
  if (link) { link.click(); return true }
  return false
}

// The bar's entries, in order, and the one that is current: the previous or next tab.
function stepBar(step: -1 | 1): boolean {
  const links = Array.from(document.querySelectorAll<HTMLAnchorElement>('[data-nav-zone="bar"] a[href]')).filter((a) => shown(a) && !a.matches('.brand, .top-brand, [data-nav-nostep]'))
  const i = links.findIndex((a) => a.getAttribute('aria-current') === 'page' || a.classList.contains('active'))
  const to = links[i + step]
  if (i < 0 || !to) return false
  to.click()
  return true
}

// Options: what a right click would do on the thing in focus; failing that, the screen's own options button;
// failing that, the Dock, where a screen's options always are.
function options(): boolean {
  const cur = document.activeElement as HTMLElement | null
  if (cur && cur !== document.body && zoneOf(cur) !== 'bar' && !cur.closest('.dock')) {
    const r = cur.getBoundingClientRect()
    const ev = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 })
    if (!cur.dispatchEvent(ev)) return true // something answered and kept it
  }
  const btn = Array.from(document.querySelectorAll<HTMLElement>('[data-tv-options]')).find(shown)
  if (btn) { btn.click(); return true }
  return focusZone('dock')
}

// A pad's LT and RT page the page (a keyboard's Page keys do it by themselves, and aren't taken).
function pageStage(dir: 1 | -1) {
  window.scrollBy({ top: dir * window.innerHeight * 0.85, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
}

function top() {
  window.scrollTo({ top: 0, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
  const first = document.querySelector<HTMLElement>('#main [data-tv-first], #main .btn.primary, #main a[href], #main button')
  if (first && shown(first)) first.focus({ preventScroll: true })
}

export function runAction(a: Action, e: KeyboardEvent): boolean {
  switch (a) {
    case 'search': return search()
    case 'help': window.dispatchEvent(new Event(HELP_EVENT)); return true
    case 'tabPrev': return stepBar(-1)
    case 'tabNext': return stepBar(1)
    case 'options': return options()
    case 'dock': return focusNextZone()
    case 'top': top(); return true
    case 'pageUp': case 'pageDown':
      // Only a pad's synthetic press: a real Page key scrolls the page itself, and in a bar it switches tabs (TvFrame).
      if (e.isTrusted || zoneOf(document.activeElement) === 'bar') return false
      pageStage(a === 'pageDown' ? 1 : -1)
      return true
    case 'back':
      // From the Dock, Back returns to the Stage (the Back ladder's second step); the rest is the page's.
      if (zoneOf(document.activeElement) === 'dock' && !document.querySelector(BUSY)) return focusZone('stage') || focusZone('bar')
      return false
    default: return false
  }
}

let installed = false
export function installShortcuts() {
  if (installed) return
  installed = true
  rememberZones()
  window.addEventListener('keydown', (e) => {
    if (e.defaultPrevented) return
    const a = shortcut(e, typingIn(e.target as Element | null))
    if (!a) return
    // Search and help are wanted even from the page behind nothing; with a dialog or the player open the keys are
    // theirs, except Ctrl+K and ?.
    if (document.querySelector(BUSY) && a !== 'help') return
    if (a === 'back' && !(e.target as Element | null)?.closest?.('[data-nav-zone="dock"]')) return
    if (runAction(a, e)) { e.preventDefault(); e.stopImmediatePropagation() }
  }, true)
}
