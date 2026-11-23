// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { api, type Library } from '../../api'
import { IconCheck, IconDevices, IconTv, IconUsers } from '../../icons'
import type { ModuleReport } from '../../moduleStore'
import { MusicOnlineStep, SeparatorStep } from '../MusicOnline'
import { Button, Dialog, Spinner } from '../../ui'
import { AddProfile } from '../Profiles'
import { setTvMode, useTvMode } from '../../tvmode/mode'
import { canMake, describeOutcome, type Facts, type Outcome } from './plan'
import { Customize, Option, TickRow } from './parts'
import { Details } from './Details'
import { Listings } from './Listings'
import { Logos } from './Logos'
import { Network } from './HomeNetwork'
import { list, t, tn } from '../../i18n'

// The same screen as Settings, Remote Access: one short code and a QR code for each device, details further in.
const ConnectDevice = lazy(() => import('../../components/ConnectDevice'))

// The last page says what's really so, from the server, rather than what was pressed along the way.
async function readOutcome(): Promise<Outcome> {
  const [libs, mods] = await Promise.all([
    api.get<Library[]>('/libraries').catch(() => [] as Library[]),
    api.get<ModuleReport>('/modules').catch(() => null),
  ])
  return {
    libraries: libs.map((l) => l.name),
    // What was added: what's part of the server was never a choice.
    modules: (mods?.modules || []).filter((m) => m.installed && !m.core && m.delivery !== 'builtin').map((m) => m.name),
  }
}

// Step 5: Ready. Start watching, with the three things people usually want next (another device, the family, this screen
// as a TV) one press away, and every optional extra under Customize. Each extra is off, or as it comes, until changed.
export function Done({ facts, onFinish }: { facts: Facts; onFinish: (to?: string) => void }) {
  const [outcome, setOutcome] = useState<Outcome | null>(null)
  const [add, setAdd] = useState<'' | 'adult' | 'kids'>('')
  const [added, setAdded] = useState(0)
  const [connect, setConnect] = useState(false)
  const [busy, setBusy] = useState(false)
  const tv = useTvMode()
  useEffect(() => { void readOutcome().then(setOutcome) }, [])
  const music = canMake(facts, 'music')
  const films = canMake(facts, 'movies') || canMake(facts, 'shows')
  const finish = (to?: string) => { setBusy(true); onFinish(to) }
  return (
    <section className="oobe-done">
      <div className="oobe-check" aria-hidden="true"><IconCheck size={44} /></div>
      <h1>{t('You’re all set')}</h1>
      <p className="lead">
        {outcome?.libraries.length
          ? tn('Your library is being scanned now. Titles and posters appear as each one is found.', 'Your {count} libraries are being scanned now. Titles and posters appear as each one is found.', outcome.libraries.length)
          : t('You can add your first library any time in Settings, Libraries.')}
      </p>
      {!outcome ? <Spinner /> : (
        <ul className="su-summary" aria-label={t('What you chose')}>
          {describeOutcome(outcome, { t, tn, list }).map((l) => <li key={l} className="on"><IconCheck size={16} /><span>{l}</span></li>)}
        </ul>
      )}
      <div className="oobe-next">
        <div className="oobe-next-card">
          <IconDevices size={22} />
          <b>{t('Watch on your other devices')}</b>
          <span>{t('Phones, TVs and computers connect with a short code, at home or away.')}</span>
          <Button size="sm" onClick={() => setConnect(true)}>{t('Connect a device')}</Button>
        </div>
        <div className="oobe-next-card">
          <IconUsers size={22} />
          <b>{t('Add the family')}</b>
          <span>{added ? tn('{count} profile added.', '{count} profiles added.', added) : t('A profile for each person. A Kids profile only shows titles for their age.')}</span>
          <span className="su-row">
            <Button size="sm" onClick={() => setAdd('adult')}>{t('Add a profile')}</Button>
            <Button size="sm" onClick={() => setAdd('kids')}>{t('Add a Kids profile')}</Button>
          </span>
        </div>
        <div className="oobe-next-card">
          <IconTv size={22} />
          <b>{t('This screen')}</b>
          <TickRow kind="switch" checked={tv} onChange={setTvMode} title={t('Use this screen as a TV')} text={t('Big and simple, for a remote or a controller. Only on this device.')} />
        </div>
      </div>
      {add && <AddProfile admin initialKind={add} onClose={() => setAdd('')} onDone={() => { setAdd(''); setAdded((n) => n + 1) }} />}
      {connect && (
        <Dialog title={t('Connect a device')} size="wide" onClose={() => setConnect(false)}>
          <Suspense fallback={<Spinner />}><ConnectDevice /></Suspense>
        </Dialog>
      )}
      <div className="oobe-actions oobe-finish">
        <Customize title={t('More options')}>
          <Option title={t('Home network')} text={t('Found automatically, on its usual port')}><Network /></Option>
          <Option title={t('Posters and details')} text={t('Free sources work already; keys are optional')}><Details facts={facts} /></Option>
          <Option title={t('Logos')} text={t('Plain names unless you choose logos')}><Logos /></Option>
          {music && <Option title={t('Lyrics and artists')} text={t('Off until you turn them on')}><MusicOnlineStep /></Option>}
          {music && <Option title={t('Karaoke voices')} text={t('The quick version works already')}><SeparatorStep /></Option>}
          {films && <Option title={t('Streaming lists')} text={t('Off until you turn them on')}><Listings /></Option>}
        </Customize>
        <Button variant="primary" size="big" busy={busy} autoFocus onClick={() => finish()}>{t('Start watching')}</Button>
      </div>
    </section>
  )
}
