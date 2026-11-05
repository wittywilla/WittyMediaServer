// Which remote a video gets, and which of its keys work: the rules of docs/DVD.md, "The remote".
// Every DVD and Blu-ray form gets the same remote. A disc image or folder carries the disc's
// own program, so its menu keys work; a copy of the film alone doesn't, so those keys stay on
// the remote, greyed out, with a line saying why.

// Keys of a disc player's remote.
export type RemoteKey =
  | 'eject' | 'topmenu' | 'menu' | 'return' | 'up' | 'down' | 'left' | 'right' | 'enter'
  | 'audio' | 'subtitle' | 'angle' | 'prev' | 'next' | 'rew' | 'ff' | 'play' | 'pause' | 'stop'
  | 'red' | 'green' | 'yellow' | 'blue'
  | `digit${0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9}`

export const ARROW_KEYS: RemoteKey[] = ['up', 'down', 'left', 'right', 'enter']
export const COLOUR_KEYS: RemoteKey[] = ['red', 'green', 'yellow', 'blue']
export const DIGIT_KEYS = [1, 2, 3, 4, 5, 6, 7, 8, 9, 0].map((d) => `digit${d}` as RemoteKey)
export const ALL_KEYS: RemoteKey[] = ['eject', 'topmenu', 'menu', 'return', ...ARROW_KEYS, 'audio', 'subtitle', 'angle',
  'prev', 'next', 'rew', 'ff', 'play', 'pause', 'stop', ...COLOUR_KEYS, ...DIGIT_KEYS]
// Keys only a disc's own program answers.
export const MENU_ONLY: RemoteKey[] = ['topmenu', 'menu', 'return', ...ARROW_KEYS, 'angle', ...COLOUR_KEYS]

// dvd, bluray: an image or folder, with the disc's program. dvd-copy, bluray-copy: a film
// copied from a disc, with no menus. video: anything else.
export type RemoteForm = 'dvd' | 'bluray' | 'dvd-copy' | 'bluray-copy' | 'video'

export const remoteLabel = (f: RemoteForm) => (f.startsWith('dvd') ? 'WittyDVD' : f.startsWith('bluray') ? 'WittyBluray' : 'WittyMedia')

// The form of a disc image or folder, from its kind.
export const discForm = (it: { kind: string }): RemoteForm => (it.kind.startsWith('bluray') ? 'bluray' : 'dvd')
// The form of a plain video: a copy of a disc's film when the server says it is one.
export const videoForm = (it: { discSource?: string }): RemoteForm =>
  it.discSource === 'dvd' ? 'dvd-copy' : it.discSource === 'bluray' ? 'bluray-copy' : 'video'

// What the player is showing, for the keys that depend on it.
export type RemoteState = {
  inMenu?: boolean    // a menu's buttons are on screen, or a pop-up menu is open
  bdj?: boolean       // a Blu-ray's Java applications are on screen
  popup?: boolean     // the film carries a pop-up menu
  hasMenus?: boolean  // false: the disc has no menus (a BDAV recording, say)
  playing?: boolean   // false: a still picture, where play, pause and seeking mean nothing
  audio?: number      // sound tracks
  subtitles?: number  // subtitle tracks
  angles?: number     // camera angles
  chapters?: number   // chapters of what's playing
}

const digitOf = (k: RemoteKey) => (k.startsWith('digit') ? Number(k.slice(5)) : -1)

// keyWorks says whether a key does anything for this form, in this state.
export function keyWorks(form: RemoteForm, k: RemoteKey, s: RemoteState = {}): boolean {
  if (k === 'eject') return true
  if (k === 'audio') return (s.audio || 0) > 1
  if (k === 'subtitle') return (s.subtitles || 0) > 0
  const disc = form === 'dvd' || form === 'bluray'
  if (COLOUR_KEYS.includes(k)) return form === 'bluray' && !!s.bdj
  if (!disc) {
    if (MENU_ONLY.includes(k)) return false
    // A copy's digits jump to that chapter, as a disc's do during the film.
    const d = digitOf(k)
    if (d >= 0) return form !== 'video' && d >= 1 && d <= (s.chapters || 0)
    return true
  }
  if (ARROW_KEYS.includes(k)) return !!(s.inMenu || s.bdj)
  if (k === 'angle') return (s.angles || 1) > 1
  if (k === 'rew' || k === 'ff' || k === 'play' || k === 'pause' || k === 'stop') return s.playing !== false
  if (k === 'topmenu') return s.hasMenus !== false || !!s.bdj
  if (k === 'menu') return s.hasMenus !== false || !!s.popup || !!s.bdj
  return true
}

// remoteNote is the line a remote shows when its menu keys can't work, or ''.
export function remoteNote(form: RemoteForm, s: RemoteState = {}): string {
  if (form === 'dvd-copy' || form === 'bluray-copy') return 'This copy has no disc menus'
  if ((form === 'dvd' || form === 'bluray') && s.hasMenus === false && !s.popup && !s.bdj) return 'This disc has no menus'
  return ''
}
