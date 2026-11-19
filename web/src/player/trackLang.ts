// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { languageNameHere, t } from '../i18n'

// A sound track's or subtitle's language ("eng", "de", "und") named in the app's language: "English", "Englisch".
export const trackLang = (code?: string) => {
  const c = (code || '').trim().toLowerCase()
  return !c || c === 'und' || c === 'unknown' ? t('Unknown') : languageNameHere(c)
}
