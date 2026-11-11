// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, mediaUrl } from '../../api'
import { useApp } from '../../state'
import BrandLogo, { forgetLogos, logoImageUrl, logoVariant, lookupLogo, type LogoInfo, type LogoKind } from '../BrandLogo'
import LegalNote from '../LegalNote'
import { sendPicture } from '../../metadata'
import { Button, Dialog, Notice } from '../../ui'
import { WebLink } from '../WebLink'
import './edit.css'

// Everywhere a logo can be set: search by name, choose among what comes back with its source
// and licence in view, keep your own upload, or choose none. Before downloads are agreed to,
// upload and none still work, and the search explains and links to the one agreement.

type Candidate = { source: string; sourceId: string; name: string; note?: string; imageUrl: string; pageUrl: string; license: string; licenseUrl?: string; credit?: string; exact: boolean }

export const LOGO_KIND_NAMES: Record<LogoKind, string> = { network: 'Network', studio: 'Studio', console: 'Console', gamestudio: 'Game studio', platform: 'Streaming service' }

export function LogoRows({ logos, onChanged }: { logos: LogoInfo[]; onChanged: () => void }) {
  const [changing, setChanging] = useState<LogoInfo | null>(null)
  const { user } = useApp()
  return (
    <>
      <ul className="ed-logos">
        {logos.map((l) => (
          <li key={l.kind + l.name}>
            <span className="ed-logo-plate"><BrandLogo kind={l.kind} name={l.name} /></span>
            <span className="ed-logo-text"><b>{l.name}</b><span className="muted">{LOGO_KIND_NAMES[l.kind]}</span></span>
            {user?.isAdmin && l.id && <Button onClick={() => setChanging(l)} aria-label={`Change the logo of ${l.name}`}>Change</Button>}
          </li>
        ))}
      </ul>
      {changing && <LogoChooser logo={changing} onClose={() => setChanging(null)} onChanged={() => { forgetLogos(); onChanged() }} />}
    </>
  )
}

