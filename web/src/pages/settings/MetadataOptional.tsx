// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { api } from '../../api'
import { forgetLogos, type LogoInfo, type LogoKind } from '../../components/BrandLogo'
import { ChangeLogoButton, LOGO_KIND_NAMES, LogoRows } from '../../components/edit/LogoChooser'
import LogoCredits from '../../components/LogoCredits'
import LegalNote from '../../components/LegalNote'
import type { ListingStatus } from '../../metadata'
import { toast } from '../../state'
import { Button, Notice } from '../../ui'
import { Row, Section, Switch } from './ui'
import { date, language, t, tk, tx } from '../../i18n'
import './metadataoptional.css'

// The optional downloads: logos, and which streaming services list a title. Both are off until
// someone reads what they mean and says yes. First-time setup can show the same consent parts.

type LogoStatus = { consent: 'yes' | 'no' | ''; counts: Record<string, number>; source: string; sourceUrl: string }

// The words someone agrees to before logos are downloaded.
export function LogoConsentWords() {
  return (
    <ul className="set-consent">
      <li>{t('Logos of studios, networks, TV channels, consoles and services are')} <b>{t('not part of this software')}</b>. {t("They're downloaded to your server, one at a time, from")} <a href="https://commons.wikimedia.org/wiki/Commons:Licensing" target="_blank" rel="noreferrer">Wikimedia Commons</a>.</li>
      <li>{t('Your server tells Wikidata and Wikimedia Commons the')} <b>{t('name')}</b> {t('of each studio, network, TV channel or console it finds, and nothing else: nothing about you or your files.')}</li>
      <li>{t("Only files whose own page says they're free to reuse are taken. Each one's source, licence and author are listed in Settings, Metadata agents, under Logos, and under Legal for everyone signed in.")}</li>
      <li><b>{t('Logos are still trademarks of their owners.')}</b> {t("They're shown only to name who made something, for your own use on your own server. If you share your server, that's for you to judge.")}</li>
      <li>{t('Without this, the app shows names in plain text, and nothing else changes. You can upload your own logos either way.')}</li>
    </ul>
  )
}

// Any logo on the server: find it by name, change it, or start one for a name nothing has yet.
function LogoFinder() {
  const [kind, setKind] = useState<'' | LogoKind>('')
  const [q, setQ] = useState('')
  const [list, setList] = useState<{ logos: LogoInfo[]; total: number } | null>(null)
  const [limit, setLimit] = useState(20)
  const load = useCallback(() => {
    api.get<{ logos: LogoInfo[]; total: number }>(`/logos?kind=${kind}&q=${encodeURIComponent(q)}&limit=${limit}`).then(setList).catch((e) => toast(e.message))
  }, [kind, q, limit])
  useEffect(() => { const t = setTimeout(load, q ? 250 : 0); return () => clearTimeout(t) }, [load, q])
  const names = Object.entries(LOGO_KIND_NAMES) as [LogoKind, string][]
  const exact = list?.logos.some((l) => BrandKeyOf(l.name) === BrandKeyOf(q))
  return (
    <div className="set-logofinder">
      <h3 className="set-subhead">{t('Find or set a logo')}</h3>
      <p className="muted">{t("Everything here that can have a logo. Search by name, choose among what's found with its source and licence in view, upload your own, or choose none.")}</p>
      <div className="field-row">
        <input className="input" type="search" aria-label={t('Find a logo by name')} placeholder={t('A network, studio, console…')} value={q} onChange={(e) => setQ(e.target.value)} />
        <select className="select" aria-label={t('What kind')} value={kind} onChange={(e) => setKind(e.target.value as '' | LogoKind)}>
          <option value="">{t('Everything')}</option>
          {names.map(([k, n]) => <option key={k} value={k}>{tx(n)}</option>)}
        </select>
      </div>
      {q.trim() && list && !exact && <p><ChangeLogoButton kind={kind || 'network'} name={q.trim()} label={t('Set a logo for this name')} variant="primary" onChanged={load} /></p>}
      {list && list.logos.length === 0 && !q.trim() && <p className="muted">{t('Nothing yet. Names appear as your libraries are scanned and channels are added.')}</p>}
      {list && list.logos.length > 0 && <LogoRows logos={list.logos} onChanged={load} />}
      {list && list.total > list.logos.length && <p><Button size="sm" onClick={() => setLimit((n) => Math.min(200, n + 40))}>{t('Show more ({count} left)', { count: list.total - list.logos.length })}</Button></p>}
    </div>
  )
}

const BrandKeyOf = (n: string) => n.trim().toLowerCase().replace(/[^\p{L}\p{N}]+/gu, ' ').trim()

