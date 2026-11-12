// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// What a page says, and offers, depending on who is looking (docs/PROFILES.md, docs/ACCOUNTS.md).
// The server decides what each profile may see and do; this is only the wording around it.
// Only the owner can fill a library or change a setting, so only the owner is told how. A
// Kids or Guest profile is never sent to Settings or told about folders and scans: it is
// told, kindly, that there is nothing for it here and who to ask.

export type Who = { isAdmin?: boolean; kind?: string } | null | undefined

// A Kids or Guest profile: someone else looks after it.
export const lookedAfter = (who: Who) => !!who && !who.isAdmin && (who.kind === 'kids' || who.kind === 'guest')

// emptyLibraryWords: what an empty library says. forOwner is the advice for whoever can act on it.
export function emptyLibraryWords(who: Who, forOwner: string): string {
  if (who?.isAdmin) return forOwner
  if (lookedAfter(who)) return 'There’s nothing here for your profile yet. Ask a grown-up if you think something is missing.'
  return 'There’s nothing in this library that your profile can see yet.'
}

// emptyHomeWords: the card on a home page with nothing on it.
export function emptyHomeWords(who: Who, scanning: boolean): { title: string; text: string } {
  if (scanning) return { title: who?.isAdmin ? 'Your libraries are being scanned' : 'The libraries are being scanned', text: 'Movies, shows, music and books appear here as they’re found. The first scan can take a few minutes.' }
  if (who?.isAdmin) return { title: 'Your libraries are empty', text: 'Nothing was found in their folders yet. Check the folders in Settings, Libraries, then scan again.' }
  if (lookedAfter(who)) return { title: 'Nothing to watch yet', text: 'There’s nothing here for your profile yet. Ask a grown-up to add something for you.' }
  return { title: 'Nothing to watch yet', text: 'Nothing has been added that your profile can see. Whoever runs this server can add more.' }
}

// The settings pages that hold nothing for a Kids or Guest profile: they can't make share
// links or upload, and the activity button is an administrator's to hand out.
const NOT_FOR_LOOKED_AFTER = new Set(['activity', 'sharing'])

// settingsPageHidden: whether a page of Settings is left out of the menu for this person.
// features is what the profile may do (GET /me/limits), when that is known: a profile with
// games turned off has no game settings to make.
export function settingsPageHidden(page: string, who: Who, features?: Record<string, boolean> | null): boolean {
  if (page === 'games' && features?.games === false) return true
  return lookedAfter(who) && NOT_FOR_LOOKED_AFTER.has(page)
}

// goneWords: a title this profile can't open, whether it was removed or is outside its
// limits. The two look the same on purpose: the server answers "not found" for both.
export const GONE_TEXT = 'It may have been removed, or this profile may not be allowed to see it.'
export const isGone = (message: string) => /not found/i.test(message)
