// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api } from '../../api'
import { LANGUAGES, browserLanguage } from '../../languages'
import { Button, Field, Select } from '../../ui'
import { timeZoneChoices } from './network'
import { StepActions, StepHead } from './parts'

// The first thing setup asks, before Welcome: the language and the time zone. There's no account yet, and the server
// takes settings only from one, so the choice waits in this browser and is saved as soon as the account is made.

export type LocaleChoice = { language: string; zone: string }

const KEY = 'wms.setupLocale'

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
  await api.put('/settings', { timeZone: c.zone, audioLanguage: c.language, ...(c.language ? { menuLanguage: c.language } : {}) })
  try { sessionStorage.removeItem(KEY) } catch { /* nothing kept */ }
}

const deviceZone = () => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone || '' } catch { return '' } }

export function LocaleFirst({ onDone }: { onDone: (c: LocaleChoice) => void }) {
  const before = pendingLocale()
  const [language, setLanguage] = useState(before?.language ?? browserLanguage())
  const [zone, setZone] = useState(before?.zone ?? '')
  const here = deviceZone()
  const go = () => { const c = { language, zone }; keep(c); onDone(c) }
  return (
    <section className="su-locale">
      <StepHead title="Language and time zone" lead="Before anything else: the language for disc menus and sound tracks, and the time zone the server keeps." />
      <div className="su-net">
        <Field label="Language" hint="Disc menus, and the sound track a film or show starts with when it has one in this language. Subtitles stay off until someone turns them on. This server’s own pages are in English for now.">
          <Select autoFocus value={language} onChange={(e) => setLanguage(e.target.value)} options={[{ value: '', label: 'The original language' }, ...LANGUAGES.map(([value, label]) => ({ value, label }))]} />
        </Field>
        <Field label="Time zone" hint={`${zone ? 'Guide days, schedules, recordings and the logs follow this. The server restarts once to use it, when setup is finished, if it isn’t the one it’s running in.' : 'The time zone of the computer the server runs on. Guide days, schedules, recordings and the logs follow it.'}${here ? ` This device is set to ${here.replace(/_/g, ' ')}.` : ''}`}>
          <Select value={zone} onChange={(e) => setZone(e.target.value)} options={timeZoneChoices('', zone)} />
        </Field>
      </div>
      <p className="su-later">You can change both again on the Name and address step.</p>
      <StepActions><Button variant="primary" size="big" onClick={go}>Continue</Button></StepActions>
    </section>
  )
}
