import { useEffect, useState } from 'react'
import { api, type Library } from '../../api'
import { IconCheck, IconInfo, IconPlug, IconTv, IconUsers } from '../../icons'
import type { ModuleReport } from '../../moduleStore'
import type { ListingStatus } from '../../metadata'
import { Button, Spinner } from '../../ui'
import { AddProfile } from '../Profiles'
import { PortSetting, useNetwork } from '../../components/PortSetting'
import { describeOutcome, updateChoiceFrom, type Outcome, type StepInfo } from './plan'

// The last page says what's really so, from the server, rather than what was pressed along the way.
async function readOutcome(): Promise<Outcome> {
  const [settings, libs, logos, listings, mods] = await Promise.all([
    api.get<Record<string, string>>('/settings').catch(() => ({} as Record<string, string>)),
    api.get<Library[]>('/libraries').catch(() => [] as Library[]),
    api.get<{ consent: '' | 'yes' | 'no' }>('/logos/status').catch(() => ({ consent: '' as const })),
    api.get<ListingStatus>('/listings/status').catch(() => null),
    api.get<ModuleReport>('/modules').catch(() => null),
  ])
  const names = new Intl.DisplayNames(undefined, { type: 'region' })
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
  useEffect(() => {
    void readOutcome().then(setOutcome)
    api.get<{ lanUrls?: string[] }>('/system').then((s) => setLan(s.lanUrls || [])).catch(() => setLan([]))
  }, [])
  // Addresses other devices at home can open. This one may only be "localhost".
  const here = /^(localhost|127\.|\[::1\])/.test(location.hostname) || location.hostname.endsWith('.localhost') ? '' : location.origin
  const addrs = here ? [here] : lan || []
  const lines = outcome ? describeOutcome(outcome, steps) : []
  const where = (id: string) => steps.find((s) => s.id === id)?.later?.where
  const finish = (to?: string) => { setBusy(true); onFinish(to) }
  return (
    <section className="oobe-done">
      <div className="oobe-check" aria-hidden="true"><IconCheck size={44} /></div>
      <h1>You’re all set</h1>
      <p className="lead">
        {outcome?.libraries.length
          ? `${outcome.libraries.length === 1 ? 'Your library is' : `Your ${outcome.libraries.length} libraries are`} being scanned now. Titles and posters appear as each one is found.`
          : 'You can add your first library any time in Settings, Libraries.'}
      </p>
      {!outcome ? <Spinner /> : (
        <ul className="su-summary" aria-label="What you chose">
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
          <b>Watch on every screen at home</b>
          {addrs.length > 0
            ? <><span>On the same home network (Wi-Fi or cable), open this address on your TV, phone, tablet or another computer:</span>{addrs.slice(0, 2).map((a) => <code key={a}>{a}</code>)}</>
            : <span>Right now this server only answers on this computer. To use it from your TV or phone, set <code>listen</code> to <code>:{net.view?.port || 9797}</code> where it’s set (its config file, <code>-listen</code> or <code>WMS_LISTEN</code>) and restart it.</span>}
          {net.view && (portOpen
            ? <PortSetting view={net.view} onSaved={net.setView} compact />
            : <span className="su-port">Port {net.view.next}{net.view.next !== net.view.port && ' after a restart'}. <Button size="sm" variant="ghost" onClick={() => setPortOpen(true)}>Change the port</Button></span>)}
          <small className="oobe-disclaimer">Your server runs in your home, so for now it works only on your home network. To watch away from home, follow the steps in Settings, Remote Access when you’re ready.</small>
        </div>
        <div className="oobe-next-card">
          <IconUsers size={22} />
          <b>Add the family</b>
          <span>{kidsAdded ? 'Kids profile added. Add more in Settings, Profiles.' : 'A Kids profile only shows titles for their age.'}</span>
          {!kidsAdded && <Button size="sm" variant="ghost" onClick={() => setAddKids(true)}>Add a Kids profile</Button>}
        </div>
        <div className="oobe-next-card">
          <IconPlug size={22} />
          <b>Also here when you want them</b>
          <span><b>Library transcoding</b> converts your files ahead of time, so they play anywhere without the server working as hard. It rewrites your files, so it stays off until you turn it on in Settings, Library transcoding.</span>
          <span><b>TV channels</b> are made from your libraries in one press, in Settings, Channels &amp; tuners, once they’ve been scanned.</span>
        </div>
      </div>
      {addKids && <AddProfile admin initialKind="kids" onClose={() => setAddKids(false)} onDone={() => { setAddKids(false); setKidsAdded(true) }} />}
      <div className="oobe-actions oobe-finish">
        <Button size="big" busy={busy} onClick={() => finish('/settings')}>Open Settings</Button>
        <Button variant="primary" size="big" busy={busy} autoFocus onClick={() => finish()}>Start watching</Button>
      </div>
    </section>
  )
}
