// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, type RefObject } from 'react'

// A page's own bar that sticks under the top bar (a library's sort and filter bar, an album's or artist's actions)
// takes room too: --sub-bar on the root is its measured height, and the page keeps that much more room at the top
// when it scrolls a section, a link's target or a focused control into view (styles.css, scroll-padding). Without
// it, "#October" or a control reached with Tab lands behind the bar. Written only when the height changes; removed
// when the bar goes.
export function useSubBar(ref: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const el = ref.current, root = document.documentElement
    if (!el) return
    let last = '', tallest = 0
    const measure = () => {
      // Only while it really sticks: on a phone it scrolls away with the page (position: static) and needs no room.
      // The tallest it has been: a bar that slims once stuck (an album's actions, 88 to 64) can be either height as
      // a jump lands, and a little more room never hides anything, where too little does.
      const sticks = /sticky|fixed/.test(getComputedStyle(el).position)
      tallest = sticks ? Math.max(tallest, Math.ceil(el.offsetHeight)) : 0
      const v = tallest + 'px'
      if (v !== last) { last = v; root.style.setProperty('--sub-bar', v) }
    }
    measure()
    const ro = window.ResizeObserver ? new ResizeObserver(measure) : null
    ro?.observe(el)
    addEventListener('resize', measure)
    return () => { ro?.disconnect(); removeEventListener('resize', measure); root.style.removeProperty('--sub-bar') }
  }, [ref])
}
