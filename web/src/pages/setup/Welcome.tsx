// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { api } from '../../api'
import { IconAudio, IconBook, IconGamepad, IconPalette, IconTv, IconUsers } from '../../icons'
import { Button, Field, Input } from '../../ui'
import { Customize, WhereIsTheCode } from './parts'
import { LanguageField, LocaleMore, useLocaleChoice } from './Locale'
import { applyPageTitle } from '../../theme'
import { t, tk } from '../../i18n'

const RestoreFlow = lazy(() => import('../settings/Backup').then((m) => ({ default: m.RestoreFlow })))

const FEATURES = [
  { icon: <IconTv />, title: tk('Movies, TV and discs'), text: tk('Posters, plots and “continue watching” on every screen. Copies of your discs play with their real menus.') },
  { icon: <IconAudio />, title: tk('Music and podcasts'), text: tk('Albums, audiobooks that remember your place, and shows you follow.') },
  { icon: <IconBook />, title: tk('Books and comics'), text: tk('Read them here, with highlights and notes.') },
  { icon: <IconGamepad />, title: tk('Games'), text: tk('Your own games, played right in the browser.') },
  { icon: <IconUsers />, title: tk('Everyone at home'), text: tk('A profile for each person, and Kids profiles by age.') },
  { icon: <IconPalette />, title: tk('Yours'), text: tk('It runs on your own computer. No account with anyone, and nothing is reported back.') },
]

// Step 1: the language (time zone and disc languages under Customize), then quick or advanced setup, or bring back a
// backup. Advanced setup asks every question on a page of its own; quick setup leaves them answered, under Customize.
export function Welcome({ onStart }: { onStart: (advanced: boolean) => void }) {
  const [restoring, setRestoring] = useState(false)
  // While a backup is coming back, starting something else would only get in its way.
  const [busy, setBusy] = useState(false)
  const locale = useLocaleChoice()
  useEffect(() => { applyPageTitle(t('Let’s set up your server')) }, [])
  return (
    <section className="oobe-welcome">
      <img className="oobe-welcome-logo" src="/icon.svg" alt="" />
      <h1>{t('Let’s set up your server')}</h1>
      <p className="lead">{t('It takes a few minutes, and you can change everything later.')}</p>
      {!busy && (
        <div className="su-welcome-locale" data-nav-group>
          <LanguageField c={locale} />
          <Customize title={t('Language and time zone')}><LocaleMore c={locale} /></Customize>
        </div>
      )}
      <div className="su-starts" data-nav-group>
        {!busy && <Button variant="primary" size="big" autoFocus onClick={() => { locale.confirm(); onStart(false) }}>{t('Get started')}</Button>}
        {!busy && <Button size="big" onClick={() => { locale.confirm(); onStart(true) }}>{t('Advanced setup')}</Button>}
      </div>
      {!busy && <p className="muted su-start-note">{t('Advanced setup goes through every option, one page at a time.')}</p>}
      {!busy && <p className="su-start-note"><Button variant="ghost" aria-expanded={restoring} onClick={() => setRestoring((o) => !o)}>{restoring ? t('Close restore') : t('Restore from a backup')}</Button></p>}
      {restoring && <RestorePanel onBusy={setBusy} />}
      <ul className="oobe-perks" aria-label={t('What your server does')}>
        {FEATURES.map((f) => <li key={f.title}><span className="ic">{f.icon}</span><span><b>{t(f.title)}</b> <span className="muted">{t(f.text)}</span></span></li>)}
      </ul>
    </section>
  )
}

// Restoring needs the setup code from another device, the same as making an account does.
function RestorePanel({ onBusy }: { onBusy: (b: boolean) => void }) {
  const [needCode, setNeedCode] = useState(false)
  const [code, setCode] = useState('')
  useEffect(() => { api.get<{ setupCodeRequired?: boolean }>('/status').then((s) => setNeedCode(!!s.setupCodeRequired)).catch(() => {}) }, [])
  return (
    <div className="su-restore">
      <p className="muted su-start-note">{t('Brings back your accounts, libraries, what you’ve watched and your settings. Modules download again afterwards.')}</p>
      {needCode && (
        <>
          <Field label={t('Setup code')}><Input autoComplete="off" autoCapitalize="characters" spellCheck={false} value={code} onChange={(e) => setCode(e.target.value)} /></Field>
          <WhereIsTheCode />
        </>
      )}
      {(!needCode || code.trim().length > 0) && <Suspense fallback={null}><RestoreFlow firstRun setupCode={needCode ? code.trim() : undefined} onBusy={onBusy} /></Suspense>}
    </div>
  )
}
