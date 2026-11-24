// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { date, language, t, tx, bidi } from '../i18n'

// watchOffers.ts can't import the translations (node tests load it): its copy names and details are translated here.
const copyName = (name: string) => { const m = /^Version (\d+)$/.exec(name); return m ? t('Version {number}', { number: m[1] }) : tx(name) }
const copyDetail = (detail: string) => detail.split(' · ').map(tx).map(bidi).join(' · ')
import { useApp } from '../state'
import { Select } from '../ui'
import LegalNote from './LegalNote'
import { watchView, type WatchAnswer, type WatchProvider } from './watchOffers'
import './wheretowatch.css'

// Where to watch, on a film's or a show's page: what this library has of it (its quality,
// and every version), and, when the server has a TMDB key, the services that offer it in
// the server's region, by how: included, free, with adverts, to rent, to buy. Nothing plays
// from those services; the link goes to TMDB's page of offers, and the credit the source
// asks for is always beside them. Without a key ordinary accounts see only what is here,
// and an administrator is told where the key goes. What an account may see is the server's
// business: this shows what it answers and asks for nothing more.
//
// itemId: the film, or any episode of the show (the server answers for its show).

export default function WhereToWatch({ itemId, show }: { itemId: number; show?: boolean }) {
  const { user } = useApp()
  const [a, setA] = useState<WatchAnswer | null>(null)
  const [region, setRegion] = useState('')
  const [regions, setRegions] = useState<string[]>([])
  useEffect(() => { setA(null); setRegion('') }, [itemId])
  useEffect(() => {
    const stop = new AbortController()
    api.get<WatchAnswer>(`/media/${itemId}/where-to-watch${region ? `?region=${region}` : ''}`, stop.signal).then(setA).catch(() => { /* the page is whole without it */ })
    return () => stop.abort()
  }, [itemId, region])
  const v = watchView(a, { admin: !!user?.isAdmin })
  // Another country can be looked at once there are services to look at. The list of
  // countries is the one the listings setting offers, fetched only then.
  const asked = !!v && (v.groups.length > 0 || v.note === 'nowhere')
  useEffect(() => {
    if (!asked || regions.length) return
    let on = true
    import('../pages/settings/MetadataOptional').then((m) => { if (on) setRegions(m.REGIONS) }).catch(() => {})
    return () => { on = false }
  }, [asked, regions.length])
  if (!v) return null
  const names = new Intl.DisplayNames([language()], { type: 'region' })
  // The whole sentence is one message; the link goes where {place} is.
  const [keyBefore, keyAfter = ''] = t('To also see which services stream, rent or sell this, add a free TMDB key in {place}.', { place: '\u0000' }).split('\u0000')
  const regionName = (r: string) => { try { return names.of(r) || r } catch { return r } }
  return (
    <section className="wtw" aria-label={t('Where to watch')}>
      <h2>{t('Where to watch')}</h2>
      <div className="wtw-here">
        <span className="wtw-kind">{t('In your library')}</span>
        {/* One copy says its quality in its own line. */}
        {v.quality && (show || v.copies.length !== 1 || !v.copies[0].detail) && <span className="pill">{v.quality}</span>}
        {!show && v.copies.length > 1 && (
          <ul className="wtw-copies" aria-label={t('Versions in your library')}>
            {v.copies.map((c) => (
              <li key={c.itemId}>{c.itemId === itemId ? <b>{copyName(c.name)}</b> : <Link to={`/item/${c.itemId}`}>{copyName(c.name)}</Link>}{c.detail && <span className="muted"> · {copyDetail(c.detail)}</span>}</li>
            ))}
          </ul>
        )}
        {!show && v.copies.length === 1 && v.copies[0].detail && <span className="muted">{copyDetail(v.copies[0].detail)}</span>}
      </div>
      {v.groups.map((g) => (
        <div key={g.type} className="wtw-group" role="group" aria-label={tx(g.title)}>
          <span className="wtw-kind">{tx(g.title)}</span>
          <ul className="wtw-providers">
            {g.providers.map((p) => <li key={p.key}><Provider p={p} link={v.link} /></li>)}
          </ul>
        </div>
      ))}
      {v.note === 'nowhere' && <p className="muted wtw-note">{t('No streaming service, rental or shop is listed for this in {region}.', { region: regionName(v.region) })}</p>}
      {v.note === 'unavailable' && <p className="muted wtw-note">{t('Where else this can be watched couldn’t be checked just now. It will be tried again later.')}</p>}
      {v.note === 'no-key' && <p className="muted wtw-note">{keyBefore}<Link to="/settings/metadata">{t('Settings, Metadata agents')}</Link>{keyAfter} {t('Only administrators see this note.')}</p>}
      {v.note === 'not-matched' && <p className="muted wtw-note">{t('This isn’t matched with a TMDB title, so the services that have it can’t be looked up. Choose a match under Edit.')} {t('Only administrators see this note.')}</p>}
      {asked && (
        <div className="wtw-foot">
          <p className="muted wtw-note">
            <b>{t('Nothing plays from these services.')}</b> {v.asOf
              ? t('They are other places this title is offered in {region}, as of {date}.', { region: regionName(v.region), date: date(new Date(v.asOf * 1000)) })
              : t('They are other places this title is offered in {region}.', { region: regionName(v.region) })}
            {v.link && <> <a href={v.link} target="_blank" rel="noreferrer noopener">{t('See the offers on TMDB')}<span className="sr-only"> {t('(opens in a new tab)')}</span></a>.</>} {tx(v.attribution)}
          </p>
          {regions.length > 0 && (
            <label className="wtw-region">{t('Country')}
              <Select inputSize="sm" value={v.region} onChange={(e) => setRegion(e.target.value)}>
                {(regions.includes(v.region) ? regions : [v.region, ...regions]).map((r) => <option key={r} value={r}>{regionName(r)}</option>)}
              </Select>
            </label>
          )}
          <LegalNote kind="listings" />
        </div>
      )}
    </section>
  )
}

// A service: its logo when the source gave one and it loads, otherwise its name. The name
// is always what a screen reader hears.
function Provider({ p, link }: { p: WatchProvider; link: string }) {
  const [broken, setBroken] = useState(false)
  const face = p.logo && !broken
    ? <><img src={p.logo} alt="" width={40} height={40} loading="lazy" referrerPolicy="no-referrer" onError={() => setBroken(true)} /><span>{p.name}</span></>
    : <span>{p.name}</span>
  return link
    ? <a className="wtw-provider" href={link} target="_blank" rel="noreferrer noopener" aria-label={t('{name}: see the offers on TMDB (opens in a new tab)', { name: p.name })}>{face}</a>
    : <span className="wtw-provider">{face}</span>
}
