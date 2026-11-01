import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import { forgetLogos, type LogoInfo } from '../../components/BrandLogo'
import LegalNote from '../../components/LegalNote'
import type { ListingStatus } from '../../metadata'
import { toast } from '../../state'
import { Row, Section, Switch } from './ui'
import './metadataoptional.css'

// The optional downloads: logos, and which streaming services list a title. Both are off until
// someone reads what they mean and says yes. First-time setup can show the same consent parts.

type LogoStatus = { consent: 'yes' | 'no' | ''; counts: Record<string, number>; source: string; sourceUrl: string }

// The words someone agrees to before logos are downloaded.
export function LogoConsentWords() {
  return (
    <ul className="set-consent">
      <li>Logos of studios, networks, consoles and services are <b>not part of this software</b>. They're downloaded to your server, one at a time, from <a href="https://commons.wikimedia.org/wiki/Commons:Licensing" target="_blank" rel="noreferrer">Wikimedia Commons</a>.</li>
      <li>Your server tells Wikidata and Wikimedia Commons the <b>name</b> of each studio, network or console it finds, and nothing else: nothing about you or your files.</li>
      <li>Only files whose own page says they're free to reuse are taken. Each one's source, licence and author are listed here.</li>
      <li><b>Logos are still trademarks of their owners.</b> They're shown only to name who made something, for your own use on your own server. If you share your server, that's for you to judge.</li>
      <li>Without this, the app shows names in plain text, and nothing else changes. You can upload your own logos either way.</li>
    </ul>
  )
}

export function LogoSettings() {
  const [s, setS] = useState<LogoStatus | null>(null)
  const [credits, setCredits] = useState<LogoInfo[] | null>(null)
  const load = useCallback(() => api.get<LogoStatus>('/logos/status').then(setS).catch(() => {}), [])
  useEffect(() => { load() }, [load])
  // While it's fetching, and only then, look in on it.
  useEffect(() => {
    if (s?.consent !== 'yes' || !s.counts.pending) return
    const t = setTimeout(load, 5000)
    return () => clearTimeout(t)
  }, [s, load])
  const answer = async (accepted: boolean, removeDownloaded = false) => {
    try { setS(await api.put<LogoStatus>('/logos/consent', { accepted, removeDownloaded })); forgetLogos() } catch (e: any) { toast(e.message) }
  }
  if (!s) return null
  return (
    <Section id="logos" title="Logos" desc="Logos on the cards for studios and networks, and for anything else that has one.">
      {s.consent === 'yes' ? (
        <>
          <Row label="Download logos" desc={`${s.counts.found} found, ${s.counts.pending} still to look for, ${s.counts.none} that no source has.`}>
            <Switch label="Download logos" checked onChange={() => answer(false)} />
          </Row>
          <Row label="Remove what was downloaded" desc="Turns downloads off and deletes the pictures. Logos you uploaded stay.">
            <button type="button" className="btn sm" onClick={() => answer(false, true)}>Remove them</button>
          </Row>
          <Row label="Where each logo came from" desc="The source, licence and author of each one, as their licences ask. Each was recoloured here to white and to dark." stack>
            {credits ? (
              credits.length === 0 ? <p className="muted">None yet.</p> : (
                <ul className="set-credits">
                  {credits.map((c) => <li key={c.id}><b>{c.name}</b>: {c.source === 'you' ? 'uploaded by you' : <><a href={c.sourceUrl} target="_blank" rel="noreferrer">{c.license}</a>{c.credit ? `, by ${c.credit}` : ''}</>}</li>)}
                </ul>
              )
            ) : <button type="button" className="btn sm" onClick={() => api.get<{ logos: LogoInfo[] }>('/logos/credits').then((r) => setCredits(r.logos)).catch((e) => toast(e.message))}>Show the list</button>}
          </Row>
        </>
      ) : (
        <div className="set-consent-box">
          <LogoConsentWords />
          <div className="set-consent-btns">
            <button type="button" className="btn primary" onClick={() => answer(true)}>I understand: download logos</button>
            {s.consent === '' && <button type="button" className="btn ghost" onClick={() => answer(false)}>No thanks: use plain names</button>}
          </div>
        </div>
      )}
      <LegalNote kind="logos" className="set-legal" />
    </Section>
  )
}

