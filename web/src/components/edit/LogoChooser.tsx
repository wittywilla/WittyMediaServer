// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Fragment, useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { api, mediaUrl } from '../../api'
import { useApp } from '../../state'
import BrandLogo, { forgetLogos, logoImageUrl, logoVariant, lookupLogo, type LogoInfo, type LogoKind } from '../BrandLogo'
import LegalNote from '../LegalNote'
import { t, tk, tx } from '../../i18n'
import { sendPicture } from '../../metadata'
import { Button, Dialog, Notice } from '../../ui'
import { WebLink } from '../WebLink'
import './edit.css'

// Everywhere a logo can be set: search by name, choose among what comes back with its source
// and licence in view, keep your own upload, or choose none. Before downloads are agreed to,
// upload and none still work, and the search explains and links to the one agreement. A console's or
// a game studio's logo can also come under WittyPlay's own, narrower agreement (brand logos, in
// WittyPlay Settings › Game pictures), with its warnings; logos already downloaded here are offered
// to use again, credits and all.

// A translated sentence with links in it: each {name} in the words is swapped for its piece.
function withParts(text: string, parts: Record<string, ReactNode>) {
  return text.split(/(\{\w+\})/).map((bit, i) => {
    const m = /^\{(\w+)\}$/.exec(bit)
    return <Fragment key={i}>{m && m[1] in parts ? parts[m[1]] : bit}</Fragment>
  })
}

const byCredit = (credit?: string) => (credit ? t(', by {credit}', { credit }) : '')

type Candidate = { source: string; sourceId: string; name: string; note?: string; imageUrl: string; pageUrl: string; license: string; licenseUrl?: string; credit?: string; exact: boolean }

export const LOGO_KIND_NAMES: Record<LogoKind, string> = { network: tk('Network'), studio: tk('Studio'), console: tk('Console'), gamestudio: tk('Game studio'), platform: tk('Streaming service') }
// (marked here, translated where shown: tx(LOGO_KIND_NAMES[kind]))

export function LogoRows({ logos, onChanged }: { logos: LogoInfo[]; onChanged: () => void }) {
  const [changing, setChanging] = useState<LogoInfo | null>(null)
  const { user } = useApp()
  return (
    <>
      <ul className="ed-logos">
        {logos.map((l) => (
          <li key={l.kind + l.name}>
            <span className="ed-logo-plate"><BrandLogo kind={l.kind} name={l.name} /></span>
            <span className="ed-logo-text"><b>{l.name}</b><span className="muted">{tx(LOGO_KIND_NAMES[l.kind])}</span></span>
            {user?.isAdmin && l.id && <Button onClick={() => setChanging(l)} aria-label={t('Change the logo of {name}', { name: l.name })}>{t('Change')}</Button>}
          </li>
        ))}
      </ul>
      {changing && <LogoChooser logo={changing} onClose={() => setChanging(null)} onChanged={() => { forgetLogos(); onChanged() }} />}
    </>
  )
}

// A button for any screen that shows a logo. It's only there for an administrator, and asking
// for a name's row here is what makes one, so it works for a name nothing has looked up yet.
export function ChangeLogoButton({ kind, name, label: given, variant = 'ghost', size = 'sm', onChanged }: { kind: LogoKind; name: string; label?: string; variant?: 'ghost' | 'primary'; size?: 'sm'; onChanged?: () => void }) {
  const { user } = useApp()
  const [logo, setLogo] = useState<LogoInfo | null>(null)
  if (!user?.isAdmin || !name.trim()) return null
  const label = given || t('Change logo')
  return (
    <>
      <Button size={size} variant={variant} aria-label={t('{label}: {name}', { label, name })} onClick={() => { forgetLogos(); void lookupLogo(kind, name).then(setLogo) }}>{label}</Button>
      {logo?.id && <LogoChooser logo={logo} onClose={() => setLogo(null)} onChanged={() => { forgetLogos(); onChanged?.() }} />}
    </>
  )
}

