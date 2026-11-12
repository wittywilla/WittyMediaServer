// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Where the web app shows one of its own screens although a server-driven page was
// asked for. The model has pages for films, shows, episodes, albums, collections and
// the libraries that hold them; a game's page and player, a book's front page and
// reader, a photo, a radio station, a creator's channel and the short-video feed are the
// app's own screens (docs/SDUI.md, "Native screens"), reached through the same routes.
// Only the catch-all built-in pages give way: a page an administrator wrote for such a
// library or title is drawn as written.

import type { Document, Node } from './types.ts'

// Libraries that are an experience of the app's own from top to bottom.
export const NATIVE_LIBRARY_KINDS = ['radio', 'shorts', 'channels', 'musicvideos']
// Titles whose page is the app's own screen.
export const SDUI_DETAIL_KINDS = ['movie', 'show', 'episode', 'video', 'disc', 'album', 'collection', 'audiobook', 'podcast', 'recording']

export type LibraryFacts = { id: number; type: string; console?: string }

function header(root: Node): Node | null {
  if (root.type === 'DetailHeader') return root
  for (const c of root.children || []) { const hit = header(c); if (hit) return hit }
  return null
}

// The tabs of the hand-built music page that the page model has no section for: Songs,
// Playlists, Music videos. An address asking for one opens that page on it.
export const OWN_AUDIO_TABS = ['songs', 'playlists', 'videos']
export function ownLibraryTab(params: Record<string, string>, libraries: LibraryFacts[]): boolean {
  const lib = libraries.find((l) => String(l.id) === params.library)
  return !!lib && ['music', 'audiobooks', 'podcasts'].includes(lib.type) && OWN_AUDIO_TABS.includes(params.tab || '')
}

export function nativeInstead(doc: Document, libraries: LibraryFacts[]): boolean {
  if (doc.page.kind === 'library' && ownLibraryTab(doc.page.params || {}, libraries)) return true
  if (doc.page.kind === 'library' && doc.page.id === 'library') {
    const lib = libraries.find((l) => String(l.id) === doc.page.params?.library)
    return !!lib && (NATIVE_LIBRARY_KINDS.includes(lib.type) || (lib.type === 'games' && lib.console === 'rig'))
  }
  if (doc.page.kind === 'detail' && doc.page.id === 'detail') {
    const kind = header(doc.root)?.props?.kind
    return typeof kind === 'string' && kind !== '' && !SDUI_DETAIL_KINDS.includes(kind)
  }
  return false
}

// The app's own tools a server-driven page gets under its title (extras/PageTools.tsx), if any:
// a library of films or shows, of music or spoken word, Collections, or one collection.
const VIDEO_TYPES = ['movies', 'shows', 'anime', 'discs', 'videos', 'commercials']
const AUDIO_TYPES = ['music', 'audiobooks', 'podcasts']
export function toolsFor(doc: Document, libraries: LibraryFacts[]): 'video' | 'audio' | 'collections' | 'collection' | null {
  if (doc.page.kind === 'library') {
    const lib = libraries.find((l) => String(l.id) === doc.page.params?.library)
    if (lib && VIDEO_TYPES.includes(lib.type)) return 'video'
    if (lib && AUDIO_TYPES.includes(lib.type)) return 'audio'
    return null
  }
  if (doc.page.kind === 'collections') return 'collections'
  if (doc.page.kind === 'detail' && header(doc.root)?.props?.kind === 'collection') return 'collection'
  return null
}
export const hasTools = (doc: Document, libraries: LibraryFacts[]) => toolsFor(doc, libraries) !== null
