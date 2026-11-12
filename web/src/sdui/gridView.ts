// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The Dock's "View" control: a Grid drawn as a grid of posters or as a list, remembered for this profile on
// this device (docs/SDUI.md, `control.open`). The choice is per page and grid.
import { useEffect, useState } from 'react'

const key = (page: string, grid: string, who: string | number) => `wms.sdui.view.${who}.${page}.${grid}`
const EVENT = 'wms-sdui-view'

export function getList(page: string, grid: string, who: string | number): boolean {
  try { return localStorage.getItem(key(page, grid, who)) === 'list' } catch { return false }
}
export function setList(page: string, grid: string, who: string | number, list: boolean) {
  try { if (list) localStorage.setItem(key(page, grid, who), 'list'); else localStorage.removeItem(key(page, grid, who)) } catch { /* for this visit only */ }
  window.dispatchEvent(new Event(EVENT))
}
export function useList(page: string, grid: string, who: string | number): boolean {
  const [v, set] = useState(() => getList(page, grid, who))
  useEffect(() => {
    set(getList(page, grid, who))
    const on = () => set(getList(page, grid, who))
    window.addEventListener(EVENT, on)
    return () => window.removeEventListener(EVENT, on)
  }, [page, grid, who])
  return v
}

// The letter a title is filed under for "jump to a letter": A to Z, or # for anything else.
export const letterOf = (title: string): string => {
  const c = title.trim().replace(/^(the|a|an)\s+/i, '').charAt(0).toUpperCase()
  return /[A-Z]/.test(c) ? c : '#'
}