// What it says where downloads aren't on: why there's nothing to search, and where the one
// agreement is. Never a dead end, because uploading and choosing none don't need it.
export function LogoConsentHint({ onLeave, compact }: { onLeave?: () => void; compact?: boolean }) {
  const { user } = useApp()
  const [consent, setConsent] = useState<string | null>(null)
  useEffect(() => { api.get<{ consent: string }>('/logos/status').then((r) => setConsent(r.consent)).catch(() => {}) }, [])
  if (consent === null || consent === 'yes') return null
  const words = user?.isAdmin
    ? <>{consent === 'no' ? t('Downloading logos is off, as you chose.') : t('Downloading logos is off.')} {t('Nothing is fetched until you agree.')} {withParts(t('Read what it means and turn it on in {place}.'), { place: <Link to="/settings/metadata#logos" onClick={onLeave}>{t('Settings, Metadata agents, Logos')}</Link> })} {t('Uploading your own works without it.')}</>
    : <>{t('Downloading logos is off on this server, so names show instead. Its owner can turn it on.')}</>
  return compact ? <small className="tvs-help">{words}</small> : <Notice tone="warn">{words}</Notice>
}

function Candidates({ list, busy, onPick }: { list: Candidate[]; busy: boolean; onPick: (c: Candidate) => void }) {
  return (
    <div className="ed-logo-choices">
      {list.map((c) => (
        <div key={c.sourceId} className="ed-logo-option">
          <button type="button" className="ed-logo-choice" disabled={busy} onClick={() => onPick(c)} aria-label={t('Use this logo: {name}, {license}', { name: c.name, license: c.license })}>
            <span className="ed-logo-sample"><img src={mediaUrl(`/api/logos/preview?src=${encodeURIComponent(c.imageUrl)}`)} alt="" loading="lazy" /></span>
            <b>{c.name}</b>
            {c.note && <span className="muted">{c.note}</span>}
            {!c.exact && <span className="muted">{t("A close name, so check it's the right one")}</span>}
          </button>
          <span className="ed-logo-source">
            {c.licenseUrl ? <WebLink href={c.licenseUrl}>{c.license}</WebLink> : c.license}{byCredit(c.credit)}.{' '}
            <WebLink href={c.pageUrl}>{t('Its page on {source}', { source: c.source === 'wikimedia' ? 'Wikimedia Commons' : c.source })}</WebLink>
          </span>
        </div>
      ))}
    </div>
  )
}

