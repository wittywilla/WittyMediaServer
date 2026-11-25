// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Pages the Settings menu opens that have an address of their own elsewhere in the app: Sign in a TV (/pair),
// Connect a device (/away), Your year, Collections, a module's own pages (/controllers, /m/<module>/<page>), Help.
// Opened from Settings, each is drawn inside Settings, with the menu beside it (or, on a phone, Back to it), at
// /settings/open/<its address>. Opened any other way it is the page it always was. Where the web has a Settings
// page of its own for a row (Libraries, Legal), the row goes there instead.

import { createContext, useContext } from 'react'

export const OPEN = '/settings/open'

/** True for a page drawn inside Settings (SettingsPage's page): it uses Settings' own head (PageHead), not its own. */
export const InSettings = createContext(false)
export const useInSettings = () => useContext(InSettings)

const SAME: Record<string, string> = {
  '/manage/libraries': '/settings/libraries',
  '/legal': '/settings/legal',
}

/** Where a Settings menu row's page is drawn on the web: inside Settings, whatever its own address. */
export function inSettings(path: string): string
export function inSettings(path: string | undefined): string | undefined
export function inSettings(path: string | undefined): string | undefined {
  if (!path || !path.startsWith('/') || path === '/settings' || path.startsWith('/settings/') || path.startsWith('/settings?') || path.startsWith('/settings#')) return path
  const at = path.search(/[?#]/)
  const bare = at < 0 ? path : path.slice(0, at)
  if (SAME[bare]) return SAME[bare] + (at < 0 ? '' : path.slice(at))
  return OPEN + path
}

/** The page's own address when this one is a page opened inside Settings (/settings/open/pair is /pair); null if not. */
export function openedPath(pathname: string): string | null {
  if (pathname !== OPEN && !pathname.startsWith(OPEN + '/')) return null
  return pathname.slice(OPEN.length) || '/'
}
