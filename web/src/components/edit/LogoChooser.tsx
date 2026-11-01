import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, mediaUrl } from '../../api'
import { useApp } from '../../state'
import BrandLogo, { forgetLogos, logoImageUrl, logoVariant, type LogoInfo } from '../BrandLogo'
import LegalNote from '../LegalNote'
import { sendPicture } from '../../metadata'
import { Button, Dialog, Notice } from '../../ui'

// A row for each studio or network with its logo and a Change button, and what Change opens.

type Candidate = { source: string; sourceId: string; name: string; note?: string; imageUrl: string; pageUrl: string; license: string; licenseUrl?: string; credit?: string; exact: boolean }

export function LogoRows({ logos, onChanged }: { logos: LogoInfo[]; onChanged: () => void }) {
  const [changing, setChanging] = useState<LogoInfo | null>(null)
  const { user } = useApp()
  return (
    <>
      <ul className="ed-logos">
        {logos.map((l) => (
          <li key={l.kind + l.name}>
            <span className="ed-logo-plate"><BrandLogo kind={l.kind} name={l.name} /></span>
            <span className="ed-logo-text"><b>{l.name}</b><span className="muted">{l.kind === 'network' ? 'Network' : l.kind === 'studio' ? 'Studio' : l.kind === 'console' ? 'Console' : l.kind === 'gamestudio' ? 'Game studio' : 'Service'}</span></span>
            {user?.isAdmin && l.id && <Button onClick={() => setChanging(l)} aria-label={`Change the logo of ${l.name}`}>Change</Button>}
          </li>
        ))}
      </ul>
      {changing && <LogoChooser logo={changing} onClose={() => setChanging(null)} onChanged={() => { forgetLogos(); onChanged() }} />}
    </>
  )
}

export default function LogoChooser({ logo, onClose, onChanged }: { logo: LogoInfo; onClose: () => void; onChanged: (l: LogoInfo) => void }) {
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
    try { done(await sendPicture<LogoInfo>(`/logos/${logo.id}/upload`, f)) } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }
  return (
    <Dialog title={`The logo of ${logo.name}`} size="wide" className="ed-logo-dialog" onClose={onClose} actions={<Button variant="ghost" onClick={onClose}>Done</Button>}>
        <div className="ed-logo-now">
          <span className="ed-logo-plate big">{cur.has ? <img src={logoImageUrl(cur, cur.opaque ? 'original' : logoVariant())} alt={`Logo of ${logo.name}`} /> : <span className="brand-logo-text">{logo.name}</span>}</span>
          <div className="ed-logo-text">
            {cur.has ? (cur.source === 'you' ? <span>Uploaded by you.</span> : (
              <span>From <a href={cur.sourceUrl} target="_blank" rel="noreferrer">Wikimedia Commons</a>, {cur.license}{cur.credit ? `, by ${cur.credit}` : ''}. Recoloured here.</span>
            )) : cur.status === 'text' ? <span>Shown as its name, as you chose.</span> : cur.status === 'none' ? <span>No source has one, so it's shown as its name.</span> : <span>Shown as its name for now.</span>}
            <div className="ed-art-btns">
              <Button size="sm" disabled={busy} onClick={() => input.current?.click()}>Upload your own</Button>
              {cur.status !== 'text' && <Button size="sm" variant="ghost" disabled={busy} onClick={() => status('text')}>Use the name alone</Button>}
              {cur.status === 'text' && <Button size="sm" variant="ghost" disabled={busy} onClick={() => status('pending')}>Look for one again</Button>}
            </div>
            <input ref={input} hidden type="file" accept="image/png,image/jpeg,image/webp,image/gif" aria-label="Upload a logo" onChange={(e) => { upload(e.target.files?.[0]); e.target.value = '' }} />
          </div>
        </div>
        {needsConsent ? (
          <Notice tone="warn">Downloading logos is off. Uploading your own works without it. To download them, read what it means and turn it on in <Link to="/settings/metadata" onClick={onClose}>Settings, Metadata agents</Link>.</Notice>
        ) : (
          <>
            <form className="field-row" onSubmit={(e) => { e.preventDefault(); look(q) }}>
              <input className="input" aria-label="Search for a logo" value={q} onChange={(e) => setQ(e.target.value)} />
              <Button type="submit">Search</Button>
            </form>
            {!list ? <div className="loading-box"><div className="spinner" /></div> : list.length === 0 ? <p className="muted">Wikimedia Commons has nothing freely licensed by that name.</p> : (
              <div className="ed-logo-choices">
                {list.map((c) => (
                  <button key={c.sourceId} type="button" className="ed-logo-choice" disabled={busy} onClick={() => pick(c)}>
                    <span className="ed-logo-sample"><img src={mediaUrl(`/api/logos/preview?src=${encodeURIComponent(c.imageUrl)}`)} alt="" loading="lazy" /></span>
                    <b>{c.name}</b>
                    <span className="muted">{c.license}{c.exact ? '' : ' · a close name'}</span>
                  </button>
                ))}
              </div>
            )}
          </>
        )}
        {err && <div className="error" role="alert">{err}</div>}
        <LegalNote kind="logos" />
    </Dialog>
  )
}