// The body of the chooser, for a dialog of its own or inside another.
// Logos already downloaded on this server for this kind (another console's name, say): chosen without asking any source again.
function Downloaded({ list, busy, onPick }: { list: LogoInfo[]; busy: boolean; onPick: (l: LogoInfo) => void }) {
  if (!list.length) return null
  return (
    <section className="ed-logo-downloaded" aria-labelledby="ed-logo-downloaded">
      <h3 id="ed-logo-downloaded">{t('Already downloaded on this server')}</h3>
      <div className="ed-logo-choices">
        {list.map((l) => (
          <div key={l.id} className="ed-logo-option">
            <button type="button" className="ed-logo-choice" disabled={busy} onClick={() => onPick(l)} aria-label={t('Use this logo: {name}, {license}', { name: l.name, license: l.license || t('licence unknown') })}>
              <span className="ed-logo-sample"><img src={logoImageUrl(l, l.opaque ? 'original' : logoVariant())} alt="" loading="lazy" /></span>
              <b>{l.name}</b>
            </button>
            <span className="ed-logo-source">
              {l.licenseUrl ? <WebLink href={l.licenseUrl}>{l.license}</WebLink> : l.license || t('Licence not recorded')}{byCredit(l.credit)}.{' '}
              {l.sourceUrl && <WebLink href={l.sourceUrl}>{t('Its page on {source}', { source: 'Wikimedia Commons' })}</WebLink>}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

export function LogoPanel({ logo, onChanged, onLeave }: { logo: LogoInfo; onChanged: (l: LogoInfo) => void; onLeave?: () => void }) {
  const [list, setList] = useState<Candidate[] | null>(null)
  const [downloaded, setDownloaded] = useState<LogoInfo[]>([])
  const [needsConsent, setNeedsConsent] = useState(false)
  const [q, setQ] = useState(logo.name)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [cur, setCur] = useState(logo)
  const input = useRef<HTMLInputElement | null>(null)
  const look = (query: string) => {
    setList(null); setErr('')
    api.get<{ candidates: Candidate[]; downloaded?: LogoInfo[] }>(`/logos/${logo.id}/candidates?q=${encodeURIComponent(query)}`)
      .then((r) => { setList(r.candidates); setDownloaded(r.downloaded || []) })
      .catch((e) => { setList([]); if (e.status === 409) setNeedsConsent(true); else setErr(e.message) })
  }
  useEffect(() => { look(logo.name) }, [logo.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const done = (l: LogoInfo) => { setCur(l); onChanged(l) }
  const pick = async (c: Candidate) => {
    setBusy(true); setErr('')
    try { done(await api.put<LogoInfo>(`/logos/${logo.id}`, { sourceId: c.sourceId, q })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const reuse = async (l: LogoInfo) => {
    setBusy(true); setErr('')
    try { done(await api.put<LogoInfo>(`/logos/${logo.id}`, { copyFrom: l.id })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const status = async (s: 'text' | 'pending') => {
    setBusy(true); setErr('')
    try { done(await api.put<LogoInfo>(`/logos/${logo.id}`, { status: s })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  const upload = async (f?: File) => {
    if (!f) return
    setBusy(true); setErr('')
    try { done(await sendPicture<LogoInfo>(`/logos/${logo.id}/upload`, f, 8)) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <>
      <div className="ed-logo-now">
        <span className="ed-logo-plate big">{cur.has ? <img src={logoImageUrl(cur, cur.opaque ? 'original' : logoVariant())} alt={t('Logo of {name}', { name: logo.name })} /> : <span className="brand-logo-text">{logo.name}</span>}</span>
        <div className="ed-logo-text">
          <span role="status">
            {cur.has ? (cur.source === 'you' ? t("Uploaded by you, and yours to keep: a search won't replace it unless you choose one.") : (
              <>{withParts(t('From {source}, {license}{credit}. Recoloured here.'), {
                source: <WebLink href={cur.sourceUrl}>Wikimedia Commons</WebLink>,
                license: cur.licenseUrl ? <WebLink href={cur.licenseUrl}>{cur.license}</WebLink> : cur.license,
                credit: byCredit(cur.credit),
              })}</>
            )) : cur.status === 'text' ? t("No logo: it's shown as its name, as you chose.") : cur.status === 'none' ? t("No source has one under this name, so it's shown as its name. Try a search below, or upload your own.") : t('Shown as its name for now.')}
          </span>
          <div className="ed-art-btns">
            <Button size="sm" disabled={busy} onClick={() => input.current?.click()}>{t('Upload your own')}</Button>
            {cur.status !== 'text' && <Button size="sm" variant="ghost" disabled={busy} onClick={() => status('text')}>{t('No logo: just the name')}</Button>}
            {cur.status === 'text' && <Button size="sm" variant="ghost" disabled={busy} onClick={() => status('pending')}>{t('Look for one again')}</Button>}
          </div>
          <input ref={input} hidden type="file" accept="image/png,image/jpeg,image/webp,image/gif" aria-label={t('Upload a logo')} onChange={(e) => { void upload(e.target.files?.[0]); e.target.value = '' }} />
        </div>
      </div>
      {needsConsent ? <LogoConsentHint onLeave={onLeave} /> : (
        <>
          <Downloaded list={downloaded} busy={busy} onPick={reuse} />
          <form className="field-row" onSubmit={(e) => { e.preventDefault(); look(q) }}>
            <input className="input" aria-label={t('Search for a logo by name')} value={q} onChange={(e) => setQ(e.target.value)} />
            <Button type="submit">{t('Search')}</Button>
          </form>
          {!list ? <div className="loading-box"><div className="spinner" /></div> : list.length === 0 ? <p className="muted" role="status">{t("Wikimedia Commons has nothing freely licensed by that name. Try another spelling, or the company's full name.")}</p> : <Candidates list={list} busy={busy} onPick={pick} />}
        </>
      )}
      {err && <div className="error" role="alert">{err}</div>}
      <LegalNote kind="logos" />
    </>
  )
}

export default function LogoChooser({ logo, onClose, onChanged }: { logo: LogoInfo; onClose: () => void; onChanged: (l: LogoInfo) => void }) {
  return (
    <Dialog title={t('The logo of {name}', { name: logo.name })} size="wide" className="ed-logo-dialog" onClose={onClose} actions={<Button variant="ghost" onClick={onClose}>{t('Done')}</Button>}>
      <LogoPanel logo={logo} onChanged={onChanged} onLeave={onClose} />
    </Dialog>
  )
}
