// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, type Library, type User } from '../../api'
import { IconCheck, IconInfo, IconPlug, IconTv, IconUsers } from '../../icons'
import type { ModuleReport } from '../../moduleStore'
import type { ListingStatus } from '../../metadata'
import { Button, Spinner } from '../../ui'
import { QrCode } from '../../components/QrCode'
import { toast } from '../../state'
import { copyText } from '../../gallery'
import { bestAddress, type HomeAddresses } from './address'
import { AddProfile } from '../Profiles'
import { setTvMode, useTvMode } from '../../tvmode/mode'
import { describeOutcome, updateChoiceFrom, type Facts, type Outcome, type StepInfo } from './plan'
import { Fill, TickRow } from './parts'
import { language, list, t, tn, tx } from '../../i18n'

// The last page says what's really so, from the server, rather than what was pressed along the way.
async function readOutcome(): Promise<Outcome> {
  const [settings, libs, logos, listings, mods] = await Promise.all([
    api.get<Record<string, string>>('/settings').catch(() => ({} as Record<string, string>)),
    api.get<Library[]>('/libraries').catch(() => [] as Library[]),
    api.get<{ consent: '' | 'yes' | 'no' }>('/logos/status').catch(() => ({ consent: '' as const })),
    api.get<ListingStatus>('/listings/status').catch(() => null),
    api.get<ModuleReport>('/modules').catch(() => null),
  ])
  const names = new Intl.DisplayNames([language()], { type: 'region' })
  return {
    libraries: libs.map((l) => l.name),
    // What was added: what's part of the server was never a choice.
    modules: (mods?.modules || []).filter((m) => m.installed && !m.core && m.delivery !== 'builtin').map((m) => m.name),
    logos: logos.consent,
    lyrics: settings.lyricsOnline === '1',
    artists: settings.musicInfoOnline === '1',
    listings: !!listings?.enabled,
    region: listings?.region ? names.of(listings.region) || listings.region : '',
    updates: updateChoiceFrom(settings),
  }
}