// A button for any screen that shows a logo. It's only there for an administrator, and asking
// for a name's row here is what makes one, so it works for a name nothing has looked up yet.
export function ChangeLogoButton({ kind, name, label = 'Change logo', variant = 'ghost', size = 'sm', onChanged }: { kind: LogoKind; name: string; label?: string; variant?: 'ghost' | 'primary'; size?: 'sm'; onChanged?: () => void }) {
  const { user } = useApp()
  const [logo, setLogo] = useState<LogoInfo | null>(null)
  if (!user?.isAdmin || !name.trim()) return null
  return (
    <>
      <Button size={size} variant={variant} aria-label={`${label}: ${name}`} onClick={() => { forgetLogos(); void lookupLogo(kind, name).then(setLogo) }}>{label}</Button>
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
    ? <>Downloading logos is off{consent === 'no' ? ', as you chose' : ''}. Nothing is fetched until you agree. Read what it means and turn it on in <Link to="/settings/metadata#logos" onClick={onLeave}>Settings, Metadata agents, Logos</Link>. Uploading your own works without it.</>
    : <>Downloading logos is off on this server, so names show instead. Its owner can turn it on.</>
  return compact ? <small className="tvs-help">{words}</small> : <Notice tone="warn">{words}</Notice>
}

function Candidates({ list, busy, onPick }: { list: Candidate[]; busy: boolean; onPick: (c: Candidate) => void }) {
  return (
    <div className="ed-logo-choices">
      {list.map((c) => (
        <div key={c.sourceId} className="ed-logo-option">
          <button type="button" className="ed-logo-choice" disabled={busy} onClick={() => onPick(c)} aria-label={`Use this logo: ${c.name}, ${c.license}`}>
            <span className="ed-logo-sample"><img src={mediaUrl(`/api/logos/preview?src=${encodeURIComponent(c.imageUrl)}`)} alt="" loading="lazy" /></span>
            <b>{c.name}</b>
            {c.note && <span className="muted">{c.note}</span>}
            {!c.exact && <span className="muted">A close name, so check it's the right one</span>}
          </button>
          <span className="ed-logo-source">
            {c.licenseUrl ? <WebLink href={c.licenseUrl}>{c.license}</WebLink> : c.license}{c.credit ? `, by ${c.credit}` : ''}.{' '}
            <WebLink href={c.pageUrl}>Its page on {c.source === 'wikimedia' ? 'Wikimedia Commons' : c.source}</WebLink>
          </span>
        </div>
      ))}
    </div>
  )
}

// The body of the chooser, for a dialog of its own or inside another.
export function LogoPanel({ logo, onChanged, onLeave }: { logo: LogoInfo; onChanged: (l: LogoInfo) => void; onLeave?: () => void }) {
  const [list, setList] = useState<Candidate[] | null>(null)
  const [needsConsent, setNeedsConsent] = useState(false)
  const [q, setQ] = useState(logo.name)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [cur, setCur] = useState(logo)
  const input = useRef<HTMLInputElement | null>(null)
  const look = (query: string) => {
    setList(null); setErr('')
    api.get<{ candidates: Candidate[] }>(`/logos/${logo.id}/candidates?q=${encodeURIComponent(query)}`)
      .then((r) => setList(r.candidates))
      .catch((e) => { setList([]); if (e.status === 409) setNeedsConsent(true); else setErr(e.message) })
  }
  useEffect(() => { look(logo.name) }, [logo.id]) // eslint-disable-line react-hooks/exhaustive-deps
  const done = (l: LogoInfo) => { setCur(l); onChanged(l) }
  const pick = async (c: Candidate) => {
    setBusy(true); setErr('')
    try { done(await api.put<LogoInfo>(`/logos/${logo.id}`, { sourceId: c.sourceId, q })) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
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
        <span className="ed-logo-plate big">{cur.has ? <img src={logoImageUrl(cur, cur.opaque ? 'original' : logoVariant())} alt={`Logo of ${logo.name}`} /> : <span className="brand-logo-text">{logo.name}</span>}</span>
        <div className="ed-logo-text">
          <span role="status">
            {cur.has ? (cur.source === 'you' ? <>Uploaded by you, and yours to keep: a search won't replace it unless you choose one.</> : (
              <>From <WebLink href={cur.sourceUrl}>Wikimedia Commons</WebLink>, {cur.licenseUrl ? <WebLink href={cur.licenseUrl}>{cur.license}</WebLink> : cur.license}{cur.credit ? `, by ${cur.credit}` : ''}. Recoloured here.</>
            )) : cur.status === 'text' ? <>No logo: it's shown as its name, as you chose.</> : cur.status === 'none' ? <>No source has one under this name, so it's shown as its name. Try a search below, or upload your own.</> : <>Shown as its name for now.</>}
          </span>
          <div className="ed-art-btns">
            <Button size="sm" disabled={busy} onClick={() => input.current?.click()}>Upload your own</Button>
            {cur.status !== 'text' && <Button size="sm" variant="ghost" disabled={busy} onClick={() => status('text')}>No logo: just the name</Button>}
            {cur.status === 'text' && <Button size="sm" variant="ghost" disabled={busy} onClick={() => status('pending')}>Look for one again</Button>}
          </div>
          <input ref={input} hidden type="file" accept="image/png,image/jpeg,image/webp,image/gif" aria-label="Upload a logo" onChange={(e) => { void upload(e.target.files?.[0]); e.target.value = '' }} />
        </div>
      </div>
      {needsConsent ? <LogoConsentHint onLeave={onLeave} /> : (
        <>
          <form className="field-row" onSubmit={(e) => { e.preventDefault(); look(q) }}>
            <input className="input" aria-label="Search for a logo by name" value={q} onChange={(e) => setQ(e.target.value)} />
            <Button type="submit">Search</Button>
          </form>
          {!list ? <div className="loading-box"><div className="spinner" /></div> : list.length === 0 ? <p className="muted" role="status">Wikimedia Commons has nothing freely licensed by that name. Try another spelling, or the company's full name.</p> : <Candidates list={list} busy={busy} onPick={pick} />}
        </>
      )}
      {err && <div className="error" role="alert">{err}</div>}
      <LegalNote kind="logos" />
    </>
  )
}

export default function LogoChooser({ logo, onClose, onChanged }: { logo: LogoInfo; onClose: () => void; onChanged: (l: LogoInfo) => void }) {
  return (
    <Dialog title={`The logo of ${logo.name}`} size="wide" className="ed-logo-dialog" onClose={onClose} actions={<Button variant="ghost" onClick={onClose}>Done</Button>}>
      <LogoPanel logo={logo} onChanged={onChanged} onLeave={onClose} />
    </Dialog>
  )
}
