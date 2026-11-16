// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api } from '../../api'
import { LANGUAGES, browserLanguage } from '../../languages'
import { language as appLanguage, languageNameHere, nativeName, setLanguage, t } from '../../i18n'
import { Button, Field, Select } from '../../ui'
import { timeZoneChoices } from './network'
import { StepActions, StepHead } from './parts'

// The first thing setup asks, before Welcome: the language and the time zone. There's no account yet, and the server
// takes settings only from one, so the choice waits in this browser and is saved as soon as the account is made.

// ui: the language of these pages; language: disc menus and sound tracks.
// done: Continue was pressed (a language picked but not yet confirmed is kept too, as the page redraws in it).
export type LocaleChoice = { language: string; zone: string; ui?: string; done?: boolean }

const KEY = 'wms.setupLocale'

/** Whether the language and time zone were confirmed with Continue. */
export const localeAsked = () => !!pendingLocale()?.done

export function pendingLocale(): LocaleChoice | null {
  try {
    const v = JSON.parse(sessionStorage.getItem(KEY) || 'null') as LocaleChoice | null
    return v && typeof v.language === 'string' && typeof v.zone === 'string' ? v : null
  } catch { return null }
}

function keep(c: LocaleChoice) {
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

export function LocaleFirst({ onDone }: { onDone: (c: LocaleChoice) => void }) {
  const before = pendingLocale()
  const [ui, setUi] = useState(before?.ui ?? appLanguage())
  const [language, setLanguageChoice] = useState(before?.language ?? (browserLanguage() || appLanguage()))
  // Disc menus and sound tracks follow the pages' language until someone picks another for them.
  const [ownTracks, setOwnTracks] = useState(!!before && before.language !== before.ui)
  const [zone, setZone] = useState(before?.zone ?? '')
  const here = deviceZone()
  // The pages switch as soon as a language is picked, and the choice is kept so the switch doesn't lose it.
  const pickUi = (code: string) => {
    setUi(code)
    const tracks = ownTracks ? language : code
    if (!ownTracks) setLanguageChoice(code)
    keep({ language: tracks, zone, ui: code })
    void setLanguage(code)
  }
  const go = () => { const c = { language, zone, ui, done: true }; keep(c); onDone(c) }
  const zoneHint = zone
    ? t('Guide days, schedules, recordings and the logs follow this. The server restarts once to use it, when setup is finished, if it isn’t the one it’s running in.')
    : t('The time zone of the computer the server runs on. Guide days, schedules, recordings and the logs follow it.')
  return (
    <section className="su-locale">
      <StepHead title={t('Language and time zone')} lead={t('Before anything else: the language for these pages, disc menus and sound tracks, and the time zone the server keeps.')} />
      <div className="su-net">
        <Field label={t('Language')} hint={t('Everything on these pages, for everyone who uses this server.')}>
          <Select autoFocus value={ui} onChange={(e) => pickUi(e.target.value)} options={LANGUAGES.map(([value]) => ({ value, label: value === ui ? nativeName(value) : `${nativeName(value)} (${languageNameHere(value)})` }))} />
        </Field>
        <Field label={t('Disc menus and sound tracks')} hint={t('Disc menus, and the sound track a film or show starts with when it has one in this language. Subtitles stay off until someone turns them on.')}>
          <Select value={language} onChange={(e) => { setLanguageChoice(e.target.value); setOwnTracks(true) }} options={[{ value: '', label: t('The original language') }, ...LANGUAGES.map(([value]) => ({ value, label: languageNameHere(value) }))]} />
        </Field>
        <Field label={t('Time zone')} hint={`${zoneHint}${here ? ` ${t('This device is set to {zone}.', { zone: here.replace(/_/g, ' ') })}` : ''}`}>
          <Select value={zone} onChange={(e) => setZone(e.target.value)} options={timeZoneChoices('', zone)} />
        </Field>
      </div>
      <p className="su-later">{t('You can change these again on the Name and address step, and later in Settings.')}</p>
      <StepActions><Button variant="primary" size="big" onClick={go}>{t('Continue')}</Button></StepActions>
    </section>
  )
}
