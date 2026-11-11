// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useState } from 'react'

// Viewing preferences for the player. Stored per browser, either for all
// videos ("global") or for one item (a disc), which then wins.
//
// 3D is the exception: it's only ever kept for one item, whatever Apply to says.
// Saved for everything, it once turned 3D on for every disc, and a 2D picture
// shown as 3D is half of itself stretched to fill the screen.

export type ViewPrefs = {
  seekBar: boolean         // show the timeline
  chapterMarks: boolean    // chapter ticks on the timeline
  hideAfter: number        // seconds before controls hide (0 = never)
  subtitleSize: number     // DVD subtitle scale, 0.75 - 1.5
  picture: 'fit' | 'fill'  // fit = black bars, fill = zoom to fill the screen
  remote: boolean          // show the DVD remote
  remoteStyle: 'floating' | 'bar' // a movable remote, or its buttons in a strip above the seek bar
  stereo: 'off' | 'sbs' | 'ou'    // the video is 3D: two views side by side, or one over the other
  stereoOut: '2d' | 'anaglyph'    // how to show 3D video: one eye's view flat, or red/cyan for anaglyph glasses
  forcedSubs: boolean      // discs: show forced captions (signs, foreign-language lines) while subtitles are off
  discLocks: boolean       // discs: obey a disc's bans on seeking and skipping, as a disc player does
}

export const DEFAULT_VIEW: ViewPrefs = { seekBar: true, chapterMarks: true, hideAfter: 3.5, subtitleSize: 1, picture: 'fit', remote: false, remoteStyle: 'floating', stereo: 'off', stereoOut: '2d', forcedSubs: true, discLocks: true }

const GLOBAL = 'wms.view'
const itemKey = (id: number) => `wms.view.item.${id}`
const stereoKey = (id: number) => `wms.view.3d.${id}`

type Stereo = Pick<ViewPrefs, 'stereo' | 'stereoOut'>
// split: what's shared (by Apply to) and what's this item's 3D alone.
function split(p: Partial<ViewPrefs>): [Partial<ViewPrefs>, Partial<Stereo>] {
  const { stereo, stereoOut, ...shared } = p
  const own: Partial<Stereo> = {}
  if (stereo !== undefined) own.stereo = stereo
  if (stereoOut !== undefined) own.stereoOut = stereoOut
  return [shared, own]
}

function read(key: string): Partial<ViewPrefs> | null {
  try { const v = localStorage.getItem(key); return v ? JSON.parse(v) : null } catch { return null }
}
function write(key: string, v: object | null) {
  try { v ? localStorage.setItem(key, JSON.stringify(v)) : localStorage.removeItem(key) } catch { /* ignore */ }
}

// The choices for every video on this device, as Settings, Appearance shows them. A disc or
// video with choices of its own keeps those.
// 3D is never one of them: it's kept for one item at a time (saveView).
export const globalView = (): ViewPrefs => ({ ...DEFAULT_VIEW, ...split(read(GLOBAL) || {})[0] })
export const setGlobalView = (patch: Partial<ViewPrefs>) => write(GLOBAL, split({ ...(read(GLOBAL) || {}), ...patch })[0])

export type Scope = 'item' | 'global'

// clearStuck3D takes 3D out of the preferences saved for everything and for each
// item, where earlier versions kept it (and where switching Apply to copied it).
// Run whenever a player opens: a tab still on an older version may write it again.
export function clearStuck3D() {
  try {
    for (let i = localStorage.length - 1; i >= 0; i--) {
      const k = localStorage.key(i)
      if (!k || (k !== GLOBAL && !k.startsWith('wms.view.item.'))) continue
      const v = read(k)
      if (!v || !('stereo' in v || 'stereoOut' in v)) continue
      const [shared] = split(v)
      write(k, shared)
    }
  } catch { /* storage that can't be read has nothing stuck in it */ }
}

// loadView: the preferences in force for an item, and which scope they come from.
export function loadView(itemId: number, remoteDefault = false): { prefs: ViewPrefs; scope: Scope } {
  clearStuck3D()
  const g = read(GLOBAL), i = read(itemKey(itemId)), d3 = read(stereoKey(itemId))
  return { prefs: { ...DEFAULT_VIEW, remote: remoteDefault, ...(g || {}), ...(i || {}), ...(d3 || {}) } as ViewPrefs, scope: i ? 'item' : 'global' }
}

// saveView keeps a change: 3D for this item alone, the rest where scope says.
export function saveView(itemId: number, cur: ViewPrefs, patch: Partial<ViewPrefs>, scope: Scope): ViewPrefs {
  const prefs = { ...cur, ...patch }
  const [shared, own] = split(patch)
  if (Object.keys(own).length) {
    write(stereoKey(itemId), prefs.stereo === 'off' && prefs.stereoOut === DEFAULT_VIEW.stereoOut ? null : { stereo: prefs.stereo, stereoOut: prefs.stereoOut })
  }
  if (scope === 'item') write(itemKey(itemId), split(prefs)[0])
  else if (Object.keys(shared).length) { write(GLOBAL, { ...(read(GLOBAL) || {}), ...shared }); write(itemKey(itemId), null) }
  return prefs
}

// saveScope moves an item's preferences to its own, or to everything. 3D stays put.
export function saveScope(itemId: number, prefs: ViewPrefs, scope: Scope) {
  const [shared] = split(prefs)
  if (scope === 'item') write(itemKey(itemId), shared)
  else { write(itemKey(itemId), null); write(GLOBAL, shared) }
}

// useViewPrefs returns the effective preferences for an item, which scope
// they come from, and a setter that writes to the chosen scope.
export function useViewPrefs(itemId: number, remoteDefault = false) {
  const [state, setState] = useState(() => loadView(itemId, remoteDefault))
  const update = useCallback((patch: Partial<ViewPrefs>, scope: Scope) => {
    setState((cur) => ({ prefs: saveView(itemId, cur.prefs, patch, scope), scope }))
  }, [itemId])
  const setScope = useCallback((scope: Scope) => {
    setState((cur) => { saveScope(itemId, cur.prefs, scope); return { ...cur, scope } })
  }, [itemId])
  return { prefs: state.prefs, scope: state.scope, update, setScope }
}

// Remembered track choices per disc, like a DVD player's memory: reapplied
// when the disc is inserted again.
export type DiscMemory = { audioLanguage?: string; subtitleLanguage?: string; subtitlesOff?: boolean; angle?: number }
const memKey = (id: number) => `wms.disc.${id}`
export const discMemory = (id: number): DiscMemory => (read(memKey(id)) as DiscMemory) || {}
export const rememberDisc = (id: number, patch: DiscMemory) => write(memKey(id), { ...discMemory(id), ...patch })
