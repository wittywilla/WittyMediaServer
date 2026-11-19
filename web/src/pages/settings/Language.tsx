// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { language, languageNameHere, list, nativeName, setLanguage, t, tk, tn } from '../../i18n'
import { LANGUAGES, UI_LANGUAGES, uiLanguageFor } from '../../languages'
import { toast } from '../../state'
import { timeZoneChoices } from '../setup/network'
import { Row, Section } from './ui'

// The server's language and time zone, first chosen at the very start of setup: the language of these pages (uiLanguage),
// the one disc menus and sound tracks start in (menuLanguage, audioLanguage), and the zone guide days, schedules,
// recordings and the logs follow (timeZone). Each is saved as soon as it's picked.
export default function LanguageSettings() {
  const [code, setCode] = useState(language())
  const [s, setS] = useState<Record<string, string> | null>(null)
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((v) => { setS(v); if (v.uiLanguage) setCode(uiLanguageFor(v.uiLanguage) || language()) }).catch(() => {}) }, [])
  const choose = async (next: string) => {
    const was = code
    setCode(next); setBusy(true)
    try {
      await api.put('/settings', { uiLanguage: next })
      await setLanguage(next)
      toast(t('The app is now in {language}', { language: nativeName(next) }))
    } catch (e: any) { setCode(was); toast(e.message) } finally { setBusy(false) }
  }
  const put = async (change: Record<string, string>, said: string) => {
    const was = s
    setS((x) => ({ ...(x || {}), ...change })); setBusy(true)
    try { await api.put('/settings', change); toast(said) } catch (e: any) { setS(was); toast(e.message) } finally { setBusy(false) }
  }
  const tracks = s ? s.audioLanguage || '' : ''
  const zone = s?.timeZone || ''
  const restart = !!s && zone !== '' && zone !== s.timeZoneNow
  return (
    <div id="language">
      <Section title={tk('Language and time zone')}>
        <Row label={t('Language of these pages')} desc={t('The words of the app, for everyone using this server. Films, shows and their details keep their own languages.')}>
          <select className="select" value={code} disabled={busy} onChange={(e) => void choose(e.target.value)}>
            {UI_LANGUAGES.map((c) => <option key={c} value={c} lang={c}>{nativeName(c)}{c === language() ? '' : ` (${languageNameHere(c)})`}</option>)}
          </select>
        </Row>
        {s && <>
          <Row label={t('Disc menus and sound tracks')} desc={t('Disc menus, and the sound track a film or show starts with when it has one in this language. Subtitles stay off until someone turns them on.')}>
            <select className="select" value={tracks} disabled={busy} aria-label={t('Disc menus and sound tracks')} onChange={(e) => void put({ audioLanguage: e.target.value, ...(e.target.value ? { menuLanguage: e.target.value } : {}) }, t('Saved'))}>
              <option value="">{t('The original language')}</option>
              {LANGUAGES.map(([value]) => <option key={value} value={value}>{languageNameHere(value)}</option>)}
            </select>
          </Row>
          <Row label={t('Time zone')} desc={restart ? t('Guide days, schedules, recordings and the logs follow this. The server is restarting to use it.') : t('Guide days, schedules, recordings and the logs follow this. Choosing another restarts the server once, which takes a few seconds.')}>
            <select className="select" value={zone} disabled={busy} aria-label={t('Time zone')} onChange={(e) => void put({ timeZone: e.target.value }, t('Saved'))}>
              {timeZoneChoices(s.timeZoneComputer || '', zone, undefined, { t, tn, list }).map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
            </select>
          </Row>
        </>}
      </Section>
    </div>
  )
}
