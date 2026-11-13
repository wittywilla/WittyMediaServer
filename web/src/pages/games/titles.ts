// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// Which of a game's names to show. The server sends a game's other names beside its title, on the item
// ("titles") and on WittyPlay's game cards (the Card prop "titles"): {native, english, romanized}, each only
// when known. A profile chooses in /me/ui "gameTitles" (GAME_TITLES_KEY):
//   romanized  the title as listed, in Latin letters (A Ressha de Ikou). The default, and what sorts.
//   native     the name in the game's own script (A列車で行こう), else the title.
//   english    the English release's name (A-Train), else the title.
//   both       the name in its own script, with the English name (else the title) under it, smaller.
// Pure functions only, so node --test can load this file; the hook is in useGameTitles.ts.

export type GameTitlePref = 'romanized' | 'native' | 'english' | 'both'
export const GAME_TITLES_KEY = 'gameTitles'
export const GAME_TITLE_PREFS: GameTitlePref[] = ['romanized', 'native', 'english', 'both']

export type GameTitles = { native?: string; english?: string; romanized?: string }
// Anything with a title and, perhaps, a game's other names: an item, or a game card's props.
export type Titled = { title?: string | null; titles?: GameTitles | null }

export const asGameTitlePref = (v: unknown): GameTitlePref =>
  v === 'native' || v === 'english' || v === 'both' ? v : 'romanized'

const clean = (s: unknown) => (typeof s === 'string' ? s.trim() : '')

// gameTitleParts: the line to show, and for "both" the smaller line under it (only when it says something else).
export function gameTitleParts(x: Titled | null | undefined, pref: GameTitlePref | string | null | undefined): { main: string; sub?: string } {
  const p = asGameTitlePref(pref)
  const t = x?.titles || {}
  const title = clean(t.romanized) || clean(x?.title)
  const native = clean(t.native), english = clean(t.english)
  switch (p) {
    case 'native': return { main: native || title }
    case 'english': return { main: english || title }
    case 'both': {
      if (!native) return { main: english || title }
      const sub = english || title
      return sub && sub !== native ? { main: native, sub } : { main: native }
    }
    default: return { main: title }
  }
}

// gameTitle: the one line to show, where there's room for only one (a tooltip, an aria-label, a list cell).
export function gameTitle(x: Titled | null | undefined, pref: GameTitlePref | string | null | undefined): string {
  return gameTitleParts(x, pref).main
}

// gameTitleLabel: everything the name says, for an accessible name: "A列車で行こう (A-Train)" when both show.
export function gameTitleLabel(x: Titled | null | undefined, pref: GameTitlePref | string | null | undefined): string {
  const { main, sub } = gameTitleParts(x, pref)
  return sub ? `${main} (${sub})` : main
}

// titleLang: the language a name is plainly written in, for the element's lang attribute, so screen readers read
// it in the right voice and the browser picks Japanese rather than Chinese forms of shared characters. Kana is
// Japanese, Hangul Korean; Han alone could be either (or Chinese), so it says nothing.
export function titleLang(s: string | null | undefined): string | undefined {
  if (!s) return undefined
  if (/[぀-ヿㇰ-ㇿｦ-ﾟ]/.test(s)) return 'ja'
  if (/[ᄀ-ᇿ㄰-㆏가-힯]/.test(s)) return 'ko'
  return undefined
}

// The choices, for Settings and first-run setup. The examples are what each choice shows for one game.
export const GAME_TITLE_CHOICES: { id: GameTitlePref; label: string; note: string }[] = [
  { id: 'romanized', label: 'Romanized', note: 'In Latin letters, as listed' },
  { id: 'native', label: 'Original', note: 'In the game’s own script' },
  { id: 'english', label: 'English title', note: 'The English release’s name' },
  { id: 'both', label: 'Original with English below', note: 'Both, the English name smaller' },
]

// A game to show the choices with.
export const GAME_TITLE_EXAMPLE: Titled = { title: 'A Ressha de Ikou', titles: { native: 'A列車で行こう', english: 'A-Train', romanized: 'A Ressha de Ikou' } }
