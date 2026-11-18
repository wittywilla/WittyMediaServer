// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import type { Library } from '../../api'

// Which addresses Music draws itself (pages/music/MusicShell.tsx). Kept apart from the pages
// so the router can ask without loading them. One look for every layout for now: a layout
// that draws Music its own way (server-driven) can take these addresses back here.

// The music library an address belongs to, when Music draws it: a music library's own page,
// one of its albums or one of its artists.
export function musicLibraryFor(pathname: string, libraries: Library[]): Library | null {
  const m = /^\/(library|album|artist)\/(\d+)$/.exec(pathname)
  if (!m) return null
  const lib = libraries.find((l) => String(l.id) === m[2])
  return lib && lib.type === 'music' ? lib : null
}

export const musicHome = (libId: number) => `/library/${libId}`
export const musicTab = (libId: number, tab: string, more = '') => `/library/${libId}?tab=${tab}${more}`
