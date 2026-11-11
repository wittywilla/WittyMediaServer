// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where a remote, a game pad or the arrow keys go next on a server-driven page
// (docs/SDUI.md, "Focus"). The page's order decides, never geometry: left and right move
// inside a row and stop at its ends, a grid moves by row and column, and up and down go
// to the previous or next stop in the page's order, landing where that group was left.
// Plain data in, an ID out, so the rule is tested here and the page only carries it out.

import type { Node } from './types.ts'

export type Dir = 'up' | 'down' | 'left' | 'right'

// A stop is what up and down move between: a row, a grid, a strip of tabs, or one thing
// that stands alone (a spotlight, a title's header, a banner).
export type Stop = { id: string; kind: 'row' | 'grid' | 'single'; members: string[]; columns: number }

export type FocusMap = { stops: Stop[]; stopOf: Record<string, number>; start: string }

const SIDEWAYS = new Set(['Shelf', 'Tabs', 'SeasonList'])

// focusMap: the page's stops in order. `columns` says how many columns a grid is drawn
// with now, which the page knows and the document only suggests.
export function focusMap(root: Node, columns: (gridId: string, sent: number) => number = (_id, sent) => sent): FocusMap {
  // Document order is reading order, and is what `focus.order` numbers; cards that arrived
  // later joined their group at the end, where the document has them.
  const focusable: { node: Node }[] = []
  const walk = (x: Node) => {
    if (x.focus) focusable.push({ node: x })
    for (const c of x.children || []) walk(c)
  }
  walk(root)
  const holders = new Map<string, Node>()
  const find = (x: Node) => { holders.set(x.id, x); x.children?.forEach(find) }
  find(root)
  const stops: Stop[] = []
  const stopOf: Record<string, number> = {}
  const byGroup = new Map<string, number>()
  let start = ''
  for (const { node } of focusable) {
    const f = node.focus!
    if (f.default) start = node.id
    if (!f.group) {
      stopOf[node.id] = stops.length
      stops.push({ id: node.id, kind: 'single', members: [node.id], columns: 1 })
      continue
    }
    let at = byGroup.get(f.group)
    if (at === undefined) {
      const holder = holders.get(f.group)
      const grid = holder?.type === 'Grid' || holder?.type === 'List' || holder?.type === 'TrackList'
      const sent = typeof holder?.props?.columns === 'number' ? holder.props.columns as number : 1
      const cols = holder?.type === 'Grid' ? Math.max(1, columns(f.group, sent)) : 1
      at = stops.length
      byGroup.set(f.group, at)
      stops.push({ id: f.group, kind: grid ? 'grid' : holder && !SIDEWAYS.has(holder.type) && holder.type !== 'Stack' ? 'row' : 'row', members: [], columns: cols })
    }
    stopOf[node.id] = at
    stops[at].members.push(node.id)
  }
  return { stops, stopOf, start: start || focusable[0]?.node.id || '' }
}

// next: where focus goes from `from`. `left` is each group's remembered member. null:
// nowhere that way on this page, so whatever is around the page may take the key.
export function next(map: FocusMap, from: string, dir: Dir, left: Record<string, string> = {}): string | null {
  const at = map.stopOf[from]
  if (at === undefined) return map.start || null
  const stop = map.stops[at]
  const i = stop.members.indexOf(from)
  const enter = (to: number): string | null => {
    const s = map.stops[to]
    if (!s) return null
    const was = left[s.id]
    return was && s.members.includes(was) ? was : s.members[0]
  }
  if (stop.kind === 'grid') {
    const cols = stop.columns
    if (dir === 'left') return i % cols > 0 ? stop.members[i - 1] : null
    if (dir === 'right') return i % cols < cols - 1 && i + 1 < stop.members.length ? stop.members[i + 1] : null
    if (dir === 'up') return i - cols >= 0 ? stop.members[i - cols] : enter(at - 1)
    // Down from the row above a short last row lands on that row's last card.
    if (i + cols < stop.members.length) return stop.members[i + cols]
    const lastRow = Math.floor((stop.members.length - 1) / cols)
    if (Math.floor(i / cols) < lastRow) return stop.members[stop.members.length - 1]
    return enter(at + 1)
  }
  if (dir === 'left') return i > 0 ? stop.members[i - 1] : null
  if (dir === 'right') return i < stop.members.length - 1 ? stop.members[i + 1] : null
  return enter(dir === 'up' ? at - 1 : at + 1)
}
