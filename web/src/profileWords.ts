// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// What a page says, and offers, depending on who is looking (docs/PROFILES.md, docs/ACCOUNTS.md).
// The server decides what each profile may see and do; this is only the wording around it.
// Only the owner can fill a library or change a setting, so only the owner is told how. A
// Kids or Guest profile is never sent to Settings or told about folders and scans: it is
// told, kindly, that there is nothing for it here and who to ask.
//
// The words are English, marked with tk(): whoever shows them translates them there, with tx().

const tk = <S extends string>(english: S): S => english // marks text translated where it's shown (no i18n import: node tests load this file)

export type Who = { isAdmin?: boolean; kind?: string } | null | undefined

// A Kids or Guest profile: someone else looks after it.
export const lookedAfter = (who: Who) => !!who && !who.isAdmin && (who.kind === 'kids' || who.kind === 'guest')

// emptyLibraryWords: what an empty library says. forOwner is the advice for whoever can act on it.
export function emptyLibraryWords(who: Who, forOwner: string): string {
  if (who?.isAdmin) return forOwner
  if (lookedAfter(who)) return tk('There’s nothing here for your profile yet. Ask a grown-up if you think something is missing.')
  return tk('There’s nothing in this library that your profile can see yet.')
}

// emptyLibraryState: the heading and words of a library page with nothing on it. A library whose
// scan waits behind another one's, or is going on, or has never run, isn't empty: it says so,
// rather than telling the owner to add folders that are already there. phase is the scan's own
// (GET /libraries/{id}/scan), "waiting" while it waits for its turn.
export function emptyLibraryState(who: Who, lib: { scanning?: boolean; lastScanAt?: number | null }, phase: string | undefined, forOwner: string): { title: string; text: string; state: 'waiting' | 'scanning' | 'unscanned' | 'empty' } {
  if (lib.scanning && phase === 'waiting') return { state: 'waiting', title: tk('Waiting to be scanned'), text: tk('Another library is being scanned first. What’s here appears as soon as this one’s turn comes.') }
  if (lib.scanning) return { state: 'scanning', title: tk('Scanning…'), text: tk('Titles appear as they are found.') }
  if (lib.lastScanAt == null) return { state: 'unscanned', title: tk('Not scanned yet'), text: who?.isAdmin ? tk('Scan this library to look through its folders.') : tk('This library hasn’t been looked through yet. Check back soon.') }
  return { state: 'empty', title: tk('Nothing here yet'), text: emptyLibraryWords(who, forOwner) }
}

// emptyHomeWords: the card on a home page with nothing on it.
export function emptyHomeWords(who: Who, scanning: boolean): { title: string; text: string } {
  if (scanning) return { title: who?.isAdmin ? tk('Your libraries are being scanned') : tk('The libraries are being scanned'), text: tk('Movies, shows, music and books appear here as they’re found. The first scan can take a few minutes.') }
  if (who?.isAdmin) return { title: tk('Your libraries are empty'), text: tk('Nothing was found in their folders yet. Check the folders in Settings, Libraries, then scan again.') }
  if (lookedAfter(who)) return { title: tk('Nothing to watch yet'), text: tk('There’s nothing here for your profile yet. Ask a grown-up to add something for you.') }
  return { title: tk('Nothing to watch yet'), text: tk('Nothing has been added that your profile can see. Whoever runs this server can add more.') }
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
export const GONE_TEXT = tk('It may have been removed, or this profile may not be allowed to see it.')
export const isGone = (message: string) => /not found/i.test(message)
