// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api } from '../../api'
import { LANGUAGES, UI_LANGUAGES, browserLanguage, trackLanguage, uiLanguageFor } from '../../languages'
import { language as appLanguage, languageNameHere, list, nativeName, setLanguage, t, tn } from '../../i18n'
import { Field, Select } from '../../ui'
import { timeZoneChoices } from './network'

// The language and time zone, chosen on the Welcome page. There's no account yet, and the server takes settings only
// from one, so the choice waits in this browser and is saved as soon as the account is made.

// ui: the language of these pages; language: disc menus and sound tracks; zone: '' for the computer's own.
export type LocaleChoice = { language: string; zone: string; ui?: string }

const KEY = 'wms.setupLocale'

export function pendingLocale(): LocaleChoice | null {
  try {
    const v = JSON.parse(sessionStorage.getItem(KEY) || 'null') as LocaleChoice | null
    return v && typeof v.language === 'string' && typeof v.zone === 'string' ? v : null
  } catch { return null }
}

export function keepLocale(c: LocaleChoice) {
  try { sessionStorage.setItem(KEY, JSON.stringify(c)) } catch { /* private mode: it's saved with the account all the same */ }
}

// Saves what was chosen first, once there's an account. Subtitles stay as they are: a subtitle language would turn
// them on for discs.
export async function saveLocale(c: LocaleChoice | null = pendingLocale()) {
  if (!c) return
  await api.put('/settings', { timeZone: c.zone, audioLanguage: c.language, ...(c.language ? { menuLanguage: c.language } : {}), ...(c.ui ? { uiLanguage: c.ui } : {}) })
  try { sessionStorage.removeItem(KEY) } catch { /* nothing kept */ }
}

const deviceZone = () => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone || '' } catch { return '' } }

// What the Welcome page and its Customize dialog share: every change is kept at once, so the page redrawing in a newly
// picked language, or a reload, loses nothing.
export function useLocaleChoice() {
  const before = pendingLocale()
  const [ui, setUi] = useState(uiLanguageFor(before?.ui) || appLanguage())
  const [language, setTracks] = useState(before?.language ?? (browserLanguage() || appLanguage()))
  // Disc menus and sound tracks follow the pages' language until someone picks another for them.
  const [ownTracks, setOwnTracks] = useState(!!before && before.language !== trackLanguage(before.ui || ''))
  const [zone, setZone] = useState(before?.zone ?? '')
  const pickUi = (code: string) => {
    const tracks = ownTracks ? language : trackLanguage(code)
    setUi(code)
    setTracks(tracks)
    keepLocale({ language: tracks, zone, ui: code })
    void setLanguage(code)
  }
  const pickTracks = (v: string) => { setTracks(v); setOwnTracks(true); keepLocale({ language: v, zone, ui }) }
  const pickZone = (v: string) => { setZone(v); keepLocale({ language, zone: v, ui }) }
  const confirm = () => keepLocale({ language, zone, ui })
  return { ui, language, zone, pickUi, pickTracks, pickZone, confirm }
}

export type LocaleState = ReturnType<typeof useLocaleChoice>

export function LanguageField({ c }: { c: LocaleState }) {
  return (
    <Field label={t('Language')}>
      <Select value={c.ui} onChange={(e) => c.pickUi(e.target.value)} options={UI_LANGUAGES.map((value) => ({ value, label: value === c.ui ? nativeName(value) : `${nativeName(value)} (${languageNameHere(value)})` }))} />
    </Field>
  )
}

// The Welcome page's Customize: what follows the language and the computer unless someone says otherwise.
export function LocaleMore({ c }: { c: LocaleState }) {
  const here = deviceZone()
  return (
    <div className="su-net">
      <Field label={t('Disc menus and sound tracks')} hint={t('Follows the language unless you choose another.')}>
        <Select value={c.language} onChange={(e) => c.pickTracks(e.target.value)} options={[{ value: '', label: t('The original language') }, ...LANGUAGES.map(([value]) => ({ value, label: languageNameHere(value) }))]} />
      </Field>
      <Field label={t('Time zone')} hint={here ? t('This device is set to {zone}.', { zone: here.replace(/_/g, ' ') }) : undefined}>
        <Select value={c.zone} onChange={(e) => c.pickZone(e.target.value)} options={timeZoneChoices('', c.zone, undefined, { t, tn, list })} />
      </Field>
    </div>
  )
}

// Quick or advanced setup, chosen on Welcome and kept in this browser until setup is over.
const MODE = 'wms.setupAdvanced'
export const advancedSetup = () => { try { return localStorage.getItem(MODE) === '1' } catch { return false } }
export function setAdvancedSetup(on: boolean) {
  try { if (on) localStorage.setItem(MODE, '1'); else localStorage.removeItem(MODE) } catch { /* it goes back to quick setup on a reload */ }
}
