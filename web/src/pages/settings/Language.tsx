// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { language, languageNameHere, nativeName, setLanguage, t, tk } from '../../i18n'
import { UI_LANGUAGES, uiLanguageFor } from '../../languages'
import { toast } from '../../state'
import { Row, Section } from './ui'

// The language of these pages, for everyone on this server (the uiLanguage setting, first chosen in setup).
// It's saved as soon as it's picked, and the app switches to it straight away.
export default function LanguageSettings() {
  const [code, setCode] = useState(language())
  const [busy, setBusy] = useState(false)
  useEffect(() => { api.get<Record<string, string>>('/settings').then((s) => { if (s.uiLanguage) setCode(uiLanguageFor(s.uiLanguage) || language()) }).catch(() => {}) }, [])
  const choose = async (next: string) => {
    const was = code
    setCode(next); setBusy(true)
    try {
      await api.put('/settings', { uiLanguage: next })
      await setLanguage(next)
      toast(t('The app is now in {language}', { language: nativeName(next) }))
    } catch (e: any) { setCode(was); toast(e.message) } finally { setBusy(false) }
  }
  return (
    <Section title={tk('Language')}>
      <Row label={t('Language of these pages')} desc={t('The words of the app, for everyone using this server. Films, shows and their details keep their own languages.')}>
        <select className="select" value={code} disabled={busy} onChange={(e) => void choose(e.target.value)}>
          {UI_LANGUAGES.map((c) => <option key={c} value={c} lang={c}>{nativeName(c)}{c === language() ? '' : ` (${languageNameHere(c)})`}</option>)}
        </select>
      </Row>
    </Section>
  )
}
