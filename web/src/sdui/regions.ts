// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The content a page gives the shell's optional regions (document.regions, catalogue revision 7): a status bar's profile, server and
// text, and a circular system dock's destinations. The page showing sets them and takes them away when it goes, so the shell can
// draw them. Nothing here touches the page (regions.test.mjs).
import { useSyncExternalStore } from 'react'

export type StatusPart = { id: string; kind: 'profile' | 'server' | 'text'; text?: string }
export type DockEntry = { id: string; label: string; icon?: string; route: string; as?: string }
export type PageRegions = { statusBar?: StatusPart[]; systemDock?: DockEntry[] }

let current: PageRegions | undefined
const listeners = new Set<() => void>()
export function setPageRegions(r: PageRegions | undefined) {
  const empty = !r || (!r.statusBar?.length && !r.systemDock?.length)
  const next = empty ? undefined : r
  if (JSON.stringify(next) === JSON.stringify(current)) return
  current = next
  listeners.forEach((l) => l())
}
export const usePageRegions = () => useSyncExternalStore((l) => { listeners.add(l); return () => { listeners.delete(l) } }, () => current, () => undefined)

// Where the dock's current entry is: the one whose route is the address or a parent of it.
export const dockCurrent = (entries: DockEntry[], pathname: string) => entries.findIndex((e) => pathname === e.route || pathname.startsWith(e.route + '/'))
