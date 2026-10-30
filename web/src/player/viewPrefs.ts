import { useCallback, useState } from 'react'

// Viewing preferences for the player. Stored per browser, either for all
// videos ("global") or for one item (a disc), which then wins.

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

function read(key: string): Partial<ViewPrefs> | null {
  try { const v = localStorage.getItem(key); return v ? JSON.parse(v) : null } catch { return null }
}
function write(key: string, v: object | null) {
  try { v ? localStorage.setItem(key, JSON.stringify(v)) : localStorage.removeItem(key) } catch { /* ignore */ }
}

export type Scope = 'item' | 'global'

// useViewPrefs returns the effective preferences for an item, which scope
// they come from, and a setter that writes to the chosen scope.
export function useViewPrefs(itemId: number, remoteDefault = false) {
  const load = () => {
    const g = read(GLOBAL), i = read(itemKey(itemId))
    return { prefs: { ...DEFAULT_VIEW, remote: remoteDefault, ...(g || {}), ...(i || {}) } as ViewPrefs, scope: (i ? 'item' : 'global') as Scope }
  }
  const [state, setState] = useState(load)
  const update = useCallback((patch: Partial<ViewPrefs>, scope: Scope) => {
    setState((cur) => {
      const prefs = { ...cur.prefs, ...patch }
      if (scope === 'item') write(itemKey(itemId), prefs)
      else { write(GLOBAL, { ...(read(GLOBAL) || {}), ...patch }); write(itemKey(itemId), null) }
      return { prefs, scope }
    })
  }, [itemId])
  const setScope = useCallback((scope: Scope) => {
    setState((cur) => {
      if (scope === 'item') write(itemKey(itemId), cur.prefs)
      else { write(itemKey(itemId), null); write(GLOBAL, cur.prefs) }
      return { ...cur, scope }
    })
  }, [itemId])
  return { prefs: state.prefs, scope: state.scope, update, setScope }
}

// Remembered track choices per disc, like a DVD player's memory: reapplied
// when the disc is inserted again.
export type DiscMemory = { audioLanguage?: string; subtitleLanguage?: string; subtitlesOff?: boolean; angle?: number }
const memKey = (id: number) => `wms.disc.${id}`
export const discMemory = (id: number): DiscMemory => (read(memKey(id)) as DiscMemory) || {}
export const rememberDisc = (id: number, patch: DiscMemory) => write(memKey(id), { ...discMemory(id), ...patch })