export function Done({ steps, facts, onFinish }: { steps: StepInfo[]; facts: Facts; onFinish: (to?: string) => void }) {
  const [outcome, setOutcome] = useState<Outcome | null>(null)
  const [home, setHome] = useState<HomeAddresses | null>(null)
  const [addKids, setAddKids] = useState(false)
  const [kidsAdded, setKidsAdded] = useState(false)
  const [busy, setBusy] = useState(false)
  // Whether peer-to-peer connections were turned on, on the Away from home step.
  const [away, setAway] = useState<boolean | null>(null)
  // A kids profile made on the People step: then there's nothing to offer here.
  const [hasKids, setHasKids] = useState(false)
  const tv = useTvMode()
  useEffect(() => {
    void readOutcome().then(setOutcome)
    api.get<{ settings?: { enabled?: boolean } }>('/p2p/settings').then((r) => setAway(!!r.settings?.enabled)).catch(() => setAway(null))
    api.get<User[]>('/users').then((u) => setHasKids(u.some((x) => x.kind === 'kids'))).catch(() => {})
    api.get<HomeAddresses>('/system').then(setHome).catch(() => setHome({}))
  }, [])
  const tuner = facts.modules.includes('wittytuner')
  // Always a best guess at the server's address, to write down or copy (#536): the home-network one when other devices
  // can open it, else the one this page uses, with where it will be once it's opened to the network.
  const addr = bestAddress(location, home)
  const copy = async () => { toast((await copyText(addr.url)) ? t('Address copied') : t('Couldn’t copy it: select the address and copy it')) }
  const lines = outcome ? describeOutcome(outcome, steps, { t, tn, list }) : []
  const where = (id: string) => tx(steps.find((s) => s.id === id)?.later?.where)
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
          {lines.map((l) => (
            <li key={l.text} className={l.on ? 'on' : ''}>
              {l.on ? <IconCheck size={16} /> : <IconInfo size={16} />}
              <span>{l.text}<small>{where(l.step)}</small></span>
            </li>
          ))}
          {away !== null && (
            <li className={away ? 'on' : ''}>
              {away ? <IconCheck size={16} /> : <IconInfo size={16} />}
              <span>{away ? t('Away from home: on, through peer-to-peer connections') : t('Away from home: off, so it works on your home network')}<small>{where('remote')}</small></span>
            </li>
          )}
        </ul>
      )}
      <div className="oobe-next">
        <div className="oobe-next-card">
          <IconTv size={22} />
          <b>{t('Watch on every screen at home')}</b>
          {addr.reachable
            ? <span>{t('On the same home network (Wi-Fi or cable), open this address on your TV, phone, tablet or another computer:')}</span>
            : <span>{t('Right now this server only answers on this computer, at:')}</span>}
          <div className="su-address">
            <div className="su-address-text">
              <code className="su-address-url">{addr.url}</code>
              <Button size="sm" onClick={() => void copy()} aria-label={t('Copy the address {url}', { url: addr.url })}>{t('Copy')}</Button>
              {addr.also && <small className="muted">{t('It also answers at {url}.', { url: addr.also })}</small>}
            </div>
            {addr.reachable && home && <QrCode text={addr.url} label={t('QR code holding the address {url}', { url: addr.url })} />}
          </div>
          {!addr.reachable && <span><Fill text={t('To use it from your TV or phone, set {listen} to {port} where it’s set (its config file, {flag} or {env}) and restart it.')} parts={{ listen: <code>listen</code>, port: <code>:{location.port || 9797}</code>, flag: <code>-listen</code>, env: <code>WMS_LISTEN</code> }} /></span>}
          {!addr.reachable && addr.later && <span><Fill text={t('Then other devices at home can open it at {url}.')} parts={{ url: <code>{addr.later}</code> }} /></span>}
          <TickRow kind="switch" checked={tv} onChange={setTvMode} title={t('Use this screen as a TV')} text={t('TV mode: laid out and driven like the TV apps, with a remote or a controller, for a screen across the room. Only on this device.')} />
        </div>
        {!hasKids && (
          <div className="oobe-next-card">
            <IconUsers size={22} />
            <b>{t('Add the family')}</b>
            <span>{kidsAdded ? t('Kids profile added. Add more in Settings, Profiles.') : t('A Kids profile only shows titles for their age.')}</span>
            {!kidsAdded && <Button size="sm" variant="ghost" onClick={() => setAddKids(true)}>{t('Add a Kids profile')}</Button>}
          </div>
        )}
        <div className="oobe-next-card">
          <IconPlug size={22} />
          <b>{t('Also here when you want them')}</b>
          <span><Fill text={t('{name} converts your files ahead of time, so they play anywhere without the server working as hard. It rewrites your files, so it stays off until you turn it on in Settings, Library transcoding.')} parts={{ name: <b>{t('Library transcoding')}</b> }} /></span>
          <span><Fill text={t('{name} are made from your libraries in one press, on the guide’s Channels page ({button}), once they’ve been scanned.')} parts={{ name: <b>{t('TV channels')}</b>, button: <b>{t('Edit channels')}</b> }} /></span>
          {!tuner && <span><Fill text={t('{name} from a tuner box, a TV card or an internet TV list comes with the WittyTuner module, in Settings, Module store.')} parts={{ name: <b>{t('Live TV')}</b> }} /></span>}
        </div>
      </div>
      {addKids && <AddProfile admin initialKind="kids" onClose={() => setAddKids(false)} onDone={() => { setAddKids(false); setKidsAdded(true) }} />}
      <div className="oobe-actions oobe-finish">
        <Button size="big" busy={busy} onClick={() => finish('/settings')}>{t('Open Settings')}</Button>
        <Button variant="primary" size="big" busy={busy} autoFocus onClick={() => finish()}>{t('Start watching')}</Button>
      </div>
    </section>
  )
}
