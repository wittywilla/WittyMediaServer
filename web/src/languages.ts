// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
// The languages people choose between, as two-letter codes with their English names: Library
// Transcoding's track lists and first-run setup's Language.
export const LANGUAGES: [string, string][] = [
  ['en', 'English'], ['es', 'Spanish'], ['fr', 'French'], ['de', 'German'], ['it', 'Italian'], ['pt', 'Portuguese'], ['nl', 'Dutch'],
  ['sv', 'Swedish'], ['da', 'Danish'], ['no', 'Norwegian'], ['fi', 'Finnish'], ['pl', 'Polish'], ['cs', 'Czech'], ['hu', 'Hungarian'], ['el', 'Greek'], ['tr', 'Turkish'],
  ['ru', 'Russian'], ['uk', 'Ukrainian'], ['ja', 'Japanese'], ['ko', 'Korean'], ['zh', 'Chinese'], ['hi', 'Hindi'], ['ar', 'Arabic'], ['he', 'Hebrew'], ['th', 'Thai'], ['vi', 'Vietnamese'],
]
export const languageName = (code: string) => LANGUAGES.find(([c]) => c === code)?.[1] ?? code.toUpperCase()

// The languages of the app's own pages. They're the ones above, except that Portuguese comes as written in Brazil and
// in Portugal (pt-BR, pt-PT). Disc menus and sound tracks keep the two-letter codes above.
export const UI_LANGUAGES: string[] = LANGUAGES.flatMap(([c]) => (c === 'pt' ? ['pt-BR', 'pt-PT'] : [c]))

// A pages' language from any way of writing one: "pt" (as saved before there were two) is Brazil's, "pt-pt" Portugal's.
export function uiLanguageFor(code: string | undefined | null): string {
  if (!code) return ''
  const [lang, region] = code.replace('_', '-').split('-')
  const base = (lang || '').toLowerCase()
  if (base === 'pt') return region && region.toUpperCase() !== 'BR' ? 'pt-PT' : 'pt-BR'
  return UI_LANGUAGES.includes(base) ? base : ''
}

// The two-letter code of a pages' language, for disc menus and sound tracks ("pt-PT" is "pt").
export const trackLanguage = (ui: string) => ui.split('-')[0]

// The browser's language, when it is one of these: what a new server starts from.
// The browser's language for the pages (with Portugal's or Brazil's Portuguese).
export function browserUiLanguage(): string {
  try { return uiLanguageFor(navigator.language) } catch { return '' }
}

export function browserLanguage(): string {
  try {
    const code = new Intl.Locale(navigator.language).language
    return LANGUAGES.some(([c]) => c === code) ? code : ''
  } catch { return '' }
}
