// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { api, type Library } from '../../api'
import { IconCheck, IconInfo, IconPlug, IconTv, IconUsers } from '../../icons'
import type { ModuleReport } from '../../moduleStore'
import type { ListingStatus } from '../../metadata'
import { Button, Spinner } from '../../ui'
import { AddProfile } from '../Profiles'
import { PortSetting, useNetwork } from '../../components/PortSetting'
import { describeOutcome, updateChoiceFrom, type Outcome, type StepInfo } from './plan'
import { Fill } from './parts'
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

export function Done({ steps, onFinish }: { steps: StepInfo[]; onFinish: (to?: string) => void }) {
  const [outcome, setOutcome] = useState<Outcome | null>(null)
  const [lan, setLan] = useState<string[] | null>(null)
  const [addKids, setAddKids] = useState(false)
  const [kidsAdded, setKidsAdded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [portOpen, setPortOpen] = useState(false)
  const net = useNetwork()
  // In the words Settings, Remote Access uses, from the one place they're written.
  const [remote, setRemote] = useState<{ subject: string; away: string } | null>(null)
  useEffect(() => {
    void readOutcome().then(setOutcome)
    api.get<{ subject: string; away: string }>('/remote/status').then(setRemote).catch(() => setRemote(null))
    api.get<{ lanUrls?: string[] }>('/system').then((s) => setLan(s.lanUrls || [])).catch(() => setLan([]))
  }, [])
  // Addresses other devices at home can open. This one may only be "localhost".
  const here = /^(localhost|127\.|\[::1\])/.test(location.hostname) || location.hostname.endsWith('.localhost') ? '' : location.origin
  const addrs = here ? [here] : lan || []
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
        </ul>
      )}
      <div className="oobe-next">
        <div className="oobe-next-card">
          <IconTv size={22} />
          <b>{t('Watch on every screen at home')}</b>
          {addrs.length > 0
            ? <><span>{t('On the same home network (Wi-Fi or cable), open this address on your TV, phone, tablet or another computer:')}</span>{addrs.slice(0, 2).map((a) => <code key={a}>{a}</code>)}</>
            : <span><Fill text={t('Right now this server only answers on this computer. To use it from your TV or phone, set {listen} to {port} where it’s set (its config file, {flag} or {env}) and restart it.')} parts={{ listen: <code>listen</code>, port: <code>:{net.view?.port || 9797}</code>, flag: <code>-listen</code>, env: <code>WMS_LISTEN</code> }} /></span>}
          {net.view && (portOpen
            ? <PortSetting view={net.view} onSaved={net.setView} compact />
            : <span className="su-port">{net.view.next !== net.view.port ? t('Port {port} after a restart.', { port: String(net.view.next) }) : t('Port {port}.', { port: String(net.view.next) })} <Button size="sm" variant="ghost" onClick={() => setPortOpen(true)}>{t('Change the port')}</Button></span>)}
          <small className="oobe-disclaimer">{remote ? <><b>{tx(remote.subject)}.</b> {tx(remote.away)}. </> : `${t('Your server runs in your home, so for now it works only on your home network.')} `}{t('To watch away from home, choose how in Settings, Remote Access when you’re ready.')}</small>
        </div>
        <div className="oobe-next-card">
          <IconUsers size={22} />
          <b>{t('Add the family')}</b>
          <span>{kidsAdded ? t('Kids profile added. Add more in Settings, Profiles.') : t('A Kids profile only shows titles for their age.')}</span>
          {!kidsAdded && <Button size="sm" variant="ghost" onClick={() => setAddKids(true)}>{t('Add a Kids profile')}</Button>}
        </div>
        <div className="oobe-next-card">
          <IconPlug size={22} />
          <b>{t('Also here when you want them')}</b>
          <span><Fill text={t('{name} converts your files ahead of time, so they play anywhere without the server working as hard. It rewrites your files, so it stays off until you turn it on in Settings, Library transcoding.')} parts={{ name: <b>{t('Library transcoding')}</b> }} /></span>
          <span><Fill text={t('{name} are made from your libraries in one press, on the guide’s Channels page ({button}), once they’ve been scanned.')} parts={{ name: <b>{t('TV channels')}</b>, button: <b>{t('Edit channels')}</b> }} /></span>
          <span><Fill text={t('{name} from a tuner box, a TV card or a provider’s playlist: {button} in Settings, Channels & tuners walks you through it, finding what it can and taking the rest by hand.')} parts={{ name: <b>{t('Live TV')}</b>, button: <b>{t('Add live TV')}</b> }} /></span>
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