// What someone has to accept, one at a time, before listings can be turned on.
export const LISTING_STATEMENTS: { id: string; text: string }[] = [
  { id: 'not-streaming', text: 'Nothing streams from these services. This server plays only the files already on it. No account with any service is used or needed, and listing a title doesn\'t give anyone access to it there.' },
  { id: 'no-affiliation', text: 'WittyMediaServer has no connection with any streaming service. Their names appear only to say which of your own titles they list. They haven\'t made, approved or checked this.' },
  { id: 'may-be-wrong', text: 'Listings can be out of date or wrong, and depend on the region I choose. They\'re dated wherever they appear. They come from JustWatch, through TMDB, whose terms apply and don\'t allow commercial use.' },
  { id: 'sends-titles', text: 'My server will send TMDB the names or TMDB ids of the titles in my film and TV libraries, using my own TMDB key, about once every two weeks. No files and nothing about me are sent.' },
]

const REGIONS = ['AR', 'AT', 'AU', 'BE', 'BR', 'CA', 'CH', 'CL', 'CO', 'CZ', 'DE', 'DK', 'ES', 'FI', 'FR', 'GB', 'GR', 'HK', 'HU', 'ID', 'IE', 'IL', 'IN', 'IT', 'JP', 'KR', 'MX', 'MY', 'NL', 'NO', 'NZ', 'PH', 'PL', 'PT', 'RO', 'SE', 'SG', 'TH', 'TR', 'TW', 'US', 'ZA']

export function ListingSettings() {
  const [s, setS] = useState<ListingStatus | null>(null)
  const [ticked, setTicked] = useState<Set<string>>(new Set())
  const [region, setRegion] = useState('US')
  const [err, setErr] = useState('')
  const load = useCallback(() => api.get<ListingStatus>('/listings/status').then((r) => { setS(r); setRegion(r.region) }).catch(() => {}), [])
  useEffect(() => { load() }, [load])
  useEffect(() => {
    if (!s?.running) return
    const t = setTimeout(load, 4000)
    return () => clearTimeout(t)
  }, [s, load])
  const send = async (enabled: boolean, reg = region) => {
    setErr('')
    try { setS(await api.put<ListingStatus>('/listings/settings', { enabled, region: reg, accepted: [...ticked] })); if (!enabled) setTicked(new Set()) } catch (e: any) { setErr(e.message) }
  }
  if (!s) return null
  const names = new Intl.DisplayNames(undefined, { type: 'region' })
  const regionSelect = (
    <select className="select" aria-label="Region" value={region} onChange={(e) => { setRegion(e.target.value); if (s.enabled) send(true, e.target.value) }}>
      {REGIONS.map((r) => <option key={r} value={r}>{names.of(r) || r}</option>)}
    </select>
  )
  return (
    <Section id="listings" title="Streaming listings" desc="See which of the titles on your server a streaming service currently lists, if you're used to finding things that way. Off unless you turn it on.">
      {s.enabled ? (
        <>
          <Row label="Show streaming listings" desc={s.running ? `Checking: ${s.done} of ${s.total}.` : `${s.listed} of ${s.titles} titles checked are listed somewhere${s.newest ? `, as of ${new Date(s.newest * 1000).toLocaleDateString()}` : ''}.`}>
            <Switch label="Show streaming listings" checked onChange={() => send(false)} />
          </Row>
          <Row label="Region" desc="Listings differ by country.">{regionSelect}</Row>
          <Row label="Check again" desc="Each title is checked about once every two weeks by itself.">
            <button type="button" className="btn sm" onClick={() => api.post<ListingStatus>('/listings/refresh').then(setS).catch((e) => toast(e.message))}>Check now</button>
          </Row>
        </>
      ) : (
        <div className="set-consent-box">
          <p><b>Please read each of these.</b> Listings stay off until you've accepted all of them, and turning them off forgets everything they fetched.</p>
          <fieldset className="set-consent-ticks">
            <legend className="sr-only">What you're accepting</legend>
            {LISTING_STATEMENTS.map((a) => (
              <label key={a.id}>
                <input type="checkbox" checked={ticked.has(a.id)} onChange={(e) => { const n = new Set(ticked); if (e.target.checked) n.add(a.id); else n.delete(a.id); setTicked(n) }} />
                <span>{a.text}</span>
              </label>
            ))}
          </fieldset>
          <div className="set-consent-btns">
            <label className="set-consent-region">Region {regionSelect}</label>
            <button type="button" className="btn primary" disabled={ticked.size !== LISTING_STATEMENTS.length || !s.hasKey} onClick={() => send(true)}>Turn on listings</button>
          </div>
          {!s.hasKey && <p className="set-note bad">Listings need a free TMDB key: add one under Built-in agents and save first. Without a key, this stays off.</p>}
        </div>
      )}
      {err && <div className="error" role="alert">{err}</div>}
      <p className="muted set-attrib">{s.attribution}</p>
      <LegalNote kind="listings" className="set-legal" />
    </Section>
  )
}
