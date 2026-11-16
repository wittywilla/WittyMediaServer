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

// The browser's language, when it is one of these: what a new server starts from.
export function browserLanguage(): string {
  try {
    const code = new Intl.Locale(navigator.language).language
    return LANGUAGES.some(([c]) => c === code) ? code : ''
  } catch { return '' }
}
