// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { lazy, Suspense, useEffect, useState } from 'react'
import { api, type Library } from '../../api'
import { IconCheck, IconDevices, IconTv, IconUsers } from '../../icons'
import type { ModuleReport } from '../../moduleStore'
import { MusicOnlineStep, SeparatorStep } from '../MusicOnline'
import { Button, Dialog, Spinner } from '../../ui'
import { toast } from '../../state'
import { copyText } from '../../gallery'
import { bestAddress, type HomeAddresses } from './address'
import { AddProfile } from '../Profiles'
import { setTvMode, useTvMode } from '../../tvmode/mode'
import { canMake, describeOutcome, type Facts, type Outcome } from './plan'
import { Customize, Option, TickRow } from './parts'
import { Details } from './Details'
import { Listings } from './Listings'
import { Logos } from './Logos'
import { HomeNetwork } from './HomeNetwork'
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

// Step 6: Ready. Start watching, with the three things people usually want next (another device, the family, this screen
// as a TV) one press away, and every optional extra under Customize (advanced setup has had a page for each). Each extra
// is off, or as it comes, until changed.
export function Done({ facts, onFinish }: { facts: Facts; onFinish: (to?: string) => void }) {
  const [outcome, setOutcome] = useState<Outcome | null>(null)
  const [add, setAdd] = useState<'' | 'adult' | 'kids'>('')
  const [added, setAdded] = useState(0)
  const [connect, setConnect] = useState(false)
  const [busy, setBusy] = useState(false)
  const tv = useTvMode()
  const [home, setHome] = useState<HomeAddresses | null>(null)
  useEffect(() => {
    void readOutcome().then(setOutcome)
    api.get<HomeAddresses>('/system').then(setHome).catch(() => setHome({}))
  }, [])
  // Always a best guess at the server's address, to write down or copy (#536): the home-network one when other devices
  // can open it, else the one this page uses.
  const addr = bestAddress(location, home)
  const copy = async () => { toast((await copyText(addr.url)) ? t('Address copied') : t('Couldn’t copy it: select the address and copy it')) }
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
          <div className="su-address">
            <div className="su-address-text">
              <code className="su-address-url">{addr.url}</code>
              <Button size="sm" variant="ghost" onClick={() => void copy()} aria-label={t('Copy the address {url}', { url: addr.url })}>{t('Copy')}</Button>
              {!addr.reachable && <small className="muted">{t('Right now this server only answers on this computer.')}</small>}
            </div>
          </div>
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
        <Dialog title={t('Connect a device')} size="wide" className="su-connect" onClose={() => setConnect(false)}>
          <Suspense fallback={<Spinner />}><ConnectDevice /></Suspense>
        </Dialog>
      )}
      <div className="oobe-actions oobe-finish">
        {!facts.advanced && <Customize title={t('More options')}>
          <Option title={t('Home network')} text={t('How apps at home find this server')}><HomeNetwork /></Option>
          <Option title={t('Posters and details')} text={t('More sources, and details already beside your files')}><Details facts={facts} /></Option>
          <Option title={t('Logos')} text={t('Real logos on cards instead of plain names')}><Logos /></Option>
          {music && <Option title={t('Lyrics and artists')} text={t('Lyrics and artist pages from the web')}><MusicOnlineStep /></Option>}
          {music && <Option title={t('Karaoke voices')} text={t('Cleaner voice removal, a large download')}><SeparatorStep /></Option>}
          {films && <Option title={t('Streaming lists')} text={t('Which streaming services list your titles')}><Listings /></Option>}
        </Customize>}
        <Button variant="primary" size="big" busy={busy} autoFocus onClick={() => finish()}>{t('Start watching')}</Button>
      </div>
    </section>
  )
}
