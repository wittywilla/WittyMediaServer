// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// overlayFoot: how much of the window's foot a menu, popover or tooltip placed by script must keep
// clear of, in pixels: --overlay-foot, which is the app's floating bar inside WittyMediaClient
// (shell.css) and nothing otherwise. The bar is drawn over the page, so the page can't cover it.
// Measured rather than parsed, as the value is a calc() with the safe area in it.
export function overlayFoot(): number {
  if (typeof document === 'undefined' || !document.body) return 0
  const probe = document.createElement('div')
  probe.style.cssText = 'position:fixed;left:0;bottom:0;width:0;height:var(--overlay-foot, 0px);visibility:hidden;pointer-events:none'
  document.body.appendChild(probe)
  const h = probe.getBoundingClientRect().height
  probe.remove()
  return Number.isFinite(h) && h > 0 ? h : 0
}

// usableHeight: the window's height less that foot: the bottom edge anything placed over the page keeps above.
export function usableHeight(): number {
  return Math.max(0, window.innerHeight - overlayFoot())
}