export function LogoSettings() {
  const [s, setS] = useState<LogoStatus | null>(null)
  const [credits, setCredits] = useState(false)
  const box = useRef<HTMLDivElement | null>(null)
  const { hash } = useLocation()
  useEffect(() => { if (hash === '#logos' && s) box.current?.scrollIntoView({ block: 'start' }) }, [hash, !!s]) // eslint-disable-line react-hooks/exhaustive-deps
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
    <div ref={box}><Section id="logos" title={tk('Logos')} desc={t('Logos on the cards for studios and networks, TV channels, consoles, game studios and streaming services.')}>
      {s.consent === 'yes' ? (
        <>
          <Row label={t('Download logos')} desc={t('{found} found, {pending} still to look for, {none} that no source has.', { found: s.counts.found, pending: s.counts.pending, none: s.counts.none })}>
            <Switch label={t('Download logos')} checked onChange={() => answer(false)} />
          </Row>
          <Row label={t('Remove what was downloaded')} desc={t('Turns downloads off and deletes the pictures. Logos you uploaded stay.')}>
            <Button size="sm" onClick={() => answer(false, true)}>{t('Remove them')}</Button>
          </Row>
          <Row label={t('Where each logo came from')} desc={t('The source, licence and author of each one, as their licences ask. Everyone signed in can see this list too, under Legal.')} stack>
            {credits ? <LogoCredits /> : <Button size="sm" onClick={() => setCredits(true)}>{t('Show the list')}</Button>}
          </Row>
        </>
      ) : (
        <div className="set-consent-box">
          <LogoConsentWords />
          <div className="set-consent-btns">
            <Button variant="primary" onClick={() => answer(true)}>{t('I understand: download logos')}</Button>
            {s.consent === '' && <Button variant="ghost" onClick={() => answer(false)}>{t('No thanks: use plain names')}</Button>}
          </div>
        </div>
      )}
      {s && <LogoFinder />}
      <LegalNote kind="logos" className="set-legal" />
    </Section></div>
  )
}

// What someone has to accept, one at a time, before listings can be turned on. Show each text with tx().
export const LISTING_STATEMENTS: { id: string; text: string }[] = [
  { id: 'not-streaming', text: tk("Nothing streams from these services. This server plays only the files already on it. No account with any service is used or needed, and listing a title doesn't give anyone access to it there.") },
  { id: 'no-affiliation', text: tk("WittyMediaServer has no connection with any streaming service. Their names appear only to say which of your own titles they list. They haven't made, approved or checked this.") },
  { id: 'may-be-wrong', text: tk("Listings can be out of date or wrong, and depend on the region I choose. They're dated wherever they appear. They come from JustWatch, through TMDB, whose terms apply and don't allow commercial use.") },
  { id: 'sends-titles', text: tk('My server will send TMDB the names or TMDB ids of the titles in my film and TV libraries, using my own TMDB key, about once every two weeks. No files and nothing about me are sent.') },
]

export const REGIONS = ['AR', 'AT', 'AU', 'BE', 'BR', 'CA', 'CH', 'CL', 'CO', 'CZ', 'DE', 'DK', 'ES', 'FI', 'FR', 'GB', 'GR', 'HK', 'HU', 'ID', 'IE', 'IL', 'IN', 'IT', 'JP', 'KR', 'MX', 'MY', 'NL', 'NO', 'NZ', 'PH', 'PL', 'PT', 'RO', 'SE', 'SG', 'TH', 'TR', 'TW', 'US', 'ZA']

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
  const names = new Intl.DisplayNames([language()], { type: 'region' })
  const regionSelect = (
    <select className="select" aria-label={t('Region')} value={region} onChange={(e) => { setRegion(e.target.value); if (s.enabled) send(true, e.target.value) }}>
      {REGIONS.map((r) => <option key={r} value={r}>{names.of(r) || r}</option>)}
    </select>
  )
  return (
    <Section id="listings" title={tk('Streaming listings')} desc={t("See which of the titles on your server a streaming service currently lists, if you're used to finding things that way. Off unless you turn it on.")}>
      {s.enabled ? (
        <>
          <Row label={t('Show streaming listings')} desc={s.running ? t('Checking: {done} of {total}.', { done: s.done, total: s.total })
            : s.newest ? t('{listed} of {titles} titles checked are listed somewhere, as of {date}.', { listed: s.listed, titles: s.titles, date: date(s.newest * 1000, { dateStyle: 'short' }) })
            : t('{listed} of {titles} titles checked are listed somewhere.', { listed: s.listed, titles: s.titles })}>
            <Switch label={t('Show streaming listings')} checked onChange={() => send(false)} />
          </Row>
          <Row label={t('Region')} desc={t('Listings differ by country.')}>{regionSelect}</Row>
          <Row label={t('Check again')} desc={t('Each title is checked about once every two weeks by itself.')}>
            <Button size="sm" onClick={() => api.post<ListingStatus>('/listings/refresh').then(setS).catch((e) => toast(e.message))}>{t('Check now')}</Button>
          </Row>
        </>
      ) : (
        <div className="set-consent-box">
          <p><b>{t('Please read each of these.')}</b> {t("Listings stay off until you've accepted all of them, and turning them off forgets everything they fetched.")}</p>
          <fieldset className="set-consent-ticks">
            <legend className="sr-only">{t("What you're accepting")}</legend>
            {LISTING_STATEMENTS.map((a) => (
              <label key={a.id}>
                <input type="checkbox" checked={ticked.has(a.id)} onChange={(e) => { const n = new Set(ticked); if (e.target.checked) n.add(a.id); else n.delete(a.id); setTicked(n) }} />
                <span>{tx(a.text)}</span>
              </label>
            ))}
          </fieldset>
          <div className="set-consent-btns">
            <label className="set-consent-region">{t('Region')} {regionSelect}</label>
            <Button variant="primary" disabled={ticked.size !== LISTING_STATEMENTS.length || !s.hasKey} onClick={() => send(true)}>{t('Turn on listings')}</Button>
          </div>
          {!s.hasKey && <Notice tone="warn">{t('Listings need a free TMDB key: add one under Built-in agents and save first. Without a key, this stays off.')}</Notice>}
        </div>
      )}
      {err && <div className="error" role="alert">{err}</div>}
      <p className="muted set-attrib">{tx(s.attribution)}</p>
      <LegalNote kind="listings" className="set-legal" />
    </Section>
  )
}
